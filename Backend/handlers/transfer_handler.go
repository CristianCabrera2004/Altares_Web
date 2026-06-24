package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"libreria-altares/middleware"
)

// TransferenciaProductoInput define el formato para un producto individual a transferir.
type TransferenciaProductoInput struct {
	IdProducto int `json:"id_producto"`
	Cantidad   int `json:"cantidad"`
}

// TransferenciaInput define la estructura esperada para el POST de transferencia.
type TransferenciaInput struct {
	IdTiendaOrigen  int                          `json:"id_tienda_origen"` // Opcional, usado por admin
	IdTiendaDestino int                          `json:"id_tienda_destino"`
	Observacion     string                       `json:"observacion"`
	Productos       []TransferenciaProductoInput `json:"productos"`
}

// TransferenciaDetalle define la estructura del detalle del producto en la respuesta.
type TransferenciaDetalle struct {
	IdProducto     int    `json:"id_producto"`
	NombreProducto string `json:"nombre_producto"`
	Cantidad       int    `json:"cantidad"`
	StockOrigen    int    `json:"stock_origen"`
}

// TransferenciaResponse define la estructura de salida para el listado de transferencias.
type TransferenciaResponse struct {
	IdTransferencia             int                    `json:"id_transferencia"`
	IdTiendaOrigen              int                    `json:"id_tienda_origen"`
	TiendaOrigenNombre          string                 `json:"tienda_origen_nombre"`
	IdTiendaDestino             int                    `json:"id_tienda_destino"`
	TiendaDestinoNombre         string                 `json:"tienda_destino_nombre"`
	IdUsuario                   int                    `json:"id_usuario"`
	UsuarioNombre               string                 `json:"usuario_nombre"`
	Fecha                       string                 `json:"fecha"`
	Observacion                 string                 `json:"observacion"`
	Estado                      string                 `json:"estado"`
	RequiereConfirmacionDestino bool                   `json:"requiere_confirmacion_destino"`
	Parcial                     bool                   `json:"parcial"`
	Productos                   []TransferenciaDetalle `json:"productos"`
}

// TransferenciasHandler despacha según el método HTTP.
func TransferenciasHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			getTransferencias(db, w, r)
		case http.MethodPost:
			createTransferencia(db, w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Método HTTP no soportado."})
		}
	}
}

// getTransferencias lista el historial de transferencias.
func getTransferencias(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "No autorizado."})
		return
	}

	idTienda := GetTiendaIDFromCtxOrDb(db, r)
	isAdmin := claims.Rol == "admin_libreria"

	// Si es admin, filtramos con $1 = 0 (mostrar todo)
	// Si es operador, filtramos con $1 = 1 (mostrar solo lo que involucre a su idTienda)
	filterMode := 1
	if isAdmin {
		filterMode = 0
	}

	query := `
		SELECT 
			t.id_transferencia,
			t.id_tienda_origen,
			to_store.nombre AS tienda_origen_nombre,
			t.id_tienda_destino,
			td_store.nombre AS tienda_destino_nombre,
			t.id_usuario,
			u.nombre AS usuario_nombre,
			TO_CHAR(t.fecha, 'YYYY-MM-DD HH24:MI:SS') AS fecha,
			COALESCE(t.observacion, '') AS observacion,
			t.estado,
			t.requiere_confirmacion_destino,
			t.parcial,
			COALESCE(
				(SELECT json_agg(
					json_build_object(
						'id_producto', dt.id_producto,
						'nombre_producto', p.nombre,
						'cantidad', dt.cantidad,
						'stock_origen', COALESCE(st.stock_actual, 0)
					)
				) FROM inventario.detalle_transferencias dt
				  JOIN inventario.productos p ON dt.id_producto = p.id_producto
				  LEFT JOIN inventario.stock_tiendas st ON st.id_producto = dt.id_producto AND st.id_tienda = t.id_tienda_origen
				  WHERE dt.id_transferencia = t.id_transferencia
				),
				'[]'::json
			) AS productos
		FROM inventario.transferencias t
		JOIN configuracion.tiendas to_store ON t.id_tienda_origen = to_store.id_tienda
		JOIN configuracion.tiendas td_store ON t.id_tienda_destino = td_store.id_tienda
		JOIN seguridad.usuarios u ON t.id_usuario = u.id_usuario
		WHERE ($1 = 0 OR t.id_tienda_origen = $2 OR t.id_tienda_destino = $2)
		ORDER BY t.fecha DESC
	`

	rows, err := db.Query(query, filterMode, idTienda)
	if err != nil {
		log.Printf("ERROR transfer_handler.go: Error al consultar las transferencias: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error interno al consultar las transferencias."})
		return
	}
	defer rows.Close()

	transferencias := []TransferenciaResponse{}
	for rows.Next() {
		var t TransferenciaResponse
		var productosRaw []byte

		err := rows.Scan(
			&t.IdTransferencia,
			&t.IdTiendaOrigen, &t.TiendaOrigenNombre,
			&t.IdTiendaDestino, &t.TiendaDestinoNombre,
			&t.IdUsuario, &t.UsuarioNombre,
			&t.Fecha, &t.Observacion,
			&t.Estado, &t.RequiereConfirmacionDestino, &t.Parcial,
			&productosRaw,
		)
		if err != nil {
			continue
		}

		t.Productos = []TransferenciaDetalle{}
		if len(productosRaw) > 0 {
			json.Unmarshal(productosRaw, &t.Productos)
		}

		transferencias = append(transferencias, t)
	}

	json.NewEncoder(w).Encode(transferencias)
}

