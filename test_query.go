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
			'2026-07-08 00:31:37'::timestamp as t_raw,
			('2026-07-08 00:31:37'::timestamp AT TIME ZONE 'UTC') as t_utc,
			('2026-07-08 00:31:37'::timestamp AT TIME ZONE 'UTC' AT TIME ZONE 'America/Guayaquil') as t_gye
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
		fmt.Printf("Raw: %s | UTC: %s | GYE: %s\n", f1, f2, f3)
	}
}
