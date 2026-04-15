package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ==========================================
// ESTRUCTURAS DE DATOS
// ==========================================

// Estructuras para Login
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Mensaje string `json:"mensaje"`
	Token   string `json:"token"`
	Usuario struct {
		Nombre string `json:"nombre"`
		Rol    string `json:"rol"`
	} `json:"usuario"`
}

// Estructura para Registro
type RegisterRequest struct {
	Nombre   string `json:"nombre"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// ==========================================
// CONTROLADOR DE LOGIN
// ==========================================

func LoginHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// 1. Obtener la clave secreta del .env
		jwtSecret := os.Getenv("JWT_SECRET")
		if jwtSecret == "" {
			http.Error(w, `{"error": "Configuración de servidor incompleta (JWT)"}`, http.StatusInternalServerError)
			log.Println("ERROR CRÍTICO: JWT_SECRET no está definido en el entorno")
			return
		}
		var jwtKey = []byte(jwtSecret)

		// 2. Decodificar el JSON del cliente
		var req LoginRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			http.Error(w, `{"error": "Cuerpo de solicitud inválido"}`, http.StatusBadRequest)
			return
		}

		// 3. Consultar a la BD usando pgcrypto
		query := `
			SELECT id_usuario, nombre, rol, estado 
			FROM seguridad.usuarios 
			WHERE email = $1 AND contrasena_hash = crypt($2, contrasena_hash)
		`

		var idUsuario int
		var nombre, rol, estado string

		err = db.QueryRow(query, req.Email, req.Password).Scan(&idUsuario, &nombre, &rol, &estado)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, `{"error": "Credenciales inválidas"}`, http.StatusUnauthorized)
				return
			}
			http.Error(w, `{"error": "Error interno del servidor"}`, http.StatusInternalServerError)
			return
		}

		// 4. Validar si el usuario está activo
		if estado != "activo" {
			http.Error(w, `{"error": "Usuario inactivo. Contacte al administrador."}`, http.StatusForbidden)
			return
		}

		// 5. Generar el Token JWT
		claims := jwt.MapClaims{
			"id":  idUsuario,
			"rol": rol,
			"exp": time.Now().Add(time.Hour * 8).Unix(), // 8 horas de expiración
		}

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenString, err := token.SignedString(jwtKey)
		if err != nil {
			http.Error(w, `{"error": "Error al generar el token"}`, http.StatusInternalServerError)
			return
		}

		// (Opcional) Actualizar la fecha de última sesión
		_, _ = db.Exec(`UPDATE seguridad.usuarios SET ultima_sesion = CURRENT_TIMESTAMP WHERE id_usuario = $1`, idUsuario)

		// 6. Enviar respuesta exitosa
		response := LoginResponse{
			Mensaje: "Login exitoso",
			Token:   tokenString,
		}
		response.Usuario.Nombre = nombre
		response.Usuario.Rol = rol

		json.NewEncoder(w).Encode(response)
	}
}

// ==========================================
// CONTROLADOR DE REGISTRO
// ==========================================

func RegisterHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// 1. Decodificar lo que nos envía Angular
		var req RegisterRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			http.Error(w, `{"error": "Datos inválidos"}`, http.StatusBadRequest)
			return
		}

		// 2. Validar que no vengan campos vacíos
		if req.Nombre == "" || req.Email == "" || req.Password == "" {
			http.Error(w, `{"error": "Todos los campos son obligatorios"}`, http.StatusBadRequest)
			return
		}

		// 3. Insertar en la Base de Datos usando pgcrypto
		// Nota: El rol por defecto será 'cliente' y el estado 'activo'
		query := `
			INSERT INTO seguridad.usuarios (nombre, email, contrasena_hash, rol, estado)
			VALUES ($1, $2, crypt($3, gen_salt('bf')), 'cliente', 'activo')
			RETURNING id_usuario
		`

		var idNuevoUsuario int
		err = db.QueryRow(query, req.Nombre, req.Email, req.Password).Scan(&idNuevoUsuario)

		if err != nil {
			// Si el correo ya existe, la base de datos lanzará un error por la restricción UNIQUE
			http.Error(w, `{"error": "No se pudo crear la cuenta, tal vez el correo ya está en uso"}`, http.StatusInternalServerError)
			return
		}

		// 4. Responder con éxito (Código HTTP 201 Created)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"mensaje": "¡Cuenta creada exitosamente!",
		})
	}
}
