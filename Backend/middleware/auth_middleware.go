package middleware

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// Clave (key) personalizada para guardar datos en el contexto de la petición
type contextKey string

const UserContextKey = contextKey("userClaims")

// RequireAuth es el middleware que protege las rutas
func RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// 1. Obtener el token del encabezado (Header) Authorization
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, `{"error": "Falta el token de autorización"}`, http.StatusUnauthorized)
			return
		}

		// El formato debe ser "Bearer <token>"
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			http.Error(w, `{"error": "Formato de token inválido. Use 'Bearer <token>'"}`, http.StatusUnauthorized)
			return
		}

		// 2. Obtener el secreto JWT del .env
		jwtSecret := os.Getenv("JWT_SECRET")
		if jwtSecret == "" {
			http.Error(w, `{"error": "Error interno: JWT_SECRET no configurado"}`, http.StatusInternalServerError)
			return
		}

		// 3. Validar el Token
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			// Validar el algoritmo de firma
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("método de firma inesperado: %v", token.Header["alg"])
			}
			return []byte(jwtSecret), nil
		})

		if err != nil || !token.Valid {
			http.Error(w, `{"error": "Token inválido o expirado"}`, http.StatusUnauthorized)
			return
		}

		// 4. Extraer los datos del token (Claims) y pasarlos al siguiente Handler
		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			// Guardamos los claims en el contexto de la petición (request)
			ctx := context.WithValue(r.Context(), UserContextKey, claims)

			// Pasamos al siguiente controlador (ej: Inventario o Ventas) con el nuevo contexto
			next.ServeHTTP(w, r.WithContext(ctx))
		} else {
			http.Error(w, `{"error": "Error al procesar los datos del token"}`, http.StatusUnauthorized)
		}
	}
	
}

