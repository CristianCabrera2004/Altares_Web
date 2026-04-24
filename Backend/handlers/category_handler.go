// Backend/handlers/category_handler.go
// ─────────────────────────────────────────────────────────────────────────────
// CRUD completo para inventario.categorias (GET, POST, PUT, DELETE).
// Todas las escrituras se envuelven en transacciones SQL (CA 45).
// ─────────────────────────────────────────────────────────────────────────────
package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
)

// Categoria mapea la tabla inventario.categorias.
type Categoria struct {
	IdCategoria int    `json:"id_categoria"`
	Nombre      string `json:"nombre"`
	Detalle     string `json:"detalle"`
	TasaIva     int    `json:"tasa_iva"`
}

// CategoryHandler despacha por método HTTP.
func CategoryHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			getCategories(db, w, r)
		case http.MethodPost:
			createCategory(db, w, r)
		case http.MethodPut:
			updateCategory(db, w, r)
		case http.MethodDelete:
			deleteCategory(db, w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Método HTTP no soportado."})
		}
	}
}

func getCategories(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT id_categoria, nombre, COALESCE(detalle, ''), tasa_iva
		FROM inventario.categorias
		ORDER BY nombre ASC`)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar las categorías."})
		return
	}
	defer rows.Close()

	categorias := []Categoria{}
	for rows.Next() {
		var c Categoria
		rows.Scan(&c.IdCategoria, &c.Nombre, &c.Detalle, &c.TasaIva)
		categorias = append(categorias, c)
	}
	json.NewEncoder(w).Encode(categorias)
}

func createCategory(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	var c Categoria
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido."})
		return
	}
	if c.Nombre == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "El campo 'nombre' es obligatorio."})
		return
	}

	tx, err := db.Begin() // BEGIN
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "No se pudo iniciar la transacción."})
		return
	}
	defer tx.Rollback()

	err = tx.QueryRow(
		`INSERT INTO inventario.categorias (nombre, detalle, tasa_iva) VALUES ($1, $2, $3) RETURNING id_categoria`,
		c.Nombre, c.Detalle, c.TasaIva,
	).Scan(&c.IdCategoria)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al crear la categoría."})
		return
	}

	tx.Commit() // COMMIT
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(c)
}

func updateCategory(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "El parámetro ?id es obligatorio."})
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "El parámetro ?id debe ser un entero."})
		return
	}

	var c Categoria
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
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

	res, err := tx.Exec(
		`UPDATE inventario.categorias SET nombre=$1, detalle=$2, tasa_iva=$3 WHERE id_categoria=$4`,
		c.Nombre, c.Detalle, c.TasaIva, id,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al actualizar la categoría."})
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "Categoría no encontrada."})
		return
	}

	tx.Commit() // COMMIT
	c.IdCategoria = id
	json.NewEncoder(w).Encode(map[string]interface{}{"mensaje": "Categoría actualizada.", "categoria": c})
}

func deleteCategory(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "El parámetro ?id es obligatorio."})
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "El parámetro ?id debe ser un entero."})
		return
	}

	tx, err := db.Begin() // BEGIN
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "No se pudo iniciar la transacción."})
		return
	}
	defer tx.Rollback()

	res, err := tx.Exec(`DELETE FROM inventario.categorias WHERE id_categoria=$1`, id)
	if err != nil {
		// Error 500: probablemente FK constraint (hay productos en esa categoría)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "No se puede eliminar: la categoría tiene productos asociados."})
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "Categoría no encontrada."})
		return
	}

	tx.Commit() // COMMIT
	json.NewEncoder(w).Encode(map[string]string{"mensaje": "Categoría eliminada exitosamente."})
}
