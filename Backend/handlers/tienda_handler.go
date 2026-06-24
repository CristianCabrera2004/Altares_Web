package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
)

type Tienda struct {
	IdTienda  int    `json:"id_tienda"`
	Nombre    string `json:"nombre"`
	Direccion string `json:"direccion"`
	Telefono  string `json:"telefono"`
	Estado    string `json:"estado"`
}

func TiendaHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			getTiendas(db, w, r)
		case http.MethodPost:
			createTienda(db, w, r)
		case http.MethodPut:
			updateTienda(db, w, r)
		case http.MethodDelete:
			deleteTienda(db, w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Método HTTP no soportado."})
		}
	}
}

func getTiendas(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`SELECT id_tienda, nombre, COALESCE(direccion, ''), COALESCE(telefono, ''), estado FROM configuracion.tiendas ORDER BY nombre ASC`)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar las tiendas."})
		return
	}
	defer rows.Close()

	var tiendas []Tienda
	for rows.Next() {
		var t Tienda
		if err := rows.Scan(&t.IdTienda, &t.Nombre, &t.Direccion, &t.Telefono, &t.Estado); err == nil {
			tiendas = append(tiendas, t)
		}
	}
	if tiendas == nil {
		tiendas = []Tienda{}
	}
	json.NewEncoder(w).Encode(tiendas)
}

func createTienda(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	var t Tienda
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido."})
		return
	}
	if t.Nombre == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "El nombre de la tienda es obligatorio."})
		return
	}

	var id int
	err := db.QueryRow(`
		INSERT INTO configuracion.tiendas (nombre, direccion, telefono, estado)
		VALUES ($1, $2, $3, 'activa') RETURNING id_tienda`,
		t.Nombre, t.Direccion, t.Telefono,
	).Scan(&id)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al crear la tienda."})
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"mensaje": "Tienda creada exitosamente.", "id_tienda": id})
}

func updateTienda(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "ID de tienda inválido."})
		return
	}

	var t Tienda
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido."})
		return
	}

	_, err = db.Exec(`
		UPDATE configuracion.tiendas
		SET nombre = $1, direccion = $2, telefono = $3, estado = $4
		WHERE id_tienda = $5`,
		t.Nombre, t.Direccion, t.Telefono, t.Estado, id,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al actualizar la tienda."})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"mensaje": "Tienda actualizada exitosamente."})
}

func deleteTienda(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "ID de tienda inválido."})
		return
	}

	// Baja lógica
	_, err = db.Exec(`UPDATE configuracion.tiendas SET estado = 'inactiva' WHERE id_tienda = $1`, id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al desactivar la tienda."})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"mensaje": "Tienda desactivada exitosamente."})
}

// TiendasActivasHandler lista solo las sucursales con estado 'activa'.
// Accesible para operadores y administradores.
func TiendasActivasHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Solo se acepta GET en este endpoint."})
			return
		}

		rows, err := db.Query(`
			SELECT id_tienda, nombre, COALESCE(direccion, ''), COALESCE(telefono, ''), estado 
			FROM configuracion.tiendas 
			WHERE estado = 'activa' 
			ORDER BY nombre ASC`)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar las tiendas activas."})
			return
		}
		defer rows.Close()

		var tiendas []Tienda
		for rows.Next() {
			var t Tienda
			if err := rows.Scan(&t.IdTienda, &t.Nombre, &t.Direccion, &t.Telefono, &t.Estado); err == nil {
				tiendas = append(tiendas, t)
			}
		}
		if tiendas == nil {
			tiendas = []Tienda{}
		}
		json.NewEncoder(w).Encode(tiendas)
	}
}

