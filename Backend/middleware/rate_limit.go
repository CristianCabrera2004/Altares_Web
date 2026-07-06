package middleware

import (
	"log"
	"net/http"
	"sync"

	"golang.org/x/time/rate"
)

// client almacena el limitador de tasa de cada IP
type client struct {
	limiter *rate.Limiter
}

var (
	// mu protege el acceso a clients
	mu sync.Mutex
	// clients es un mapa de direcciones IP a su respectivo limitador
	clients = make(map[string]*client)
)

// getClient devuelve o crea un rate limiter para la IP proporcionada.
// Configuramos un límite de 5 peticiones por minuto con ráfagas máximas de 5.
func getClient(ip string) *rate.Limiter {
	mu.Lock()
	defer mu.Unlock()

	c, exists := clients[ip]
	if !exists {
		// rate.Limit de 5/60.0 por segundo = 1 cada 12 segundos (5 por minuto), bucket de 5
		limiter := rate.NewLimiter(rate.Limit(5.0/60.0), 5)
		clients[ip] = &client{limiter: limiter}
		return limiter
	}

	return c.limiter
}

// RateLimit es el middleware que envuelve un handler y rechaza peticiones
// si superan la tasa de 5 peticiones por minuto.
func RateLimit(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// En producción real, puedes necesitar leer X-Forwarded-For si usas Nginx
		ip := r.RemoteAddr
		if forwardedFor := r.Header.Get("X-Forwarded-For"); forwardedFor != "" {
			ip = forwardedFor
		}

		limiter := getClient(ip)
		if !limiter.Allow() {
			log.Printf("⚠️ RATE LIMIT: Bloqueando acceso a %s (Demasiados intentos de login)", ip)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error": "Demasiados intentos. Por favor, intenta de nuevo en 1 minuto."}`))
			return
		}

		next.ServeHTTP(w, r)
	}
}
