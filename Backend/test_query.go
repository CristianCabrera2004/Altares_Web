package main

import (
	"fmt"
	"log"

	"libreria-altares/database"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()
	db := database.Connect()
	defer db.Close()

    var idVenta int
    err := db.QueryRow("SELECT id_venta FROM operaciones.facturas ORDER BY id_factura DESC LIMIT 1").Scan(&idVenta)
    if err != nil {
        log.Fatal(err)
    }

	fmt.Println("Testing id_venta =", idVenta)

	rows, errItems := db.Query(`
		SELECT p.nombre, d.cantidad, d.precio_unitario, d.iva_aplicado, d.subtotal
		FROM operaciones.detalle_ventas d
		JOIN inventario.productos p ON d.id_producto = p.id_producto
		WHERE d.id_venta = $1
	`, idVenta)

	if errItems != nil {
		log.Fatal(errItems)
	}
	defer rows.Close()

    count := 0
	for rows.Next() {
		var p string
        var c, pu, iva, sub int
		if err := rows.Scan(&p, &c, &pu, &iva, &sub); err != nil {
			fmt.Println("Scan error:", err)
		} else {
            count++
			fmt.Printf("Item: %s x%d = %d (iva %d)\n", p, c, sub, iva)
		}
	}
    fmt.Println("Total items:", count)
}
