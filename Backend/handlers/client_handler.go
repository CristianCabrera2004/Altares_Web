// Backend/handlers/client_handler.go
// ─────────────────────────────────────────────────────────────────────────────
// Catálogo de Clientes (HU-Clientes / Anexo 3)
//
// Endpoints:
//   GET    /api/clientes            → Lista todos los clientes
//   GET    /api/clientes?id=X       → Un cliente específico
//   POST   /api/clientes            → Crea un nuevo cliente
//   PUT    /api/clientes?id=X       → Actualiza datos del cliente
//   GET    /api/clientes/buscar?q=  → Autocompletado por nombre o cédula (pg_trgm)
//
// La tabla operaciones.clientes ya existe en init.sql con índices trigram.
// ─────────────────────────────────────────────────────────────────────────────
package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// Cliente mapea la tabla operaciones.clientes.
type Cliente struct {
	IdCliente int    `json:"id_cliente"`
	CedulaRuc string `json:"cedula_ruc"`
	Nombre    string `json:"nombre"`
	Direccion string `json:"direccion,omitempty"`
	Telefono  string `json:"telefono,omitempty"`
	Email     string `json:"email,omitempty"`
}

// ClientHandler despacha por método HTTP.
func ClientHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			getClients(db, w, r)
		case http.MethodPost:
			createClient(db, w, r)
		case http.MethodPut:
			updateClient(db, w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Método HTTP no soportado."})
		}
	}
}

// BuscarClienteHandler busca clientes por nombre o cédula/RUC con autocompletado.
func BuscarClienteHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Solo se acepta GET en este endpoint."})
			return
		}

		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if q == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "El parámetro ?q es obligatorio."})
			return
		}

		// Buscar por coincidencia exacta de cédula o por similitud de nombre (trigram)
		rows, err := db.Query(`
			SELECT id_cliente, cedula_ruc, nombre,
			       COALESCE(direccion, ''), COALESCE(telefono, ''), COALESCE(email, '')
			FROM operaciones.clientes
			WHERE cedula_ruc LIKE $1 || '%'
			   OR nombre % $1
			   OR nombre ILIKE '%' || $1 || '%'
			ORDER BY similarity(nombre, $1) DESC, nombre ASC
			LIMIT 15`, q)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al buscar clientes."})
			return
		}
		defer rows.Close()

		clientes := []Cliente{}
		for rows.Next() {
			var c Cliente
			if err := rows.Scan(&c.IdCliente, &c.CedulaRuc, &c.Nombre, &c.Direccion, &c.Telefono, &c.Email); err != nil {
				continue
			}
			clientes = append(clientes, c)
		}
		json.NewEncoder(w).Encode(clientes)
	}
}

// ─── GET ─────────────────────────────────────────────────────────────────────
func getClients(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")

	if idStr != "" {
		id, err := strconv.Atoi(idStr)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "?id debe ser un entero."})
			return
		}
		var c Cliente
		err = db.QueryRow(`
			SELECT id_cliente, cedula_ruc, nombre,
			       COALESCE(direccion, ''), COALESCE(telefono, ''), COALESCE(email, '')
			FROM operaciones.clientes WHERE id_cliente = $1`, id,
		).Scan(&c.IdCliente, &c.CedulaRuc, &c.Nombre, &c.Direccion, &c.Telefono, &c.Email)
		if err == sql.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Cliente no encontrado."})
			return
		}
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar el cliente."})
			return
		}
		json.NewEncoder(w).Encode(c)
		return
	}

	rows, err := db.Query(`
		SELECT id_cliente, cedula_ruc, nombre,
		       COALESCE(direccion, ''), COALESCE(telefono, ''), COALESCE(email, '')
		FROM operaciones.clientes
		ORDER BY nombre ASC`)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al listar clientes."})
		return
	}
	defer rows.Close()

	clientes := []Cliente{}
	for rows.Next() {
		var c Cliente
		if err := rows.Scan(&c.IdCliente, &c.CedulaRuc, &c.Nombre, &c.Direccion, &c.Telefono, &c.Email); err != nil {
			continue
		}
		clientes = append(clientes, c)
	}
	json.NewEncoder(w).Encode(clientes)
}

// ─── POST ────────────────────────────────────────────────────────────────────
func createClient(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	var c Cliente
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido."})
		return
	}

	if c.CedulaRuc == "" || c.Nombre == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "'cedula_ruc' y 'nombre' son obligatorios."})
		return
	}

	// Validar longitud de cédula/RUC (10 o 13 dígitos en Ecuador)
	if len(c.CedulaRuc) != 10 && len(c.CedulaRuc) != 13 && c.CedulaRuc != "9999999999999" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "La cédula debe tener 10 dígitos o el RUC 13 dígitos."})
		return
	}

	err := db.QueryRow(`
		INSERT INTO operaciones.clientes (cedula_ruc, nombre, direccion, telefono, email)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id_cliente`,
		c.CedulaRuc, c.Nombre, nullIfEmpty(c.Direccion), nullIfEmpty(c.Telefono), nullIfEmpty(c.Email),
	).Scan(&c.IdCliente)
	if err != nil {
		if strings.Contains(err.Error(), "uq_clientes_cedula_ruc") {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{"error": "Ya existe un cliente con esa cédula/RUC."})
			return
		}
		log.Printf("ERROR client_handler.go: Error al crear el cliente: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al crear el cliente."})
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"mensaje":  "Cliente registrado exitosamente.",
		"cliente":  c,
	})
}

// ─── PUT ─────────────────────────────────────────────────────────────────────
func updateClient(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "El parámetro ?id es obligatorio."})
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "?id debe ser un entero."})
		return
	}

	var c Cliente
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido."})
		return
	}

	if c.Nombre == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "'nombre' es obligatorio."})
		return
	}

	result, err := db.Exec(`
		UPDATE operaciones.clientes
		SET nombre = $1, direccion = $2, telefono = $3, email = $4
		WHERE id_cliente = $5`,
		c.Nombre, nullIfEmpty(c.Direccion), nullIfEmpty(c.Telefono), nullIfEmpty(c.Email), id,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al actualizar el cliente."})
		return
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "Cliente no encontrado."})
		return
	}

	c.IdCliente = id
	json.NewEncoder(w).Encode(map[string]interface{}{
		"mensaje": "Cliente actualizado exitosamente.",
		"cliente": c,
	})
}

// nullIfEmpty devuelve nil para strings vacíos (→ NULL en PostgreSQL).
func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
