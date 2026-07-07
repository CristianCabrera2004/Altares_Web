package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
)

type ConfigInput struct {
	Clave string `json:"clave"`
	Valor string `json:"valor"`
}

// GET /api/configuracion
func GetConfigHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Solo GET permitido"})
			return
		}

		clave := r.URL.Query().Get("clave")
		if clave == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Debe proporcionar una clave"})
			return
		}

		var valor string
		err := db.QueryRow("SELECT valor FROM configuracion.parametros WHERE clave = $1", clave).Scan(&valor)
		if err != nil {
			if err == sql.ErrNoRows {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]string{"error": "Configuración no encontrada"})
			} else {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "Error interno del servidor"})
			}
			return
		}

		json.NewEncoder(w).Encode(map[string]string{
			"clave": clave,
			"valor": valor,
		})
	}
}

// PUT /api/configuracion
func PutConfigHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Solo PUT permitido"})
			return
		}

		var input ConfigInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Cuerpo JSON inválido"})
			return
		}

		if input.Clave == "" || input.Valor == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Clave y valor requeridos"})
			return
		}

		// Validar que si la clave es tasa_iva_grabado, el valor sea un entero >= 0
		if input.Clave == "tasa_iva_grabado" {
			val, err := strconv.Atoi(input.Valor)
			if err != nil || val < 0 || val > 100 {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": "El valor del IVA debe ser un número entero entre 0 y 100"})
				return
			}
		}

		_, err := db.Exec(`
			INSERT INTO configuracion.parametros (clave, valor) VALUES ($1, $2)
			ON CONFLICT (clave) DO UPDATE SET valor = EXCLUDED.valor`,
			input.Clave, input.Valor)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al actualizar la configuración"})
			return
		}

		json.NewEncoder(w).Encode(map[string]string{
			"mensaje": "Configuración actualizada correctamente",
		})
	}
}
