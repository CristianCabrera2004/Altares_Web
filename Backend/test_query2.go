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

    var count int
    err := db.QueryRow("SELECT count(*) FROM operaciones.facturas").Scan(&count)
    if err != nil {
        log.Fatal(err)
    }

	fmt.Println("Total facturas =", count)
}
