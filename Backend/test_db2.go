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

	var idVenta int
	err := db.QueryRow("SELECT id_venta FROM operaciones.facturas ORDER BY id_factura DESC LIMIT 1").Scan(&idVenta)
	if err != nil {
		log.Fatal("Error getting latest factura:", err)
	}
	fmt.Println("Latest Venta ID from facturas:", idVenta)
}
