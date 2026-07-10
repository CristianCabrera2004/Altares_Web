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

	// Query for id_venta = 3943 (or whichever latest)
	rows, err := db.Query("SELECT id_producto, cantidad, precio_unitario, iva_aplicado, subtotal FROM operaciones.detalle_ventas WHERE id_venta = 3943")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
		var id_producto, cantidad, precio, iva, sub int
		rows.Scan(&id_producto, &cantidad, &precio, &iva, &sub)
		fmt.Printf("Prod: %d, Cant: %d, Precio: %d, IVA: %d, Sub: %d\n", id_producto, cantidad, precio, iva, sub)
	}
	fmt.Printf("Total rows: %d\n", count)

	if count == 0 {
		fmt.Println("Let's check if the venta 3943 exists in ventas")
		var v string
		db.QueryRow("SELECT estado FROM operaciones.ventas WHERE id_venta = 3943").Scan(&v)
		fmt.Println("Estado venta 3943:", v)
	}
}
