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

	rows, err := db.Query("SELECT column_name, is_nullable, data_type FROM information_schema.columns WHERE table_schema = 'operaciones' AND table_name = 'detalle_ventas'")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var name, nullable, datatype string
		rows.Scan(&name, &nullable, &datatype)
		fmt.Printf("Col: %s | Nullable: %s | Type: %s\n", name, nullable, datatype)
	}
}
