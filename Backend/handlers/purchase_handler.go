package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

type FacturaCompra struct {
	IdFactura      int    `json:"id_factura"`
	NumeroFactura  string `json:"numero_factura"`
	FechaCompra    string `json:"fecha_compra"`
	IdProveedor    *int   `json:"id_proveedor"`
	Proveedor      string `json:"proveedor"`
	IdUsuario      int    `json:"id_usuario"`
	Total          int    `json:"total"`
	FechaRegistro  string `json:"fecha_registro"`
}

type CompraDetail struct {
	Producto       string `json:"producto"`
	Cantidad       int    `json:"cantidad"`
	PrecioUnitario int    `json:"precio_unitario"`
	Subtotal       int    `json:"subtotal"`
}

type FacturaCompraResponse struct {
	FacturaCompra
	Items []CompraDetail `json:"items"`
}

// ComprasReporteListHandler returns the list of purchase invoices.
func ComprasReporteListHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Método no permitido."})
			return
		}

		idTienda := GetTiendaIDFromCtxOrDb(db, r)
		
		q := r.URL.Query()
		fecha := q.Get("fecha")
		
		var query string
		var args []interface{}
		
		if fecha != "" {
			query = `
				SELECT f.id_factura, f.numero_factura, TO_CHAR(f.fecha_compra, 'YYYY-MM-DD'), f.id_proveedor, 
				       COALESCE(p.nombre_proveedor, '— Sin proveedor —'), f.id_usuario, f.total,
				       TO_CHAR(f.fecha_registro AT TIME ZONE 'UTC' AT TIME ZONE 'America/Guayaquil', 'YYYY-MM-DD HH24:MI:SS')
				FROM inventario.facturas_compra f
				LEFT JOIN inventario.proveedores p ON f.id_proveedor = p.id_proveedor
				WHERE f.id_tienda = $1 AND f.fecha_compra = $2
				ORDER BY f.fecha_registro DESC
			`
			args = []interface{}{idTienda, fecha}
		} else {
			// Si no hay fecha, traemos las más recientes (últimos 30 días o límite)
			query = `
				SELECT f.id_factura, f.numero_factura, TO_CHAR(f.fecha_compra, 'YYYY-MM-DD'), f.id_proveedor, 
				       COALESCE(p.nombre_proveedor, '— Sin proveedor —'), f.id_usuario, f.total,
				       TO_CHAR(f.fecha_registro AT TIME ZONE 'UTC' AT TIME ZONE 'America/Guayaquil', 'YYYY-MM-DD HH24:MI:SS')
				FROM inventario.facturas_compra f
				LEFT JOIN inventario.proveedores p ON f.id_proveedor = p.id_proveedor
				WHERE f.id_tienda = $1
				ORDER BY f.fecha_registro DESC
				LIMIT 100
			`
			args = []interface{}{idTienda}
		}

		rows, err := db.Query(query, args...)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al obtener facturas de compra."})
			return
		}
		defer rows.Close()

		var facturas []FacturaCompra
		for rows.Next() {
			var f FacturaCompra
			if err := rows.Scan(&f.IdFactura, &f.NumeroFactura, &f.FechaCompra, &f.IdProveedor, &f.Proveedor, &f.IdUsuario, &f.Total, &f.FechaRegistro); err != nil {
				continue
			}
			facturas = append(facturas, f)
		}

		if facturas == nil {
			facturas = make([]FacturaCompra, 0)
		}

		json.NewEncoder(w).Encode(facturas)
	}
}

// ComprasReporteDetailHandler returns details for a specific purchase invoice.
func ComprasReporteDetailHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Método no permitido."})
			return
		}

		idFacturaStr := r.PathValue("id")
		if idFacturaStr == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "ID de factura requerido."})
			return
		}

		idTienda := GetTiendaIDFromCtxOrDb(db, r)

		var resp FacturaCompraResponse
		err := db.QueryRow(`
			SELECT f.id_factura, f.numero_factura, TO_CHAR(f.fecha_compra, 'YYYY-MM-DD'), f.id_proveedor, 
			       COALESCE(p.nombre_proveedor, '— Sin proveedor —'), f.id_usuario, f.total,
			       TO_CHAR(f.fecha_registro AT TIME ZONE 'UTC' AT TIME ZONE 'America/Guayaquil', 'YYYY-MM-DD HH24:MI:SS')
			FROM inventario.facturas_compra f
			LEFT JOIN inventario.proveedores p ON f.id_proveedor = p.id_proveedor
			WHERE f.id_factura = $1 AND f.id_tienda = $2
		`, idFacturaStr, idTienda).Scan(&resp.IdFactura, &resp.NumeroFactura, &resp.FechaCompra, &resp.IdProveedor, &resp.Proveedor, &resp.IdUsuario, &resp.Total, &resp.FechaRegistro)

		if err == sql.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Factura no encontrada."})
			return
		} else if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar la factura."})
			return
		}

		rows, err := db.Query(`
			SELECT pr.nombre, i.cantidad_ingresada, i.costo_unitario, i.subtotal
			FROM inventario.ingreso_inventario i
			JOIN inventario.productos pr ON i.id_producto = pr.id_producto
			WHERE i.id_factura_compra = $1
			ORDER BY pr.nombre ASC
		`, idFacturaStr)
		
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al obtener detalles de la factura."})
			return
		}
		defer rows.Close()

		for rows.Next() {
			var d CompraDetail
			if err := rows.Scan(&d.Producto, &d.Cantidad, &d.PrecioUnitario, &d.Subtotal); err == nil {
				resp.Items = append(resp.Items, d)
			}
		}

		if resp.Items == nil {
			resp.Items = make([]CompraDetail, 0)
		}

		json.NewEncoder(w).Encode(resp)
	}
}
