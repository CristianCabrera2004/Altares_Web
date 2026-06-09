// Backend/handlers/devolucion_handler.go
// ─────────────────────────────────────────────────────────────────────────────
// HU-04 Módulo de Devoluciones (Arquitectura de Bajas y Devoluciones)
//
// POST /api/devoluciones
//   Registra la devolución/cambio de un producto. Puede combinarse con merma:
//     1. Devolución buen estado → repone stock
//     2. Devolución mal estado  → baja merma, no repone
//     3. Cambio buen estado     → repone orig + entrega nuevo + dif. precio
//     4. Cambio mal estado      → baja merma orig + entrega nuevo + dif. precio
//
// GET /api/devoluciones
//   Lista el historial de devoluciones con JOIN para obtener productos de cambio
// ─────────────────────────────────────────────────────────────────────────────
package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// DevolucionInput es el cuerpo esperado en POST /api/devoluciones.
type DevolucionInput struct {
	IdProducto       int    `json:"id_producto"`
	IdVenta          int    `json:"id_venta"` // 0 = sin venta asociada
	IdUsuario        int    `json:"id_usuario"`
	CantidadDevuelta int    `json:"cantidad_devuelta"`
	Motivo           string `json:"motivo"`
	
	Tipo             string `json:"tipo"`               // "DEVOLUCION" o "CAMBIO"
	EnMalEstado      bool   `json:"en_mal_estado"`      // true = baja merma
	IdProductoCambio int    `json:"id_producto_cambio"` // para CAMBIO
	CantidadCambio   int    `json:"cantidad_cambio"`    // para CAMBIO
}

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

// getTiendaIDForDevolucion has been replaced by GetTiendaIDFromCtxOrDb


