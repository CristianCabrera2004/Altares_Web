package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/lib/pq"
)

func main() {
	db, err := sql.Open("postgres", "host=localhost port=5432 user=postgres password=cace2004 dbname=libreria_los_altares_V2 sslmode=disable")
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	defer db.Close()

	fmt.Println("Ejecutando alteración de tabla inventario.transferencias...")
	queries := []string{
		`ALTER TABLE inventario.transferencias ADD COLUMN IF NOT EXISTS estado VARCHAR(20) NOT NULL DEFAULT 'Pendiente'`,
		`ALTER TABLE inventario.transferencias ADD COLUMN IF NOT EXISTS requiere_confirmacion_destino BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE inventario.transferencias ADD COLUMN IF NOT EXISTS parcial BOOLEAN NOT NULL DEFAULT FALSE`,
	}

	for _, q := range queries {
		_, err := db.Exec(q)
		if err != nil {
			log.Fatalf("Error ejecutando query (%s): %v", q, err)
		}
	}
	fmt.Println("Migración rápida completada con éxito!")
}
