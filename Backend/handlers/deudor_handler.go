// Backend/handlers/deudor_handler.go
// ─────────────────────────────────────────────────────────────────────────────
// Módulo de Deudores/Fiados (Anexo 4)
//
// Endpoints:
//   GET    /api/deudores              → Lista deudores por tienda (filtro estado)
//   POST   /api/deudores              → Registra nueva deuda (dinero o producto)
//   PUT    /api/deudores?id=X         → Actualiza datos de la deuda
//   PATCH  /api/deudores/abono        → Registra abono parcial
//   DELETE /api/deudores?id=X         → Marca como pagado (baja lógica)
//
// Contexto: localidad rural donde la dueña conoce a todos; se fía dinero o
// productos y se necesita llevar registro formal de deudas y abonos.
// ─────────────────────────────────────────────────────────────────────────────
package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"libreria-altares/middleware"
)

// Deudor mapea la tabla operaciones.deudores.
type Deudor struct {
	IdDeuda        int    `json:"id_deuda"`
	IdUsuario      int    `json:"id_usuario"`
	NombreUsuario  string `json:"nombre_usuario,omitempty"`
	IdTienda       int    `json:"id_tienda"`
	NombreDeudor   string `json:"nombre_deudor"`
	Telefono       string `json:"telefono,omitempty"`
	TipoDeuda      string `json:"tipo_deuda"`      // "dinero" o "producto"
	MontoDeuda     int    `json:"monto_deuda"`      // centavos (si dinero)
	MontoAbonado   int    `json:"monto_abonado"`    // total abonado hasta ahora
	DetalleProducto string `json:"detalle_producto,omitempty"` // desc (si producto)
	Motivo         string `json:"motivo,omitempty"`
	Estado         string `json:"estado"`           // pendiente, parcial, pagado
	FechaRegistro  string `json:"fecha_registro"`
	FechaPago      string `json:"fecha_pago,omitempty"`
}

// AbonoInput es el body de PATCH /api/deudores/abono.
type AbonoInput struct {
	IdDeuda     int    `json:"id_deuda"`
	MontoAbono  int    `json:"monto_abono"`
	Observacion string `json:"observacion"`
}

// AbonoRow para listar abonos de una deuda.
type AbonoRow struct {
	IdAbono     int    `json:"id_abono"`
	MontoAbono  int    `json:"monto_abono"`
	Observacion string `json:"observacion,omitempty"`
	FechaAbono  string `json:"fecha_abono"`
}

// DeudorHandler despacha por método HTTP.
func DeudorHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			getDeudores(db, w, r)
		case http.MethodPost:
			createDeudor(db, w, r)
		case http.MethodPut:
			updateDeudor(db, w, r)
		case http.MethodDelete:
			pagarDeudor(db, w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Método HTTP no soportado."})
		}
	}
}

// AbonoHandler registra un abono parcial a una deuda.
func AbonoHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Solo se acepta POST en este endpoint."})
			return
		}
		registrarAbono(db, w, r)
	}
}

// AbonosListHandler lista abonos de una deuda.
func AbonosListHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Solo se acepta GET."})
			return
		}

		idDeudaStr := r.URL.Query().Get("id_deuda")
		if idDeudaStr == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "?id_deuda es obligatorio."})
			return
		}

		rows, err := db.Query(`
			SELECT id_abono, monto_abono, COALESCE(observacion, ''),
			       TO_CHAR(fecha_abono, 'YYYY-MM-DD HH24:MI:SS')
			FROM operaciones.abonos_deuda
			WHERE id_deuda = $1
			ORDER BY fecha_abono DESC`, idDeudaStr)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar abonos."})
			return
		}
		defer rows.Close()

		abonos := []AbonoRow{}
		for rows.Next() {
			var a AbonoRow
			if err := rows.Scan(&a.IdAbono, &a.MontoAbono, &a.Observacion, &a.FechaAbono); err != nil {
				continue
			}
			abonos = append(abonos, a)
		}
		json.NewEncoder(w).Encode(abonos)
	}
}

