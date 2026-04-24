// Backend/handlers/inventory_handler.go
// ─────────────────────────────────────────────────────────────────────────────
// Endpoints transaccionales de inventario (CA 45: BEGIN/COMMIT/ROLLBACK):
//
//   POST /api/inventario/ingreso      → Registra entrada de stock
//   POST /api/inventario/baja         → Registra baja/merma de stock
//   GET  /api/inventario/movimientos  → Consulta el historial de movimientos
//
// Cada operación de escritura es ATÓMICA: actualiza ingreso/baja + stock
// del producto + movimientos_stock en la MISMA transacción.
// Si falla cualquier paso → ROLLBACK total. (CA 45)
// ─────────────────────────────────────────────────────────────────────────────
package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
)

// IngresoInput es el cuerpo esperado en POST /api/inventario/ingreso.
type IngresoInput struct {
	IdProducto        int    `json:"id_producto"`
	IdProveedor       int    `json:"id_proveedor"`        // 0 = sin proveedor
	IdUsuario         int    `json:"id_usuario"`
	CantidadIngresada int    `json:"cantidad_ingresada"`
	CostoUnitario     int    `json:"costo_unitario"`      // centavos
	Observacion       string `json:"observacion"`
}

// BajaInput es el cuerpo esperado en POST /api/inventario/baja.
type BajaInput struct {
	IdProducto   int    `json:"id_producto"`
	IdUsuario    int    `json:"id_usuario"`
	CantidadBaja int    `json:"cantidad_baja"`
	Motivo       string `json:"motivo"`
}

// ─── POST /api/inventario/ingreso ────────────────────────────────────────────
// IngresoHandler registra una entrada de mercadería de forma transaccional.
// Pasos dentro de la transacción:
//  1. SELECT FOR UPDATE del stock actual (bloqueo optimista de fila)
//  2. INSERT en ingreso_inventario
//  3. UPDATE stock_actual del producto
//  4. INSERT en movimientos_stock
func IngresoHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Solo se acepta POST en este endpoint."})
			return
		}

		var ing IngresoInput
		if err := json.NewDecoder(r.Body).Decode(&ing); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido o malformado."})
			return
		}

		// Validaciones → HTTP 400 (CA 45)
		if ing.IdProducto <= 0 || ing.IdUsuario <= 0 || ing.CantidadIngresada <= 0 || ing.CostoUnitario < 0 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "'id_producto', 'id_usuario', 'cantidad_ingresada' (>0) y 'costo_unitario' (>=0) son obligatorios.",
			})
			return
		}

		// ══════════════════════════════════════════════════
		// TRANSACCIÓN SQL — BEGIN (CA 45)
		// Atomicidad: todos los pasos ocurren juntos o ninguno.
		// ══════════════════════════════════════════════════
		tx, err := db.Begin() // BEGIN
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "No se pudo iniciar la transacción."})
			return
		}
		defer tx.Rollback() // ROLLBACK automático si Commit() no se llama

		// Paso 1 — Obtener y bloquear el stock actual (FOR UPDATE evita condiciones de carrera)
		var stockActual int
		err = tx.QueryRow(
			`SELECT stock_actual FROM inventario.productos WHERE id_producto = $1 FOR UPDATE`,
			ing.IdProducto,
		).Scan(&stockActual)
		if err == sql.ErrNoRows {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Producto no encontrado con el id_producto proporcionado."})
			return
		}
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar el stock del producto."})
			return
		}

		nuevoStock := stockActual + ing.CantidadIngresada

		// Paso 2 — Insertar el registro de ingreso
		var idIngreso int
		var proveedorNullable *int
		if ing.IdProveedor > 0 {
			proveedorNullable = &ing.IdProveedor
		}
		err = tx.QueryRow(`
			INSERT INTO inventario.ingreso_inventario
			  (id_producto, id_proveedor, id_usuario, cantidad_ingresada, costo_unitario, observacion)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id_ingreso`,
			ing.IdProducto, proveedorNullable, ing.IdUsuario,
			ing.CantidadIngresada, ing.CostoUnitario, ing.Observacion,
		).Scan(&idIngreso)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al registrar el ingreso en la base de datos."})
			return
		}

		// Paso 3 — Actualizar el stock del producto
		_, err = tx.Exec(
			`UPDATE inventario.productos SET stock_actual = $1 WHERE id_producto = $2`,
			nuevoStock, ing.IdProducto,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al actualizar el stock del producto."})
			return
		}

		// Paso 4 — Registrar en movimientos_stock (trazabilidad completa)
		_, err = tx.Exec(`
			INSERT INTO inventario.movimientos_stock
			  (id_producto, id_usuario, tipo_movimiento, cantidad, stock_resultante, referencia_id)
			VALUES ($1, $2, 'INGRESO', $3, $4, $5)`,
			ing.IdProducto, ing.IdUsuario, ing.CantidadIngresada, nuevoStock, idIngreso,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al registrar el movimiento de stock."})
			return
		}

		// COMMIT — todos los pasos fueron exitosos
		if err := tx.Commit(); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al confirmar la transacción."})
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"mensaje":     "Ingreso registrado y stock actualizado exitosamente.",
			"id_ingreso":  idIngreso,
			"stock_nuevo": nuevoStock,
		})
	}
}

