package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

type IngresoMultipleItem struct {
	IdProducto        int `json:"id_producto"`
	CantidadIngresada int `json:"cantidad_ingresada"`
	CostoUnitario     int `json:"costo_unitario"`
}

type IngresoMultipleInput struct {
	IdProveedor int                   `json:"id_proveedor"`
	IdUsuario   int                   `json:"id_usuario"`
	Observacion string                `json:"observacion"`
	Items       []IngresoMultipleItem `json:"items"`
}

// IngresoMultipleHandler registra múltiples entradas de mercadería de forma transaccional.
func IngresoMultipleHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Solo se acepta POST en este endpoint."})
			return
		}

		var input IngresoMultipleInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido o malformado."})
			return
		}

		if len(input.Items) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Debe enviar al menos un producto."})
			return
		}

		if input.IdUsuario <= 0 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "'id_usuario' es obligatorio."})
			return
		}

		for _, item := range input.Items {
			if item.IdProducto <= 0 || item.CantidadIngresada <= 0 || item.CostoUnitario < 0 {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{
					"error": "Todos los productos deben tener 'id_producto', 'cantidad_ingresada' (>0) y 'costo_unitario' (>=0).",
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

		var proveedorNullable *int
		if input.IdProveedor > 0 {
			proveedorNullable = &input.IdProveedor
		}

		for _, item := range input.Items {
			// Upsert del stock en la tienda
			var nuevoStock int
			err = tx.QueryRow(`
				INSERT INTO inventario.stock_tiendas (id_tienda, id_producto, stock_actual, stock_alerta_min)
				VALUES ($1, $2, $3, 5)
				ON CONFLICT (id_tienda, id_producto)
				DO UPDATE SET stock_actual = inventario.stock_tiendas.stock_actual + $3
				RETURNING stock_actual`,
				idTienda, item.IdProducto, item.CantidadIngresada,
			).Scan(&nuevoStock)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "Error al actualizar el stock de la tienda."})
				return
			}

			// Insertar registro de ingreso
			var idIngreso int
			err = tx.QueryRow(`
				INSERT INTO inventario.ingreso_inventario
				  (id_producto, id_proveedor, id_usuario, id_tienda, cantidad_ingresada, costo_unitario, observacion)
				VALUES ($1, $2, $3, $4, $5, $6, $7)
				RETURNING id_ingreso`,
				item.IdProducto, proveedorNullable, input.IdUsuario, idTienda,
				item.CantidadIngresada, item.CostoUnitario, input.Observacion,
			).Scan(&idIngreso)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "Error al registrar el ingreso en la base de datos."})
				return
			}

			// Registrar movimiento
			_, err = tx.Exec(`
				INSERT INTO inventario.movimientos_stock
				  (id_producto, id_usuario, id_tienda, tipo_movimiento, cantidad, stock_resultante, referencia_id)
				VALUES ($1, $2, $3, 'INGRESO', $4, $5, $6)`,
				item.IdProducto, input.IdUsuario, idTienda, item.CantidadIngresada, nuevoStock, idIngreso,
			)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "Error al registrar el movimiento de stock."})
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
			"mensaje": "Ingreso múltiple registrado y stock actualizado exitosamente.",
			"items":   len(input.Items),
		})
	}
}