// getTiendaIDForDeudor has been replaced by GetTiendaIDFromCtxOrDb


// ─── GET ─────────────────────────────────────────────────────────────────────
func getDeudores(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	idTienda := GetTiendaIDFromCtxOrDb(db, r)
	estado := r.URL.Query().Get("estado") // filtro opcional

	query := `
		SELECT d.id_deuda, d.id_usuario, u.nombre, d.id_tienda,
		       d.nombre_deudor, COALESCE(d.telefono, ''),
		       d.tipo_deuda, d.monto_deuda,
		       COALESCE(d.monto_abonado, 0),
		       COALESCE(d.detalle_producto, ''),
		       COALESCE(d.motivo, ''), d.estado,
		       TO_CHAR(d.fecha_registro, 'YYYY-MM-DD HH24:MI:SS'),
		       COALESCE(TO_CHAR(d.fecha_pago, 'YYYY-MM-DD HH24:MI:SS'), '')
		FROM operaciones.deudores d
		JOIN seguridad.usuarios u ON d.id_usuario = u.id_usuario
		WHERE d.id_tienda = $1`

	args := []interface{}{idTienda}
	if estado != "" && estado != "todos" {
		args = append(args, estado)
		query += fmt.Sprintf(" AND d.estado = $%d", len(args))
	}
	query += " ORDER BY d.fecha_registro DESC LIMIT 500"

	rows, err := db.Query(query, args...)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar deudores."})
		return
	}
	defer rows.Close()

	deudores := []Deudor{}
	for rows.Next() {
		var d Deudor
		if err := rows.Scan(
			&d.IdDeuda, &d.IdUsuario, &d.NombreUsuario, &d.IdTienda,
			&d.NombreDeudor, &d.Telefono,
			&d.TipoDeuda, &d.MontoDeuda, &d.MontoAbonado,
			&d.DetalleProducto, &d.Motivo, &d.Estado,
			&d.FechaRegistro, &d.FechaPago,
		); err != nil {
			continue
		}
		deudores = append(deudores, d)
	}
	json.NewEncoder(w).Encode(deudores)
}

// ─── POST ────────────────────────────────────────────────────────────────────
func createDeudor(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	var d Deudor
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido."})
		return
	}

	// Obtener id_usuario desde JWT
	claims, ok := middleware.GetClaims(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Sesión no válida."})
		return
	}
	d.IdUsuario = claims.IdUsuario
	d.IdTienda = GetTiendaIDFromCtxOrDb(db, r)

	if d.NombreDeudor == "" || d.TipoDeuda == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "'nombre_deudor' y 'tipo_deuda' son obligatorios."})
		return
	}
	if d.TipoDeuda != "dinero" && d.TipoDeuda != "producto" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "tipo_deuda debe ser 'dinero' o 'producto'."})
		return
	}
	if d.TipoDeuda == "dinero" && d.MontoDeuda <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Para deuda de dinero, 'monto_deuda' debe ser > 0."})
		return
	}
	if d.TipoDeuda == "producto" && d.DetalleProducto == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Para deuda de producto, 'detalle_producto' es obligatorio."})
		return
	}

	err := db.QueryRow(`
		INSERT INTO operaciones.deudores
		  (id_usuario, id_tienda, nombre_deudor, telefono, tipo_deuda,
		   monto_deuda, monto_abonado, detalle_producto, motivo, estado)
		VALUES ($1, $2, $3, $4, $5, $6, 0, $7, $8, 'pendiente')
		RETURNING id_deuda`,
		d.IdUsuario, d.IdTienda, d.NombreDeudor,
		nullIfEmpty(d.Telefono), d.TipoDeuda, d.MontoDeuda,
		nullIfEmpty(d.DetalleProducto), nullIfEmpty(d.Motivo),
	).Scan(&d.IdDeuda)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al registrar la deuda."})
		return
	}

	d.Estado = "pendiente"
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"mensaje": "Deuda registrada exitosamente.",
		"deuda":   d,
	})
}

