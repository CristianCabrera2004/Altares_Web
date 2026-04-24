// Backend/handlers/sales_handler.go
// ─────────────────────────────────────────────────────────────────────────────
// Endpoints transaccionales de ventas (CA 45: BEGIN/COMMIT/ROLLBACK):
//
//   POST /api/ventas           → Registra una venta unitaria
//   POST /api/ventas/cuaderno  → Carga masiva del cuaderno del día (BULK)
//
// El "cuaderno" es el caso de uso crítico del CA 45: se recibe un array de
// productos vendidos durante el día y se inserta TODO en una sola transacción.
// Si falla un solo producto → ROLLBACK completo. Nada queda sin consistencia.
//
// procesarVenta() es la función atómica interna compartida por ambos handlers.
// ─────────────────────────────────────────────────────────────────────────────
package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
)

// DetalleVentaInput representa una línea del pedido.
// IvaAplicado es 0 ó 15 — coincide con tasa_iva de inventario.categorias (HU-01 CA 3).
type DetalleVentaInput struct {
	IdProducto     int `json:"id_producto"`
	Cantidad       int `json:"cantidad"`
	PrecioUnitario int `json:"precio_unitario"` // base sin IVA, en centavos
	IvaAplicado    int `json:"iva_aplicado"`    // 0 ó 15
}

// VentaInput es el cuerpo de POST /api/ventas.
type VentaInput struct {
	IdUsuario int                 `json:"id_usuario"`
	Items     []DetalleVentaInput `json:"items"`
}

// CuadernoInput es el cuerpo de POST /api/ventas/cuaderno (carga masiva).
type CuadernoInput struct {
	IdUsuario     int                 `json:"id_usuario"`
	Items         []DetalleVentaInput `json:"items"`
	ClienteId     string              `json:"cliente_identificacion"`
	ClienteNombre string              `json:"cliente_nombre"`
}

// ─── POST /api/ventas ────────────────────────────────────────────────────────
// SalesHandler registra una venta individual de forma transaccional.
func SalesHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Solo se acepta POST en este endpoint."})
			return
		}

		var input VentaInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido o malformado."})
			return
		}

		// Validaciones → 400 (CA 45)
		if input.IdUsuario <= 0 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "'id_usuario' es obligatorio y debe ser positivo."})
			return
		}
		if len(input.Items) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "La venta debe incluir al menos un producto en 'items'."})
			return
		}
		for i, item := range input.Items {
			if item.IdProducto <= 0 || item.Cantidad <= 0 || item.PrecioUnitario < 0 {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error":       "Cada item requiere 'id_producto', 'cantidad' (>0) y 'precio_unitario' (>=0) válidos.",
					"item_indice": i,
				})
				return
			}
		}

		idVenta, total, err := procesarVenta(db, input.IdUsuario, input.Items,
			"9999999999999", "Consumidor Final")
		if err != nil {
			httpCode := http.StatusInternalServerError
			if err.Error() == "stock_insuficiente" || err.Error() == "producto_no_encontrado" {
				httpCode = http.StatusBadRequest
			}
			w.WriteHeader(httpCode)
			json.NewEncoder(w).Encode(map[string]string{"error": tradError(err.Error())})
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"mensaje":  "Venta registrada exitosamente.",
			"id_venta": idVenta,
			"total":    total,
		})
	}
}

