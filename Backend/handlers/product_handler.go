// Backend/handlers/product_handler.go
// ─────────────────────────────────────────────────────────────────────────────
// HT-02 — Catálogo de Productos (MULTITIENDA)
//
// Implementa los cuatro métodos HTTP estándar sobre /api/productos (CA 43):
//   GET    /api/productos          → Lista todo el catálogo activo con stock de la tienda
//   GET    /api/productos?id=X     → Devuelve un producto específico con stock de la tienda
//   POST   /api/productos          → Crea un producto (con transacción SQL)
//   PUT    /api/productos?id=X     → Actualiza un producto (con transacción SQL)
//   DELETE /api/productos?id=X     → Baja lógica: estado = 'inactivo' (con transacción SQL)
//
// El stock se obtiene de inventario.stock_tiendas filtrado por la tienda del usuario JWT.
// ─────────────────────────────────────────────────────────────────────────────
package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"libreria-altares/middleware"
	"libreria-altares/utils"
)

// Producto mapea la tabla inventario.productos + stock de la tienda actual.
// precio_venta se maneja en centavos (INT en la BD, igual que en el esquema).
// tasa_iva proviene del JOIN con inventario.categorias (0 ó 15).
type Producto struct {
	IdProducto      int    `json:"id_producto"`
	Nombre          string `json:"nombre"`
	IdCategoria     int    `json:"id_categoria"`
	NombreCategoria string `json:"nombre_categoria,omitempty"`
	TasaIva         int    `json:"tasa_iva"`           // 0% (papel.) o 15% (HU-01 CA 3)
	StockActual     int    `json:"stock_actual"`
	StockAlertaMin  int    `json:"stock_alerta_min"`
	PrecioVenta     int    `json:"precio_venta"` // centavos
	Estado          string `json:"estado"`
	CodigoBarras    string `json:"codigo_barras,omitempty"` // código EAN/UPC ligado al producto
}

// ProductHandler despacha las peticiones según el método HTTP recibido (CA 43).
func ProductHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// CA 44: todas las respuestas son JSON
		w.Header().Set("Content-Type", "application/json")

		switch r.Method {
		case http.MethodGet:
			getProducts(db, w, r)
		case http.MethodPost:
			createProduct(db, w, r)
		case http.MethodPut:
			updateProduct(db, w, r)
		case http.MethodDelete:
			deleteProduct(db, w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Método HTTP no soportado en este endpoint."})
		}
	}
}

// getTiendaFromRequest has been replaced by GetTiendaIDFromCtxOrDb


// ─── GET ─────────────────────────────────────────────────────────────────────
// getProducts lista todo el catálogo o un producto específico si se pasa ?id=X.
// El stock viene de inventario.stock_tiendas filtrado por la tienda del JWT.
func getProducts(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	idTienda := GetTiendaIDFromCtxOrDb(db, r)

	// Si se especifica un parámetro tienda, permitimos leer el stock de esa sucursal
	tiendaParam := r.URL.Query().Get("tienda")
	if tiendaParam != "" {
		if t, err := strconv.Atoi(tiendaParam); err == nil && t > 0 {
			idTienda = t
		}
	}

	// Consulta de un producto específico
	if idStr != "" {
		id, err := strconv.Atoi(idStr)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "El parámetro ?id debe ser un número entero positivo."})
			return
		}

		var p Producto
		err = db.QueryRow(`
			SELECT p.id_producto, p.nombre, p.id_categoria, c.nombre, c.tasa_iva,
			       COALESCE(st.stock_actual, 0), COALESCE(st.stock_alerta_min, 5),
			       p.precio_venta, p.estado
			FROM inventario.productos p
			JOIN inventario.categorias c ON p.id_categoria = c.id_categoria
			LEFT JOIN inventario.stock_tiendas st ON p.id_producto = st.id_producto AND st.id_tienda = $2
			WHERE p.id_producto = $1`, id, idTienda,
		).Scan(&p.IdProducto, &p.Nombre, &p.IdCategoria, &p.NombreCategoria, &p.TasaIva,
			&p.StockActual, &p.StockAlertaMin, &p.PrecioVenta, &p.Estado)

		if err == sql.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Producto no encontrado."})
			return
		}
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error interno al consultar el producto."})
			return
		}
		json.NewEncoder(w).Encode(p)
		return
	}

	// Listar todo el catálogo activo con stock de la tienda.
	// Soporte de ?stock_bajo=true para filtrar solo productos con stock <= stock_alerta_min
	stockBajoFilter := r.URL.Query().Get("stock_bajo") == "true"
	query := `
		SELECT p.id_producto, p.nombre, p.id_categoria, c.nombre, c.tasa_iva,
		       COALESCE(st.stock_actual, 0), COALESCE(st.stock_alerta_min, 5),
		       p.precio_venta, p.estado
		FROM inventario.productos p
		JOIN inventario.categorias c ON p.id_categoria = c.id_categoria
		LEFT JOIN inventario.stock_tiendas st ON p.id_producto = st.id_producto AND st.id_tienda = $1
		WHERE p.estado = 'activo'`
	if stockBajoFilter {
		query += ` AND COALESCE(st.stock_actual, 0) <= COALESCE(st.stock_alerta_min, 5)`
	}
	query += ` ORDER BY p.nombre ASC`

	rows, err := db.Query(query, idTienda)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error interno al listar el catálogo de productos."})
		return
	}
	defer rows.Close()

	productos := []Producto{}
	for rows.Next() {
		var p Producto
		if err := rows.Scan(
			&p.IdProducto, &p.Nombre, &p.IdCategoria, &p.NombreCategoria, &p.TasaIva,
			&p.StockActual, &p.StockAlertaMin, &p.PrecioVenta, &p.Estado,
		); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error interno al leer filas del catálogo."})
			return
		}
		productos = append(productos, p)
	}
	json.NewEncoder(w).Encode(productos)
}


