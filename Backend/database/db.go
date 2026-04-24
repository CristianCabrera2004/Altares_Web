// Backend/database/db.go
// ─────────────────────────────────────────────────────────────────────────────
// Gestiona la conexión al pool de PostgreSQL.
// El pool de conexiones está optimizado para garantizar que los endpoints de
// lectura respondan por debajo de 200ms en entorno local (CA 46).
// ─────────────────────────────────────────────────────────────────────────────
package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
)

// Connect abre y verifica la conexión a PostgreSQL usando las variables de entorno.
// Retorna *sql.DB listo para usar. Hace log.Fatal si no puede conectar.
func Connect() *sql.DB {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"),
		os.Getenv("DB_SSLMODE"),
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("❌ Error al abrir la conexión con Postgres: %v", err)
	}

	if err = db.Ping(); err != nil {
		log.Fatalf("❌ No se pudo alcanzar PostgreSQL en %s:%s — %v",
			os.Getenv("DB_HOST"), os.Getenv("DB_PORT"), err)
	}

	// ── Configuración del pool de conexiones (CA 46: latencia < 200ms) ──────
	// 25 conexiones abiertas máximo: evita overhead de abrir/cerrar sockets.
	db.SetMaxOpenConns(25)
	// 10 conexiones en idle: reutilizables sin esperar nuevo handshake TCP.
	db.SetMaxIdleConns(10)
	// Conexiones inactivas más de 5 minutos se reciclan para liberar recursos.
	db.SetConnMaxIdleTime(5 * time.Minute)
	// Ninguna conexión vive más de 1 hora (evita problemas con timeouts de BD).
	db.SetConnMaxLifetime(1 * time.Hour)

	log.Println("✅ Conexión a PostgreSQL establecida correctamente")
	return db
}
