package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type ReporteItem struct {
	FechaVenta     string `json:"fecha_venta"`
	IdProducto     int    `json:"id_producto"`
	Producto       string `json:"producto"`
	Categoria      string `json:"categoria"`
	Cantidad       int    `json:"cantidad"`
	PrecioUnitario int    `json:"precio_unitario"`
	Total          int    `json:"total"`
}

// getTiendaIDForReports has been replaced by GetTiendaIDFromCtxOrDb


// ReportesVentasHandler devuelve las ventas en un rango de fechas.
func ReportesVentasHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Método no permitido."})
			return
		}

		q := r.URL.Query()
		startDate := q.Get("start_date")
		endDate := q.Get("end_date")
		categoria := q.Get("categoria")

		if startDate == "" || endDate == "" {
			endDate = time.Now().Format("2006-01-02")
			startDate = time.Now().AddDate(0, -1, 0).Format("2006-01-02")
		}

		idTienda := GetTiendaIDFromCtxOrDb(db, r)

		args := []interface{}{startDate, endDate, idTienda}
		query := `
			SELECT 
				TO_CHAR(v.fecha_venta AT TIME ZONE 'UTC' AT TIME ZONE 'America/Guayaquil', 'YYYY-MM-DD') as fecha_v,
				p.id_producto,
				p.nombre as producto,
				c.nombre as categoria,
				SUM(d.cantidad) as cantidad,
				d.precio_unitario,
				SUM(d.subtotal) as total
			FROM operaciones.detalle_ventas d
			JOIN operaciones.ventas v ON d.id_venta = v.id_venta
			JOIN inventario.productos p ON d.id_producto = p.id_producto
			JOIN inventario.categorias c ON p.id_categoria = c.id_categoria
			WHERE DATE(v.fecha_venta AT TIME ZONE 'UTC' AT TIME ZONE 'America/Guayaquil') >= $1 AND DATE(v.fecha_venta AT TIME ZONE 'UTC' AT TIME ZONE 'America/Guayaquil') <= $2
			AND v.id_tienda = $3
			AND v.estado = 'completada'
		`

		if categoria != "" && categoria != "Todas" {
			args = append(args, categoria)
			query += fmt.Sprintf(" AND c.nombre = $%d", len(args))
		}

		query += `
			GROUP BY TO_CHAR(v.fecha_venta AT TIME ZONE 'UTC' AT TIME ZONE 'America/Guayaquil', 'YYYY-MM-DD'), p.id_producto, p.nombre, c.nombre, d.precio_unitario
			ORDER BY fecha_v DESC, total DESC
			LIMIT 50000
		`

		rows, err := db.Query(query, args...)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al generar el reporte de ventas."})
			return
		}
		defer rows.Close()

		var items = []ReporteItem{}
		for rows.Next() {
			var i ReporteItem
			if err := rows.Scan(&i.FechaVenta, &i.IdProducto, &i.Producto, &i.Categoria, &i.Cantidad, &i.PrecioUnitario, &i.Total); err != nil {
				continue
			}
			items = append(items, i)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(items)
	}
}

type GraficaData struct {
	Fecha string `json:"fecha"`
	Total int    `json:"total"` // en centavos
}

