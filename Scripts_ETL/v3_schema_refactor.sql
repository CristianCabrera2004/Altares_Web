-- =============================================================================
-- MIGRACIÓN v3: Refactorización del Esquema - Librería Los Altares
-- Base de datos: libreria_los_altares_V2
-- Fecha: 2026-05-19
--
-- Cambios aplicados:
--   1. Crear tabla operaciones.clientes (catálogo normalizado de clientes)
--   2. Agregar FK id_cliente en operaciones.facturas (tabla vacía, sin riesgo)
--   3. Agregar campo fecha_hora_cierre TIMESTAMP en operaciones.cierres_diarios
--   4. Habilitar múltiples cierres por día (documentado; la restricción era en código Go)
--
-- GARANTÍAS:
--   - Idempotente: puede ejecutarse varias veces sin errores (IF NOT EXISTS / IF EXISTS)
--   - Los 1.802 registros de operaciones.ventas NO son alterados
--   - Los permisos de los roles existentes se extienden a la nueva tabla
--
-- EJECUTAR:
--   psql -U postgres -d libreria_los_altares_V2 -f v3_schema_refactor.sql
-- =============================================================================

BEGIN;

-- ─────────────────────────────────────────────────────────────────────────────
-- PASO 1: Crear tabla operaciones.clientes
-- Catálogo normalizado de clientes con Cédula/RUC único.
-- Un único registro "Consumidor Final" (9999999999999) es el placeholder anónimo.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS operaciones.clientes (
    id_cliente  SERIAL PRIMARY KEY,
    cedula_ruc  VARCHAR(15) NOT NULL,
    nombre      VARCHAR(150) NOT NULL,
    direccion   TEXT,
    telefono    VARCHAR(20),
    CONSTRAINT uq_clientes_cedula_ruc UNIQUE (cedula_ruc)
);

DO $$
BEGIN
    RAISE NOTICE '✓ [PASO 1] Tabla operaciones.clientes verificada/creada.';
END;
$$;

-- ─────────────────────────────────────────────────────────────────────────────
-- PASO 2: Insertar cliente "Consumidor Final" por defecto
-- Es el único registro anónimo. La constraint UNIQUE garantiza que no se duplique.
-- ─────────────────────────────────────────────────────────────────────────────
INSERT INTO operaciones.clientes (cedula_ruc, nombre, direccion, telefono)
VALUES ('9999999999999', 'Consumidor Final', NULL, NULL)
ON CONFLICT (cedula_ruc) DO NOTHING;

DO $$
DECLARE v_id INT;
BEGIN
    SELECT id_cliente INTO v_id
    FROM operaciones.clientes
    WHERE cedula_ruc = '9999999999999';
    RAISE NOTICE '✓ [PASO 2] Cliente "Consumidor Final" en id_cliente = %', v_id;
END;
$$;

-- ─────────────────────────────────────────────────────────────────────────────
-- PASO 3: Agregar FK id_cliente en operaciones.facturas
-- La tabla facturas tiene 0 filas → sin riesgo para datos existentes.
-- La columna es NULL-able para mantener compatibilidad con ventas sin datos.
-- Las ventas históricas en operaciones.ventas NO son afectadas.
-- ─────────────────────────────────────────────────────────────────────────────
ALTER TABLE operaciones.facturas
    ADD COLUMN IF NOT EXISTS id_cliente INT
        REFERENCES operaciones.clientes(id_cliente)
        ON DELETE SET NULL;

-- Índice para acelerar los JOINs factura → cliente
CREATE INDEX IF NOT EXISTS idx_facturas_id_cliente
    ON operaciones.facturas(id_cliente);

DO $$
BEGIN
    RAISE NOTICE '✓ [PASO 3] Columna id_cliente añadida a operaciones.facturas con FK.';
END;
$$;

-- ─────────────────────────────────────────────────────────────────────────────
-- PASO 4: Agregar campo fecha_hora_cierre TIMESTAMP en operaciones.cierres_diarios
-- Permite registrar la hora exacta de cada cierre, habilitando múltiples
-- cierres parciales en el mismo día calendario.
-- DEFAULT NOW() asegura que filas existentes (actualmente 0) sean válidas.
-- ─────────────────────────────────────────────────────────────────────────────
ALTER TABLE operaciones.cierres_diarios
    ADD COLUMN IF NOT EXISTS fecha_hora_cierre TIMESTAMP NOT NULL DEFAULT NOW();

