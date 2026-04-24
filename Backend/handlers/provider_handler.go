// Backend/handlers/provider_handler.go
// ─────────────────────────────────────────────────────────────────────────────
// CRUD completo para inventario.proveedores (GET, POST, PUT, DELETE).
// Todas las escrituras se envuelven en transacciones SQL (CA 45).
// ─────────────────────────────────────────────────────────────────────────────
package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
)

// Proveedor mapea la tabla inventario.proveedores.
type Proveedor struct {
	IdProveedor     int    `json:"id_proveedor"`
	Identificacion  string `json:"identificacion"`
	NombreProveedor string `json:"nombre_proveedor"`
	Contacto        string `json:"contacto"`
	Email           string `json:"email"`
	Telefono        string `json:"telefono"`
}

// ProviderHandler despacha por método HTTP.
func ProviderHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			getProviders(db, w, r)
		case http.MethodPost:
			createProvider(db, w, r)
		case http.MethodPut:
			updateProvider(db, w, r)
		case http.MethodDelete:
			deleteProvider(db, w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Método HTTP no soportado."})
		}
	}
}

func getProviders(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT id_proveedor, identificacion, nombre_proveedor,
		       COALESCE(contacto,''), COALESCE(email,''), COALESCE(telefono,'')
		FROM inventario.proveedores
		ORDER BY nombre_proveedor ASC`)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar proveedores."})
		return
	}
	defer rows.Close()

	proveedores := []Proveedor{}
	for rows.Next() {
		var p Proveedor
		rows.Scan(&p.IdProveedor, &p.Identificacion, &p.NombreProveedor,
			&p.Contacto, &p.Email, &p.Telefono)
		proveedores = append(proveedores, p)
	}
	json.NewEncoder(w).Encode(proveedores)
}

func createProvider(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	var p Proveedor
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido."})
		return
	}
	if p.Identificacion == "" || p.NombreProveedor == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "'identificacion' y 'nombre_proveedor' son obligatorios."})
		return
	}

	tx, err := db.Begin() // BEGIN
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "No se pudo iniciar la transacción."})
		return
	}
	defer tx.Rollback()

	err = tx.QueryRow(`
		INSERT INTO inventario.proveedores (identificacion, nombre_proveedor, contacto, email, telefono)
		VALUES ($1, $2, $3, $4, $5) RETURNING id_proveedor`,
		p.Identificacion, p.NombreProveedor, p.Contacto, p.Email, p.Telefono,
	).Scan(&p.IdProveedor)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al crear proveedor. La identificación puede estar duplicada."})
		return
	}

	tx.Commit() // COMMIT
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(p)
}

func updateProvider(db *sql.DB, w http.ResponseWriter, r *http.Request) {
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

	var p Proveedor
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido."})
		return
	}

	tx, err := db.Begin() // BEGIN
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "No se pudo iniciar la transacción."})
		return
	}
	defer tx.Rollback()

	res, err := tx.Exec(`
		UPDATE inventario.proveedores
		SET identificacion=$1, nombre_proveedor=$2, contacto=$3, email=$4, telefono=$5
		WHERE id_proveedor=$6`,
		p.Identificacion, p.NombreProveedor, p.Contacto, p.Email, p.Telefono, id,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al actualizar el proveedor."})
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "Proveedor no encontrado."})
		return
	}

	tx.Commit() // COMMIT
	p.IdProveedor = id
	json.NewEncoder(w).Encode(map[string]interface{}{"mensaje": "Proveedor actualizado.", "proveedor": p})
}

func deleteProvider(db *sql.DB, w http.ResponseWriter, r *http.Request) {
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

	tx, err := db.Begin() // BEGIN
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "No se pudo iniciar la transacción."})
		return
	}
	defer tx.Rollback()

	res, err := tx.Exec(`DELETE FROM inventario.proveedores WHERE id_proveedor=$1`, id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "No se puede eliminar: el proveedor tiene ingresos de inventario vinculados."})
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "Proveedor no encontrado."})
		return
	}

	tx.Commit() // COMMIT
	json.NewEncoder(w).Encode(map[string]string{"mensaje": "Proveedor eliminado exitosamente."})
}
