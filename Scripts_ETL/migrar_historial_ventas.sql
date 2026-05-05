-- =============================================================================
-- SCRIPT DE MIGRACIÓN v3: Tesis_DB → libreria_los_altares_V2
-- Objetivo: Poblar inventario.movimientos_stock con historial de ventas reales
--           para el motor de predicción (requiere >= 14 días con datos).
--
-- ESTRATEGIA:
--   Los datos en Tesis_DB llegan hasta 2026-02-10. El motor de predicción
--   analiza los ÚLTIMOS 30 días desde hoy. Para que los datos sean detectados,
--   se REFECHAN: el día más reciente de Tesis_DB = hoy, y los anteriores
--   mantienen su diferencia relativa de días.
--
-- EJECUTAR:
--   psql -U postgres -d libreria_los_altares_V2 -f migrar_historial_ventas.sql
-- =============================================================================

CREATE EXTENSION IF NOT EXISTS dblink;

-- Paso 1: Conectar a Tesis_DB
DO $$
BEGIN
  PERFORM dblink_connect(
    'tesis_conn',
    'host=localhost port=5432 dbname=Tesis_DB user=postgres password=cace2004'
  );
  RAISE NOTICE '✓ Conexión a Tesis_DB establecida';
END;
$$;

-- Paso 2: Cruzar productos por nombre entre ambas BDs
DROP TABLE IF EXISTS tmp_cruce_productos;
CREATE TEMP TABLE tmp_cruce_productos AS
SELECT
  dest.id_producto,
  dest.nombre AS nombre_dest,
  origen.cod_item
FROM inventario.productos dest
JOIN dblink(
  'tesis_conn',
  'SELECT cod_item, TRIM(nombre) AS nombre FROM public.inve_items'
) AS origen(cod_item VARCHAR, nombre VARCHAR)
  ON UPPER(TRIM(dest.nombre)) = UPPER(origen.nombre)
WHERE dest.estado = 'activo';

DO $$
DECLARE v_n INT;
BEGIN
  SELECT COUNT(*) INTO v_n FROM tmp_cruce_productos;
  RAISE NOTICE '✓ Productos cruzados: %', v_n;
END;
$$;

-- Paso 3: Traer historial de los últimos 90 días de datos REALES de Tesis_DB
--         y refechalos relativos a hoy.
--
--   fecha_nueva = CURRENT_DATE - (fecha_max_tesis - fecha_original)
--   Así el día más reciente queda = hoy, y los anteriores mantienen su distancia.
DROP TABLE IF EXISTS tmp_ventas_origen;
CREATE TEMP TABLE tmp_ventas_origen AS
SELECT
  cruce.id_producto,
  -- Refecha: desplazar las fechas para que el dato más reciente sea hoy
  (CURRENT_DATE - (raw.fecha_max - raw.fecha_venta))::DATE AS fecha_nueva,
  raw.cantidad_total::INT  AS cantidad_total
FROM dblink(
  'tesis_conn',
  $REMOTE$
    SELECT
      d.cod_item,
      DATE(v.fec_venta)                              AS fecha_venta,
      MAX(DATE(vp.fec_venta)) OVER ()                AS fecha_max,
      SUM(d.cantidad)                                AS cantidad_total
    FROM public.vent_ventas_detalle d
    JOIN public.vent_ventas_principal v
      ON d.cod_venta = v.cod_venta AND d.fec_venta = v.fec_venta
    JOIN (
      SELECT MAX(DATE(fec_venta)) AS fec_venta FROM public.vent_ventas_principal
    ) vp ON TRUE
    WHERE DATE(v.fec_venta) >= (
      SELECT MAX(DATE(fec_venta)) - 89 FROM public.vent_ventas_principal
    )
    AND d.cantidad > 0
    GROUP BY d.cod_item, DATE(v.fec_venta), DATE(vp.fec_venta)
  $REMOTE$
) AS raw(cod_item VARCHAR, fecha_venta DATE, fecha_max DATE, cantidad_total FLOAT)
JOIN tmp_cruce_productos cruce ON cruce.cod_item = raw.cod_item
-- Solo fechas que queden dentro de los últimos 30 días (ventana del motor)
WHERE (CURRENT_DATE - (raw.fecha_max - raw.fecha_venta)) >= (CURRENT_DATE - 30);

DO $$
DECLARE
  v_registros INT;
  v_dias      INT;
  v_productos INT;
BEGIN
  SELECT COUNT(*), COUNT(DISTINCT fecha_nueva), COUNT(DISTINCT id_producto)
  INTO v_registros, v_dias, v_productos
  FROM tmp_ventas_origen;
  RAISE NOTICE '✓ Registros a insertar: % | Días cubiertos: % | Productos: %',
    v_registros, v_dias, v_productos;
END;
$$;

-- Paso 4: Insertar en inventario.movimientos_stock (evitar duplicados)
INSERT INTO inventario.movimientos_stock
  (id_producto, id_usuario, tipo_movimiento, cantidad, stock_resultante,
   referencia_id, fecha_movimiento)
SELECT
  vo.id_producto,
  (SELECT id_usuario FROM seguridad.usuarios WHERE estado = 'activo' ORDER BY id_usuario LIMIT 1),
  'VENTA',
  -vo.cantidad_total,
  0,
  NULL,
  vo.fecha_nueva::TIMESTAMP + INTERVAL '10 hours' -- simular hora de venta
FROM tmp_ventas_origen vo
WHERE NOT EXISTS (
  SELECT 1
  FROM inventario.movimientos_stock ms
  WHERE ms.id_producto      = vo.id_producto
    AND DATE(ms.fecha_movimiento) = vo.fecha_nueva
    AND ms.tipo_movimiento  = 'VENTA'
    AND ms.referencia_id    IS NULL
);

-- Paso 5: Reporte final
DO $$
DECLARE
  v_total     INT;
  v_dias      INT;
  v_productos INT;
BEGIN
  SELECT COUNT(*), COUNT(DISTINCT DATE(fecha_movimiento)), COUNT(DISTINCT id_producto)
  INTO v_total, v_dias, v_productos
  FROM inventario.movimientos_stock
  WHERE tipo_movimiento = 'VENTA' AND referencia_id IS NULL;

  RAISE NOTICE '';
  RAISE NOTICE '========= MIGRACIÓN COMPLETADA =========';
  RAISE NOTICE 'Movimientos migrados : %', v_total;
  RAISE NOTICE 'Días con datos       : %', v_dias;
  RAISE NOTICE 'Productos cubiertos  : %', v_productos;

  IF v_dias >= 14 THEN
    RAISE NOTICE '✓ Motor de predicción HABILITADO (>= 14 días)';
  ELSE
    RAISE WARNING '⚠ Solo % días. Amplía el rango en INTERVAL si es necesario.', v_dias;
  END IF;
  RAISE NOTICE '=========================================';
END;
$$;

-- Cerrar conexión dblink
SELECT dblink_disconnect('tesis_conn');
