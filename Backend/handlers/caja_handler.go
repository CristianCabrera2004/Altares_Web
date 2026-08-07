package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

type CajaResponse struct {
	SaldoCaja int `json:"saldo_caja"`
}

type CajaUpdateRequest struct {
	SaldoCaja int `json:"saldo_caja"`
}

func CajaHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Content-Type", "application/json")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		idTienda := GetTiendaIDFromCtxOrDb(db, r)
		if idTienda == 0 {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{"error": "No tienes una tienda asignada o no se pudo determinar."})
			return
		}

		switch r.Method {
		case http.MethodGet:
			var saldo int
			err := db.QueryRow("SELECT saldo_caja FROM configuracion.tiendas WHERE id_tienda = $1", idTienda).Scan(&saldo)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar el saldo de caja."})
				return
			}
			json.NewEncoder(w).Encode(CajaResponse{SaldoCaja: saldo})

		case http.MethodPut:
			var req CajaUpdateRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido."})
				return
			}

			_, err := db.Exec("UPDATE configuracion.tiendas SET saldo_caja = $1 WHERE id_tienda = $2", req.SaldoCaja, idTienda)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "Error al actualizar el saldo de caja."})
				return
			}

			json.NewEncoder(w).Encode(map[string]string{"mensaje": "Saldo actualizado correctamente."})

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Método no permitido."})
		}
	}
}
