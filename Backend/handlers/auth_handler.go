// Backend/handlers/auth_handler.go
// ─────────────────────────────────────────────────────────────────────────────
// HT-04: Gestión de Autenticación y Criptografía
//
// Endpoints de autenticación:
//   POST /api/auth/login             → Valida credenciales, emite JWT (CA 51, 52)
//   POST /api/auth/logout            → Invalida la sesión activa en BD
//   GET  /api/auth/perfil            → Devuelve el perfil del usuario autenticado
//   PUT  /api/auth/cambiar-password  → Cambia la contraseña con verificación BCrypt (CA 51)
//   POST /api/auth/reenviar-codigo   → Reenvía código de verificación de email
//   GET  /api/auth/2fa/setup         → Genera secreto TOTP temporal para el usuario
//   POST /api/auth/2fa/enable        → Verifica código y activa 2FA para el usuario
//   POST /api/auth/2fa/disable       → Desactiva 2FA (requiere código TOTP + contraseña)
//
// Seguridad implementada:
//   CA 51 → Las contraseñas NUNCA se almacenan en texto plano.
//   CA 52 → El JWT se emite con exp = now() + 8h (jornada laboral)
//   AC-10 → Concurrencia de sesión única: revoca sesiones anteriores al iniciar una nueva.
//   IA-2(1) → MFA obligatorio para accesos administrativos (cuando 2FA está habilitado).
// ─────────────────────────────────────────────────────────────────────────────
package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"libreria-altares/middleware"
	"libreria-altares/utils"
)

// ─── Tipos de Solicitud y Respuesta ─────────────────────────────────────────

// LoginRequest es el body esperado en POST /api/auth/login.
type LoginRequest struct {
	Email            string `json:"email"`
	Password         string `json:"password"`
	TwoFactorCode    string `json:"two_factor_code,omitempty"`
	VerificationCode string `json:"verification_code,omitempty"`
}

// LoginResponse es la respuesta JSON exitosa del login.
type LoginResponse struct {
	Token        string `json:"token,omitempty"`
	Rol          string `json:"rol,omitempty"`
	Nombre       string `json:"nombre,omitempty"`
	IdUsuario    int    `json:"id_usuario,omitempty"`
	IdTienda     int    `json:"id_tienda,omitempty"`
	NombreTienda string `json:"nombre_tienda,omitempty"`
	ExpiresAt    string `json:"expires_at,omitempty"`
	// Para el flujo de 2FA interactivo
	TwoFactorRequired bool `json:"two_factor_required,omitempty"`
	// Para el flujo de verificación de email en primer login
	EmailVerificationRequired bool   `json:"email_verification_required,omitempty"`
	EmailHint                 string `json:"email_hint,omitempty"`
}

// PerfilResponse devuelve los datos públicos del usuario autenticado.
type PerfilResponse struct {
	IdUsuario        int    `json:"id_usuario"`
	Nombre           string `json:"nombre"`
	Email            string `json:"email"`
	Rol              string `json:"rol"`
	UltimaSesion     string `json:"ultima_sesion,omitempty"`
	TwoFactorEnabled bool   `json:"two_factor_enabled"`
}

