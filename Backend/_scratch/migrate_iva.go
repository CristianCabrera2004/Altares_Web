package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	err := godotenv.Load(".env")
	dbUrl := os.Getenv("DATABASE_URL")
	if dbUrl == "" {
		host := os.Getenv("DB_HOST")
		port := os.Getenv("DB_PORT")
		user := os.Getenv("DB_USER")
		password := os.Getenv("DB_PASSWORD")
		dbname := os.Getenv("DB_NAME")
		sslmode := os.Getenv("DB_SSLMODE")
		if sslmode == "" {
			sslmode = "disable"
		}
		dbUrl = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", user, password, host, port, dbname, sslmode)
	}
	if dbUrl == "" {
		log.Fatal("DATABASE_URL not set")
	}

	db, err := sql.Open("postgres", dbUrl)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		log.Fatal(err)
	}

	queries := []string{
		`CREATE TABLE IF NOT EXISTS configuracion.parametros (
			clave VARCHAR(50) PRIMARY KEY,
			valor TEXT NOT NULL
		);`,
		`INSERT INTO configuracion.parametros (clave, valor) VALUES ('tasa_iva_grabado', '15') ON CONFLICT (clave) DO NOTHING;`,
		`ALTER TABLE inventario.productos ADD COLUMN IF NOT EXISTS tipo_iva VARCHAR(20) DEFAULT '0%';`,
		`UPDATE inventario.productos p 
		 SET tipo_iva = 'grabado' 
		 FROM inventario.categorias c 
		 WHERE p.id_categoria = c.id_categoria AND c.tasa_iva > 0 AND p.tipo_iva = '0%';`,
	}

	for _, q := range queries {
		fmt.Println("Executing:", q)
		if _, err := tx.Exec(q); err != nil {
			tx.Rollback()
			log.Fatalf("Error executing query %s: %v", q, err)
		}
	}

	if err := tx.Commit(); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Migration successful!")
}
