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
		categoria := q.Get("categoria") // opcional, puede ser nombre o ID

		if startDate == "" || endDate == "" {
			// Por defecto los últimos 30 días
			endDate = time.Now().Format("2006-01-02")
			startDate = time.Now().AddDate(0, -1, 0).Format("2006-01-02")
		}

		// Prevenir inyección usando parámetros nombrados o posicionales dinámicos.
		args := []interface{}{startDate, endDate}
		query := `
			SELECT 
				TO_CHAR(v.fecha_venta, 'YYYY-MM-DD'),
				p.id_producto,
				p.nombre as producto,
				c.nombre as categoria,
				d.cantidad,
				d.precio_unitario,
				d.subtotal as total
			FROM operaciones.detalle_ventas d
			JOIN operaciones.ventas v ON d.id_venta = v.id_venta
			JOIN inventario.productos p ON d.id_producto = p.id_producto
			JOIN inventario.categorias c ON p.id_categoria = c.id_categoria
			WHERE DATE(v.fecha_venta) >= $1 AND DATE(v.fecha_venta) <= $2
			AND v.estado = 'completada'
		`

		if categoria != "" && categoria != "Todas" {
			args = append(args, categoria)
			query += fmt.Sprintf(" AND c.nombre = $%d", len(args))
		}

		query += " ORDER BY v.fecha_venta DESC LIMIT 1000" // Protección de memoria

		rows, err := db.Query(query, args...)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al generar el reporte de ventas."})
			return
		}
		defer rows.Close()

		var items []ReporteItem
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
// Acepta ?periodo= con los valores: 7, 15, 30, 365, 0 (general/todos).
// Por defecto usa 15 días si no se especifica.
func ReporteGraficaHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Método no permitido."})
			return
		}

		// Leer parámetro de periodo (días hacia atrás; 0 = sin límite)
		periodo := r.URL.Query().Get("periodo")

		var whereClause string
		switch periodo {
		case "7":
			whereClause = "AND fecha_venta >= CURRENT_DATE - INTERVAL '6 days'"
		case "30":
			whereClause = "AND fecha_venta >= CURRENT_DATE - INTERVAL '29 days'"
		case "365":
			whereClause = "AND fecha_venta >= CURRENT_DATE - INTERVAL '364 days'"
		case "0":
			whereClause = "" // Sin límite de fecha → histórico completo
		default: // "15" o cualquier otro valor
			whereClause = "AND fecha_venta >= CURRENT_DATE - INTERVAL '14 days'"
		}

		query := fmt.Sprintf(`
			SELECT 
				TO_CHAR(fecha_venta, 'YYYY-MM-DD') as fecha,
				SUM(total) as total
			FROM operaciones.ventas
			WHERE estado = 'completada'
			  %s
			GROUP BY TO_CHAR(fecha_venta, 'YYYY-MM-DD')
			ORDER BY TO_CHAR(fecha_venta, 'YYYY-MM-DD') ASC
		`, whereClause)

		rows, err := db.Query(query)
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