// ─── POST /api/auth/login ───────────────────────────────────────────────────
// LoginHandler autentica al usuario, verifica 2FA si está activo y emite un JWT firmado.
func LoginHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Decodificar body
		var req LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Cuerpo de petición inválido."})
			return
		}
		if req.Email == "" || req.Password == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Los campos 'email' y 'password' son obligatorios."})
			return
		}

		// Buscar usuario activo por email
		var (
			idUsuario        int
			nombre           string
			email            string
			contrasenaHash   string
			rol              string
			idTiendaNull     sql.NullInt64
			twoFactorEnabled bool
			twoFactorSecret  sql.NullString
			emailVerificado  bool
		)
		err := db.QueryRow(
			`SELECT id_usuario, nombre, email, contrasena_hash, rol, id_tienda,
			        two_factor_enabled, two_factor_secret, email_verificado
			 FROM seguridad.usuarios
			 WHERE email = $1 AND estado = 'activo'`,
			req.Email,
		).Scan(&idUsuario, &nombre, &email, &contrasenaHash, &rol, &idTiendaNull,
			&twoFactorEnabled, &twoFactorSecret, &emailVerificado)

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

		// CA 51: Comparar contraseña con hash BCrypt
		if err := bcrypt.CompareHashAndPassword([]byte(contrasenaHash), []byte(req.Password)); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "Credenciales incorrectas."})
			return
		}

		// ── Verificación de Email en Primer Login ──────────────────────────────
		if !emailVerificado {
			if req.VerificationCode == "" {
				// Generar y enviar código de verificación por email
				// Verificar si ya hay un código vigente
				var codigoExistente sql.NullString
				var expiraCodigo sql.NullTime
				db.QueryRow(
					`SELECT codigo_verificacion, codigo_verificacion_expira FROM seguridad.usuarios WHERE id_usuario = $1`,
					idUsuario,
				).Scan(&codigoExistente, &expiraCodigo)

				// Si no hay código vigente, generar uno nuevo
				if !codigoExistente.Valid || !expiraCodigo.Valid || time.Now().UTC().After(expiraCodigo.Time) {
					codigo := generarCodigo6Digitos()
					expira := time.Now().UTC().Add(24 * time.Hour)
					db.Exec(
						`UPDATE seguridad.usuarios SET codigo_verificacion = $1, codigo_verificacion_expira = $2 WHERE id_usuario = $3`,
						codigo, expira, idUsuario,
					)
					// Enviar email con el código
					enviarCodigoVerificacion(email, nombre, codigo)
				} else {
					// Reenviar el código existente que aún no expira
					enviarCodigoVerificacion(email, nombre, codigoExistente.String)
				}

				// Generar una pista del email para mostrar en el frontend
				emailHint := generarEmailHint(email)

				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(LoginResponse{
					EmailVerificationRequired: true,
					EmailHint:                 emailHint,
				})
				return
			}

			// Validar el código de verificación enviado
			var codigoGuardado sql.NullString
			var expiraCodigo sql.NullTime
			db.QueryRow(
				`SELECT codigo_verificacion, codigo_verificacion_expira FROM seguridad.usuarios WHERE id_usuario = $1`,
				idUsuario,
			).Scan(&codigoGuardado, &expiraCodigo)

			if !codigoGuardado.Valid || !expiraCodigo.Valid {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": "No se encontró un código de verificación. Intente iniciar sesión nuevamente."})
				return
			}

			if time.Now().UTC().After(expiraCodigo.Time) {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": "El código de verificación ha expirado. Inicie sesión nuevamente para recibir uno nuevo."})
				return
			}

			if req.VerificationCode != codigoGuardado.String {
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "Código de verificación incorrecto."})
				return
			}

			// Código correcto: marcar email como verificado y limpiar código
			db.Exec(
				`UPDATE seguridad.usuarios SET email_verificado = TRUE, codigo_verificacion = NULL, codigo_verificacion_expira = NULL WHERE id_usuario = $1`,
				idUsuario,
			)

			// Registrar en auditoría
			db.Exec(
				`INSERT INTO seguridad.logs_auditoria (id_usuario, accion, tabla_afectada, ip_origen)
				 VALUES ($1, 'EMAIL_VERIFICADO', 'usuarios', $2)`,
				idUsuario, getIP(r),
			)
		}

		// IA-2(1): Verificar Autenticación Multifactor si está activa
		if twoFactorEnabled {
			if req.TwoFactorCode == "" {
				// Retornar indicación de que 2FA es necesario
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(LoginResponse{
					TwoFactorRequired: true,
				})
				return
			}

			// Validar el código TOTP
			if !twoFactorSecret.Valid || !utils.VerifyTOTP(twoFactorSecret.String, req.TwoFactorCode) {
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "Código 2FA incorrecto o expirado."})
				return
			}
		}

		// Resolver la tienda del usuario
		var idTienda int
		var nombreTienda string
		if idTiendaNull.Valid {
			idTienda = int(idTiendaNull.Int64)
			db.QueryRow(`SELECT nombre FROM configuracion.tiendas WHERE id_tienda = $1`, idTienda).Scan(&nombreTienda)
		} else {
			idTienda = 0
			nombreTienda = "Todas las tiendas"
		}

		// CA 52: Construir claims del JWT con expiración de 8 horas
		expiresAt := time.Now().Add(8 * time.Hour)
		claims := &middleware.Claims{
			IdUsuario:    idUsuario,
			Nombre:       nombre,
			Email:        email,
			Rol:          rol,
			IdTienda:     idTienda,
			NombreTienda: nombreTienda,
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(expiresAt),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				Subject:   email,
				Issuer:    "libreria-los-altares-api",
			},
		}

		// Firmar el JWT
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenStr, err := token.SignedString([]byte(os.Getenv("JWT_SECRET")))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al generar el token de sesión."})
			return
		}

		ipOrigen := getIP(r)

		// AC-10: Control de Concurrencia de Sesión Única
		// Invalidar cualquier sesión activa previa del usuario antes de crear la nueva
		res, err := db.Exec(`
			UPDATE seguridad.sesiones
			SET activa = FALSE
			WHERE id_usuario = $1 AND activa = TRUE`,
			idUsuario,
		)
		if err == nil {
			rows, _ := res.RowsAffected()
			if rows > 0 {
				// Registrar en auditoría la revocación por concurrencia
				db.Exec(`
					INSERT INTO seguridad.logs_auditoria (id_usuario, accion, tabla_afectada, ip_origen, valor_anterior, valor_nuevo)
					VALUES ($1, 'SESION_CONCURRENTE_REVOCADA', 'sesiones', $2, 'activa=true', 'activa=false')`,
					idUsuario, ipOrigen,
				)
			}
		}

		// Persistir la sesión en seguridad.sesiones
		db.Exec(`
			INSERT INTO seguridad.sesiones (id_usuario, token_jwt, fecha_expiracion, ip_origen, activa)
			VALUES ($1, $2, $3, $4, TRUE)`,
			idUsuario, tokenStr, expiresAt, ipOrigen,
		)

		// Actualizar última fecha de sesión y registrar en auditoría el login
		db.Exec(`UPDATE seguridad.usuarios SET ultima_sesion = NOW() WHERE id_usuario = $1`, idUsuario)
		db.Exec(`
			INSERT INTO seguridad.logs_auditoria (id_usuario, accion, tabla_afectada, ip_origen)
			VALUES ($1, 'LOGIN', 'sesiones', $2)`,
			idUsuario, ipOrigen,
		)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(LoginResponse{
			Token:        tokenStr,
			Rol:          rol,
			Nombre:       nombre,
			IdUsuario:    idUsuario,
			IdTienda:     idTienda,
			NombreTienda: nombreTienda,
			ExpiresAt:    expiresAt.Format(time.RFC3339),
		})
	}
}

