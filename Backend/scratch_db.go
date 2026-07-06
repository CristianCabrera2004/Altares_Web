package main

import (
	"database/sql"
	"fmt"
	"log"
	_ "github.com/lib/pq"
)

func main() {
	db, err := sql.Open("postgres", "host=localhost port=5432 user=postgres password=cace2004 dbname=libreria_los_altares_V2 sslmode=disable")
	if err != nil { log.Fatalf("Error: %v", err) }
	defer db.Close()

	var total, withStock int
	err = db.QueryRow("SELECT COUNT(*) FROM inventario.productos").Scan(&total)
	err = db.QueryRow("SELECT COUNT(*) FROM inventario.stock_tiendas WHERE stock_actual > 0").Scan(&withStock)
	
	fmt.Printf("Total Productos: %d\nProductos con stock > 0: %d\n", total, withStock)
}
