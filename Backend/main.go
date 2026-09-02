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
	"strings"

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

	// Migración automática para añadir metodo_pago si no existe
	_, err := db.Exec(`ALTER TABLE operaciones.ventas ADD COLUMN IF NOT EXISTS metodo_pago VARCHAR(50) NOT NULL DEFAULT 'efectivo'`)
	if err != nil {
		log.Printf("Error al aplicar migracion metodo_pago: %v", err)
	}

	// RUN TEMP MIGRATION FOR NEW COLUMNS
	_, err = db.Exec(`
		ALTER TABLE seguridad.usuarios ADD COLUMN IF NOT EXISTS two_factor_enabled BOOLEAN NOT NULL DEFAULT FALSE;
		ALTER TABLE seguridad.usuarios ADD COLUMN IF NOT EXISTS two_factor_secret VARCHAR(100) DEFAULT NULL;
		ALTER TABLE seguridad.usuarios ADD COLUMN IF NOT EXISTS email_verificado BOOLEAN NOT NULL DEFAULT FALSE;
		ALTER TABLE seguridad.usuarios ADD COLUMN IF NOT EXISTS codigo_verificacion VARCHAR(6) DEFAULT NULL;
		ALTER TABLE seguridad.usuarios ADD COLUMN IF NOT EXISTS codigo_verificacion_expira TIMESTAMP DEFAULT NULL;
	`)
	if err != nil {
		log.Printf("Advertencia: Falló migración temporal: %v", err)
	} else {
		log.Println("Migración temporal de columnas aplicada correctamente.")
	}

	// Migración automática para añadir saldo_caja si no existe
	_, err = db.Exec(`ALTER TABLE configuracion.tiendas ADD COLUMN IF NOT EXISTS saldo_caja INT NOT NULL DEFAULT 0`)
	if err != nil {
		log.Printf("Error al aplicar migracion saldo_caja: %v", err)
	}

	// 3. Crear el mux y registrar todas las rutas
	mux := http.NewServeMux()

	// ─── Rutas Públicas (sin JWT) ────────────────────────────────────────────

	// Health check — útil para verificar que el servidor está activo
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok","service":"API Librería Los Altares","version":"1.0.1"}`)
	})

	// ── HT-04: Endpoints de Autenticación (CA 51, 52, 53, 54) ─────────────────
	// POST /api/auth/login            → BCrypt verify + JWT HS256 8h (CA 51, 52)
	mux.HandleFunc("/api/auth/login", middleware.RateLimit(onlyMethod(http.MethodPost, handlers.LoginHandler(db))))

	// POST /api/auth/reenviar-codigo  → Reenvía código de verificación de email (público)
	mux.HandleFunc("/api/auth/reenviar-codigo", onlyMethod(http.MethodPost, handlers.ReenviarCodigoHandler(db)))

	// POST /api/auth/logout           → Invalida sesión activa en seguridad.sesiones
	mux.HandleFunc("/api/auth/logout", middleware.RequireAuth(db, handlers.LogoutHandler(db)))

	// GET  /api/auth/perfil           → Datos del usuario autenticado (desde claims JWT)
	mux.HandleFunc("/api/auth/perfil", middleware.RequireAuth(db, handlers.PerfilHandler(db)))

	// PUT  /api/auth/cambiar-password → BCrypt verify actual + nuevo hash (CA 51)
	mux.HandleFunc("/api/auth/cambiar-password", middleware.RequireAuth(db, handlers.CambiarPasswordHandler(db)))

	// ── Endpoints de 2FA (TOTP) ──────────────────────────────────────────────
	mux.HandleFunc("/api/auth/2fa/setup", middleware.RequireAuth(db, handlers.Setup2FAHandler(db)))
	mux.HandleFunc("/api/auth/2fa/enable", middleware.RequireAuth(db, handlers.Enable2FAHandler(db)))
	mux.HandleFunc("/api/auth/2fa/disable", middleware.RequireAuth(db, handlers.Disable2FAHandler(db)))

	// ─── HT-02: Catálogo de Productos (CA 43, 44, 45, 46) ───────────────────
	// IMPORTANTE: /api/productos/buscar se registra ANTES de /api/productos
	mux.HandleFunc("/api/productos/buscar", middleware.RequireRole(db, "operador_caja")(handlers.BuscarProductoHandler(db)))

	// GET/POST/PUT/DELETE /api/productos
	mux.HandleFunc("/api/productos", middleware.RequireRole(db, "operador_caja")(handlers.ProductHandler(db)))

	// POST /api/productos/{id}/codigos-barras (enlace rápido)
	mux.HandleFunc("/api/productos/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/codigos-barras") {
			if r.Method == http.MethodPost {
				middleware.RequireRole(db, "operador_caja")(handlers.LinkBarcodeHandler(db))(w, r)
				return
			} else if r.Method == http.MethodDelete {
				middleware.RequireRole(db, "operador_caja")(handlers.UnlinkBarcodeHandler(db))(w, r)
				return
			}
		}
		// Fallback para si alguien llama a /api/productos/ de alguna manera (aunque el mux de Go prefiere el exact match)
		middleware.RequireRole(db, "operador_caja")(handlers.ProductHandler(db))(w, r)
	})
	// ─── HT-02: Categorías y Proveedores ─────────────────────────────────────
	mux.HandleFunc("/api/categorias", middleware.RequireRole(db, "operador_caja")(handlers.CategoryHandler(db)))
	mux.HandleFunc("/api/proveedores", middleware.RequireRole(db, "operador_caja")(handlers.ProviderHandler(db)))

	// ─── HT-02: Inventario Transaccional (CA 45) ────────────────────────────
	mux.HandleFunc("/api/inventario/ingreso", middleware.RequireRole(db, "operador_caja")(handlers.IngresoHandler(db)))
	mux.HandleFunc("/api/inventario/ingreso-multiple", middleware.RequireRole(db, "operador_caja")(handlers.IngresoMultipleHandler(db)))
	mux.HandleFunc("/api/inventario/baja", middleware.RequireRole(db, "operador_caja")(handlers.BajaHandler(db)))
	mux.HandleFunc("/api/inventario/movimientos", middleware.RequireRole(db, "operador_caja")(handlers.MovimientosHandler(db)))
	mux.HandleFunc("/api/inventario/transferencias/responder", middleware.RequireRole(db, "operador_caja")(handlers.ResponderTransferenciaHandler(db)))
	mux.HandleFunc("/api/inventario/transferencias/confirmar-parcial", middleware.RequireRole(db, "operador_caja")(handlers.ConfirmarParcialTransferenciaHandler(db)))
	mux.HandleFunc("/api/inventario/transferencias/recibir", middleware.RequireRole(db, "operador_caja")(handlers.RecibirTransferenciaHandler(db)))
	mux.HandleFunc("/api/inventario/transferencias", middleware.RequireRole(db, "operador_caja")(handlers.TransferenciasHandler(db)))

	// ── HT-07: Configuración de Sistema ──────────────────────────────────────────
	mux.HandleFunc("/api/configuracion", middleware.RequireRole(db, "admin_libreria")(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handlers.GetConfigHandler(db)(w, r)
		} else if r.Method == http.MethodPut {
			handlers.PutConfigHandler(db)(w, r)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))

	// ── HT-10: Auditoría ───────────────────────────────────────────────────────
	mux.HandleFunc("/api/devoluciones", middleware.RequireRole(db, "operador_caja")(handlers.DevolucionHandler(db)))

	// ─── HT-02: Ventas y Cuaderno Transaccional ───────────────────────────────
	// HU-02: Factura Global (Cierre)
	mux.HandleFunc("/api/ventas/factura-cierre", middleware.RequireRole(db, "operador_caja")(handlers.InvoiceHandler(db)))
	// POST /api/facturas -> Crear factura; GET /api/facturas -> Consultar factura
	mux.HandleFunc("/api/facturas", middleware.RequireRole(db, "operador_caja")(handlers.FacturasHandler(db)))
	// POST /api/ventas/cuaderno → Carga masiva del cuaderno del día
	mux.HandleFunc("/api/ventas/cuaderno", middleware.RequireRole(db, "operador_caja")(handlers.CuadernoHandler(db)))
	// POST /api/ventas → Venta individual
	mux.HandleFunc("/api/ventas", middleware.RequireRole(db, "operador_caja")(handlers.SalesHandler(db)))
	mux.HandleFunc("/api/facturas/reenviar", middleware.RequireRole(db, "operador_caja")(handlers.ReenviarFacturaHandler(db)))
	mux.HandleFunc("/api/caja", middleware.RequireRole(db, "operador_caja")(handlers.CajaHandler(db)))

	// ─── HU-08: Auditoría y Logs (Solo Administrador) ────────────────────────
	mux.HandleFunc("/api/auditoria", middleware.RequireRole(db, "admin_libreria")(handlers.AuditHandler(db)))

	// ─── HU-07: Reportes (Solo Operador) ─────────────────────────────────────
	mux.HandleFunc("/api/reportes/ventas", middleware.RequireRole(db, "operador_caja")(handlers.ReportesVentasHandler(db)))
	mux.HandleFunc("/api/reportes/factura-diaria", middleware.RequireRole(db, "operador_caja")(handlers.FacturaDiariaConsumidorFinalHandler(db)))
	// GET /api/dashboard/grafica -> Reportes Gráficos
	mux.HandleFunc("/api/dashboard/grafica", middleware.RequireRole(db, "operador_caja")(handlers.ReporteGraficaHandler(db)))

	// ─── Gestión de Usuarios y Tiendas (Solo Administrador) ───────────────────
	mux.HandleFunc("/api/usuarios/verificar-email", middleware.RequireRole(db, "admin_libreria")(handlers.VerificarEmailHandler(db)))
	mux.HandleFunc("/api/usuarios", middleware.RequireRole(db, "admin_libreria")(handlers.UserHandler(db)))
	mux.HandleFunc("/api/tiendas/activas", middleware.RequireRole(db, "operador_caja")(handlers.TiendasActivasHandler(db)))
	mux.HandleFunc("/api/tiendas", middleware.RequireRole(db, "admin_libreria")(handlers.TiendaHandler(db)))


	// ─── HT-03: Motor de Predicción Analítica (Solo Operador) ────────────────
	mux.HandleFunc("/api/predicciones/lista-compras", middleware.RequireRole(db, "operador_caja")(handlers.PredictionHandler(db)))
	mux.HandleFunc("/api/predicciones", middleware.RequireRole(db, "operador_caja")(handlers.PredictionHandler(db)))

	// ─── Catálogo de Clientes (Anexo 3) ───────────────────────────────────────
	mux.HandleFunc("/api/clientes/buscar", middleware.RequireRole(db, "operador_caja")(handlers.BuscarClienteHandler(db)))
	mux.HandleFunc("/api/clientes", middleware.RequireRole(db, "operador_caja")(handlers.ClientHandler(db)))


	// ─── Facturas de Compras (Ingresos Múltiples) ──────────────────────────────
	mux.HandleFunc("/api/reportes/compras", middleware.RequireRole(db, "operador_caja")(handlers.ComprasReporteListHandler(db)))
	mux.HandleFunc("GET /api/reportes/compras/{id}", middleware.RequireRole(db, "operador_caja")(handlers.ComprasReporteDetailHandler(db)))
	
	// ─── Módulo de Deudores/Fiados (Anexo 4) ──────────────────────────────────
	mux.HandleFunc("/api/deudores/abono", middleware.RequireRole(db, "operador_caja")(handlers.AbonoHandler(db)))
	mux.HandleFunc("/api/deudores/abonos", middleware.RequireRole(db, "operador_caja")(handlers.AbonosListHandler(db)))
	mux.HandleFunc("/api/deudores", middleware.RequireRole(db, "operador_caja")(handlers.DeudorHandler(db)))

	mux.HandleFunc("/api/wipe-test-sales", func(w http.ResponseWriter, r *http.Request) {
		db.Exec(`DELETE FROM operaciones.detalle_ventas WHERE id_venta IN (4111, 4112, 4113)`)
		db.Exec(`DELETE FROM operaciones.facturas WHERE id_venta IN (4111, 4112, 4113)`)
		db.Exec(`DELETE FROM inventario.movimientos_stock WHERE referencia_id IN (4111, 4112, 4113) AND tipo_movimiento = 'VENTA'`)
		db.Exec(`DELETE FROM operaciones.ventas WHERE id_venta IN (4111, 4112, 4113)`)
		db.Exec(`UPDATE inventario.stock_tiendas SET stock_actual = stock_actual + 3 WHERE id_producto = (SELECT id_producto FROM inventario.productos WHERE nombre = 'Golpes 0.25' LIMIT 1)`)
		w.Write([]byte("Borradas las ventas de prueba 4111, 4112, 4113 y stock restaurado."))
	})

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
	fmt.Printf("║  GET|POST|PUT  /api/clientes         [Catálogo]      ║\n")
	fmt.Printf("║  GET    /api/clientes/buscar         [Autocompletado]║\n")
	fmt.Printf("║  GET|POST|PUT|DEL  /api/deudores     [Fiados]        ║\n")
	fmt.Printf("║  POST   /api/deudores/abono          [Abono parcial] ║\n")
	fmt.Printf("╠══════════════════════════════════════════════════════╣\n")
	fmt.Printf("║  ADMIN ONLY (rol: admin_libreria)                    ║\n")
	fmt.Printf("║  GET|POST|PUT|DELETE  /api/usuarios                  ║\n")
	fmt.Printf("║  GET|POST|PUT|DELETE  /api/tiendas                   ║\n")
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

// corsMiddleware habilita el acceso desde Angular y permite configurar el
// origen permitido mediante la variable de entorno ALLOWED_ORIGIN.
// En desarrollo, si ALLOWED_ORIGIN no está definido, se permite '*'.
// En producción, configurar: ALLOWED_ORIGIN=https://tu-dominio.com
func corsMiddleware(next http.Handler) http.Handler {
	allowedOrigin := os.Getenv("ALLOWED_ORIGIN")
	appEnv := os.Getenv("APP_ENV")

	if appEnv == "production" && (allowedOrigin == "" || allowedOrigin == "*") {
		log.Fatal("🚨 FATAL: En producción (APP_ENV=production) DEBES configurar un ALLOWED_ORIGIN específico. El servidor se detendrá por seguridad.")
	}

	if allowedOrigin == "" {
		allowedOrigin = "*" // solo para desarrollo
		log.Println("⚠️  CORS: ALLOWED_ORIGIN no configurado, usando '*' (solo válido en desarrollo)")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("📥 %s %s", r.Method, r.URL.Path)

		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
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
