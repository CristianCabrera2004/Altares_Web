// Backend/main.go
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"libreria-backend/database"
	"libreria-backend/handlers"
	"libreria-backend/middleware"

	"github.com/joho/godotenv"
)

func main() {
	// 1. Cargar variables de entorno
	err := godotenv.Load()
	if err != nil {
		log.Println("Advertencia: No se encontró archivo .env, usando variables del sistema")
	}

	// 2. Conectar a PostgreSQL
	db := database.Connect()
	defer db.Close()

	// =========================================================
	// 3. RUTAS PÚBLICAS (No requieren Token)
	// =========================================================

	// Ruta para iniciar sesión
	http.HandleFunc("/api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error": "Método no permitido. Use POST."}`, http.StatusMethodNotAllowed)
			return
		}
		handlers.LoginHandler(db)(w, r)
	})

	// 🆕 NUEVA: Ruta para crear cuentas
	http.HandleFunc("/api/auth/register", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error": "Método no permitido. Use POST."}`, http.StatusMethodNotAllowed)
			return
		}
		handlers.RegisterHandler(db)(w, r)
	})

	// =========================================================
	// 4. RUTAS PROTEGIDAS (Requieren Token JWT)
	// =========================================================

	http.HandleFunc("/api/inventario", middleware.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		claims := r.Context().Value(middleware.UserContextKey)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"mensaje": "¡Acceso concedido a la bóveda de Inventario!", "tus_datos": %v}`, claims)
	}))

	// =========================================================
	// 5. CONFIGURACIÓN DEL SERVIDOR Y CORS
	// =========================================================

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("\n🚀 Servidor de Los Altares corriendo en http://localhost:%s\n", port)
	fmt.Println("Rutas disponibles:")
	fmt.Println(" -> POST /api/auth/login    (Pública)")
	fmt.Println(" -> POST /api/auth/register (Pública)")
	fmt.Println(" -> GET  /api/inventario    (Protegida con JWT)")

	// Middleware global de CORS y Debugging
	corsMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			// Campanita para ver las peticiones en la terminal
			log.Printf("🔔 Petición recibida: %s %s", r.Method, r.URL.Path)

			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	// Arrancamos el servidor
	log.Fatal(http.ListenAndServe(":"+port, corsMiddleware(http.DefaultServeMux)))
}
