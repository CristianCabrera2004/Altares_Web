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

	// Mostrar columnas de inventario.productos
	fmt.Println("--- Columnas de inventario.productos ---")
	rows, err := db.Query(`
		SELECT column_name, data_type 
		FROM information_schema.columns 
		WHERE table_schema = 'inventario' AND table_name = 'productos'
	`)
	if err != nil {
		log.Fatal(err)
	}
	for rows.Next() {
		var col, typ string
		if err := rows.Scan(&col, &typ); err == nil {
			fmt.Printf("Columna: %s, Tipo: %s\n", col, typ)
		}
	}
	rows.Close()
}