// ─── POST /api/inventario/baja ───────────────────────────────────────────────
// BajaHandler registra una baja/merma de stock de forma transaccional.
// Verifica que haya stock suficiente antes de permitir la baja. (CA 45: 400)
func BajaHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Solo se acepta POST en este endpoint."})
			return
		}

		var baja BajaInput
		if err := json.NewDecoder(r.Body).Decode(&baja); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido o malformado."})
			return
		}

		// Validar motivo (CA 15 — HU-04: solo valores permitidos)
		motivosPermitidos := map[string]bool{"Caducidad": true, "Daño": true, "Pérdida": true}
		if baja.IdProducto <= 0 || baja.IdUsuario <= 0 || baja.CantidadBaja <= 0 || baja.Motivo == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "'id_producto', 'id_usuario', 'cantidad_baja' (>0) y 'motivo' son obligatorios.",
			})
			return
		}
		if !motivosPermitidos[baja.Motivo] {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "El motivo debe ser uno de: Caducidad, Daño, Pérdida.",
			})
			return
		}

		// ══════════════════════════════════════════════════
		// TRANSACCIÓN SQL — BEGIN (CA 45)
		// ══════════════════════════════════════════════════
		tx, err := db.Begin() // BEGIN
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "No se pudo iniciar la transacción."})
			return
		}
		defer tx.Rollback() // ROLLBACK automático

		// Paso 1 — Bloquear y leer stock actual
		var stockActual int
		err = tx.QueryRow(
			`SELECT stock_actual FROM inventario.productos WHERE id_producto = $1 FOR UPDATE`,
			baja.IdProducto,
		).Scan(&stockActual)
		if err == sql.ErrNoRows {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Producto no encontrado."})
			return
		}
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar el stock."})
			return
		}

		// Validación de negocio: stock suficiente (CA 19 — HU-04)
		if baja.CantidadBaja > stockActual {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "Stock insuficiente. No se puede dar de baja " + strconv.Itoa(baja.CantidadBaja) +
					" unidades. Stock disponible: " + strconv.Itoa(stockActual) + ".",
			})
			return
		}

		nuevoStock := stockActual - baja.CantidadBaja

		// Paso 2 — Insertar registro de baja
		var idBaja int
		err = tx.QueryRow(`
			INSERT INTO inventario.bajas_inventario (id_producto, id_usuario, cantidad_baja, motivo)
			VALUES ($1, $2, $3, $4) RETURNING id_baja`,
			baja.IdProducto, baja.IdUsuario, baja.CantidadBaja, baja.Motivo,
		).Scan(&idBaja)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al registrar la baja."})
			return
		}

		// Paso 3 — Actualizar stock
		_, err = tx.Exec(
			`UPDATE inventario.productos SET stock_actual = $1 WHERE id_producto = $2`,
			nuevoStock, baja.IdProducto,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al actualizar el stock."})
			return
		}

		// Paso 4 — Registrar movimiento tipo BAJA_MERMA (CA 16 — excluido de predicción)
		_, err = tx.Exec(`
			INSERT INTO inventario.movimientos_stock
			  (id_producto, id_usuario, tipo_movimiento, cantidad, stock_resultante, referencia_id)
			VALUES ($1, $2, 'BAJA_MERMA', $3, $4, $5)`,
			baja.IdProducto, baja.IdUsuario, -baja.CantidadBaja, nuevoStock, idBaja,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al registrar el movimiento de stock."})
			return
		}

		// Paso 5 — Log de auditoría (CA 18 — HU-04: usuario, fecha, IP)
		// Extrae IP del request; prioriza X-Forwarded-For si hay proxy/balanceador
		ip := r.Header.Get("X-Forwarded-For")
		if ip == "" {
			ip = r.RemoteAddr
		}
		valorAnterior := strconv.Itoa(stockActual)
		valorNuevo := strconv.Itoa(nuevoStock)
		_, err = tx.Exec(`
			INSERT INTO seguridad.logs_auditoria
			  (id_usuario, accion, tabla_afectada, id_registro_afectado, valor_anterior, valor_nuevo, ip_origen)
			VALUES ($1, 'BAJA_MERMA', 'inventario.productos', $2, $3, $4, $5)`,
			baja.IdUsuario, baja.IdProducto, valorAnterior, valorNuevo, ip,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al registrar el log de auditoría."})
			return
		}

		// COMMIT
		if err := tx.Commit(); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al confirmar la transacción."})
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"mensaje":     "Baja de merma registrada exitosamente. Stock y auditoría actualizados.",
			"id_baja":     idBaja,
			"stock_nuevo": nuevoStock,
			"motivo":      baja.Motivo,
		})
	}
}