// createTransferencia procesa una nueva solicitud de transferencia en estado Pendiente.
func createTransferencia(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "No autorizado."})
		return
	}

	// El administrador no puede realizar transferencias (solo ver historial)
	if claims.Rol == "admin_libreria" {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "El administrador no tiene permitido crear transferencias de productos, únicamente visualizar el historial."})
		return
	}

	var input TransferenciaInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido."})
		return
	}

	// El origen es el seleccionado por el usuario, el destino es la tienda del usuario logueado (A)
	idTiendaDestino := GetTiendaIDFromCtxOrDb(db, r)
	idTiendaOrigen := input.IdTiendaOrigen

	if idTiendaOrigen <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "La tienda de origen es obligatoria."})
		return
	}

	if idTiendaOrigen == idTiendaDestino {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "La tienda de origen y de destino no pueden ser la misma."})
		return
	}

	if len(input.Productos) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Debe agregar al menos un producto al pedido."})
		return
	}

	// Verificar si la tienda origen existe y está activa
	var origenActivo bool
	err := db.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM configuracion.tiendas WHERE id_tienda = $1 AND estado = 'activa')`,
		idTiendaOrigen,
	).Scan(&origenActivo)
	if err != nil || !origenActivo {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "La sucursal de origen no existe o no está activa."})
		return
	}

	// Iniciar la transacción
	tx, err := db.Begin()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al iniciar la transacción."})
		return
	}
	defer tx.Rollback()

	// Insertar el registro principal en estado 'Pendiente'
	var idTransferencia int
	err = tx.QueryRow(`
		INSERT INTO inventario.transferencias (id_tienda_origen, id_tienda_destino, id_usuario, observacion, estado, requiere_confirmacion_destino, parcial)
		VALUES ($1, $2, $3, $4, 'Pendiente', FALSE, FALSE)
		RETURNING id_transferencia`,
		idTiendaOrigen, idTiendaDestino, claims.IdUsuario, input.Observacion,
	).Scan(&idTransferencia)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al registrar la transferencia principal."})
		return
	}

	// Insertar los productos en detalle_transferencias
	for _, item := range input.Productos {
		if item.IdProducto <= 0 || item.Cantidad <= 0 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "ID de producto o cantidad inválidos."})
			return
		}

		_, err = tx.Exec(`
			INSERT INTO inventario.detalle_transferencias (id_transferencia, id_producto, cantidad)
			VALUES ($1, $2, $3)`,
			idTransferencia, item.IdProducto, item.Cantidad,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al registrar el detalle de la transferencia."})
			return
		}
	}

	if err := tx.Commit(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al confirmar la transacción."})
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"mensaje":          "Pedido de transferencia creado exitosamente en estado Pendiente.",
		"id_transferencia": idTransferencia,
	})
}

// ResponderTransferenciaInput define la entrada para aceptar/rechazar/modificar un pedido de transferencia.
type ResponderTransferenciaInput struct {
	IdTransferencia int                          `json:"id_transferencia"`
	Accion          string                       `json:"accion"` // "aceptar" o "rechazar"
	Productos       []TransferenciaProductoInput `json:"productos,omitempty"`
}

// ResponderTransferenciaHandler procesa la respuesta de la sucursal de origen (B).
func ResponderTransferenciaHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Método no soportado."})
			return
		}

		claims, ok := middleware.GetClaims(r)
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "No autorizado."})
			return
		}

		var input ResponderTransferenciaInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido."})
			return
		}

		idTienda := GetTiendaIDFromCtxOrDb(db, r)

		// Buscar transferencia
		var idTiendaOrigen, idTiendaDestino int
		var estado string
		err := db.QueryRow(`
			SELECT id_tienda_origen, id_tienda_destino, estado 
			FROM inventario.transferencias 
			WHERE id_transferencia = $1`,
			input.IdTransferencia,
		).Scan(&idTiendaOrigen, &idTiendaDestino, &estado)
		if err == sql.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Transferencia no encontrada."})
			return
		} else if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar la transferencia."})
			return
		}

		if estado != "Pendiente" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Solo se pueden responder transferencias en estado Pendiente."})
			return
		}

		// Solo la tienda de origen puede responder
		if idTienda != idTiendaOrigen {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{"error": "Solo la sucursal de origen puede responder a este pedido."})
			return
		}

		tx, err := db.Begin()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al iniciar la transacción."})
			return
		}
		defer tx.Rollback()

		if input.Accion == "rechazar" {
			_, err = tx.Exec(`
				UPDATE inventario.transferencias 
				SET estado = 'Cancelada' 
				WHERE id_transferencia = $1`,
				input.IdTransferencia,
			)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "Error al rechazar el pedido."})
				return
			}
			if err := tx.Commit(); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "Error al confirmar los cambios."})
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"mensaje": "Pedido rechazado con éxito."})
			return
		}

		if input.Accion != "aceptar" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Acción inválida. Debe ser 'aceptar' o 'rechazar'."})
			return
		}

		// Si es "aceptar", evaluamos los productos del pedido
		var originalItems []TransferenciaProductoInput
		rows, err := tx.Query(`
			SELECT id_producto, cantidad 
			FROM inventario.detalle_transferencias 
			WHERE id_transferencia = $1`,
			input.IdTransferencia,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar detalles originales del pedido."})
			return
		}
		for rows.Next() {
			var item TransferenciaProductoInput
			if err := rows.Scan(&item.IdProducto, &item.Cantidad); err == nil {
				originalItems = append(originalItems, item)
			}
		}
		rows.Close()

		// Determinar si hay modificaciones manuales de B, o si hay stock insuficiente que requiera transaccion parcial.
		hasModifications := false
		var finalItems []TransferenciaProductoInput

		// Si B envió una lista de productos modificada, la usamos
		if len(input.Productos) > 0 {
			hasModifications = true
			finalItems = input.Productos
		} else {
			// Si B no envió cambios, verificamos el stock de cada producto en la tienda de origen (B)
			for _, item := range originalItems {
				var stockActual int
				err = tx.QueryRow(`
					SELECT COALESCE(stock_actual, 0) 
					FROM inventario.stock_tiendas 
					WHERE id_producto = $1 AND id_tienda = $2`,
					item.IdProducto, idTiendaOrigen,
				).Scan(&stockActual)
				if err == sql.ErrNoRows {
					stockActual = 0
				} else if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					json.NewEncoder(w).Encode(map[string]string{"error": "Error al verificar stock."})
					return
				}

				if stockActual < item.Cantidad {
					hasModifications = true
					item.Cantidad = stockActual // ajustar a lo disponible
				}
				finalItems = append(finalItems, item)
			}
		}

		// Chequear si todas las cantidades en el pedido final son 0
		allZero := true
		for _, item := range finalItems {
			if item.Cantidad > 0 {
				allZero = false
				break
			}
		}

		if allZero {
			// Auto-rechazo
			_, err = tx.Exec(`
				UPDATE inventario.transferencias 
				SET estado = 'Cancelada', requiere_confirmacion_destino = FALSE, parcial = TRUE 
				WHERE id_transferencia = $1`,
				input.IdTransferencia,
			)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "Error al auto-cancelar el pedido."})
				return
			}
			// Eliminar todos los detalles
			_, err = tx.Exec(`
				DELETE FROM inventario.detalle_transferencias 
				WHERE id_transferencia = $1`,
				input.IdTransferencia,
			)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "Error al limpiar detalles."})
				return
			}

			if err := tx.Commit(); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "Error al confirmar transacción."})
				return
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{
				"mensaje": "Pedido auto-rechazado debido a que ninguno de los productos solicitados tiene stock en la sucursal de origen.",
				"estado": "Cancelada",
			})
			return
		}

		if hasModifications {
			// Actualizar los detalles en detalle_transferencias con las nuevas cantidades
			// Primero borramos
			_, err = tx.Exec(`DELETE FROM inventario.detalle_transferencias WHERE id_transferencia = $1`, input.IdTransferencia)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "Error al actualizar productos."})
				return
			}
			// Insertamos las cantidades modificadas > 0
			for _, item := range finalItems {
				if item.Cantidad <= 0 {
					continue
				}
				_, err = tx.Exec(`
					INSERT INTO inventario.detalle_transferencias (id_transferencia, id_producto, cantidad)
					VALUES ($1, $2, $3)`,
					input.IdTransferencia, item.IdProducto, item.Cantidad,
				)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					json.NewEncoder(w).Encode(map[string]string{"error": "Error al insertar detalle modificado."})
					return
				}
			}

			// Actualizamos estado y flags en la transferencia
			_, err = tx.Exec(`
				UPDATE inventario.transferencias 
				SET requiere_confirmacion_destino = TRUE, parcial = TRUE 
				WHERE id_transferencia = $1`,
				input.IdTransferencia,
			)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "Error al actualizar estado del pedido."})
				return
			}

			if err := tx.Commit(); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "Error al guardar confirmación."})
				return
			}

			json.NewEncoder(w).Encode(map[string]string{
				"mensaje": "El pedido ha sido modificado/parcializado y espera la aceptación de la sucursal destino.",
				"estado": "Pendiente",
				"requiere_confirmacion": "true",
			})
			return
		}

		// Si hay stock completo y sin modificaciones -> Cambiar estado a 'En Progreso'
		// y restar stock en origen (B)
		_, err = tx.Exec(`
			UPDATE inventario.transferencias 
			SET estado = 'En Progreso', requiere_confirmacion_destino = FALSE 
			WHERE id_transferencia = $1`,
			input.IdTransferencia,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al actualizar estado a En Progreso."})
			return
		}

		for _, item := range finalItems {
			var stockActual int
			err = tx.QueryRow(`
				SELECT stock_actual 
				FROM inventario.stock_tiendas 
				WHERE id_producto = $1 AND id_tienda = $2 
				FOR UPDATE`,
				item.IdProducto, idTiendaOrigen,
			).Scan(&stockActual)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "Error al bloquear stock."})
				return
			}

			if stockActual < item.Cantidad {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": "Stock insuficiente durante el procesamiento final."})
				return
			}

			nuevoStock := stockActual - item.Cantidad
			_, err = tx.Exec(`
				UPDATE inventario.stock_tiendas 
				SET stock_actual = $1 
				WHERE id_producto = $2 AND id_tienda = $3`,
				nuevoStock, item.IdProducto, idTiendaOrigen,
			)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "Error al actualizar stock de origen."})
				return
			}

			// Movimiento stock de salida
			_, err = tx.Exec(`
				INSERT INTO inventario.movimientos_stock 
				  (id_producto, id_usuario, id_tienda, tipo_movimiento, cantidad, stock_resultante, referencia_id)
				VALUES ($1, $2, $3, 'TRANSFER_SALIDA', $4, $5, $6)`,
				item.IdProducto, claims.IdUsuario, idTiendaOrigen, -item.Cantidad, nuevoStock, input.IdTransferencia,
			)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "Error al registrar movimiento de salida."})
				return
			}
		}

		if err := tx.Commit(); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al procesar la aceptación."})
			return
		}

		json.NewEncoder(w).Encode(map[string]string{
			"mensaje": "Pedido aceptado y enviado con éxito (En Progreso).",
			"estado": "En Progreso",
		})
	}
}

// ConfirmarParcialInput define la entrada para aceptar o rechazar la modificación de A.
type ConfirmarParcialInput struct {
	IdTransferencia int    `json:"id_transferencia"`
	Accion          string `json:"accion"` // "aceptar" o "rechazar"
}

// ConfirmarParcialTransferenciaHandler procesa la aceptación o rechazo de A sobre una propuesta parcial.
func ConfirmarParcialTransferenciaHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Método no soportado."})
			return
		}

		claims, ok := middleware.GetClaims(r)
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "No autorizado."})
			return
		}

		var input ConfirmarParcialInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido."})
			return
		}

		idTienda := GetTiendaIDFromCtxOrDb(db, r)

		var idTiendaOrigen, idTiendaDestino int
		var estado string
		var requiereConfirmacion bool
		err := db.QueryRow(`
			SELECT id_tienda_origen, id_tienda_destino, estado, requiere_confirmacion_destino 
			FROM inventario.transferencias 
			WHERE id_transferencia = $1`,
			input.IdTransferencia,
		).Scan(&idTiendaOrigen, &idTiendaDestino, &estado, &requiereConfirmacion)
		if err == sql.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Transferencia no encontrada."})
			return
		} else if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar la transferencia."})
			return
		}

		if estado != "Pendiente" || !requiereConfirmacion {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Esta transferencia no requiere confirmación del destino."})
			return
		}

		// Solo la tienda de destino (A) puede confirmar
		if idTienda != idTiendaDestino {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{"error": "Solo la sucursal de destino puede confirmar o rechazar esta transferencia."})
			return
		}

		tx, err := db.Begin()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al iniciar la transacción."})
			return
		}
		defer tx.Rollback()

		if input.Accion == "rechazar" {
			_, err = tx.Exec(`
				UPDATE inventario.transferencias 
				SET estado = 'Cancelada', requiere_confirmacion_destino = FALSE 
				WHERE id_transferencia = $1`,
				input.IdTransferencia,
			)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "Error al actualizar estado."})
				return
			}
			if err := tx.Commit(); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "Error al confirmar cambios."})
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"mensaje": "Transferencia parcial rechazada y cancelada."})
			return
		}

		if input.Accion != "aceptar" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Acción inválida. Debe ser 'aceptar' o 'rechazar'."})
			return
		}

		// Si A acepta la transferencia parcial -> Cambiar a 'En Progreso', y restar stock de B
		_, err = tx.Exec(`
			UPDATE inventario.transferencias 
			SET estado = 'En Progreso', requiere_confirmacion_destino = FALSE 
			WHERE id_transferencia = $1`,
			input.IdTransferencia,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al cambiar estado a En Progreso."})
			return
		}

		// Restar stock de origen B
		rows, err := tx.Query(`
			SELECT id_producto, cantidad 
			FROM inventario.detalle_transferencias 
			WHERE id_transferencia = $1`,
			input.IdTransferencia,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar detalles de la transferencia."})
			return
		}
		defer rows.Close()

		type itemStock struct {
			IdProducto int
			Cantidad   int
		}
		var itemsToDeduct []itemStock
		for rows.Next() {
			var item itemStock
			if err := rows.Scan(&item.IdProducto, &item.Cantidad); err == nil {
				itemsToDeduct = append(itemsToDeduct, item)
			}
		}
		rows.Close()

		for _, item := range itemsToDeduct {
			var stockActual int
			err = tx.QueryRow(`
				SELECT stock_actual 
				FROM inventario.stock_tiendas 
				WHERE id_producto = $1 AND id_tienda = $2 
				FOR UPDATE`,
				item.IdProducto, idTiendaOrigen,
			).Scan(&stockActual)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "Error al verificar stock de origen."})
				return
			}

			if stockActual < item.Cantidad {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("Stock insuficiente en origen para producto ID %d.", item.IdProducto)})
				return
			}

			nuevoStock := stockActual - item.Cantidad
			_, err = tx.Exec(`
				UPDATE inventario.stock_tiendas 
				SET stock_actual = $1 
				WHERE id_producto = $2 AND id_tienda = $3`,
				nuevoStock, item.IdProducto, idTiendaOrigen,
			)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "Error al restar stock de origen."})
				return
			}

			// Movimiento stock de salida
			_, err = tx.Exec(`
				INSERT INTO inventario.movimientos_stock 
				  (id_producto, id_usuario, id_tienda, tipo_movimiento, cantidad, stock_resultante, referencia_id)
				VALUES ($1, $2, $3, 'TRANSFER_SALIDA', $4, $5, $6)`,
				item.IdProducto, claims.IdUsuario, idTiendaOrigen, -item.Cantidad, nuevoStock, input.IdTransferencia,
			)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "Error al registrar movimiento de salida."})
				return
			}
		}

		if err := tx.Commit(); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al confirmar cambios parciales."})
			return
		}

		json.NewEncoder(w).Encode(map[string]string{
			"mensaje": "Transferencia parcial aceptada. Productos en camino (En Progreso).",
			"estado": "En Progreso",
		})
	}
}

// RecibirTransferenciaInput define la entrada para recibir productos.
type RecibirTransferenciaInput struct {
	IdTransferencia int `json:"id_transferencia"`
}

// RecibirTransferenciaHandler procesa la recepción de la mercadería en la tienda destino (A).
func RecibirTransferenciaHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Método no soportado."})
			return
		}

		claims, ok := middleware.GetClaims(r)
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "No autorizado."})
			return
		}

		var input RecibirTransferenciaInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido."})
			return
		}

		idTienda := GetTiendaIDFromCtxOrDb(db, r)

		var idTiendaOrigen, idTiendaDestino int
		var estado string
		err := db.QueryRow(`
			SELECT id_tienda_origen, id_tienda_destino, estado 
			FROM inventario.transferencias 
			WHERE id_transferencia = $1`,
			input.IdTransferencia,
		).Scan(&idTiendaOrigen, &idTiendaDestino, &estado)
		if err == sql.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Transferencia no encontrada."})
			return
		} else if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar la transferencia."})
			return
		}

		if estado != "En Progreso" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Solo se pueden recibir productos de transferencias en estado En Progreso."})
			return
		}

		// Solo la tienda de destino (A) puede marcar como recibida
		if idTienda != idTiendaDestino {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{"error": "Solo la sucursal de destino puede marcar esta transferencia como recibida."})
			return
		}

		tx, err := db.Begin()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al iniciar la transacción."})
			return
		}
		defer tx.Rollback()

		// Actualizar estado a 'Recibida'
		_, err = tx.Exec(`
			UPDATE inventario.transferencias 
			SET estado = 'Recibida' 
			WHERE id_transferencia = $1`,
			input.IdTransferencia,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al actualizar estado a Recibida."})
			return
		}

		// Obtener productos de la transferencia
		rows, err := tx.Query(`
			SELECT id_producto, cantidad 
			FROM inventario.detalle_transferencias 
			WHERE id_transferencia = $1`,
			input.IdTransferencia,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar detalles de la transferencia."})
			return
		}
		defer rows.Close()

		type itemStock struct {
			IdProducto int
			Cantidad   int
		}
		var itemsToReceive []itemStock
		for rows.Next() {
			var item itemStock
			if err := rows.Scan(&item.IdProducto, &item.Cantidad); err == nil {
				itemsToReceive = append(itemsToReceive, item)
			}
		}
		rows.Close()

		// Incrementar stock en destino (A) y registrar movimiento
		for _, item := range itemsToReceive {
			var nuevoStockDestino int
			err = tx.QueryRow(`
				INSERT INTO inventario.stock_tiendas (id_tienda, id_producto, stock_actual, stock_alerta_min)
				VALUES ($1, $2, $3, 5)
				ON CONFLICT (id_tienda, id_producto)
				DO UPDATE SET stock_actual = inventario.stock_tiendas.stock_actual + $3
				RETURNING stock_actual`,
				idTiendaDestino, item.IdProducto, item.Cantidad,
			).Scan(&nuevoStockDestino)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "Error al actualizar stock de destino."})
				return
			}

			// Movimiento de Entrada (Destino)
			_, err = tx.Exec(`
				INSERT INTO inventario.movimientos_stock
				  (id_producto, id_usuario, id_tienda, tipo_movimiento, cantidad, stock_resultante, referencia_id)
				VALUES ($1, $2, $3, 'TRANSFER_ENTRADA', $4, $5, $6)`,
				item.IdProducto, claims.IdUsuario, idTiendaDestino, item.Cantidad, nuevoStockDestino, input.IdTransferencia,
			)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "Error al registrar movimiento de entrada."})
				return
			}
		}

		if err := tx.Commit(); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al confirmar los cambios."})
			return
		}

		json.NewEncoder(w).Encode(map[string]string{
			"mensaje": "Productos recibidos y cargados al inventario de la sucursal con éxito.",
			"estado": "Recibida",
		})
	}
}