// ─── PUT ─────────────────────────────────────────────────────────────────────
func updateDeudor(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "?id es obligatorio."})
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "?id debe ser un entero."})
		return
	}

	var d Deudor
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido."})
		return
	}

	result, err := db.Exec(`
		UPDATE operaciones.deudores
		SET nombre_deudor = $1, telefono = $2, monto_deuda = $3,
		    detalle_producto = $4, motivo = $5
		WHERE id_deuda = $6`,
		d.NombreDeudor, nullIfEmpty(d.Telefono), d.MontoDeuda,
		nullIfEmpty(d.DetalleProducto), nullIfEmpty(d.Motivo), id,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al actualizar la deuda."})
		return
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "Deuda no encontrada."})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"mensaje": "Deuda actualizada exitosamente."})
}

// ─── ABONO ───────────────────────────────────────────────────────────────────
func registrarAbono(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	var input AbonoInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido."})
		return
	}

	if input.IdDeuda <= 0 || input.MontoAbono <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "'id_deuda' e 'monto_abono' (>0) son obligatorios."})
		return
	}

	tx, err := db.Begin()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error iniciando transacción."})
		return
	}
	defer tx.Rollback()

	// Obtener deuda actual con bloqueo
	var montoDeuda, montoAbonado int
	var tipoDeuda, estado string
	err = tx.QueryRow(`
		SELECT monto_deuda, COALESCE(monto_abonado, 0), tipo_deuda, estado
		FROM operaciones.deudores WHERE id_deuda = $1 FOR UPDATE`,
		input.IdDeuda,
	).Scan(&montoDeuda, &montoAbonado, &tipoDeuda, &estado)
	if err == sql.ErrNoRows {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "Deuda no encontrada."})
		return
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error consultando deuda."})
		return
	}

	if estado == "pagado" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Esta deuda ya fue pagada."})
		return
	}

	// Insertar abono
	var idAbono int
	err = tx.QueryRow(`
		INSERT INTO operaciones.abonos_deuda (id_deuda, monto_abono, observacion)
		VALUES ($1, $2, $3) RETURNING id_abono`,
		input.IdDeuda, input.MontoAbono, nullIfEmpty(input.Observacion),
	).Scan(&idAbono)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error registrando abono."})
		return
	}

	// Actualizar monto abonado y estado
	nuevoAbonado := montoAbonado + input.MontoAbono
	nuevoEstado := "parcial"
	if nuevoAbonado >= montoDeuda {
		nuevoEstado = "pagado"
	}

	_, err = tx.Exec(`
		UPDATE operaciones.deudores
		SET monto_abonado = $1, estado = $2,
		    fecha_pago = CASE WHEN $2 = 'pagado' THEN NOW() ELSE fecha_pago END
		WHERE id_deuda = $3`,
		nuevoAbonado, nuevoEstado, input.IdDeuda,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error actualizando deuda."})
		return
	}

	if err := tx.Commit(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error confirmando transacción."})
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"mensaje":        "Abono registrado exitosamente.",
		"id_abono":       idAbono,
		"monto_abonado":  nuevoAbonado,
		"saldo_restante": montoDeuda - nuevoAbonado,
		"estado":         nuevoEstado,
	})
}

// ─── DELETE (Marcar como pagado) ─────────────────────────────────────────────
func pagarDeudor(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "?id es obligatorio."})
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "?id debe ser un entero."})
		return
	}

	result, err := db.Exec(`
		UPDATE operaciones.deudores SET estado = 'pagado', fecha_pago = NOW()
		WHERE id_deuda = $1 AND estado != 'pagado'`, id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al marcar como pagado."})
		return
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "Deuda no encontrada o ya pagada."})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"mensaje": "Deuda marcada como pagada exitosamente."})
}
