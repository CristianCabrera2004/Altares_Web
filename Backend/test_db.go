package main

import (
	"database/sql"
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

	// Get latest venta
	var idVenta int
	err := db.QueryRow("SELECT id_venta FROM operaciones.ventas ORDER BY id_venta DESC LIMIT 1").Scan(&idVenta)
	if err != nil {
		log.Fatal("Error getting latest venta:", err)
	}
	fmt.Println("Latest Venta ID:", idVenta)

	// Get detalles
	rows, err := db.Query("SELECT id_producto, cantidad, precio_unitario, iva_aplicado, subtotal FROM operaciones.detalle_ventas WHERE id_venta = $1", idVenta)
	if err != nil {
		log.Fatal("Error getting detalles:", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var idProd, cant, precio, sub int
		var ivaNull sql.NullInt64
		err := rows.Scan(&idProd, &cant, &precio, &ivaNull, &sub)
		if err != nil {
			fmt.Println("Error scanning:", err)
		} else {
			fmt.Printf("Detalle: prod=%d cant=%d precio=%d iva=%v sub=%d\n", idProd, cant, precio, ivaNull, sub)
		}
		count++
	}
	fmt.Println("Total detalles:", count)
}