// ─── GET /api/inventario/movimientos ────────────────────────────────────────
// MovimientosHandler consulta el historial de movimientos de stock.
// Acepta filtro opcional ?id_producto=X. Usa el índice idx_movimientos_fecha (CA 46).
func MovimientosHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Solo se acepta GET en este endpoint."})
			return
		}

		type Movimiento struct {
			IdMovimiento    int    `json:"id_movimiento"`
			IdProducto      int    `json:"id_producto"`
			NombreProducto  string `json:"nombre_producto"`
			TipoMovimiento  string `json:"tipo_movimiento"`
			Cantidad        int    `json:"cantidad"`
			StockResultante int    `json:"stock_resultante"`
			ReferenciaId    *int64 `json:"referencia_id"`
			FechaMovimiento string `json:"fecha_movimiento"`
		}

		idProductoStr := r.URL.Query().Get("id_producto")

		var rows *sql.Rows
		var err error
		const baseQuery = `
			SELECT m.id_movimiento, m.id_producto, p.nombre,
			       m.tipo_movimiento, m.cantidad, m.stock_resultante,
			       m.referencia_id, TO_CHAR(m.fecha_movimiento, 'YYYY-MM-DD HH24:MI:SS')
			FROM inventario.movimientos_stock m
			JOIN inventario.productos p ON m.id_producto = p.id_producto`

		if idProductoStr != "" {
			rows, err = db.Query(baseQuery+` WHERE m.id_producto = $1 ORDER BY m.fecha_movimiento DESC LIMIT 200`, idProductoStr)
		} else {
			rows, err = db.Query(baseQuery + ` ORDER BY m.fecha_movimiento DESC LIMIT 200`)
		}

		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar movimientos de stock."})
			return
		}
		defer rows.Close()

		movimientos := []Movimiento{}
		for rows.Next() {
			var m Movimiento
			var refId sql.NullInt64 // referencia_id puede ser NULL
			if err := rows.Scan(
				&m.IdMovimiento, &m.IdProducto, &m.NombreProducto,
				&m.TipoMovimiento, &m.Cantidad, &m.StockResultante,
				&refId, &m.FechaMovimiento,
			); err != nil {
				continue
			}
			if refId.Valid {
				m.ReferenciaId = &refId.Int64
			}
			movimientos = append(movimientos, m)
		}
		json.NewEncoder(w).Encode(movimientos)
	}
}
