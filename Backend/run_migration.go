package main

import (
	"fmt"
	"log"
	"os"

	"libreria-altares/database"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println(".env no encontrado")
	}

	db := database.Connect()
	defer db.Close()

	// Drop existing schemas
	dropSQL := `DROP SCHEMA IF EXISTS configuracion CASCADE;
DROP SCHEMA IF EXISTS seguridad CASCADE;
DROP SCHEMA IF EXISTS inventario CASCADE;
DROP SCHEMA IF EXISTS operaciones CASCADE;`

	_, err := db.Exec(dropSQL)
	if err != nil {
		log.Fatalf("Error borrando schemas: %v", err)
	}
	
	// Read init.sql
	sqlBytes, err := os.ReadFile("database/init.sql")
	if err != nil {
		log.Fatalf("Error leyendo init.sql: %v", err)
	}
	
	_, err = db.Exec(string(sqlBytes))
	if err != nil {
		log.Fatalf("Error ejecutando init.sql: %v", err)
	}

	fmt.Println("Migración SQL ejecutada con éxito! Base de datos recreada.")
}