// ─── POST ────────────────────────────────────────────────────────────────────
// createProduct verifica primero si el código de barras ya existe:
//   - Si existe  → incrementa stock de la tienda del producto ligado (sin duplicar)
//   - Si no existe → crea el producto e inicializa stock en la tienda
func createProduct(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	var p Producto
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Cuerpo JSON inválido o malformado."})
		return
	}

	idTienda := GetTiendaIDFromCtxOrDb(db, r)

	// ── VERIFICACIÓN DE CÓDIGO DE BARRAS (Capa Lógica de Negocio) ────────────
	// Si se envía codigo_barras, buscar si ya está ligado a algún producto.
	if p.CodigoBarras != "" {
		var idExistente int
		err := db.QueryRow(
			`SELECT id_producto FROM inventario.codigos_barras WHERE codigo = $1`,
			p.CodigoBarras,
		).Scan(&idExistente)

		if err == nil {
			// ── PRODUCTO EXISTENTE: incrementar stock de la tienda ──────────
			cantidad := p.StockActual
			if cantidad <= 0 {
				cantidad = 1
			}
			tx, err := db.Begin()
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "No se pudo iniciar la transacción."})
				return
			}
			defer tx.Rollback()

			// UPSERT: si ya existe stock para esta tienda, incrementar; si no, crear
			_, err = tx.Exec(`
				INSERT INTO inventario.stock_tiendas (id_tienda, id_producto, stock_actual, stock_alerta_min)
				VALUES ($1, $2, $3, 5)
				ON CONFLICT (id_tienda, id_producto)
				DO UPDATE SET stock_actual = inventario.stock_tiendas.stock_actual + $3`,
				idTienda, idExistente, cantidad)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "Error al incrementar el stock del producto."})
				return
			}

			// Recuperar producto actualizado para respuesta
			var updated Producto
			err = tx.QueryRow(`
				SELECT p.id_producto, p.nombre, p.id_categoria,
				       COALESCE(st.stock_actual, 0), COALESCE(st.stock_alerta_min, 5),
				       p.precio_venta, p.estado
				FROM inventario.productos p
				LEFT JOIN inventario.stock_tiendas st ON p.id_producto = st.id_producto AND st.id_tienda = $2
				WHERE p.id_producto = $1`,
				idExistente, idTienda,
			).Scan(&updated.IdProducto, &updated.Nombre, &updated.IdCategoria,
				&updated.StockActual, &updated.StockAlertaMin, &updated.PrecioVenta, &updated.Estado)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "Error al recuperar producto actualizado."})
				return
			}

			if err := tx.Commit(); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "Error al confirmar la transacción."})
				return
			}
			updated.CodigoBarras = p.CodigoBarras
			// HTTP 200: stock actualizado (no es un recurso nuevo)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"accion":   "stock_incrementado",
				"mensaje":  "Stock actualizado exitosamente.",
				"producto": updated,
			})
			return
		}
		if err != sql.ErrNoRows {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al verificar el código de barras."})
			return
		}
		// err == sql.ErrNoRows → código nuevo, continuar con la creación
	}

	// ── PRODUCTO NUEVO: validaciones y creación ───────────────────────────────
	if p.Nombre == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "El campo 'nombre' es obligatorio."})
		return
	}
	if p.IdCategoria <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "El campo 'id_categoria' debe ser un entero positivo válido."})
		return
	}
	if p.PrecioVenta < 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "El campo 'precio_venta' (en centavos) no puede ser negativo."})
		return
	}

	var categoriaExiste bool
	db.QueryRow(`SELECT EXISTS(SELECT 1 FROM inventario.categorias WHERE id_categoria = $1)`,
		p.IdCategoria).Scan(&categoriaExiste)
	if !categoriaExiste {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "La categoría con id_categoria especificado no existe."})
		return
	}

	// ── TRANSACCIÓN SQL: INSERT producto + stock tienda + código de barras ────
	tx, err := db.Begin()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "No se pudo iniciar la transacción."})
		return
	}
	defer tx.Rollback()

	err = tx.QueryRow(`
		INSERT INTO inventario.productos (nombre, id_categoria, precio_venta, estado)
		VALUES ($1, $2, $3, 'activo')
		RETURNING id_producto`,
		p.Nombre, p.IdCategoria, p.PrecioVenta,
	).Scan(&p.IdProducto)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al insertar el producto en la base de datos."})
		return
	}

	// Insertar stock inicial para la tienda del usuario
	_, err = tx.Exec(`
		INSERT INTO inventario.stock_tiendas (id_tienda, id_producto, stock_actual, stock_alerta_min)
		VALUES ($1, $2, $3, $4)`,
		idTienda, p.IdProducto, p.StockActual, p.StockAlertaMin)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al inicializar el stock del producto en la tienda."})
		return
	}

	// Liga el código de barras si fue provisto (misma transacción)
	if p.CodigoBarras != "" {
		_, err = tx.Exec(
			`INSERT INTO inventario.codigos_barras (id_producto, codigo) VALUES ($1, $2)`,
			p.IdProducto, p.CodigoBarras,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al registrar el código de barras (puede estar duplicado)."})
			return
		}
	}

	if err := tx.Commit(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al confirmar la transacción."})
		return
	}

	p.Estado = "activo"
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"accion":   "producto_creado",
		"mensaje":  "Producto creado exitosamente.",
		"producto": p,
	})
}

