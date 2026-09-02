package main

import (
	"fmt"
	"log"

	"libreria-altares/database"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	godotenv.Load()
	db := database.Connect()
	defer db.Close()

	rows, err := db.Query("SELECT id_factura, id_venta FROM operaciones.facturas ORDER BY id_factura DESC LIMIT 10")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var f, v int
		rows.Scan(&f, &v)
		fmt.Printf("Factura: %d, Venta: %d\n", f, v)
	}
}
