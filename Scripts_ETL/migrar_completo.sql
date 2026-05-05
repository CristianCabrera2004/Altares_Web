-- =============================================================================
-- MIGRACIÓN COMPLETA v2: Tesis_DB → libreria_los_altares_V2
-- Módulos:
--   1. UPDATE stock_actual y precio_venta en inventario.productos
--   2. INSERT ventas históricas en operaciones.ventas
--   3. INSERT líneas en operaciones.detalle_ventas
--   4. Reconstruir movimientos_stock tipo 'VENTA' para el motor de predicción
--
-- EJECUTAR:
--   psql -U postgres -d libreria_los_altares_V2 -f migrar_completo.sql
-- =============================================================================

CREATE EXTENSION IF NOT EXISTS dblink;

-- Agregar columna de control para evitar duplicados en re-ejecuciones
ALTER TABLE operaciones.ventas
  ADD COLUMN IF NOT EXISTS referencia_externa VARCHAR(30) DEFAULT NULL;

DROP INDEX IF EXISTS idx_ventas_referencia_externa;
CREATE UNIQUE INDEX idx_ventas_referencia_externa
  ON operaciones.ventas(referencia_externa)
  WHERE referencia_externa IS NOT NULL;

-- ─────────────────────────────────────────────────────────────────────────────
-- CONEXIÓN
-- ─────────────────────────────────────────────────────────────────────────────
DO $$
BEGIN
  PERFORM dblink_connect(
    'tesis',
    'host=localhost port=5432 dbname=Tesis_DB user=postgres password=cace2004'
  );
  RAISE NOTICE '✓ Conexión a Tesis_DB OK';
END;
$$;

-- ─────────────────────────────────────────────────────────────────────────────
-- TABLA AUXILIAR: Cruce de productos por nombre
-- ─────────────────────────────────────────────────────────────────────────────
DROP TABLE IF EXISTS tmp_cruce;
CREATE TEMP TABLE tmp_cruce AS
SELECT
  dest.id_producto,
  dest.id_categoria,
  origen.cod_item,
  origen.stock_origen,
  origen.stock_min,
  origen.pvp_centavos
FROM inventario.productos dest
JOIN dblink('tesis',
  $R$
    SELECT
      cod_item,
      TRIM(nombre)                      AS nombre,
      GREATEST(0, ROUND(stock))::INT    AS stock_origen,
      COALESCE(ROUND(stock_min), 5)::INT AS stock_min,
      ROUND(pvp * 100)::INT             AS pvp_centavos
    FROM public.inve_items
    WHERE estado = 'ACTIVO'
  $R$
) AS origen(cod_item TEXT, nombre TEXT, stock_origen INT, stock_min INT, pvp_centavos INT)
  ON UPPER(TRIM(dest.nombre)) = UPPER(origen.nombre)
WHERE dest.estado = 'activo';

DO $$
DECLARE v_n INT;
BEGIN
  SELECT COUNT(*) INTO v_n FROM tmp_cruce;
  RAISE NOTICE '✓ Productos cruzados: %', v_n;
END;
$$;

-- ═════════════════════════════════════════════════════════════════════════════
-- MÓDULO 1: Actualizar stock_actual y precio_venta
-- ═════════════════════════════════════════════════════════════════════════════
UPDATE inventario.productos dest
SET
  stock_actual     = tc.stock_origen,
  stock_alerta_min = GREATEST(tc.stock_min, 3),
  precio_venta     = CASE WHEN tc.pvp_centavos > 0 THEN tc.pvp_centavos
                          ELSE dest.precio_venta END
FROM tmp_cruce tc
WHERE dest.id_producto = tc.id_producto;

DO $$
DECLARE v_con INT; v_tot INT;
BEGIN
  SELECT COUNT(*), COUNT(CASE WHEN stock_actual > 0 THEN 1 END)
  INTO v_tot, v_con
  FROM inventario.productos WHERE estado = 'activo';
  RAISE NOTICE '✓ [MÓD 1] Stocks: % productos con stock > 0 de % totales', v_con, v_tot;
END;
$$;

