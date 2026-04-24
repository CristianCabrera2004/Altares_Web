// Backend/main.go
// ─────────────────────────────────────────────────────────────────────────────
// Punto de entrada del servidor API REST - Librería Los Altares
//
// Registra todos los endpoints del sistema y aplica:
//  - CORS para comunicación con Angular (localhost:4200)
//  - middleware.RequireAuth en rutas protegidas por JWT
//  - middleware.RequireRole("admin_libreria") para rutas exclusivas de admin
//
// Nota sobre el enrutador Go stdlib:
//  Los paths más específicos deben registrarse ANTES que los generales.
//  Ejemplo: /api/ventas/cuaderno ANTES que /api/ventas
// ─────────────────────────────────────────────────────────────────────────────
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"libreria-altares/database"
	"libreria-altares/handlers"
	"libreria-altares/middleware"

	"github.com/joho/godotenv"
)

func main() {
	// 1. Cargar variables de entorno desde .env
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️  .env no encontrado, usando variables del sistema operativo")
	}

	// 2. Conectar al pool de PostgreSQL (optimizado para < 200ms — CA 46)
	db := database.Connect()
	defer db.Close()

	// 3. Crear el mux y registrar todas las rutas
	mux := http.NewServeMux()

	// ─── Rutas Públicas (sin JWT) ────────────────────────────────────────────

	// Health check — útil para verificar que el servidor está activo
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok","service":"API Librería Los Altares","version":"1.0.0"}`)
	})

	// ── HT-04: Endpoints de Autenticación (CA 51, 52, 53, 54) ─────────────────
	// POST /api/auth/login            → BCrypt verify + JWT HS256 8h (CA 51, 52)
	mux.HandleFunc("/api/auth/login", onlyMethod(http.MethodPost, handlers.LoginHandler(db)))

	// POST /api/auth/logout           → Invalida sesión activa en seguridad.sesiones
	mux.HandleFunc("/api/auth/logout", middleware.RequireAuth(handlers.LogoutHandler(db)))

	// GET  /api/auth/perfil           → Datos del usuario autenticado (desde claims JWT)
	mux.HandleFunc("/api/auth/perfil", middleware.RequireAuth(handlers.PerfilHandler(db)))

	// PUT  /api/auth/cambiar-password → BCrypt verify actual + nuevo hash (CA 51)
	mux.HandleFunc("/api/auth/cambiar-password", middleware.RequireAuth(handlers.CambiarPasswordHandler(db)))

	// ─── HT-02: Catálogo de Productos (CA 43, 44, 45, 46) ───────────────────
	// IMPORTANTE: /api/productos/buscar se registra ANTES de /api/productos
	// porque Go stdlib usa el prefijo más largo para rutas más específicas.
	//
	// GET /api/productos/buscar?codigo=XXX → Busca producto por código de barras
	//   Usado por el frontend para verificar si el producto ya existe antes de guardar.
	mux.HandleFunc("/api/productos/buscar", middleware.RequireAuth(handlers.BuscarProductoHandler(db)))

	// GET    → Lista catálogo activo con JOIN de categoría (índice GIN, < 200ms)
	// POST   → Crea producto (si barcode existe → incrementa stock; si no → INSERT + liga barcode)
	// PUT    → Actualiza producto en transacción SQL
	// DELETE → Baja lógica en transacción SQL
	mux.HandleFunc("/api/productos", middleware.RequireAuth(handlers.ProductHandler(db)))

	// ─── HT-02: Categorías (CA 43, 44, 45) ──────────────────────────────────
	mux.HandleFunc("/api/categorias", middleware.RequireAuth(handlers.CategoryHandler(db)))

	// ─── HT-02: Proveedores (CA 43, 44, 45) ─────────────────────────────────
	mux.HandleFunc("/api/proveedores", middleware.RequireAuth(handlers.ProviderHandler(db)))

	// ─── HT-02: Inventario Transaccional (CA 45) ────────────────────────────
	// BEGIN → INSERT ingreso/baja + UPDATE stock + INSERT movimiento → COMMIT/ROLLBACK
	mux.HandleFunc("/api/inventario/ingreso", middleware.RequireAuth(handlers.IngresoHandler(db)))
	mux.HandleFunc("/api/inventario/baja", middleware.RequireAuth(handlers.BajaHandler(db)))
	mux.HandleFunc("/api/inventario/movimientos", middleware.RequireAuth(handlers.MovimientosHandler(db)))

	// ─── HU-04: Devoluciones (Arquitectura de Bajas y Devoluciones) ─────────
	// POST → Registra devolución: repone stock + INSERT devoluciones + auditoría
	// GET  → Lista historial de devoluciones (filtro opcional ?id_producto=X)
	mux.HandleFunc("/api/devoluciones", middleware.RequireAuth(handlers.DevolucionHandler(db)))

	// ─── HT-02: Ventas y Cuaderno Transaccional (CA 45) ─────────────────────
	// IMPORTANTE: /api/ventas/cuaderno se registra ANTES de /api/ventas
	// porque Go stdlib usa prefijo más largo para rutas más específicas.
	//
	// POST /api/ventas/cuaderno → Carga masiva del cuaderno del día
	//   Todo el array se procesa en UNA transacción: si falla uno → ROLLBACK total
	mux.HandleFunc("/api/ventas/cuaderno", middleware.RequireAuth(handlers.CuadernoHandler(db)))

	// POST /api/ventas → Venta individual (también transaccional)
	mux.HandleFunc("/api/ventas", middleware.RequireAuth(handlers.SalesHandler(db)))

	// ─── Gestión de Usuarios (Solo Administrador) ───────────────────────────
	// RequireRole bloquea con HTTP 403 si el JWT no contiene rol = admin_libreria
	mux.HandleFunc("/api/usuarios", middleware.RequireRole("admin_libreria")(handlers.UserHandler(db)))

	// 4. Puerto del servidor
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// 5. Imprimir tabla de endpoints al arrancar
	fmt.Printf("\n╔══════════════════════════════════════════════════════╗\n")
	fmt.Printf("║    🚀  API Librería Los Altares — Puerto :%s       ║\n", port)
	fmt.Printf("╠══════════════════════════════════════════════════════╣\n")
	fmt.Printf("║  PÚBLICO                                             ║\n")
	fmt.Printf("║  GET    /api/health                                  ║\n")
	fmt.Printf("╠══════════════════════════════════════════════════════╣\n")
	fmt.Printf("║  HT-04 AUTENTICACIÓN (CA 51-54)                      ║\n")
	fmt.Printf("║  POST   /api/auth/login          BCrypt+JWT 8h       ║\n")
	fmt.Printf("║  POST   /api/auth/logout         Invalida sesión     ║\n")
	fmt.Printf("║  GET    /api/auth/perfil         Perfil del JWT      ║\n")
	fmt.Printf("║  PUT    /api/auth/cambiar-password  BCrypt update     ║\n")
	fmt.Printf("╠══════════════════════════════════════════════════════╣\n")
	fmt.Printf("║  HT-02 PROTEGIDO (Bearer JWT — CA 43-46)             ║\n")
	fmt.Printf("║  GET|POST|PUT|DELETE  /api/productos                 ║\n")
	fmt.Printf("║  GET|POST|PUT|DELETE  /api/categorias                ║\n")
	fmt.Printf("║  GET|POST|PUT|DELETE  /api/proveedores               ║\n")
	fmt.Printf("║  POST   /api/inventario/ingreso      [TXN]           ║\n")
	fmt.Printf("║  POST   /api/inventario/baja         [TXN]           ║\n")
	fmt.Printf("║  GET    /api/inventario/movimientos                  ║\n")
	fmt.Printf("║  POST   /api/ventas                  [TXN]           ║\n")
	fmt.Printf("║  POST   /api/ventas/cuaderno         [BULK-TXN]      ║\n")
	fmt.Printf("╠══════════════════════════════════════════════════════╣\n")
	fmt.Printf("║  ADMIN ONLY (rol: admin_libreria)                    ║\n")
	fmt.Printf("║  GET|POST|PUT|DELETE  /api/usuarios                  ║\n")
	fmt.Printf("╚══════════════════════════════════════════════════════╝\n\n")

	// 6. Iniciar servidor con CORS habilitado para Angular
	log.Fatal(http.ListenAndServe(":"+port, corsMiddleware(mux)))
}

// onlyMethod es un wrapper que garantiza que un handler solo acepte un método HTTP.
// Devuelve 405 Method Not Allowed con JSON si el método no coincide (CA 44).
func onlyMethod(method string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusMethodNotAllowed)
			fmt.Fprintf(w, `{"error":"Este endpoint solo acepta %s."}`, method)
			return
		}
		h(w, r)
	}
}

// corsMiddleware habilita el acceso desde Angular (localhost:4200) y cualquier
// cliente de desarrollo. Responde correctamente a las peticiones OPTIONS (preflight).
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("📥 %s %s", r.Method, r.URL.Path)

		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// Responder inmediatamente a peticiones OPTIONS (CORS preflight de Angular)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
