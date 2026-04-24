// Backend/handlers/auth_handler.go
// ─────────────────────────────────────────────────────────────────────────────
// HT-04: Gestión de Autenticación y Criptografía
//
// Endpoints de autenticación:
//   POST /api/auth/login             → Valida credenciales, emite JWT (CA 51, 52)
//   POST /api/auth/logout            → Invalida la sesión activa en BD
//   GET  /api/auth/perfil            → Devuelve el perfil del usuario autenticado
//   PUT  /api/auth/cambiar-password  → Cambia la contraseña con verificación BCrypt (CA 51)
//
// Seguridad implementada:
//   CA 51 → Las contraseñas NUNCA se almacenan en texto plano.
//            Login: se compara con bcrypt.CompareHashAndPassword()
//            Registro/cambio: se hashea con pgcrypto crypt($1, gen_salt('bf', 10))
//   CA 52 → El JWT se emite con exp = now() + 8h (jornada laboral)
//   CA 53 → Todas las rutas protegidas exigen Authorization: Bearer <token> (via middleware)
//   CA 54 → HTTP 401 en token ausente, inválido, expirado o con firma incorrecta (via middleware)
// ─────────────────────────────────────────────────────────────────────────────
package handlers

import (
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"libreria-altares/middleware"
)

// ─── Tipos de Solicitud y Respuesta ─────────────────────────────────────────

// LoginRequest es el body esperado en POST /api/auth/login.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResponse es la respuesta JSON exitosa del login (CA 52: incluye token y expiración).
type LoginResponse struct {
	Token     string `json:"token"`
	Rol       string `json:"rol"`
	Nombre    string `json:"nombre"`
	IdUsuario int    `json:"id_usuario"`
	ExpiresAt string `json:"expires_at"` // ISO 8601 — útil para que Angular calcule el contador de sesión
}

// PerfilResponse devuelve los datos públicos del usuario autenticado.
type PerfilResponse struct {
	IdUsuario    int    `json:"id_usuario"`
	Nombre       string `json:"nombre"`
	Email        string `json:"email"`
	Rol          string `json:"rol"`
	UltimaSesion string `json:"ultima_sesion,omitempty"`
}

// ─── POST /api/auth/login ───────────────────────────────────────────────────
// LoginHandler autentica al usuario y emite un JWT firmado con HS256.
//
// Flujo:
//  1. Decodifica y valida el body JSON
//  2. Busca el usuario activo por email
//  3. CA 51: Compara la contraseña con el hash bcrypt de la BD
//  4. CA 52: Genera JWT firmado con HS256, válido exactamente 8 horas
//  5. Persiste la sesión en seguridad.sesiones (trazabilidad)
//  6. Registra el evento en seguridad.logs_auditoria (auditoría)
func LoginHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Decodificar body
		var req LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Cuerpo de petición inválido. Se esperan 'email' y 'password'."})
			return
		}
		if req.Email == "" || req.Password == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Los campos 'email' y 'password' son obligatorios."})
			return
		}

		// Buscar usuario activo por email
		var (
			idUsuario      int
			nombre         string
			email          string
			contrasenaHash string
			rol            string
		)
		err := db.QueryRow(
			`SELECT id_usuario, nombre, email, contrasena_hash, rol
			 FROM seguridad.usuarios
			 WHERE email = $1 AND estado = 'activo'`,
			req.Email,
		).Scan(&idUsuario, &nombre, &email, &contrasenaHash, &rol)

		// Usar el mismo mensaje para "usuario no existe" y "contraseña incorrecta"
		// para evitar enumeración de usuarios (timing attack mitigation)
		if err == sql.ErrNoRows {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "Credenciales incorrectas."})
			return
		}
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error interno al verificar las credenciales."})
			return
		}

		// CA 51: Comparar contraseña con hash BCrypt almacenado por pgcrypto.
		// pgcrypto con gen_salt('bf', 10) genera hashes $2a$ estándar, compatibles
		// con la librería golang.org/x/crypto/bcrypt de Go.
		if err := bcrypt.CompareHashAndPassword([]byte(contrasenaHash), []byte(req.Password)); err != nil {
			// Mismo mensaje que "usuario no existe" → evita revelar si el email está registrado
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "Credenciales incorrectas."})
			return
		}

		// CA 52: Construir claims del JWT con expiración exacta de 8 horas
		expiresAt := time.Now().Add(8 * time.Hour)
		claims := &middleware.Claims{
			IdUsuario: idUsuario,
			Nombre:    nombre,
			Email:     email,
			Rol:       rol,
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(expiresAt),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				Subject:   email,
				Issuer:    "libreria-los-altares-api",
			},
		}

		// Firmar el JWT con HMAC-SHA256 (HS256) usando la clave secreta del servidor
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenStr, err := token.SignedString([]byte(os.Getenv("JWT_SECRET")))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al generar el token de sesión."})
			return
		}

		ipOrigen := getIP(r)

		// Persistir la sesión en seguridad.sesiones (no crítico: si falla, el login igual responde)
		db.Exec(`
			INSERT INTO seguridad.sesiones (id_usuario, token_jwt, fecha_expiracion, ip_origen, activa)
			VALUES ($1, $2, $3, $4, TRUE)`,
			idUsuario, tokenStr, expiresAt, ipOrigen,
		)

		// Actualizar última fecha de sesión y registrar en auditoría
		db.Exec(`UPDATE seguridad.usuarios SET ultima_sesion = NOW() WHERE id_usuario = $1`, idUsuario)
		db.Exec(`
			INSERT INTO seguridad.logs_auditoria (id_usuario, accion, tabla_afectada, ip_origen)
			VALUES ($1, 'LOGIN', 'sesiones', $2)`,
			idUsuario, ipOrigen,
		)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(LoginResponse{
			Token:     tokenStr,
			Rol:       rol,
			Nombre:    nombre,
			IdUsuario: idUsuario,
			ExpiresAt: expiresAt.Format(time.RFC3339),
		})
	}
}

