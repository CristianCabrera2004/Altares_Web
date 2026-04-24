// Backend/handlers/devolucion_handler.go
// ─────────────────────────────────────────────────────────────────────────────
// HU-04 Módulo de Devoluciones (Arquitectura de Bajas y Devoluciones)
//
// POST /api/devoluciones
//   Registra la devolución de unidades de un producto previamente vendido.
//   Dentro de UNA transacción atómica (BEGIN/COMMIT/ROLLBACK):
//     1. Valida que la venta exista (opcional, si se provee id_venta)
//     2. Aumenta stock_actual del producto
//     3. Inserta en operaciones.devoluciones
//     4. Registra en inventario.movimientos_stock (tipo: DEVOLUCION)
//     5. Registra en seguridad.logs_auditoria (usuario, fecha, IP)
//
// GET /api/devoluciones
//   Lista el historial de devoluciones (con filtro opcional ?id_producto=X)
// ─────────────────────────────────────────────────────────────────────────────
package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
)

// DevolucionInput es el cuerpo esperado en POST /api/devoluciones.
type DevolucionInput struct {
	IdProducto       int    `json:"id_producto"`
	IdVenta          int    `json:"id_venta"`           // 0 = sin venta asociada
	IdUsuario        int    `json:"id_usuario"`
	CantidadDevuelta int    `json:"cantidad_devuelta"`
	Motivo           string `json:"motivo"`
}

// DevolucionHandler despacha GET y POST sobre /api/devoluciones.
func DevolucionHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			crearDevolucion(db, w, r)
		case http.MethodGet:
			listarDevoluciones(db, w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Método HTTP no soportado."})
		}
	}
}

// ─── POST ────────────────────────────────────────────────────────────────────
func crearDevolucion(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	var d DevolucionInput
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido o malformado."})
		return
	}

	// Validaciones de negocio
	if d.IdProducto <= 0 || d.IdUsuario <= 0 || d.CantidadDevuelta <= 0 || d.Motivo == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "'id_producto', 'id_usuario', 'cantidad_devuelta' (>0) y 'motivo' son obligatorios.",
		})
		return
	}

	// ── TRANSACCIÓN SQL: BEGIN / COMMIT / ROLLBACK ────────────────────────────
	tx, err := db.Begin()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "No se pudo iniciar la transacción."})
		return
	}
	defer tx.Rollback()

	// Paso 1 — Leer y bloquear stock actual del producto
	var stockActual int
	var nombreProducto string
	err = tx.QueryRow(
		`SELECT stock_actual, nombre FROM inventario.productos WHERE id_producto = $1 FOR UPDATE`,
		d.IdProducto,
	).Scan(&stockActual, &nombreProducto)
	if err == sql.ErrNoRows {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Producto no encontrado."})
		return
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar el producto."})
		return
	}

	// Si se provee id_venta, verificar que exista
	if d.IdVenta > 0 {
		var ventaExiste bool
		tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM operaciones.ventas WHERE id_venta = $1)`, d.IdVenta).Scan(&ventaExiste)
		if !ventaExiste {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "La venta con id_venta especificado no existe."})
			return
		}
	}

	nuevoStock := stockActual + d.CantidadDevuelta

	// Paso 2 — Aumenta el stock del producto (devolución repone mercadería)
	_, err = tx.Exec(
		`UPDATE inventario.productos SET stock_actual = $1 WHERE id_producto = $2`,
		nuevoStock, d.IdProducto,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al actualizar el stock."})
		return
	}

	// Paso 3 — Registrar en operaciones.devoluciones (relación con la venta)
	var idDevolucion int
	var ventaNullable *int
	if d.IdVenta > 0 {
		ventaNullable = &d.IdVenta
	}
	err = tx.QueryRow(`
		INSERT INTO operaciones.devoluciones
		  (id_venta, id_producto, id_usuario, cantidad_devuelta, motivo)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id_devolucion`,
		ventaNullable, d.IdProducto, d.IdUsuario, d.CantidadDevuelta, d.Motivo,
	).Scan(&idDevolucion)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al registrar la devolución."})
		return
	}

	// Paso 4 — Registrar en movimientos_stock (tipo: DEVOLUCION — aumenta stock)
	_, err = tx.Exec(`
		INSERT INTO inventario.movimientos_stock
		  (id_producto, id_usuario, tipo_movimiento, cantidad, stock_resultante, referencia_id)
		VALUES ($1, $2, 'DEVOLUCION', $3, $4, $5)`,
		d.IdProducto, d.IdUsuario, d.CantidadDevuelta, nuevoStock, idDevolucion,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al registrar el movimiento de stock."})
		return
	}

	// Paso 5 — Log de auditoría (usuario, fecha automática, IP)
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.RemoteAddr
	}
	_, err = tx.Exec(`
		INSERT INTO seguridad.logs_auditoria
		  (id_usuario, accion, tabla_afectada, id_registro_afectado, valor_anterior, valor_nuevo, ip_origen)
		VALUES ($1, 'DEVOLUCION', 'inventario.productos', $2, $3, $4, $5)`,
		d.IdUsuario, d.IdProducto,
		strconv.Itoa(stockActual), strconv.Itoa(nuevoStock), ip,
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
		"mensaje":        "Devolución registrada. Stock repuesto exitosamente.",
		"id_devolucion":  idDevolucion,
		"stock_nuevo":    nuevoStock,
		"nombre_producto": nombreProducto,
	})
}

// ─── GET ─────────────────────────────────────────────────────────────────────
func listarDevoluciones(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	type DevolucionRow struct {
		IdDevolucion     int    `json:"id_devolucion"`
		IdVenta          *int   `json:"id_venta"`
		IdProducto       int    `json:"id_producto"`
		NombreProducto   string `json:"nombre_producto"`
		IdUsuario        int    `json:"id_usuario"`
		CantidadDevuelta int    `json:"cantidad_devuelta"`
		Motivo           string `json:"motivo"`
		FechaDevolucion  string `json:"fecha_devolucion"`
	}

	idProductoStr := r.URL.Query().Get("id_producto")
	const base = `
		SELECT d.id_devolucion, d.id_venta, d.id_producto, p.nombre,
		       d.id_usuario, d.cantidad_devuelta, d.motivo,
		       TO_CHAR(d.fecha_devolucion, 'YYYY-MM-DD HH24:MI:SS')
		FROM operaciones.devoluciones d
		JOIN inventario.productos p ON d.id_producto = p.id_producto`

	var rows *sql.Rows
	var err error
	if idProductoStr != "" {
		rows, err = db.Query(base+` WHERE d.id_producto = $1 ORDER BY d.fecha_devolucion DESC LIMIT 200`, idProductoStr)
	} else {
		rows, err = db.Query(base + ` ORDER BY d.fecha_devolucion DESC LIMIT 200`)
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar devoluciones."})
		return
	}
	defer rows.Close()

	result := []DevolucionRow{}
	for rows.Next() {
		var d DevolucionRow
		var idVenta sql.NullInt64
		if err := rows.Scan(
			&d.IdDevolucion, &idVenta, &d.IdProducto, &d.NombreProducto,
			&d.IdUsuario, &d.CantidadDevuelta, &d.Motivo, &d.FechaDevolucion,
		); err != nil {
			continue
		}
		if idVenta.Valid {
			v := int(idVenta.Int64)
			d.IdVenta = &v
		}
		result = append(result, d)
	}
	json.NewEncoder(w).Encode(result)
}
