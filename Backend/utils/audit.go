package utils

import (
	"database/sql"
	"log"
)

// LogAction inserta un registro en seguridad.logs_auditoria.
// No debe interrumpir el flujo principal si falla, por lo que los errores solo se loggean.
func LogAction(db *sql.DB, idUsuario int, accion, tablaAfectada string, idRegistro *int, valorAnterior, valorNuevo, ipOrigen string) {
	query := `
		INSERT INTO seguridad.logs_auditoria (id_usuario, accion, tabla_afectada, id_registro_afectado, valor_anterior, valor_nuevo, ip_origen)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := db.Exec(query, idUsuario, accion, tablaAfectada, idRegistro, valorAnterior, valorNuevo, ipOrigen)
	if err != nil {
		log.Printf("ERROR AUDITORIA: %v", err)
	}
}