-- ═════════════════════════════════════════════════════════════════════════════
-- MÓDULO 2: Migrar ventas → operaciones.ventas
-- Refecha: el día más reciente de Tesis_DB = HOY
-- ═════════════════════════════════════════════════════════════════════════════
DROP TABLE IF EXISTS tmp_ventas;
CREATE TEMP TABLE tmp_ventas AS
SELECT
  src.cod_venta,
  (CURRENT_DATE - (src.fecha_max - DATE(src.fec_venta)))::DATE AS fecha_nueva,
  ROUND(src.total_pagar * 100)::INT  AS total_centavos,
  ROUND(src.impuesto   * 100)::INT   AS iva_centavos
FROM dblink('tesis',
  $R$
    SELECT
      cod_venta,
      fec_venta,
      total_pagar,
      impuesto,
      MAX(DATE(fec_venta)) OVER ()  AS fecha_max
    FROM public.vent_ventas_principal
    WHERE fec_venta >= (
      SELECT MAX(fec_venta) - INTERVAL '90 days' FROM public.vent_ventas_principal
    )
  $R$
) AS src(cod_venta BIGINT, fec_venta TIMESTAMP, total_pagar FLOAT,
         impuesto FLOAT, fecha_max DATE);

DO $$
DECLARE v_n INT; v_d INT;
BEGIN
  SELECT COUNT(*), COUNT(DISTINCT fecha_nueva) INTO v_n, v_d FROM tmp_ventas;
  RAISE NOTICE '✓ [MÓD 2 prep] Ventas a migrar: % en % días', v_n, v_d;
END;
$$;

INSERT INTO operaciones.ventas
  (id_usuario, fecha_venta, subtotal, total_iva, total, estado, referencia_externa)
SELECT
  (SELECT id_usuario FROM seguridad.usuarios WHERE estado = 'activo' ORDER BY id_usuario LIMIT 1),
  tv.fecha_nueva::TIMESTAMP + INTERVAL '9 hours',
  tv.total_centavos - tv.iva_centavos,
  tv.iva_centavos,
  tv.total_centavos,
  'completada',
  'MIG-' || tv.cod_venta::TEXT
FROM tmp_ventas tv
WHERE NOT EXISTS (
  SELECT 1 FROM operaciones.ventas v
  WHERE v.referencia_externa = 'MIG-' || tv.cod_venta::TEXT
);

DO $$
DECLARE v_n INT;
BEGIN
  SELECT COUNT(*) INTO v_n FROM operaciones.ventas WHERE referencia_externa LIKE 'MIG-%';
  RAISE NOTICE '✓ [MÓD 2] Ventas insertadas: %', v_n;
END;
$$;

-- ═════════════════════════════════════════════════════════════════════════════
-- MÓDULO 3: Migrar detalle_ventas
-- ═════════════════════════════════════════════════════════════════════════════
DROP TABLE IF EXISTS tmp_detalles;
CREATE TEMP TABLE tmp_detalles AS
SELECT
  v_dest.id_venta,
  tc.id_producto,
  (SELECT tasa_iva FROM inventario.categorias WHERE id_categoria = tc.id_categoria) AS iva_cat,
  ROUND(src.cantidad)::INT          AS cantidad,
  ROUND(src.pvp * 100)::INT         AS precio_unit
FROM dblink('tesis',
  $R$
    SELECT
      d.cod_venta,
      d.cod_item,
      d.cantidad,
      d.pvp
    FROM public.vent_ventas_detalle d
    WHERE d.cod_venta IN (
      SELECT cod_venta FROM public.vent_ventas_principal
      WHERE fec_venta >= (
        SELECT MAX(fec_venta) - INTERVAL '90 days' FROM public.vent_ventas_principal
      )
    )
    AND d.cantidad > 0
  $R$
) AS src(cod_venta BIGINT, cod_item TEXT, cantidad FLOAT, pvp FLOAT)
JOIN operaciones.ventas v_dest
  ON v_dest.referencia_externa = 'MIG-' || src.cod_venta::TEXT
JOIN tmp_cruce tc
  ON tc.cod_item = src.cod_item;

INSERT INTO operaciones.detalle_ventas
  (id_venta, id_producto, cantidad, precio_unitario, iva_aplicado, subtotal)
SELECT
  td.id_venta,
  td.id_producto,
  td.cantidad,
  td.precio_unit,
  COALESCE(td.iva_cat, 0),
  td.cantidad * td.precio_unit