// ─── GET /api/productos/buscar ────────────────────────────────────────────────
// BuscarProductoHandler busca un producto por su código de barras.
// Devuelve stock de la tienda del usuario JWT.
func BuscarProductoHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Solo se acepta GET en este endpoint."})
			return
		}
		codigo := r.URL.Query().Get("codigo")
		if codigo == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "El parámetro ?codigo es obligatorio."})
			return
		}

		idTienda := GetTiendaIDFromCtxOrDb(db, r)

		var p Producto
		err := db.QueryRow(`
			SELECT p.id_producto, p.nombre, p.id_categoria, c.nombre, c.tasa_iva,
			       COALESCE(st.stock_actual, 0), COALESCE(st.stock_alerta_min, 5),
			       p.precio_venta, p.estado, cb.codigo
			FROM inventario.codigos_barras cb
			JOIN inventario.productos p ON cb.id_producto = p.id_producto
			JOIN inventario.categorias c ON p.id_categoria = c.id_categoria
			LEFT JOIN inventario.stock_tiendas st ON p.id_producto = st.id_producto AND st.id_tienda = $2
			WHERE cb.codigo = $1`, codigo, idTienda,
		).Scan(&p.IdProducto, &p.Nombre, &p.IdCategoria, &p.NombreCategoria, &p.TasaIva,
			&p.StockActual, &p.StockAlertaMin, &p.PrecioVenta, &p.Estado, &p.CodigoBarras)

		if err == sql.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "No se encontró ningún producto con ese código de barras."})
			return
		}
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al buscar el producto."})
			return
		}
		json.NewEncoder(w).Encode(p)
	}
}