// ─── POST /api/auth/logout ───────────────────────────────────────────────────
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
// leyendo el id_usuario desde los claims del JWT e incluyendo el estado de 2FA.
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
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "No se pudieron leer los datos de la sesión."})
			return
		}

		// Consulta para obtener UltimaSesion y el estado de 2FA
		var (
			ultimaSesion     string
			twoFactorEnabled bool
		)
		err := db.QueryRow(
			`SELECT COALESCE(TO_CHAR(ultima_sesion, 'YYYY-MM-DD HH24:MI:SS'), ''), two_factor_enabled
			 FROM seguridad.usuarios WHERE id_usuario = $1`,
			claims.IdUsuario,
		).Scan(&ultimaSesion, &twoFactorEnabled)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al obtener perfil de la base de datos."})
			return
		}

		json.NewEncoder(w).Encode(PerfilResponse{
			IdUsuario:        claims.IdUsuario,
			Nombre:           claims.Nombre,
			Email:            claims.Email,
			Rol:              claims.Rol,
			UltimaSesion:     ultimaSesion,
			TwoFactorEnabled: twoFactorEnabled,
		})
	}
}

// ─── PUT /api/auth/cambiar-password ─────────────────────────────────────────
// CambiarPasswordHandler permite a un usuario autenticado cambiar su contraseña.
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

		if err := bcrypt.CompareHashAndPassword([]byte(hashActual), []byte(body.PasswordActual)); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "La contraseña actual es incorrecta."})
			return
		}

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

// ─── GET /api/auth/2fa/setup ──────────────────────────────────────────────────
// Setup2FAHandler genera un secreto TOTP y un URI de aprovisionamiento temporal.
func Setup2FAHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Solo se acepta GET en este endpoint."})
			return
		}

		claims, ok := middleware.GetClaims(r)
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "Token no válido."})
			return
		}

		// Generar secreto TOTP
		secret, err := utils.GenerateTOTPSecret()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al generar el secreto 2FA."})
			return
		}

		// Guardar temporalmente en base de datos (con enabled = false)
		_, err = db.Exec(`
			UPDATE seguridad.usuarios
			SET two_factor_secret = $1, two_factor_enabled = FALSE
			WHERE id_usuario = $2`,
			secret, claims.IdUsuario,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al guardar el secreto temporal."})
			return
		}

		uri := utils.GenerateTOTPURI(claims.Email, secret)

		json.NewEncoder(w).Encode(map[string]string{
			"secret": secret,
			"qr_uri": uri,
		})
	}
}

