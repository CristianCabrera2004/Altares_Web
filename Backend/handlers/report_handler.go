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