// ReporteGraficaHandler devuelve las ventas totales agrupadas por día.
func ReporteGraficaHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Método no permitido."})
			return
		}

		periodo := r.URL.Query().Get("periodo")
		idTienda := GetTiendaIDFromCtxOrDb(db, r)

		var selectClause string
		var groupClause string
		var whereClause string

		switch periodo {
		case "7":
			whereClause = "AND fecha_venta AT TIME ZONE 'UTC' AT TIME ZONE 'America/Guayaquil' >= CURRENT_DATE - INTERVAL '6 days'"
			selectClause = "TO_CHAR(fecha_venta AT TIME ZONE 'UTC' AT TIME ZONE 'America/Guayaquil', 'YYYY-MM-DD') as fecha"
			groupClause = "TO_CHAR(fecha_venta AT TIME ZONE 'UTC' AT TIME ZONE 'America/Guayaquil', 'YYYY-MM-DD')"
		case "30":
			whereClause = "AND fecha_venta AT TIME ZONE 'UTC' AT TIME ZONE 'America/Guayaquil' >= CURRENT_DATE - INTERVAL '11 months'"
			selectClause = "TO_CHAR(fecha_venta AT TIME ZONE 'UTC' AT TIME ZONE 'America/Guayaquil', 'YYYY-MM') as fecha"
			groupClause = "TO_CHAR(fecha_venta AT TIME ZONE 'UTC' AT TIME ZONE 'America/Guayaquil', 'YYYY-MM')"
		case "365":
			whereClause = "AND fecha_venta AT TIME ZONE 'UTC' AT TIME ZONE 'America/Guayaquil' >= CURRENT_DATE - INTERVAL '4 years'"
			selectClause = "TO_CHAR(fecha_venta AT TIME ZONE 'UTC' AT TIME ZONE 'America/Guayaquil', 'YYYY') as fecha"
			groupClause = "TO_CHAR(fecha_venta AT TIME ZONE 'UTC' AT TIME ZONE 'America/Guayaquil', 'YYYY')"
		case "0":
			whereClause = ""
			selectClause = "TO_CHAR(fecha_venta AT TIME ZONE 'UTC' AT TIME ZONE 'America/Guayaquil', 'YYYY') as fecha"
			groupClause = "TO_CHAR(fecha_venta AT TIME ZONE 'UTC' AT TIME ZONE 'America/Guayaquil', 'YYYY')"
		default:
			whereClause = "AND fecha_venta AT TIME ZONE 'UTC' AT TIME ZONE 'America/Guayaquil' >= CURRENT_DATE - INTERVAL '14 days'"
			selectClause = "TO_CHAR(fecha_venta AT TIME ZONE 'UTC' AT TIME ZONE 'America/Guayaquil', 'YYYY-MM-DD') as fecha"
			groupClause = "TO_CHAR(fecha_venta AT TIME ZONE 'UTC' AT TIME ZONE 'America/Guayaquil', 'YYYY-MM-DD')"
		}

		query := fmt.Sprintf(`
			SELECT 
				%s,
				SUM(total) as total
			FROM operaciones.ventas
			WHERE estado = 'completada' AND id_tienda = $1
			  %s
			GROUP BY %s
			ORDER BY %s ASC
		`, selectClause, whereClause, groupClause, groupClause)

		rows, err := db.Query(query, idTienda)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar datos de la gráfica."})
			return
		}
		defer rows.Close()

		var data []GraficaData
		for rows.Next() {
			var g GraficaData
			if err := rows.Scan(&g.Fecha, &g.Total); err != nil {
				continue
			}
			data = append(data, g)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(data)
	}
}

// FacturaDiariaConsumidorFinalHandler aggregates all sales for "Consumidor Final" on a given day.
func FacturaDiariaConsumidorFinalHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Método no permitido."})
			return
		}

		q := r.URL.Query()
		fechaStr := q.Get("fecha") // Formato YYYY-MM-DD
		if fechaStr == "" {
			fechaStr = time.Now().Format("2006-01-02")
		}

		idTienda := GetTiendaIDFromCtxOrDb(db, r)

		// Agrupar los detalles de ventas del día actual para consumidor final
		query := `
			SELECT 
				p.nombre, 
				SUM(d.cantidad) as cantidad, 
				d.precio_unitario, 
				COALESCE(d.iva_aplicado, 0) as iva_aplicado, 
				SUM(d.subtotal) as subtotal
			FROM operaciones.detalle_ventas d
			JOIN operaciones.ventas v ON d.id_venta = v.id_venta
			JOIN inventario.productos p ON d.id_producto = p.id_producto
			WHERE v.id_tienda = $1 
			  AND DATE(v.fecha_venta AT TIME ZONE 'UTC' AT TIME ZONE 'America/Guayaquil') = $2
			  AND (v.id_cliente = '9999999999999' OR v.id_cliente IS NULL)
			  AND v.estado = 'completada'
			GROUP BY p.nombre, d.precio_unitario, COALESCE(d.iva_aplicado, 0)
			ORDER BY p.nombre ASC
		`

		rows, err := db.Query(query, idTienda, fechaStr)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar la factura diaria."})
			return
		}
		defer rows.Close()

		var items []InvoiceDetail
		for rows.Next() {
			var item InvoiceDetail
			if err := rows.Scan(&item.Producto, &item.Cantidad, &item.PrecioUnitario, &item.IvaAplicado, &item.Subtotal); err == nil {
				items = append(items, item)
			}
		}

		var f FacturaResponse
		f.IdVenta = 0
		f.NombreTipoFactura = "Factura Global Diaria"
		f.ClienteIdentificacion = "9999999999999"
		f.ClienteNombre = "Consumidor Final"
		f.FechaEmision = time.Now().Format("2006-01-02 15:04:05")
		
		var subtotal, totalIva, total int
		for _, item := range items {
			f.Items = append(f.Items, item)
			
			lineTotal := item.Subtotal
			lineBase := lineTotal
			ivaLinea := 0
			if item.IvaAplicado > 0 {
				lineBase = int(float64(lineTotal) / (1.0 + float64(item.IvaAplicado)/100.0))
				ivaLinea = lineTotal - lineBase
			}
			subtotal += lineBase
			totalIva += ivaLinea
			total += lineTotal
		}
		f.Subtotal = subtotal
		f.TotalIva = totalIva
		f.Total = total

		if f.Items == nil {
			f.Items = make([]InvoiceDetail, 0)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(f)
	}
}
