package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

type AuditLog struct {
	IdLog          int    `json:"id_log"`
	NombreUsuario  string `json:"nombre_usuario"` // Desde JOIN con seguridad.usuarios
	Accion         string `json:"accion"`
	TablaAfectada  string `json:"tabla_afectada"`
	RegistroID     *int   `json:"id_registro_afectado"`
	ValorAnterior  string `json:"valor_anterior"`
	ValorNuevo     string `json:"valor_nuevo"`
	IpOrigen       string `json:"ip_origen"`
	Fecha          string `json:"fecha"`
}

// AuditHandler retorna los logs de auditoría. Solo debería ser accesible por Administradores.
func AuditHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Método no permitido."})
			return
		}

		query := `
			SELECT 
				l.id_log,
				u.nombre,
				l.accion,
				l.tabla_afectada,
				l.id_registro_afectado,
				COALESCE(l.valor_anterior, ''),
				COALESCE(l.valor_nuevo, ''),
				COALESCE(l.ip_origen, ''),
				TO_CHAR(l.fecha, 'YYYY-MM-DD HH24:MI:SS') as fecha
			FROM seguridad.logs_auditoria l
			JOIN seguridad.usuarios u ON l.id_usuario = u.id_usuario
			ORDER BY l.fecha DESC
			LIMIT 500
		`

		rows, err := db.Query(query)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar los logs de auditoría."})
			return
		}
		defer rows.Close()

		var logs []AuditLog
		for rows.Next() {
			var log AuditLog
			if err := rows.Scan(
				&log.IdLog, &log.NombreUsuario, &log.Accion, &log.TablaAfectada,
				&log.RegistroID, &log.ValorAnterior, &log.ValorNuevo, &log.IpOrigen, &log.Fecha,
			); err != nil {
				continue
			}
			logs = append(logs, log)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(logs)
	}
}