// ─── POST /api/auth/logout ───────────────────────────────────────────────────
// LogoutHandler invalida la sesión activa del usuario en seguridad.sesiones.
//
// CA 54: Al cerrar sesión, el mismo token ya no será encontrado en sesiones activas.
// El JWT seguirá siendo técnicamente válido hasta su exp, pero marcarlo como inactivo
// permite que implementaciones futuras lo verifiquen en BD para una revocación estricta.
//
// Este endpoint requiere RequireAuth (el token debe ser válido para poder invalidarlo).
func LogoutHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Solo se acepta POST en este endpoint."})
			return
		}

		// Extraer el token crudo del encabezado (RequireAuth ya lo validó)
		tokenStr := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")

		// Marcar la sesión como inactiva en la BD
		result, err := db.Exec(
			`UPDATE seguridad.sesiones SET activa = FALSE WHERE token_jwt = $1 AND activa = TRUE`,
			tokenStr,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al cerrar la sesión en el servidor."})
			return
		}

		rowsAffected, _ := result.RowsAffected()

		// Registrar el evento de logout en auditoría
		claims, ok := middleware.GetClaims(r)
		if ok {
			db.Exec(`
				INSERT INTO seguridad.logs_auditoria (id_usuario, accion, tabla_afectada, ip_origen)
				VALUES ($1, 'LOGOUT', 'sesiones', $2)`,
				claims.IdUsuario, getIP(r),
			)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"mensaje":          "Sesión cerrada exitosamente. El token ha sido invalidado.",
			"sesiones_cerradas": rowsAffected,
		})
	}
}

// ─── GET /api/auth/perfil ────────────────────────────────────────────────────
// PerfilHandler devuelve los datos del usuario actualmente autenticado,
// leyendo el id_usuario desde los claims del JWT sin consultar la BD.
// (Lectura ultra-rápida para el dashboard de Angular — CA 46)
func PerfilHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Solo se acepta GET en este endpoint."})
			return
		}

		claims, ok := middleware.GetClaims(r)
		if !ok {
			// No debería ocurrir si RequireAuth está aplicado, pero por robustez:
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "No se pudieron leer los datos de la sesión."})
			return
		}

		// Consulta liviana para obtener UltimaSesion actualizada desde la BD
		var ultimaSesion string
		db.QueryRow(
			`SELECT COALESCE(TO_CHAR(ultima_sesion, 'YYYY-MM-DD HH24:MI:SS'), '')
			 FROM seguridad.usuarios WHERE id_usuario = $1`,
			claims.IdUsuario,
		).Scan(&ultimaSesion)

		json.NewEncoder(w).Encode(PerfilResponse{
			IdUsuario:    claims.IdUsuario,
			Nombre:       claims.Nombre,
			Email:        claims.Email,
			Rol:          claims.Rol,
			UltimaSesion: ultimaSesion,
		})
	}
}

