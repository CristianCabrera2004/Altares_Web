package main

import (
	"fmt"
	"log"
	"os"

	"libreria-altares/database"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()
	db := database.Connect()
	defer db.Close()

	sqlScript, err := os.ReadFile("database/migracion_facturas_compras.sql")
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec(string(sqlScript))
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Migración facturas completada exitosamente.")
}