// ─── POST /api/auth/2fa/enable ────────────────────────────────────────────────
// Enable2FAHandler valida el código enviado por el usuario para habilitar 2FA de forma permanente.
func Enable2FAHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Solo se acepta POST."})
			return
		}

		claims, ok := middleware.GetClaims(r)
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "Token no válido."})
			return
		}

		var body struct {
			Code string `json:"code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Cuerpo de petición inválido."})
			return
		}

		if body.Code == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "El código de verificación de 6 dígitos es obligatorio."})
			return
		}

		// Obtener secreto temporal
		var secret sql.NullString
		err := db.QueryRow(
			`SELECT two_factor_secret FROM seguridad.usuarios WHERE id_usuario = $1`,
			claims.IdUsuario,
		).Scan(&secret)
		if err != nil || !secret.Valid || secret.String == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Inicie el proceso de configuración de 2FA primero."})
			return
		}

		// Validar el código TOTP
		if !utils.VerifyTOTP(secret.String, body.Code) {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "El código 2FA ingresado es incorrecto o ha expirado."})
			return
		}

		// Confirmar habilitación
		_, err = db.Exec(`
			UPDATE seguridad.usuarios
			SET two_factor_enabled = TRUE
			WHERE id_usuario = $1`,
			claims.IdUsuario,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al habilitar 2FA en el perfil."})
			return
		}

		// Registrar auditoría
		db.Exec(`
			INSERT INTO seguridad.logs_auditoria (id_usuario, accion, tabla_afectada, ip_origen)
			VALUES ($1, 'HABILITAR_2FA', 'usuarios', $2)`,
			claims.IdUsuario, getIP(r),
		)

		json.NewEncoder(w).Encode(map[string]string{
			"mensaje": "2FA habilitado correctamente en su cuenta.",
		})
	}
}

// ─── POST /api/auth/2fa/disable ───────────────────────────────────────────────
// Disable2FAHandler deshabilita 2FA de la cuenta del usuario.
// Requiere tanto el código TOTP de la app autenticadora como la contraseña de la cuenta.
func Disable2FAHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Solo se acepta POST."})
			return
		}

		claims, ok := middleware.GetClaims(r)
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "Token no válido."})
			return
		}

		var body struct {
			Code     string `json:"code"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Cuerpo de petición inválido."})
			return
		}

		if body.Code == "" || body.Password == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Se requiere el código 2FA y la contraseña para desactivar."})
			return
		}

		// Obtener secreto actual y contraseña hash
		var (
			secret           sql.NullString
			twoFactorEnabled bool
			contrasenaHash   string
		)
		err := db.QueryRow(
			`SELECT two_factor_secret, two_factor_enabled, contrasena_hash FROM seguridad.usuarios WHERE id_usuario = $1`,
			claims.IdUsuario,
		).Scan(&secret, &twoFactorEnabled, &contrasenaHash)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar estado de 2FA."})
			return
		}

		if !twoFactorEnabled {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "2FA ya está deshabilitado en esta cuenta."})
			return
		}

		// Verificar la contraseña
		if err := bcrypt.CompareHashAndPassword([]byte(contrasenaHash), []byte(body.Password)); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "La contraseña es incorrecta."})
			return
		}

		// Validar el código TOTP
		if !secret.Valid || !utils.VerifyTOTP(secret.String, body.Code) {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Código 2FA incorrecto o expirado."})
			return
		}

		// Deshabilitar 2FA
		_, err = db.Exec(`
			UPDATE seguridad.usuarios
			SET two_factor_enabled = FALSE, two_factor_secret = NULL
			WHERE id_usuario = $1`,
			claims.IdUsuario,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al deshabilitar 2FA."})
			return
		}

		// Registrar auditoría
		db.Exec(`
			INSERT INTO seguridad.logs_auditoria (id_usuario, accion, tabla_afectada, ip_origen)
			VALUES ($1, 'DESHABILITAR_2FA', 'usuarios', $2)`,
			claims.IdUsuario, getIP(r),
		)

		json.NewEncoder(w).Encode(map[string]string{
			"mensaje": "2FA deshabilitado correctamente.",
		})
	}
}