DO $$
BEGIN
    RAISE NOTICE '✓ [PASO 4] Columna fecha_hora_cierre añadida a operaciones.cierres_diarios.';
END;
$$;

-- ─────────────────────────────────────────────────────────────────────────────
-- PASO 5: Índice trigrama en clientes.nombre para autocompletado ágil
-- Consistente con el patrón de inventario.productos (idx_productos_nombre)
-- ─────────────────────────────────────────────────────────────────────────────
CREATE INDEX IF NOT EXISTS idx_clientes_nombre
    ON operaciones.clientes USING gin (nombre gin_trgm_ops);

-- Índice B-tree en cedula_ruc para búsqueda exacta rápida
CREATE INDEX IF NOT EXISTS idx_clientes_cedula
    ON operaciones.clientes(cedula_ruc);

DO $$
BEGIN
    RAISE NOTICE '✓ [PASO 5] Índices de búsqueda creados en operaciones.clientes.';
END;
$$;

-- ─────────────────────────────────────────────────────────────────────────────
-- PASO 6: Extender permisos de roles existentes a la nueva tabla
-- Consistente con los GRANTs del init.sql original.
-- ─────────────────────────────────────────────────────────────────────────────
DO $$
BEGIN
    -- admin_libreria ya tiene ALL ON SCHEMA operaciones, hereda automáticamente.
    -- Extender operador_caja explícitamente:
    IF EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'operador_caja') THEN
        EXECUTE 'GRANT SELECT, INSERT, UPDATE ON operaciones.clientes TO operador_caja';
        EXECUTE 'GRANT USAGE, SELECT ON SEQUENCE operaciones.clientes_id_cliente_seq TO operador_caja';
        RAISE NOTICE '✓ [PASO 6] Permisos de operador_caja extendidos a operaciones.clientes.';
    ELSE
        RAISE NOTICE '⚠ [PASO 6] Rol operador_caja no encontrado — omitido.';
    END IF;
END;
$$;

-- ─────────────────────────────────────────────────────────────────────────────
-- VERIFICACIÓN FINAL
-- ─────────────────────────────────────────────────────────────────────────────
DO $$
DECLARE
    v_clientes       INT;
    v_ventas         INT;
    v_facturas       INT;
    v_cierres        INT;
    v_fk_existe      BOOLEAN;
    v_ts_existe      BOOLEAN;
BEGIN
    SELECT COUNT(*) INTO v_clientes   FROM operaciones.clientes;
    SELECT COUNT(*) INTO v_ventas     FROM operaciones.ventas;
    SELECT COUNT(*) INTO v_facturas   FROM operaciones.facturas;
    SELECT COUNT(*) INTO v_cierres    FROM operaciones.cierres_diarios;

    SELECT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'operaciones'
          AND table_name   = 'facturas'
          AND column_name  = 'id_cliente'
    ) INTO v_fk_existe;

    SELECT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'operaciones'
          AND table_name   = 'cierres_diarios'
          AND column_name  = 'fecha_hora_cierre'
    ) INTO v_ts_existe;

    RAISE NOTICE '';
    RAISE NOTICE '╔═══════════════════════════════════════════════════════╗';
    RAISE NOTICE '║         MIGRACIÓN v3 - VERIFICACIÓN FINAL             ║';
    RAISE NOTICE '╠═══════════════════════════════════════════════════════╣';
    RAISE NOTICE '║ operaciones.clientes → registros:        %', v_clientes;
    RAISE NOTICE '║ operaciones.ventas   → registros (OK):   %', v_ventas;
    RAISE NOTICE '║ operaciones.facturas → registros:        %', v_facturas;
    RAISE NOTICE '║ operaciones.cierres_diarios → registros: %', v_cierres;
    RAISE NOTICE '║ FK id_cliente en facturas:               %', v_fk_existe;
    RAISE NOTICE '║ TIMESTAMP en cierres_diarios:            %', v_ts_existe;

    IF v_ventas >= 1802 THEN
        RAISE NOTICE '║ ✓ Integridad de ventas históricas: CONSERVADA';
    ELSE
        RAISE NOTICE '║ ⚠ ALERTA: registros en ventas menores a 1802. Revisar!';
    END IF;

    RAISE NOTICE '╚═══════════════════════════════════════════════════════╝';
END;
$$;

COMMIT;
