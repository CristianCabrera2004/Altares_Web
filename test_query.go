package main

import (
	"database/sql"
	"fmt"
	"log"
	_ "github.com/lib/pq"
)

func main() {
	dbURL := "postgres://backend_cinder_shape_4030:SIQJEop2oAAUbcs@localhost:5433/backend_cinder_shape_4030?sslmode=disable"
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	query2 := `
		SELECT 
			TO_CHAR(v.fecha_venta AT TIME ZONE 'America/Guayaquil', 'YYYY-MM-DD') as fecha_v,
			p.nombre,
			SUM(d.cantidad) as cantidad
		FROM operaciones.detalle_ventas d
		JOIN operaciones.ventas v ON d.id_venta = v.id_venta
		JOIN inventario.productos p ON d.id_producto = p.id_producto
		WHERE v.id_usuario = 4
		GROUP BY TO_CHAR(v.fecha_venta AT TIME ZONE 'America/Guayaquil', 'YYYY-MM-DD'), p.nombre
		ORDER BY fecha_v DESC
	`
	rows, err := db.Query(query2)
	if err != nil {
		fmt.Printf("QUERY 2 ERROR: %v\n", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var f1, f2, f3 string
		rows.Scan(&f1, &f2, &f3)
		fmt.Printf("Date: %s | Prod: %s | Cant: %s\n", f1, f2, f3)
	}
}