// ─── POST ────────────────────────────────────────────────────────────────────
func crearDevolucion(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	var d DevolucionInput
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido o malformado."})
		return
	}

	if d.IdProducto <= 0 || d.IdUsuario <= 0 || d.CantidadDevuelta <= 0 || d.Motivo == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "'id_producto', 'id_usuario', 'cantidad_devuelta' (>0) y 'motivo' son obligatorios.",
		})
		return
	}

	if d.Tipo == "" {
		d.Tipo = "DEVOLUCION"
	}

	if d.Tipo == "CAMBIO" {
		if d.IdProductoCambio <= 0 || d.CantidadCambio <= 0 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "Para un cambio, 'id_producto_cambio' y 'cantidad_cambio' (>0) son obligatorios.",
			})
			return
		}
	}

	idTienda := GetTiendaIDFromCtxOrDb(db, r)

	tx, err := db.Begin()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "No se pudo iniciar la transacción."})
		return
	}
	defer tx.Rollback()

	if d.IdVenta > 0 {
		var ventaExiste bool
		tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM operaciones.ventas WHERE id_venta = $1)`, d.IdVenta).Scan(&ventaExiste)
		if !ventaExiste {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "La venta con id_venta especificado no existe."})
			return
		}
	}

	// ── 1. PRODUCTO ORIGINAL ──
	var stockActual int
	var precioVentaOrig int
	var nombreProducto string

	// Primero obtenemos el precio y el nombre
	err = tx.QueryRow(`SELECT precio_venta, nombre FROM inventario.productos WHERE id_producto = $1`, d.IdProducto).Scan(&precioVentaOrig, &nombreProducto)
	if err == sql.ErrNoRows {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Producto original no encontrado en el catálogo."})
		return
	}

	// Luego obtenemos y bloqueamos el stock en la tienda (crear si no existe)
	err = tx.QueryRow(
		`INSERT INTO inventario.stock_tiendas (id_tienda, id_producto, stock_actual, stock_alerta_min)
		 VALUES ($1, $2, 0, 5)
		 ON CONFLICT (id_tienda, id_producto)
		 DO UPDATE SET stock_actual = inventario.stock_tiendas.stock_actual
		 RETURNING stock_actual`,
		idTienda, d.IdProducto,
	).Scan(&stockActual)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar stock original de la tienda."})
		return
	}

	nuevoStockOrig := stockActual
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" { ip = r.RemoteAddr }

	if d.EnMalEstado {
		// ── FLUJO: MAL ESTADO (MERMA) ──
		var idBaja int
		err = tx.QueryRow(`
			INSERT INTO inventario.bajas_inventario (id_producto, id_usuario, id_tienda, cantidad_baja, motivo)
			VALUES ($1, $2, $3, $4, $5) RETURNING id_baja`,
			d.IdProducto, d.IdUsuario, idTienda, d.CantidadDevuelta, "Merma de devolución: " + d.Motivo,
		).Scan(&idBaja)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al registrar merma."})
			return
		}

		_, err = tx.Exec(`
			INSERT INTO inventario.movimientos_stock
			  (id_producto, id_usuario, id_tienda, tipo_movimiento, cantidad, stock_resultante, referencia_id)
			VALUES ($1, $2, $3, 'BAJA_MERMA', $4, $5, $6)`,
			d.IdProducto, d.IdUsuario, idTienda, 0, stockActual, idBaja,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al registrar mov. merma."})
			return
		}
		
		tx.Exec(`
			INSERT INTO seguridad.logs_auditoria
			  (id_usuario, accion, tabla_afectada, id_registro_afectado, valor_anterior, valor_nuevo, ip_origen)
			VALUES ($1, 'DEVOLUCION_MERMA', 'inventario.stock_tiendas', $2, $3, $4, $5)`,
			d.IdUsuario, d.IdProducto, strconv.Itoa(stockActual), strconv.Itoa(stockActual), ip,
		)

	} else {
		// ── FLUJO: BUEN ESTADO (DEVOLUCION NORMAL) ──
		nuevoStockOrig = stockActual + d.CantidadDevuelta
		_, err = tx.Exec(`UPDATE inventario.stock_tiendas SET stock_actual = $1 WHERE id_producto = $2 AND id_tienda = $3`, nuevoStockOrig, d.IdProducto, idTienda)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error actualizando stock original."})
			return
		}
	}

	// ── 2. PRODUCTO DE CAMBIO ──
	var idProductoCambioNullable *int
	var cantidadCambioNullable *int
	diferenciaPrecio := 0

	if d.Tipo == "CAMBIO" {
		var stockCambio int
		var precioVentaNuevo int

		err = tx.QueryRow(`SELECT precio_venta FROM inventario.productos WHERE id_producto = $1`, d.IdProductoCambio).Scan(&precioVentaNuevo)
		if err == sql.ErrNoRows {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Producto de cambio no encontrado."})
			return
		}

		err = tx.QueryRow(
			`SELECT stock_actual FROM inventario.stock_tiendas WHERE id_producto = $1 AND id_tienda = $2 FOR UPDATE`,
			d.IdProductoCambio, idTienda,
		).Scan(&stockCambio)
		if err == sql.ErrNoRows {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "El producto de cambio no tiene stock asignado en esta tienda."})
			return
		}
		
		if d.CantidadCambio > stockCambio {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("Stock insuficiente para cambio. Disponible: %d", stockCambio)})
			return
		}

		nuevoStockCambio := stockCambio - d.CantidadCambio
		_, err = tx.Exec(`UPDATE inventario.stock_tiendas SET stock_actual = $1 WHERE id_producto = $2 AND id_tienda = $3`, nuevoStockCambio, d.IdProductoCambio, idTienda)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error descontando stock de cambio."})
			return
		}

		totalOriginal := precioVentaOrig * d.CantidadDevuelta
		totalNuevo := precioVentaNuevo * d.CantidadCambio
		diferenciaPrecio = totalNuevo - totalOriginal

		idProductoCambioNullable = &d.IdProductoCambio
		cantidadCambioNullable = &d.CantidadCambio
	}

	// ── 3. INSERTAR OPERACION DE DEVOLUCION ──
	var idDevolucion int
	var ventaNullable *int
	if d.IdVenta > 0 { ventaNullable = &d.IdVenta }
	
	err = tx.QueryRow(`
		INSERT INTO operaciones.devoluciones
		  (id_venta, id_producto, id_usuario, id_tienda, cantidad_devuelta, motivo, tipo, en_mal_estado, id_producto_cambio, cantidad_cambio, diferencia_precio)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id_devolucion`,
		ventaNullable, d.IdProducto, d.IdUsuario, idTienda, d.CantidadDevuelta, d.Motivo,
		d.Tipo, d.EnMalEstado, idProductoCambioNullable, cantidadCambioNullable, diferenciaPrecio,
	).Scan(&idDevolucion)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al registrar la operación de devolución."})
		return
	}

	// ── 4. MOVIMIENTOS STOCK ──
	if !d.EnMalEstado {
		_, err = tx.Exec(`
			INSERT INTO inventario.movimientos_stock
			  (id_producto, id_usuario, id_tienda, tipo_movimiento, cantidad, stock_resultante, referencia_id)
			VALUES ($1, $2, $3, 'DEVOLUCION', $4, $5, $6)`,
			d.IdProducto, d.IdUsuario, idTienda, d.CantidadDevuelta, nuevoStockOrig, idDevolucion,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error registrando mov. reposición."})
			return
		}
	}

	if d.Tipo == "CAMBIO" {
		var stockRealCambio int
		tx.QueryRow(`SELECT stock_actual FROM inventario.stock_tiendas WHERE id_producto = $1 AND id_tienda = $2`, d.IdProductoCambio, idTienda).Scan(&stockRealCambio)

		_, err = tx.Exec(`
			INSERT INTO inventario.movimientos_stock
			  (id_producto, id_usuario, id_tienda, tipo_movimiento, cantidad, stock_resultante, referencia_id)
			VALUES ($1, $2, $3, 'VENTA', $4, $5, $6)`,
			d.IdProductoCambio, d.IdUsuario, idTienda, -d.CantidadCambio, stockRealCambio, idDevolucion,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error registrando mov. cambio."})
			return
		}
	}

	if err := tx.Commit(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error confirmando la transacción."})
		return
	}

	msg := "Devolución registrada."
	if d.Tipo == "CAMBIO" {
		msg = "Cambio registrado."
		if d.EnMalEstado {
			msg += " Producto original dado de baja por merma."
		}
	} else if d.EnMalEstado {
		msg = "Merma registrada por devolución."
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"mensaje":        msg,
		"id_devolucion":  idDevolucion,
		"tipo":           d.Tipo,
		"en_mal_estado":  d.EnMalEstado,
		"stock_nuevo":    nuevoStockOrig, // Del original
		"nombre_producto": nombreProducto,
		"diferencia_precio": diferenciaPrecio,
	})
}

// ─── GET ─────────────────────────────────────────────────────────────────────
func listarDevoluciones(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	type DevolucionRow struct {
		IdDevolucion         int    `json:"id_devolucion"`
		IdVenta              *int   `json:"id_venta"`
		IdProducto           int    `json:"id_producto"`
		NombreProducto       string `json:"nombre_producto"`
		IdUsuario            int    `json:"id_usuario"`
		CantidadDevuelta     int    `json:"cantidad_devuelta"`
		Motivo               string `json:"motivo"`
		Tipo                 string `json:"tipo"`
		EnMalEstado          bool   `json:"en_mal_estado"`
		IdProductoCambio     *int   `json:"id_producto_cambio"`
		NombreProductoCambio *string `json:"nombre_producto_cambio"`
		CantidadCambio       *int   `json:"cantidad_cambio"`
		DiferenciaPrecio     int    `json:"diferencia_precio"`
		FechaDevolucion      string `json:"fecha_devolucion"`
	}

	idProductoStr := r.URL.Query().Get("id_producto")
	idTienda := GetTiendaIDFromCtxOrDb(db, r)

	const base = `
		SELECT d.id_devolucion, d.id_venta, d.id_producto, p.nombre,
		       d.id_usuario, d.cantidad_devuelta, d.motivo,
		       d.tipo, d.en_mal_estado, d.id_producto_cambio, pc.nombre AS nombre_cambio, d.cantidad_cambio, d.diferencia_precio,
		       TO_CHAR(d.fecha_devolucion, 'YYYY-MM-DD HH24:MI:SS')
		FROM operaciones.devoluciones d
		JOIN inventario.productos p ON d.id_producto = p.id_producto
		LEFT JOIN inventario.productos pc ON d.id_producto_cambio = pc.id_producto
		WHERE d.id_tienda = $1`

	var rows *sql.Rows
	var err error
	if idProductoStr != "" {
		rows, err = db.Query(base+` AND d.id_producto = $2 ORDER BY d.fecha_devolucion DESC LIMIT 200`, idTienda, idProductoStr)
	} else {
		rows, err = db.Query(base + ` ORDER BY d.fecha_devolucion DESC LIMIT 200`, idTienda)
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
		var idProdCambio sql.NullInt64
		var cantCambio sql.NullInt64
		var nomCambio sql.NullString

		if err := rows.Scan(
			&d.IdDevolucion, &idVenta, &d.IdProducto, &d.NombreProducto,
			&d.IdUsuario, &d.CantidadDevuelta, &d.Motivo,
			&d.Tipo, &d.EnMalEstado, &idProdCambio, &nomCambio, &cantCambio, &d.DiferenciaPrecio,
			&d.FechaDevolucion,
		); err != nil {
			continue
		}
		
		if idVenta.Valid { v := int(idVenta.Int64); d.IdVenta = &v }
		if idProdCambio.Valid { v := int(idProdCambio.Int64); d.IdProductoCambio = &v }
		if cantCambio.Valid { v := int(cantCambio.Int64); d.CantidadCambio = &v }
		if nomCambio.Valid { d.NombreProductoCambio = &nomCambio.String }

		result = append(result, d)
	}
	json.NewEncoder(w).Encode(result)
}