// ─── POST /api/ventas/cuaderno ───────────────────────────────────────────────
// CuadernoHandler implementa la CARGA MASIVA del cuaderno de ventas del día.
//
// Este es el endpoint transaccional crítico del CA 45: se recibe un array
// completo de ítems y se procesa TODO en una única transacción SQL atómica.
// Si falla CUALQUIER producto → ROLLBACK de TODO el cuaderno.
// Éxito total → COMMIT. No hay estados intermedios.
func CuadernoHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Solo se acepta POST en este endpoint."})
			return
		}

		var input CuadernoInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido o malformado."})
			return
		}

		// Validaciones de entrada → 400 (CA 45)
		if input.IdUsuario <= 0 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "'id_usuario' es obligatorio."})
			return
		}
		if len(input.Items) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "El cuaderno debe contener al menos un producto."})
			return
		}

		// Validar todos los ítems antes de abrir la transacción
		for i, item := range input.Items {
			if item.IdProducto <= 0 || item.Cantidad <= 0 || item.PrecioUnitario < 0 {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error": fmt.Sprintf(
						"El ítem en posición %d es inválido: 'id_producto', 'cantidad' (>0) y 'precio_unitario' (>=0) son obligatorios.", i,
					),
					"item_indice": i,
				})
				return
			}
		}

		// Asignar valores por defecto para cliente
		if input.ClienteId == "" {
			input.ClienteId = "9999999999999"
		}
		if input.ClienteNombre == "" {
			input.ClienteNombre = "Consumidor Final"
		}

		// ════════════════════════════════════════════════════════════════════
		// TRANSACCIÓN MASIVA DEL CUADERNO (CA 45)
		// Todo el array de ítems se registra en UNA SOLA transacción SQL.
		// Garantía: o todas las ventas se registran, o ninguna.
		// ════════════════════════════════════════════════════════════════════
		idVenta, total, err := procesarVenta(db, input.IdUsuario, input.Items,
			input.ClienteId, input.ClienteNombre)
		if err != nil {
			httpCode := http.StatusInternalServerError
			if err.Error() == "stock_insuficiente" || err.Error() == "producto_no_encontrado" {
				httpCode = http.StatusBadRequest
			}
			w.WriteHeader(httpCode)
			json.NewEncoder(w).Encode(map[string]string{"error": tradError(err.Error())})
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"mensaje":        "Cuaderno del día cargado exitosamente como venta de jornada.",
			"id_venta":       idVenta,
			"total":          total,
			"items_cargados": len(input.Items),
		})
	}
}

