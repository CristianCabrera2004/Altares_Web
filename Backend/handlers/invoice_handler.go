package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"libreria-altares/middleware"
	"libreria-altares/utils"
)

// InvoiceSummary representa el cierre global de ventas de un día.
type InvoiceSummary struct {
	FechaEmision  string          `json:"fecha_emision"`
	RucCliente    string          `json:"ruc_cliente"`
	NombreCliente string          `json:"nombre_cliente"`
	SubtotalBase  int             `json:"subtotal_base"`
	TotalIva15    int             `json:"total_iva_15"`
	TotalIva0     int             `json:"total_iva_0"`
	TotalGlobal   int             `json:"total_global"`
	Detalles      []InvoiceDetail `json:"detalles"`
	XmlGenerado   string          `json:"xml_sri_mock"`
}

type InvoiceDetail struct {
	Producto       string `json:"producto"`
	Cantidad       int    `json:"cantidad"`
	PrecioUnitario int    `json:"precio_unitario"`
	IvaAplicado    int    `json:"iva_aplicado"`
	Subtotal       int    `json:"subtotal"`
}

// InvoiceHandler procesa el cierre de caja y devuelve la data de la factura global.
// HU-02: Si ya se generó un cierre hoy (POST), retorna HTTP 409 con los datos
// ya calculados para evitar duplicar el registro de auditoría.
func InvoiceHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodPost && r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Método no permitido."})
			return
		}

		hoy := time.Now().Format("2006-01-02")

		// HU-02: Protección contra doble cierre del mismo día.
		// Solo aplica al POST (generación), no al GET (consulta).
		if r.Method == http.MethodPost {
			var cierresHoy int
			db.QueryRow(`
				SELECT COUNT(*) FROM seguridad.logs_auditoria
				WHERE accion = 'CIERRE_CAJA' AND DATE(fecha) = $1`, hoy,
			).Scan(&cierresHoy)

			if cierresHoy > 0 {
				w.WriteHeader(http.StatusConflict) // HTTP 409
				json.NewEncoder(w).Encode(map[string]string{
					"error": "Ya se generó la factura de cierre para el día de hoy (" + hoy + "). " +
						"Solo se permite un cierre por jornada. Utilice la opción de consulta para verla.",
				})
				return
			}
		}

		// Consultar todas las ventas de la jornada
		query := `
			SELECT 
				p.nombre, 
				SUM(d.cantidad) as total_cantidad, 
				d.precio_unitario, 
				d.iva_aplicado, 
				SUM(d.subtotal) as total_subtotal
			FROM operaciones.detalle_ventas d
			JOIN operaciones.ventas v ON d.id_venta = v.id_venta
			JOIN inventario.productos p ON d.id_producto = p.id_producto
			WHERE DATE(v.fecha_venta) = $1 AND v.estado = 'completada'
			GROUP BY p.nombre, d.precio_unitario, d.iva_aplicado
		`

		rows, err := db.Query(query, hoy)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar las ventas del día."})
			return
		}
		defer rows.Close()

		var summary InvoiceSummary
		summary.FechaEmision = hoy
		summary.RucCliente = "9999999999999" // CA 7
		summary.NombreCliente = "Consumidor Final"

		for rows.Next() {
			var d InvoiceDetail
			if err := rows.Scan(&d.Producto, &d.Cantidad, &d.PrecioUnitario, &d.IvaAplicado, &d.Subtotal); err != nil {
				continue
			}

			// CA 8: IVA Diferenciado
			subBase := d.PrecioUnitario * d.Cantidad
			summary.SubtotalBase += subBase

			if d.IvaAplicado == 15 {
				summary.TotalIva15 += (d.Subtotal - subBase)
			} else {
				summary.TotalIva0 += (d.Subtotal - subBase)
			}
			summary.TotalGlobal += d.Subtotal

			summary.Detalles = append(summary.Detalles, d)
		}

		if len(summary.Detalles) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "No hay ventas registradas el día de hoy para facturar."})
			return
		}

		// CA 9: Simulador de XML SRI
		summary.XmlGenerado = fmt.Sprintf("<factura><infoTributaria><ruc>1234567890001</ruc><razonSocial>Librería Los Altares</razonSocial></infoTributaria><infoFactura><fechaEmision>%s</fechaEmision><identificacionComprador>9999999999999</identificacionComprador><totalConImpuestos>%d</totalConImpuestos></infoFactura></factura>", hoy, summary.TotalGlobal)

		// Registrar evento en auditoría (HU-08)
		claims, ok := middleware.GetClaims(r)
		if ok {
			utils.LogAction(db, claims.IdUsuario, "CIERRE_CAJA", "operaciones.ventas", nil, "", fmt.Sprintf("Total: %d", summary.TotalGlobal), r.RemoteAddr)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(summary)
	}
}