// ─── POST /api/auth/reenviar-codigo ──────────────────────────────────────────
// ReenviarCodigoHandler reenvía el código de verificación de email.
func ReenviarCodigoHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Solo se acepta POST."})
			return
		}

		var body struct {
			Email string `json:"email"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Email == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "El campo 'email' es obligatorio."})
			return
		}

		// Buscar usuario no verificado
		var idUsuario int
		var nombre string
		var emailVerificado bool
		err := db.QueryRow(
			`SELECT id_usuario, nombre, email_verificado FROM seguridad.usuarios WHERE email = $1 AND estado = 'activo'`,
			body.Email,
		).Scan(&idUsuario, &nombre, &emailVerificado)

		if err == sql.ErrNoRows {
			// No revelar si el email existe o no (seguridad)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"mensaje": "Si el correo está registrado, se ha enviado un nuevo código."})
			return
		}

		if emailVerificado {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"mensaje": "Si el correo está registrado, se ha enviado un nuevo código."})
			return
		}

		// Generar nuevo código
		codigo := generarCodigo6Digitos()
		expira := time.Now().UTC().Add(24 * time.Hour)
		db.Exec(
			`UPDATE seguridad.usuarios SET codigo_verificacion = $1, codigo_verificacion_expira = $2 WHERE id_usuario = $3`,
			codigo, expira, idUsuario,
		)

		// Enviar email
		enviarCodigoVerificacion(body.Email, nombre, codigo)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"mensaje": "Se ha enviado un nuevo código de verificación a tu correo electrónico."})
	}
}

// ─── Helpers ────────────────────────────────────────────────────────────────

// generarCodigo6Digitos genera un código numérico aleatorio de 6 dígitos criptográficamente seguro.
func generarCodigo6Digitos() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		// Fallback poco probable
		return "123456"
	}
	return fmt.Sprintf("%06d", n.Int64())
}

// generarEmailHint genera una pista del email ocultando parte del usuario.
// Ej: "admin@losaltares.com" → "ad***@losaltares.com"
func generarEmailHint(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return "***@***"
	}
	user := parts[0]
	if len(user) <= 2 {
		return user[:1] + "***@" + parts[1]
	}
	return user[:2] + "***@" + parts[1]
}

// enviarCodigoVerificacion envía el código de verificación de email al usuario.
func enviarCodigoVerificacion(emailDest, nombre, codigo string) {
	asunto := "Código de verificación — Librería Los Altares"
	body := fmt.Sprintf(`
		<div style="font-family: 'Segoe UI', Arial, sans-serif; max-width: 480px; margin: 0 auto; padding: 30px; background: #f8fafc; border-radius: 12px;">
			<div style="text-align: center; margin-bottom: 24px;">
				<div style="width: 50px; height: 50px; background: linear-gradient(135deg, #4F8EF7, #7B61FF); border-radius: 12px; display: inline-flex; align-items: center; justify-content: center; margin-bottom: 12px;">
					<span style="color: white; font-size: 24px; font-weight: 700;">📚</span>
				</div>
				<h2 style="color: #1e293b; margin: 0; font-size: 1.3rem;">Librería Los Altares</h2>
			</div>
			<p style="color: #334155; font-size: 0.95rem; margin-bottom: 8px;">Hola <strong>%s</strong>,</p>
			<p style="color: #475569; font-size: 0.9rem; line-height: 1.5;">Tu código de verificación para acceder al sistema es:</p>
			<div style="text-align: center; margin: 24px 0;">
				<div style="display: inline-block; padding: 16px 32px; background: white; border: 2px solid #e2e8f0; border-radius: 10px; font-size: 2rem; font-weight: 700; letter-spacing: 0.3em; color: #1e293b;">%s</div>
			</div>
			<p style="color: #64748b; font-size: 0.82rem; text-align: center;">Este código expira en <strong>24 horas</strong>.</p>
			<hr style="border: none; border-top: 1px solid #e2e8f0; margin: 20px 0;" />
			<p style="color: #94a3b8; font-size: 0.75rem; text-align: center;">Si no solicitaste este código, ignora este mensaje.</p>
		</div>
	`, nombre, codigo)

	if err := utils.SendEmail(emailDest, asunto, body, "", ""); err != nil {
		fmt.Printf("⚠️ Error al enviar email de verificación a %s: %v\n", emailDest, err)
	}
}

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
