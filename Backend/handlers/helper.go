package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"libreria-altares/middleware"
)

// GetTiendaIDFromCtxOrDb obtiene el id_tienda del usuario consultando en tiempo real la base de datos
// para evitar desincronizaciones debidas a tokens JWT con datos obsoletos tras reasignaciones de sucursal.
func GetTiendaIDFromCtxOrDb(db *sql.DB, r *http.Request) int {
	claims, ok := middleware.GetClaims(r)
	if !ok {
		return 1 // Fallback predeterminado
	}

	var currentTiendaNull sql.NullInt64
	var currentRol string

	// Consultar el estado y tienda real del usuario en la base de datos
	err := db.QueryRow(
		`SELECT id_tienda, rol FROM seguridad.usuarios WHERE id_usuario = $1`,
		claims.IdUsuario,
	).Scan(&currentTiendaNull, &currentRol)

	var currentTienda int
	if err == nil {
		if currentTiendaNull.Valid {
			currentTienda = int(currentTiendaNull.Int64)
		}
	} else {
		// Si falla la consulta, recurrimos al valor del JWT como respaldo
		currentTienda = claims.IdTienda
		currentRol = claims.Rol
	}

	// Si el usuario es administrador, puede usar ?tienda=X para consultar/operar en otra tienda
	if currentRol == "admin_libreria" {
		tiendaStr := r.URL.Query().Get("tienda")
		if tiendaStr != "" {
			if t, err := strconv.Atoi(tiendaStr); err == nil && t > 0 {
				return t
			}
		}
		if currentTienda == 0 {
			return 1 // Fallback para administradores globales
		}
	}

	if currentTienda <= 0 {
		return 1
	}

	return currentTienda
}