// ─── PUT ─────────────────────────────────────────────────────────────────────
// updateProduct actualiza todos los campos de un producto en transacción (CA 45).
// Actualiza el catálogo global y el stock de la tienda del usuario.
func updateProduct(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "El parámetro ?id es obligatorio para actualizar un producto."})
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "El parámetro ?id debe ser un número entero positivo."})
		return
	}

	var p Producto
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Cuerpo JSON inválido."})
		return
	}

	idTienda := GetTiendaIDFromCtxOrDb(db, r)

	// Validaciones de negocio → HTTP 400 (CA 45)
	if p.Nombre == "" || p.IdCategoria <= 0 || p.PrecioVenta < 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Los campos 'nombre', 'id_categoria' (>0) y 'precio_venta' (>=0) son obligatorios y válidos.",
		})
		return
	}
	if p.Estado == "" {
		p.Estado = "activo"
	}

	// Recuperar el precio actual para auditoría (HU-08 CA 37)
	var precioAnterior int
	err = db.QueryRow(`SELECT precio_venta FROM inventario.productos WHERE id_producto = $1`, id).Scan(&precioAnterior)
	if err != nil && err != sql.ErrNoRows {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error interno al recuperar el producto."})
		return
	}

	// ── TRANSACCIÓN SQL: BEGIN / COMMIT / ROLLBACK (CA 45) ───────────────────
	tx, err := db.Begin() // BEGIN
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "No se pudo iniciar la transacción."})
		return
	}
	defer tx.Rollback()

	// Actualizar catálogo global (nombre, categoría, precio, estado)
	result, err := tx.Exec(`
		UPDATE inventario.productos
		SET nombre = $1, id_categoria = $2, precio_venta = $3, estado = $4
		WHERE id_producto = $5`,
		p.Nombre, p.IdCategoria, p.PrecioVenta, p.Estado, id,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al actualizar el producto."})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "No se encontró un producto con el id_producto proporcionado."})
		return
	}

	// Actualizar stock de la tienda (UPSERT)
	_, err = tx.Exec(`
		INSERT INTO inventario.stock_tiendas (id_tienda, id_producto, stock_actual, stock_alerta_min)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id_tienda, id_producto)
		DO UPDATE SET stock_actual = $3, stock_alerta_min = $4`,
		idTienda, id, p.StockActual, p.StockAlertaMin)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al actualizar el stock de la tienda."})
		return
	}

	// Manejo del código de barras
	if p.CodigoBarras != "" {
		// Verificar que el código no pertenezca a OTRO producto
		var idExistente int
		errCb := tx.QueryRow(`SELECT id_producto FROM inventario.codigos_barras WHERE codigo = $1`, p.CodigoBarras).Scan(&idExistente)
		if errCb == nil && idExistente != id {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "El código de barras proporcionado ya pertenece a otro producto."})
			return
		}

		// Borramos los anteriores (si los hay) y lo insertamos/actualizamos
		_, err = tx.Exec(`DELETE FROM inventario.codigos_barras WHERE id_producto = $1`, id)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al actualizar los códigos de barras."})
			return
		}
		_, err = tx.Exec(`INSERT INTO inventario.codigos_barras (id_producto, codigo) VALUES ($1, $2)`, id, p.CodigoBarras)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al guardar el nuevo código de barras."})
			return
		}
	} else {
		// Si se envía vacío, significa que se quitó el código de barras
		_, err = tx.Exec(`DELETE FROM inventario.codigos_barras WHERE id_producto = $1`, id)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al eliminar el código de barras."})
			return
		}
	}

	if err := tx.Commit(); err != nil { // COMMIT
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al confirmar la transacción."})
		return
	}

	// Registrar en auditoría solo si el precio cambió (HU-08 CA 37)
	if precioAnterior != p.PrecioVenta {
		claims, ok := middleware.GetClaims(r)
		if ok {
			utils.LogAction(db, claims.IdUsuario, "MODIFICACION_PRECIO", "inventario.productos", &id, 
				fmt.Sprintf("%d", precioAnterior), fmt.Sprintf("%d", p.PrecioVenta), r.RemoteAddr)
		}
	}

	p.IdProducto = id
	json.NewEncoder(w).Encode(map[string]interface{}{
		"mensaje":  "Producto actualizado exitosamente.",
		"producto": p,
	})
}

// ─── DELETE ──────────────────────────────────────────────────────────────────
// deleteProduct realiza una baja LÓGICA (estado = 'inactivo') para preservar
// la integridad referencial con ventas e ingresos históricos (CA 45).
func deleteProduct(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "El parámetro ?id es obligatorio para dar de baja un producto."})
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "El parámetro ?id debe ser un número entero positivo."})
		return
	}

	// ── TRANSACCIÓN SQL: BEGIN / COMMIT / ROLLBACK (CA 45) ───────────────────
	// Usamos baja lógica (estado = 'inactivo') para no romper referencias en
	// ventas, ingresos o movimientos históricos ya registrados.
	tx, err := db.Begin() // BEGIN
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "No se pudo iniciar la transacción."})
		return
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		`UPDATE inventario.productos SET estado = 'inactivo' WHERE id_producto = $1`, id,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error interno al dar de baja el producto."})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "No se encontró un producto con el id_producto proporcionado."})
		return
	}

	if err := tx.Commit(); err != nil { // COMMIT
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al confirmar la baja lógica."})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{
		"mensaje": "Producto dado de baja exitosamente (estado: inactivo). Los registros históricos se conservan.",
	})
}
