package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"libreria-altares/middleware"
	"libreria-altares/utils"
)

// InvoiceSummary representa el resumen de un cierre de caja.
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
	IdCierre      int             `json:"id_cierre,omitempty"`
}

type InvoiceDetail struct {
	Producto       string `json:"producto"`
	Cantidad       int    `json:"cantidad"`
	PrecioUnitario int    `json:"precio_unitario"`
	IvaAplicado    int    `json:"iva_aplicado"`
	Subtotal       int    `json:"subtotal"`
}

// getTiendaIDForInvoice has been replaced by GetTiendaIDFromCtxOrDb


// InvoiceHandler procesa un cierre de caja y devuelve el resumen de la jornada.
func InvoiceHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodPost && r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Método no permitido."})
			return
		}

		hoy := time.Now().Format("2006-01-02")
		idTienda := GetTiendaIDFromCtxOrDb(db, r)

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
			WHERE DATE(v.fecha_venta) = $1 AND v.id_tienda = $2 AND v.estado = 'completada'
			GROUP BY p.nombre, d.precio_unitario, d.iva_aplicado
		`

		rows, err := db.Query(query, hoy, idTienda)
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
			json.NewEncoder(w).Encode(map[string]string{"error": "No hay ventas registradas el día de hoy en esta tienda para facturar."})
			return
		}

		summary.XmlGenerado = fmt.Sprintf("<factura><infoTributaria><ruc>1234567890001</ruc><razonSocial>Librería Los Altares</razonSocial></infoTributaria><infoFactura><fechaEmision>%s</fechaEmision><identificacionComprador>9999999999999</identificacionComprador><totalConImpuestos>%d</totalConImpuestos></infoFactura></factura>", hoy, summary.TotalGlobal)

		claims, ok := middleware.GetClaims(r)
		if r.Method == http.MethodPost {
			if ok {
				var idCierre int
				insertErr := db.QueryRow(`
					INSERT INTO operaciones.cierres_diarios
					  (id_usuario, id_tienda, fecha_cierre, total_recaudado, estado, fecha_hora_cierre)
					VALUES ($1, $2, $3, $4, 'cuadrado', NOW())
					RETURNING id_cierre`,
					claims.IdUsuario,
					idTienda,
					hoy,
					summary.TotalGlobal,
				).Scan(&idCierre)
				if insertErr == nil {
					summary.IdCierre = idCierre
				}
				utils.LogAction(db, claims.IdUsuario, "CIERRE_CAJA", "operaciones.cierres_diarios",
					&idCierre, "", fmt.Sprintf("Total: %d centavos | Tienda: %d | Cierre #%d", summary.TotalGlobal, idTienda, idCierre), r.RemoteAddr)
			}
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(summary)
	}
}

// CreateInvoiceInput es el cuerpo de POST /api/facturas.
type CreateInvoiceInput struct {
	IdVenta       int    `json:"id_venta"`
	IdTipoFactura int    `json:"id_tipo_factura"` // 1: Consumidor Final, 2: Factura con Datos, 3: Factura Electrónica
	IdCliente     *int   `json:"id_cliente,omitempty"`
	PdfBase64     string `json:"pdf_base64,omitempty"` // Opcional base64 para envío por email
}

// FacturaResponse es el formato de salida para una factura.
type FacturaResponse struct {
	IdFactura             int    `json:"id_factura"`
	IdVenta               int    `json:"id_venta"`
	IdTipoFactura         int    `json:"id_tipo_factura"`
	NombreTipoFactura     string `json:"nombre_tipo_factura"`
	IdCliente             *int   `json:"id_cliente,omitempty"`
	ClienteIdentificacion string `json:"cliente_identificacion"`
	ClienteNombre         string `json:"cliente_nombre"`
	ClienteDireccion      string `json:"cliente_direccion,omitempty"`
	ClienteTelefono       string `json:"cliente_telefono,omitempty"`
	ClienteEmail          string `json:"cliente_email,omitempty"`
	ArchivoPdf            string `json:"archivo_pdf,omitempty"`
	FechaEmision          string `json:"fecha_emision"`
	Subtotal              int    `json:"subtotal"`
	TotalIva              int    `json:"total_iva"`
	Total                 int    `json:"total"`
}

// FacturasHandler despacha peticiones de factura
func FacturasHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Content-Type", "application/json")
		
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		switch r.Method {
		case http.MethodGet:
			getFactura(db, w, r)
		case http.MethodPost:
			createFactura(db, w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Método no soportado en este endpoint."})
		}
	}
}

func getFactura(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	idVentaStr := r.URL.Query().Get("venta")
	idFacturaStr := r.URL.Query().Get("id")

	var query string
	var arg interface{}

	if idVentaStr != "" {
		query = `
			SELECT f.id_factura, f.id_venta, f.id_tipo_factura, tf.nombre,
			       f.id_cliente, f.cliente_identificacion, f.cliente_nombre,
			       COALESCE(c.direccion, ''), COALESCE(c.telefono, ''), COALESCE(c.email, ''),
			       COALESCE(f.archivo_pdf, ''), TO_CHAR(f.fecha_emision, 'YYYY-MM-DD HH24:MI:SS'),
			       v.subtotal, v.total_iva, v.total
			FROM operaciones.facturas f
			JOIN operaciones.tipo_factura tf ON f.id_tipo_factura = tf.id_tipo_factura
			JOIN operaciones.ventas v ON f.id_venta = v.id_venta
			LEFT JOIN operaciones.clientes c ON f.id_cliente = c.id_cliente
			WHERE f.id_venta = $1`
		idVenta, err := strconv.Atoi(idVentaStr)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "El parámetro 'venta' debe ser un número entero."})
			return
		}
		arg = idVenta
	} else if idFacturaStr != "" {
		query = `
			SELECT f.id_factura, f.id_venta, f.id_tipo_factura, tf.nombre,
			       f.id_cliente, f.cliente_identificacion, f.cliente_nombre,
			       COALESCE(c.direccion, ''), COALESCE(c.telefono, ''), COALESCE(c.email, ''),
			       COALESCE(f.archivo_pdf, ''), TO_CHAR(f.fecha_emision, 'YYYY-MM-DD HH24:MI:SS'),
			       v.subtotal, v.total_iva, v.total
			FROM operaciones.facturas f
			JOIN operaciones.tipo_factura tf ON f.id_tipo_factura = tf.id_tipo_factura
			JOIN operaciones.ventas v ON f.id_venta = v.id_venta
			LEFT JOIN operaciones.clientes c ON f.id_cliente = c.id_cliente
			WHERE f.id_factura = $1`
		idFactura, err := strconv.Atoi(idFacturaStr)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "El parámetro 'id' debe ser un número entero."})
			return
		}
		arg = idFactura
	} else {
		// Listar últimas 100 facturas
		rows, err := db.Query(`
			SELECT f.id_factura, f.id_venta, f.id_tipo_factura, tf.nombre,
			       f.id_cliente, f.cliente_identificacion, f.cliente_nombre,
			       COALESCE(c.direccion, ''), COALESCE(c.telefono, ''), COALESCE(c.email, ''),
			       COALESCE(f.archivo_pdf, ''), TO_CHAR(f.fecha_emision, 'YYYY-MM-DD HH24:MI:SS'),
			       v.subtotal, v.total_iva, v.total
			FROM operaciones.facturas f
			JOIN operaciones.tipo_factura tf ON f.id_tipo_factura = tf.id_tipo_factura
			JOIN operaciones.ventas v ON f.id_venta = v.id_venta
			LEFT JOIN operaciones.clientes c ON f.id_cliente = c.id_cliente
			ORDER BY f.fecha_emision DESC
			LIMIT 100`)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar facturas."})
			return
		}
		defer rows.Close()

		facturas := []FacturaResponse{}
		for rows.Next() {
			var f FacturaResponse
			err = rows.Scan(
				&f.IdFactura, &f.IdVenta, &f.IdTipoFactura, &f.NombreTipoFactura,
				&f.IdCliente, &f.ClienteIdentificacion, &f.ClienteNombre,
				&f.ClienteDireccion, &f.ClienteTelefono, &f.ClienteEmail,
				&f.ArchivoPdf, &f.FechaEmision, &f.Subtotal, &f.TotalIva, &f.Total,
			)
			if err != nil {
				continue
			}
			facturas = append(facturas, f)
		}
		json.NewEncoder(w).Encode(facturas)
		return
	}

	var f FacturaResponse
	err := db.QueryRow(query, arg).Scan(
		&f.IdFactura, &f.IdVenta, &f.IdTipoFactura, &f.NombreTipoFactura,
		&f.IdCliente, &f.ClienteIdentificacion, &f.ClienteNombre,
		&f.ClienteDireccion, &f.ClienteTelefono, &f.ClienteEmail,
		&f.ArchivoPdf, &f.FechaEmision, &f.Subtotal, &f.TotalIva, &f.Total,
	)
	if err == sql.ErrNoRows {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "Factura no encontrada."})
		return
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar la factura."})
		return
	}

	json.NewEncoder(w).Encode(f)
}

func createFactura(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	var input CreateInvoiceInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido."})
		return
	}

	if input.IdVenta <= 0 || input.IdTipoFactura <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "'id_venta' e 'id_tipo_factura' son obligatorios."})
		return
	}

	// Obtener datos del cliente si id_cliente está especificado
	identificacion := "9999999999999"
	nombre := "Consumidor Final"
	email := ""

	if input.IdCliente != nil && *input.IdCliente > 0 {
		err := db.QueryRow(`
			SELECT cedula_ruc, nombre, COALESCE(email, '')
			FROM operaciones.clientes
			WHERE id_cliente = $1`, *input.IdCliente,
		).Scan(&identificacion, &nombre, &email)
		if err == sql.ErrNoRows {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Cliente especificado no existe."})
			return
		} else if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al validar el cliente."})
			return
		}
	}

	archivoPdfName := fmt.Sprintf("factura_%d.pdf", input.IdVenta)

	tx, err := db.Begin()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error de base de datos."})
		return
	}
	defer tx.Rollback()

	// Validar que la venta exista
	var ventaExiste bool
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM operaciones.ventas WHERE id_venta = $1)", input.IdVenta).Scan(&ventaExiste)
	if err != nil || !ventaExiste {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "La venta especificada no existe."})
		return
	}

	// Validar que no exista factura previa para esta venta
	var facturaExiste bool
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM operaciones.facturas WHERE id_venta = $1)", input.IdVenta).Scan(&facturaExiste)
	if err != nil || facturaExiste {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Ya se ha generado una factura para esta venta."})
		return
	}

	var idFactura int
	var fechaEmision time.Time
	err = tx.QueryRow(`
		INSERT INTO operaciones.facturas 
		  (id_venta, id_tipo_factura, id_cliente, cliente_identificacion, cliente_nombre, archivo_pdf, fecha_emision)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		RETURNING id_factura, fecha_emision`,
		input.IdVenta, input.IdTipoFactura, input.IdCliente, identificacion, nombre, archivoPdfName,
	).Scan(&idFactura, &fechaEmision)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al registrar la factura en la base de datos."})
		return
	}

	if err := tx.Commit(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al confirmar factura."})
		return
	}

	// Si es Factura Electrónica (tipo 3) y el cliente tiene email, enviar por correo
	if input.IdTipoFactura == 3 && email != "" {
		// Enviar por correo en segundo plano
		go func() {
			subject := fmt.Sprintf("Factura Electrónica #%d - Librería Los Altares", idFactura)
			bodyHTML := fmt.Sprintf(`
				<div style="font-family: Arial, sans-serif; color: #333; max-width: 600px; margin: 0 auto; border: 1px solid #ddd; padding: 20px; border-radius: 8px;">
					<h2 style="color: #4F8EF7; border-bottom: 2px solid #4F8EF7; padding-bottom: 10px;">Librería Los Altares</h2>
					<p>Estimado(a) <strong>%s</strong>,</p>
					<p>Le agradecemos su preferencia por nuestra librería. Adjunto a este correo encontrará su <strong>Factura Electrónica</strong> en formato PDF correspondiente a su compra.</p>
					<table style="width: 100%%; border-collapse: collapse; margin-top: 20px; margin-bottom: 20px;">
						<tr style="background-color: #f8f9fa;">
							<td style="padding: 10px; border: 1px solid #ddd;"><strong>Nº Factura:</strong></td>
							<td style="padding: 10px; border: 1px solid #ddd;">%d</td>
						</tr>
						<tr>
							<td style="padding: 10px; border: 1px solid #ddd;"><strong>Fecha:</strong></td>
							<td style="padding: 10px; border: 1px solid #ddd;">%s</td>
						</tr>
						<tr style="background-color: #f8f9fa;">
							<td style="padding: 10px; border: 1px solid #ddd;"><strong>Cliente:</strong></td>
							<td style="padding: 10px; border: 1px solid #ddd;">%s</td>
						</tr>
						<tr>
							<td style="padding: 10px; border: 1px solid #ddd;"><strong>Identificación:</strong></td>
							<td style="padding: 10px; border: 1px solid #ddd;">%s</td>
						</tr>
					</table>
					<p style="font-size: 12px; color: #777; margin-top: 30px; border-top: 1px solid #eee; padding-top: 10px; text-align: center;">
						Este es un correo automático. Por favor no responda directamente a este mensaje.
					</p>
				</div>
			`, nombre, idFactura, fechaEmision.Format("2006-01-02 15:04:05"), nombre, identificacion)

			sendErr := utils.SendEmail(email, subject, bodyHTML, input.PdfBase64, archivoPdfName)
			if sendErr != nil {
				fmt.Printf("⚠️ ERROR AL ENVIAR CORREO FACTURA: %v\n", sendErr)
			} else {
				fmt.Printf("✅ CORREO FACTURA ENVIADO EXITOSAMENTE A %s\n", email)
			}
		}()
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"mensaje":    "Factura creada exitosamente.",
		"id_factura": idFactura,
		"factura": map[string]interface{}{
			"id_factura":             idFactura,
			"id_venta":               input.IdVenta,
			"id_tipo_factura":         input.IdTipoFactura,
			"id_cliente":             input.IdCliente,
			"cliente_identificacion": identificacion,
			"cliente_nombre":         nombre,
			"archivo_pdf":            archivoPdfName,
			"fecha_emision":          fechaEmision.Format("2006-01-02 15:04:05"),
		},
	})
}
