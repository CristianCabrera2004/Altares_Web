package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	_ "github.com/lib/pq"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load(".env")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbUser := os.Getenv("DB_USER")
	dbPass := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dbSSL := os.Getenv("DB_SSLMODE")

	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		dbHost, dbPort, dbUser, dbPass, dbName, dbSSL)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	query := `
		SELECT p.id_producto, p.nombre, p.id_categoria, c.nombre, p.tipo_iva,
		       CASE WHEN p.tipo_iva = 'grabado' THEN (SELECT valor::INT FROM configuracion.parametros WHERE clave='tasa_iva_grabado') ELSE 0 END as tasa_iva,
		       COALESCE(st.stock_actual, 0), COALESCE(st.stock_alerta_min, 5),
		       p.precio_venta, p.estado,
		       COALESCE((
		         SELECT array_agg(codigo) 
		         FROM inventario.codigos_barras 
		         WHERE id_producto = p.id_producto
		       ), '{}')
		FROM inventario.productos p
		JOIN inventario.categorias c ON p.id_categoria = c.id_categoria
		LEFT JOIN inventario.stock_tiendas st ON p.id_producto = st.id_producto AND st.id_tienda = 1
		WHERE p.estado = 'activo'`
	
	rows, err := db.Query(query)
	if err != nil {
		fmt.Printf("QUERY ERROR: %v\n", err)
		return
	}
	defer rows.Close()
	fmt.Println("QUERY OK")
}
