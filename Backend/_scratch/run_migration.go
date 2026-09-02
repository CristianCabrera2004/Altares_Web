package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/lib/pq"
)

func main() {
	connStr := "host=localhost port=5432 user=postgres password=cace2004 dbname=libreria_los_altares_V2 sslmode=disable"
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec("ALTER TABLE configuracion.tiendas ADD COLUMN IF NOT EXISTS saldo_caja INT NOT NULL DEFAULT 0;")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Migración exitosa: saldo_caja agregado a configuracion.tiendas")
}