// ─── FUNCIÓN INTERNA TRANSACCIONAL ───────────────────────────────────────────
// procesarVenta ejecuta los 4 pasos de una venta dentro de una SOLA transacción:
//  1. INSERT en operaciones.ventas            (cabecera)
//  2. Para cada item:
//     a. SELECT FOR UPDATE del stock          (bloqueo de fila, evita concurrencia)
//     b. Validar stock suficiente             (error 400 → ROLLBACK)
//     c. INSERT en operaciones.detalle_ventas
//     d. UPDATE inventario.productos.stock_actual
//     e. INSERT en inventario.movimientos_stock
//
// Retorna (id_venta, total_centavos, error).
// Los errores "stock_insuficiente" y "producto_no_encontrado" deben mapear a HTTP 400.
func procesarVenta(db *sql.DB, idUsuario int, items []DetalleVentaInput, clienteId, clienteNombre string) (int, int, error) {
	// Calcular el total CON IVA antes de abrir la transacción (HU-01 CA 3)
	// IVA se calcula con redondeo bancario para precisión en centavos.
	var subtotalBase int // suma de precios sin IVA
	var totalIva     int // suma de IVA calculado
	for _, item := range items {
		lineBase := item.PrecioUnitario * item.Cantidad
		lineIva  := int(math.Round(float64(lineBase) * float64(item.IvaAplicado) / 100.0))
		subtotalBase += lineBase
		totalIva     += lineIva
	}
	totalConIva := subtotalBase + totalIva

	// ══════════════════════════════════════════════════════════════
	// BEGIN — Inicio de la transacción atómica
	// ══════════════════════════════════════════════════════════════
	tx, err := db.Begin()
	if err != nil {
		return 0, 0, fmt.Errorf("error_transaccion")
	}
	defer tx.Rollback() // ROLLBACK garantizado si no se llega a Commit()

	// Paso 1 — Insertar la cabecera de la venta con totales correctos (IVA diferenciado)
	var idVenta int
	err = tx.QueryRow(`
		INSERT INTO operaciones.ventas (id_usuario, subtotal, total_iva, total, estado)
		VALUES ($1, $2, $3, $4, 'completada')
		RETURNING id_venta`,
		idUsuario, subtotalBase, totalIva, totalConIva,
	).Scan(&idVenta)
	if err != nil {
		return 0, 0, fmt.Errorf("error_creando_venta")
	}

	// Paso 2 — Procesar cada ítem del cuaderno/venta
	for _, item := range items {
		// Paso 2a — Bloquear la fila del producto (SELECT FOR UPDATE)
		var stockActual int
		err = tx.QueryRow(
			`SELECT stock_actual FROM inventario.productos WHERE id_producto = $1 FOR UPDATE`,
			item.IdProducto,
		).Scan(&stockActual)
		if err == sql.ErrNoRows {
			return 0, 0, fmt.Errorf("producto_no_encontrado")
		}
		if err != nil {
			return 0, 0, fmt.Errorf("error_consultando_producto")
		}

		// Paso 2b — Validar stock suficiente
		if item.Cantidad > stockActual {
			return 0, 0, fmt.Errorf("stock_insuficiente")
		}

		// IVA y total de la línea (HU-01 CA 3: IVA diferenciado por categoría)
		lineBase    := item.PrecioUnitario * item.Cantidad
		lineIva     := int(math.Round(float64(lineBase) * float64(item.IvaAplicado) / 100.0))
		lineTotalConIva := lineBase + lineIva
		nuevoStock  := stockActual - item.Cantidad

		// Paso 2c — Insertar línea de detalle con IVA real
		_, err = tx.Exec(`
			INSERT INTO operaciones.detalle_ventas
			  (id_venta, id_producto, cantidad, precio_unitario, iva_aplicado, subtotal)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			idVenta, item.IdProducto, item.Cantidad, item.PrecioUnitario, item.IvaAplicado, lineTotalConIva,
		)
		if err != nil {
			return 0, 0, fmt.Errorf("error_insertando_detalle")
		}

		// Paso 2d — Actualizar stock del producto
		_, err = tx.Exec(
			`UPDATE inventario.productos SET stock_actual = $1 WHERE id_producto = $2`,
			nuevoStock, item.IdProducto,
		)
		if err != nil {
			return 0, 0, fmt.Errorf("error_actualizando_stock")
		}

		// Paso 2e — Registrar movimiento (cantidad negativa = salida)
		_, err = tx.Exec(`
			INSERT INTO inventario.movimientos_stock
			  (id_producto, id_usuario, tipo_movimiento, cantidad, stock_resultante, referencia_id)
			VALUES ($1, $2, 'VENTA', $3, $4, $5)`,
			item.IdProducto, idUsuario, -item.Cantidad, nuevoStock, idVenta,
		)
		if err != nil {
			return 0, 0, fmt.Errorf("error_registrando_movimiento")
		}
	}

	// COMMIT — todos los pasos fueron exitosos (CA 4)
	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("error_confirmando_transaccion")
	}

	return idVenta, totalConIva, nil
}

// tradError convierte códigos internos de error a mensajes legibles en español.
func tradError(code string) string {
	msgs := map[string]string{
		"stock_insuficiente":            "Stock insuficiente para uno o más productos. Verifique el cuaderno.",
		"producto_no_encontrado":        "Uno o más productos del cuaderno no existen en el catálogo.",
		"error_transaccion":             "Error interno al iniciar la transacción SQL.",
		"error_creando_venta":           "Error interno al crear el registro de venta.",
		"error_consultando_producto":    "Error interno al consultar un producto.",
		"error_insertando_detalle":      "Error interno al insertar el detalle de la venta.",
		"error_actualizando_stock":      "Error interno al actualizar el stock.",
		"error_registrando_movimiento":  "Error interno al registrar el movimiento de stock.",
		"error_confirmando_transaccion": "Error interno al confirmar la transacción. Se realizó ROLLBACK.",
	}
	if msg, ok := msgs[code]; ok {
		return msg
	}
	return code
}
