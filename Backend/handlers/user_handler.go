// Backend/handlers/user_handler.go
// ─────────────────────────────────────────────────────────────────────────────
// CRUD de gestión de usuarios (solo accesible con rol admin_libreria).
//   GET    /api/usuarios          → Lista todos los usuarios
//   POST   /api/usuarios          → Crea un nuevo usuario (hash bcrypt vía pgcrypto)
//   PUT    /api/usuarios?id=X     → Actualiza rol, estado y tienda
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
	IdTienda      *int   `json:"id_tienda,omitempty"` // NULL para admin global
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
		SELECT id_usuario, nombre, email, rol, estado, id_tienda,
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
		var idTienda sql.NullInt64
		rows.Scan(&u.IdUsuario, &u.Nombre, &u.Email, &u.Rol, &u.Estado, &idTienda,
			&u.FechaCreacion, &u.UltimaSesion)
		if idTienda.Valid {
			t := int(idTienda.Int64)
			u.IdTienda = &t
		}
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
		IdTienda *int   `json:"id_tienda"`
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

	tx, err := db.Begin()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "No se pudo iniciar la transacción."})
		return
	}
	defer tx.Rollback()

	var idTiendaVal interface{} = body.IdTienda
	if body.IdTienda != nil && *body.IdTienda == 0 {
		idTiendaVal = nil
	}

	var idUsuario int
	err = tx.QueryRow(`
		INSERT INTO seguridad.usuarios (nombre, email, contrasena_hash, rol, id_tienda)
		VALUES ($1, $2, crypt($3, gen_salt('bf', 10)), $4, $5)
		RETURNING id_usuario`,
		body.Nombre, body.Email, body.Password, body.Rol, idTiendaVal,
	).Scan(&idUsuario)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al crear usuario. El email puede estar ya registrado."})
		return
	}

	tx.Commit()
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
		Rol      *string `json:"rol"`
		Estado   *string `json:"estado"`
		IdTienda *int    `json:"id_tienda"` // Puede ser 0 para quitar la tienda y hacer global (si el cliente lo maneja así)
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido."})
		return
	}

	tx, err := db.Begin()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "No se pudo iniciar la transacción."})
		return
	}
	defer tx.Rollback()

	// Como las actualizaciones pueden ser parciales, construimos la query dinámicamente o actualizamos los campos dados.
	// Por simplicidad en la base de datos original solo se actualizaba rol y estado, ahora añadimos id_tienda.
	
	if body.Rol != nil {
		_, err = tx.Exec(`UPDATE seguridad.usuarios SET rol=$1 WHERE id_usuario=$2`, *body.Rol, id)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al actualizar el rol."})
			return
		}
	}
	
	if body.Estado != nil {
		_, err = tx.Exec(`UPDATE seguridad.usuarios SET estado=$1 WHERE id_usuario=$2`, *body.Estado, id)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al actualizar el estado."})
			return
		}
	}

	if body.IdTienda != nil {
		var val interface{} = *body.IdTienda
		if *body.IdTienda == 0 {
			val = nil
		}
		_, err = tx.Exec(`UPDATE seguridad.usuarios SET id_tienda=$1 WHERE id_usuario=$2`, val, id)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al actualizar la tienda asignada."})
			return
		}
	}

	tx.Commit()
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

	tx, err := db.Begin()
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

	tx.Commit()
	json.NewEncoder(w).Encode(map[string]string{"mensaje": "Usuario desactivado exitosamente (estado: inactivo)."})
}
