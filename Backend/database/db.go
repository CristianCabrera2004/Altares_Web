package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func Connect() *sql.DB {
	// Cargar el archivo .env
	// Nota: Si ejecutas el binario compilado fuera de esta carpeta, esto puede fallar.
	// En producción real, Docker o el SO inyectan estas variables directamente.
	err := godotenv.Load()
	if err != nil {
		log.Println("Advertencia: No se encontró archivo .env, usando variables del sistema")
	}

	// Leer las variables
	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	dbname := os.Getenv("DB_NAME")
	sslmode := os.Getenv("DB_SSLMODE")

	// Construir la cadena de conexión
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslmode)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("Error fatal al abrir la conexión con la base de datos: ", err)
	}

	err = db.Ping()
	if err != nil {
		log.Fatal("La base de datos no responde: ", err)
	}

	log.Println("✅ Conectado a PostgreSQL en", host)
	return db
}