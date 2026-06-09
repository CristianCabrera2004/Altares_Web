// Backend/handlers/inventory_handler.go
// ─────────────────────────────────────────────────────────────────────────────
// Endpoints transaccionales de inventario (CA 45: BEGIN/COMMIT/ROLLBACK):
//
//   POST /api/inventario/ingreso      → Registra entrada de stock
//   POST /api/inventario/baja         → Registra baja/merma de stock
//   GET  /api/inventario/movimientos  → Consulta el historial de movimientos
//
// Cada operación de escritura es ATÓMICA: actualiza ingreso/baja + stock
// en la tienda + movimientos_stock en la MISMA transacción.
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

// getTiendaID has been replaced by GetTiendaIDFromCtxOrDb


// ─── POST /api/inventario/ingreso ────────────────────────────────────────────
// IngresoHandler registra una entrada de mercadería de forma transaccional.
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

		if ing.IdProducto <= 0 || ing.IdUsuario <= 0 || ing.CantidadIngresada <= 0 || ing.CostoUnitario < 0 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "'id_producto', 'id_usuario', 'cantidad_ingresada' (>0) y 'costo_unitario' (>=0) son obligatorios.",
			})
			return
		}

		idTienda := GetTiendaIDFromCtxOrDb(db, r)

		tx, err := db.Begin()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "No se pudo iniciar la transacción."})
			return
		}
		defer tx.Rollback()

		// Upsert del stock en la tienda
		var nuevoStock int
		err = tx.QueryRow(`
			INSERT INTO inventario.stock_tiendas (id_tienda, id_producto, stock_actual, stock_alerta_min)
			VALUES ($1, $2, $3, 5)
			ON CONFLICT (id_tienda, id_producto)
			DO UPDATE SET stock_actual = inventario.stock_tiendas.stock_actual + $3
			RETURNING stock_actual`,
			idTienda, ing.IdProducto, ing.CantidadIngresada,
		).Scan(&nuevoStock)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al actualizar el stock de la tienda."})
			return
		}

		// Insertar registro de ingreso
		var idIngreso int
		var proveedorNullable *int
		if ing.IdProveedor > 0 {
			proveedorNullable = &ing.IdProveedor
		}
		err = tx.QueryRow(`
			INSERT INTO inventario.ingreso_inventario
			  (id_producto, id_proveedor, id_usuario, id_tienda, cantidad_ingresada, costo_unitario, observacion)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id_ingreso`,
			ing.IdProducto, proveedorNullable, ing.IdUsuario, idTienda,
			ing.CantidadIngresada, ing.CostoUnitario, ing.Observacion,
		).Scan(&idIngreso)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al registrar el ingreso en la base de datos."})
			return
		}

		// Registrar movimiento
		_, err = tx.Exec(`
			INSERT INTO inventario.movimientos_stock
			  (id_producto, id_usuario, id_tienda, tipo_movimiento, cantidad, stock_resultante, referencia_id)
			VALUES ($1, $2, $3, 'INGRESO', $4, $5, $6)`,
			ing.IdProducto, ing.IdUsuario, idTienda, ing.CantidadIngresada, nuevoStock, idIngreso,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al registrar el movimiento de stock."})
			return
		}

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

		idTienda := GetTiendaIDFromCtxOrDb(db, r)

		tx, err := db.Begin()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "No se pudo iniciar la transacción."})
			return
		}
		defer tx.Rollback()

		var stockActual int
		err = tx.QueryRow(
			`SELECT stock_actual FROM inventario.stock_tiendas WHERE id_producto = $1 AND id_tienda = $2 FOR UPDATE`,
			baja.IdProducto, idTienda,
		).Scan(&stockActual)
		if err == sql.ErrNoRows {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Producto no encontrado o sin stock en esta tienda."})
			return
		}
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar el stock."})
			return
		}

		if baja.CantidadBaja > stockActual {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "Stock insuficiente. No se puede dar de baja " + strconv.Itoa(baja.CantidadBaja) +
					" unidades. Stock disponible: " + strconv.Itoa(stockActual) + ".",
			})
			return
		}

		nuevoStock := stockActual - baja.CantidadBaja

		var idBaja int
		err = tx.QueryRow(`
			INSERT INTO inventario.bajas_inventario (id_producto, id_usuario, id_tienda, cantidad_baja, motivo)
			VALUES ($1, $2, $3, $4, $5) RETURNING id_baja`,
			baja.IdProducto, baja.IdUsuario, idTienda, baja.CantidadBaja, baja.Motivo,
		).Scan(&idBaja)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al registrar la baja."})
			return
		}

		_, err = tx.Exec(
			`UPDATE inventario.stock_tiendas SET stock_actual = $1 WHERE id_producto = $2 AND id_tienda = $3`,
			nuevoStock, baja.IdProducto, idTienda,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al actualizar el stock."})
			return
		}

		_, err = tx.Exec(`
			INSERT INTO inventario.movimientos_stock
			  (id_producto, id_usuario, id_tienda, tipo_movimiento, cantidad, stock_resultante, referencia_id)
			VALUES ($1, $2, $3, 'BAJA_MERMA', $4, $5, $6)`,
			baja.IdProducto, baja.IdUsuario, idTienda, -baja.CantidadBaja, nuevoStock, idBaja,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al registrar el movimiento de stock."})
			return
		}

		if err := tx.Commit(); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al confirmar la transacción."})
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"mensaje":     "Baja de merma registrada exitosamente.",
			"id_baja":     idBaja,
			"stock_nuevo": nuevoStock,
			"motivo":      baja.Motivo,
		})
	}
}

// ─── GET /api/inventario/movimientos ────────────────────────────────────────
// MovimientosHandler consulta el historial de movimientos de stock.
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
		idTienda := GetTiendaIDFromCtxOrDb(db, r)

		var rows *sql.Rows
		var err error
		const baseQuery = `
			SELECT m.id_movimiento, m.id_producto, p.nombre,
			       m.tipo_movimiento, m.cantidad, m.stock_resultante,
			       m.referencia_id, TO_CHAR(m.fecha_movimiento, 'YYYY-MM-DD HH24:MI:SS')
			FROM inventario.movimientos_stock m
			JOIN inventario.productos p ON m.id_producto = p.id_producto
			WHERE m.id_tienda = $1`

		if idProductoStr != "" {
			rows, err = db.Query(baseQuery+` AND m.id_producto = $2 ORDER BY m.fecha_movimiento DESC LIMIT 200`, idTienda, idProductoStr)
		} else {
			rows, err = db.Query(baseQuery+` ORDER BY m.fecha_movimiento DESC LIMIT 200`, idTienda)
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