// ─── PUT /api/auth/cambiar-password ─────────────────────────────────────────
// CambiarPasswordHandler permite a un usuario autenticado cambiar su contraseña.
//
// CA 51: Flujo estricto de BCrypt:
//  1. Obtiene el hash actual de la BD
//  2. bcrypt.CompareHashAndPassword() verifica la contraseña actual
//  3. Si coincide, actualiza usando crypt($1, gen_salt('bf', 10)) de pgcrypto
//
// El nuevo hash se genera en PostgreSQL para garantizar que el cost factor es
// siempre el correcto (bf, 10) sin depender del lado de Go.
func CambiarPasswordHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Solo se acepta PUT en este endpoint."})
			return
		}

		claims, ok := middleware.GetClaims(r)
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "Token no válido."})
			return
		}

		// Decodificar el body
		var body struct {
			PasswordActual string `json:"password_actual"`
			PasswordNuevo  string `json:"password_nuevo"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido."})
			return
		}

		// Validaciones → HTTP 400
		if body.PasswordActual == "" || body.PasswordNuevo == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "'password_actual' y 'password_nuevo' son obligatorios."})
			return
		}
		if len(body.PasswordNuevo) < 8 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "La nueva contraseña debe tener al menos 8 caracteres."})
			return
		}
		if body.PasswordActual == body.PasswordNuevo {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "La nueva contraseña no puede ser igual a la actual."})
			return
		}

		// CA 51: Obtener el hash actual del usuario autenticado
		var hashActual string
		err := db.QueryRow(
			`SELECT contrasena_hash FROM seguridad.usuarios WHERE id_usuario = $1 AND estado = 'activo'`,
			claims.IdUsuario,
		).Scan(&hashActual)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al obtener el perfil del usuario."})
			return
		}

		// CA 51: Verificar la contraseña actual con BCrypt antes de permitir el cambio
		if err := bcrypt.CompareHashAndPassword([]byte(hashActual), []byte(body.PasswordActual)); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "La contraseña actual es incorrecta."})
			return
		}

		// CA 51: Actualizar hash usando pgcrypto directamente en PostgreSQL
		// Esto garantiza que el work factor (bf, 10) sea siempre aplicado por la BD.
		_, err = db.Exec(
			`UPDATE seguridad.usuarios
			 SET contrasena_hash = crypt($1, gen_salt('bf', 10))
			 WHERE id_usuario = $2`,
			body.PasswordNuevo, claims.IdUsuario,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al actualizar la contraseña."})
			return
		}

		// Registrar cambio en auditoría (sin guardar el hash, solo la acción)
		db.Exec(`
			INSERT INTO seguridad.logs_auditoria (id_usuario, accion, tabla_afectada, ip_origen)
			VALUES ($1, 'CAMBIO_CONTRASENA', 'usuarios', $2)`,
			claims.IdUsuario, getIP(r),
		)

		json.NewEncoder(w).Encode(map[string]string{
			"mensaje": "Contraseña actualizada exitosamente. Inicie sesión nuevamente con la nueva contraseña.",
		})
	}
}

// ─── Helper ─────────────────────────────────────────────────────────────────

// getIP extrae la dirección IP real del cliente, considerando proxies inversos.
// Prioriza X-Forwarded-For (Nginx/Apache) > X-Real-IP > RemoteAddr directo.
func getIP(r *http.Request) string {
	// Proxy inverso (ej. Nginx que hace forward al backend Go)
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		// X-Forwarded-For puede ser "client, proxy1, proxy2" → tomar el primero
		return strings.TrimSpace(strings.Split(fwd, ",")[0])
	}
	// Proxy simple
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return realIP
	}
	// Conexión directa: RemoteAddr viene como "host:port"
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr // Fallback por si no tiene puerto
	}
	return ip
}
