// Backend/handlers/user_handler.go
// ─────────────────────────────────────────────────────────────────────────────
// CRUD de gestión de usuarios (solo accesible con rol admin_libreria).
//   GET    /api/usuarios          → Lista todos los usuarios
//   POST   /api/usuarios          → Crea un nuevo usuario (hash bcrypt vía pgcrypto)
//   PUT    /api/usuarios?id=X     → Actualiza rol y/o estado
//   DELETE /api/usuarios?id=X     → Baja lógica (estado = 'inactivo')
// ─────────────────────────────────────────────────────────────────────────────
package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
)

// Usuario mapea la tabla seguridad.usuarios (sin exponer contrasena_hash).
type Usuario struct {
	IdUsuario     int    `json:"id_usuario"`
	Nombre        string `json:"nombre"`
	Email         string `json:"email"`
	Rol           string `json:"rol"`
	Estado        string `json:"estado"`
	FechaCreacion string `json:"fecha_creacion"`
	UltimaSesion  string `json:"ultima_sesion,omitempty"`
}

// UserHandler despacha por método HTTP.
func UserHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			getUsers(db, w, r)
		case http.MethodPost:
			createUser(db, w, r)
		case http.MethodPut:
			updateUser(db, w, r)
		case http.MethodDelete:
			deactivateUser(db, w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Método HTTP no soportado."})
		}
	}
}

func getUsers(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT id_usuario, nombre, email, rol, estado,
		       TO_CHAR(fecha_creacion, 'YYYY-MM-DD HH24:MI:SS'),
		       COALESCE(TO_CHAR(ultima_sesion, 'YYYY-MM-DD HH24:MI:SS'), '')
		FROM seguridad.usuarios
		ORDER BY nombre ASC`)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar usuarios."})
		return
	}
	defer rows.Close()

	usuarios := []Usuario{}
	for rows.Next() {
		var u Usuario
		rows.Scan(&u.IdUsuario, &u.Nombre, &u.Email, &u.Rol, &u.Estado,
			&u.FechaCreacion, &u.UltimaSesion)
		usuarios = append(usuarios, u)
	}
	json.NewEncoder(w).Encode(usuarios)
}

func createUser(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	var body struct {
		Nombre   string `json:"nombre"`
		Email    string `json:"email"`
		Password string `json:"password"`
		Rol      string `json:"rol"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido."})
		return
	}

	if body.Nombre == "" || body.Email == "" || body.Password == "" || body.Rol == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "'nombre', 'email', 'password' y 'rol' son obligatorios."})
		return
	}
	if body.Rol != "admin_libreria" && body.Rol != "operador_caja" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Rol no válido. Use 'admin_libreria' u 'operador_caja'."})
		return
	}

	tx, err := db.Begin() // BEGIN
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "No se pudo iniciar la transacción."})
		return
	}
	defer tx.Rollback()

	// Usamos crypt() de pgcrypto para generar el hash bcrypt directamente en PostgreSQL
	var idUsuario int
	err = tx.QueryRow(`
		INSERT INTO seguridad.usuarios (nombre, email, contrasena_hash, rol)
		VALUES ($1, $2, crypt($3, gen_salt('bf', 10)), $4)
		RETURNING id_usuario`,
		body.Nombre, body.Email, body.Password, body.Rol,
	).Scan(&idUsuario)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al crear usuario. El email puede estar ya registrado."})
		return
	}

	tx.Commit() // COMMIT
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"mensaje":    "Usuario creado exitosamente.",
		"id_usuario": idUsuario,
	})
}

func updateUser(db *sql.DB, w http.ResponseWriter, r *http.Request) {
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

	var body struct {
		Rol    string `json:"rol"`
		Estado string `json:"estado"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido."})
		return
	}
	if body.Rol == "" && body.Estado == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Debe proporcionar al menos 'rol' o 'estado' para actualizar."})
		return
	}

	tx, err := db.Begin() // BEGIN
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "No se pudo iniciar la transacción."})
		return
	}
	defer tx.Rollback()

	var res sql.Result
	switch {
	case body.Rol != "" && body.Estado != "":
		res, err = tx.Exec(`UPDATE seguridad.usuarios SET rol=$1, estado=$2 WHERE id_usuario=$3`, body.Rol, body.Estado, id)
	case body.Rol != "":
		res, err = tx.Exec(`UPDATE seguridad.usuarios SET rol=$1 WHERE id_usuario=$2`, body.Rol, id)
	default:
		res, err = tx.Exec(`UPDATE seguridad.usuarios SET estado=$1 WHERE id_usuario=$2`, body.Estado, id)
	}

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al actualizar el usuario."})
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "Usuario no encontrado."})
		return
	}

	tx.Commit() // COMMIT
	json.NewEncoder(w).Encode(map[string]string{"mensaje": "Usuario actualizado exitosamente."})
}

func deactivateUser(db *sql.DB, w http.ResponseWriter, r *http.Request) {
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

	res, err := tx.Exec(
		`UPDATE seguridad.usuarios SET estado = 'inactivo' WHERE id_usuario = $1`, id,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al desactivar el usuario."})
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "Usuario no encontrado."})
		return
	}

	tx.Commit() // COMMIT
	json.NewEncoder(w).Encode(map[string]string{"mensaje": "Usuario desactivado exitosamente (estado: inactivo)."})
}
