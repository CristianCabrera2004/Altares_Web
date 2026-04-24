// Backend/middleware/auth_middleware.go
// ─────────────────────────────────────────────────────────────────────────────
// Implementa dos middlewares de seguridad:
//   - RequireAuth:  Valida que la petición traiga un JWT Bearer válido y no expirado.
//   - RequireRole:  Además de autenticar, exige que el claim "rol" sea el esperado.
//
// Ante cualquier fallo emite HTTP 401 (token inválido) o 403 (rol insuficiente),
// nunca expone detalles internos del error criptográfico. (HT-04)
// ─────────────────────────────────────────────────────────────────────────────
package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// contextKey es el tipo privado para las claves de contexto, evita colisiones.
type contextKey string

// ClaimsKey es la clave bajo la que se guarda el *Claims en el contexto de la request.
const ClaimsKey contextKey = "claims"

// Claims define la carga útil del JWT emitido por el backend.
type Claims struct {
	IdUsuario int    `json:"id_usuario"`
	Nombre    string `json:"nombre"`
	Email     string `json:"email"`
	Rol       string `json:"rol"`
	jwt.RegisteredClaims
}

// jsonError escribe un error formateado en JSON con el código HTTP indicado.
// Garantiza que toda respuesta de error sea application/json (CA 44).
func jsonError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// RequireAuth valida el token JWT del encabezado Authorization: Bearer <token>.
// Si el token es válido, inyecta los claims en el contexto y llama al siguiente handler.
func RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")

		// Verificar presencia y formato del encabezado
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			jsonError(w, http.StatusUnauthorized, "Token de autorización no proporcionado o malformado. Use: Authorization: Bearer <token>")
			return
		}

		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		secret := []byte(os.Getenv("JWT_SECRET"))

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
			// Rechazar si el algoritmo no es HMAC (previene ataques de "none" algorithm)
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return secret, nil
		})

		if err != nil || !token.Valid {
			jsonError(w, http.StatusUnauthorized, "Token inválido o expirado. Inicie sesión nuevamente.")
			return
		}

		// Inyectar claims en el contexto para que los handlers puedan leerlos
		ctx := context.WithValue(r.Context(), ClaimsKey, claims)
		next(w, r.WithContext(ctx))
	}
}

// RequireRole combina autenticación y autorización por rol.
// Emite HTTP 403 si el usuario autenticado no tiene el rol requerido.
func RequireRole(role string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return RequireAuth(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := r.Context().Value(ClaimsKey).(*Claims)
			if !ok || claims.Rol != role {
				jsonError(w, http.StatusForbidden,
					"Acceso denegado: su rol no tiene permisos para esta operación.")
				return
			}
			next(w, r)
		})
	}
}

// GetClaims es un helper para que los handlers extraigan los claims del contexto.
func GetClaims(r *http.Request) (*Claims, bool) {
	claims, ok := r.Context().Value(ClaimsKey).(*Claims)
	return claims, ok
}