FROM tmp_detalles td
WHERE NOT EXISTS (
  SELECT 1 FROM operaciones.detalle_ventas dv
  WHERE dv.id_venta = td.id_venta AND dv.id_producto = td.id_producto
);

DO $$
DECLARE v_n INT;
BEGIN
  SELECT COUNT(*) INTO v_n
  FROM operaciones.detalle_ventas dv
  JOIN operaciones.ventas v ON dv.id_venta = v.id_venta
  WHERE v.referencia_externa LIKE 'MIG-%';
  RAISE NOTICE '✓ [MÓD 3] Líneas de detalle: %', v_n;
END;
$$;

-- ═════════════════════════════════════════════════════════════════════════════
-- MÓDULO 4: Reconstruir movimientos_stock tipo VENTA para el motor de predicción
-- ═════════════════════════════════════════════════════════════════════════════

-- Borrar movimientos previos migrados (sin referencia real = script anterior)
DELETE FROM inventario.movimientos_stock
WHERE tipo_movimiento = 'VENTA'
  AND referencia_id IN (
    SELECT id_venta FROM operaciones.ventas WHERE referencia_externa LIKE 'MIG-%'
  );

-- Insertar desde los detalles migrados (últimos 35 días para cubrir el motor)
INSERT INTO inventario.movimientos_stock
  (id_producto, id_usuario, tipo_movimiento, cantidad, stock_resultante, referencia_id, fecha_movimiento)
SELECT
  dv.id_producto,
  v.id_usuario,
  'VENTA',
  -dv.cantidad,
  0,
  dv.id_venta,
  v.fecha_venta
FROM operaciones.detalle_ventas dv
JOIN operaciones.ventas v ON dv.id_venta = v.id_venta
WHERE v.referencia_externa LIKE 'MIG-%'
  AND v.fecha_venta >= NOW() - INTERVAL '35 days';

-- ═════════════════════════════════════════════════════════════════════════════
-- REPORTE FINAL
-- ═════════════════════════════════════════════════════════════════════════════
DO $$
DECLARE
  v_stock     INT;
  v_ventas    INT;
  v_detalles  INT;
  v_dias      INT;
  v_productos INT;
BEGIN
  SELECT COUNT(*) INTO v_stock
  FROM inventario.productos WHERE estado = 'activo' AND stock_actual > 0;

  SELECT COUNT(*) INTO v_ventas
  FROM operaciones.ventas WHERE referencia_externa LIKE 'MIG-%';

  SELECT COUNT(*) INTO v_detalles
  FROM operaciones.detalle_ventas dv
  JOIN operaciones.ventas v ON dv.id_venta = v.id_venta
  WHERE v.referencia_externa LIKE 'MIG-%';

  SELECT COUNT(DISTINCT DATE(fecha_movimiento)), COUNT(DISTINCT id_producto)
  INTO v_dias, v_productos
  FROM inventario.movimientos_stock ms
  JOIN operaciones.ventas v ON ms.referencia_id = v.id_venta
  WHERE v.referencia_externa LIKE 'MIG-%'
    AND ms.tipo_movimiento = 'VENTA';

  RAISE NOTICE '';
  RAISE NOTICE '╔══════════════════════════════════════════╗';
  RAISE NOTICE '║       MIGRACIÓN COMPLETA FINALIZADA      ║';
  RAISE NOTICE '╠══════════════════════════════════════════╣';
  RAISE NOTICE '║ Productos con stock actualizado: %', v_stock;
  RAISE NOTICE '║ Ventas históricas migradas:      %', v_ventas;
  RAISE NOTICE '║ Líneas de detalle migradas:      %', v_detalles;
  RAISE NOTICE '║ Días en ventana del motor:       %', v_dias;
  RAISE NOTICE '║ Productos analizables:           %', v_productos;
  IF v_dias >= 14 THEN
    RAISE NOTICE '║ ✓ Motor de predicción: HABILITADO';
  ELSE
    RAISE NOTICE '║ ⚠ Motor de predicción: necesita más días';
  END IF;
  RAISE NOTICE '╚══════════════════════════════════════════╝';
END;
$$;

SELECT dblink_disconnect('tesis');
