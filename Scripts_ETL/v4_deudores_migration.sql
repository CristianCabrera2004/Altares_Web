-- =============================================================================
-- MIGRACIÓN v4: Módulo de Deudores/Fiados + Email en Clientes
-- Base de datos: libreria_los_altares
-- Fecha: 2026-06-06
--
-- Cambios:
--   1. Agregar columna email en operaciones.clientes (Anexo 3)
--   2. Crear tabla operaciones.deudores (Anexo 4)
--   3. Crear tabla operaciones.abonos_deuda (Anexo 4)
--   4. Extender permisos de roles a nuevas tablas
--
-- EJECUTAR:
--   psql -U postgres -d libreria_los_altares -f v4_deudores_migration.sql
-- =============================================================================

BEGIN;

-- ─────────────────────────────────────────────────────────────────────────────
-- PASO 1: Agregar columna email en operaciones.clientes
-- ─────────────────────────────────────────────────────────────────────────────
ALTER TABLE operaciones.clientes
    ADD COLUMN IF NOT EXISTS email VARCHAR(200);

DO $$ BEGIN RAISE NOTICE '✓ [PASO 1] Columna email añadida a operaciones.clientes.'; END; $$;

-- ─────────────────────────────────────────────────────────────────────────────
-- PASO 2: Crear tabla operaciones.deudores
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS operaciones.deudores (
    id_deuda         SERIAL PRIMARY KEY,
    id_usuario       INT NOT NULL REFERENCES seguridad.usuarios(id_usuario),
    id_tienda        INT NOT NULL REFERENCES configuracion.tiendas(id_tienda),
    nombre_deudor    VARCHAR(200) NOT NULL,
    telefono         VARCHAR(20),
    tipo_deuda       VARCHAR(20) NOT NULL,       -- 'dinero' o 'producto'
    monto_deuda      INT DEFAULT 0,              -- centavos (si dinero)
    monto_abonado    INT DEFAULT 0,              -- total abonado
    detalle_producto TEXT,                         -- descripción (si producto)
    motivo           TEXT,
    estado           VARCHAR(20) DEFAULT 'pendiente', -- pendiente, parcial, pagado
    fecha_registro   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    fecha_pago       TIMESTAMP
);

DO $$ BEGIN RAISE NOTICE '✓ [PASO 2] Tabla operaciones.deudores creada.'; END; $$;

-- ─────────────────────────────────────────────────────────────────────────────
-- PASO 3: Crear tabla operaciones.abonos_deuda
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS operaciones.abonos_deuda (
    id_abono    SERIAL PRIMARY KEY,
    id_deuda    INT NOT NULL REFERENCES operaciones.deudores(id_deuda) ON DELETE CASCADE,
    monto_abono INT NOT NULL,
    observacion TEXT,
    fecha_abono TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

DO $$ BEGIN RAISE NOTICE '✓ [PASO 3] Tabla operaciones.abonos_deuda creada.'; END; $$;

-- ─────────────────────────────────────────────────────────────────────────────
-- PASO 4: Índices para rendimiento
-- ─────────────────────────────────────────────────────────────────────────────
CREATE INDEX IF NOT EXISTS idx_deudores_tienda ON operaciones.deudores(id_tienda);
CREATE INDEX IF NOT EXISTS idx_deudores_estado ON operaciones.deudores(estado);
CREATE INDEX IF NOT EXISTS idx_abonos_deuda ON operaciones.abonos_deuda(id_deuda);

DO $$ BEGIN RAISE NOTICE '✓ [PASO 4] Índices creados.'; END; $$;

-- ─────────────────────────────────────────────────────────────────────────────
-- PASO 5: Extender permisos de roles
-- ─────────────────────────────────────────────────────────────────────────────
DO $$
BEGIN
    IF EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'operador_caja') THEN
        EXECUTE 'GRANT SELECT, INSERT, UPDATE ON operaciones.deudores TO operador_caja';
        EXECUTE 'GRANT USAGE, SELECT ON SEQUENCE operaciones.deudores_id_deuda_seq TO operador_caja';
        EXECUTE 'GRANT SELECT, INSERT ON operaciones.abonos_deuda TO operador_caja';
        EXECUTE 'GRANT USAGE, SELECT ON SEQUENCE operaciones.abonos_deuda_id_abono_seq TO operador_caja';
        RAISE NOTICE '✓ [PASO 5] Permisos extendidos para operador_caja.';
    END IF;
END;
$$;

-- ─────────────────────────────────────────────────────────────────────────────
-- VERIFICACIÓN
-- ─────────────────────────────────────────────────────────────────────────────
DO $$
DECLARE
    v_deudores BOOLEAN;
    v_abonos   BOOLEAN;
    v_email    BOOLEAN;
BEGIN
    SELECT EXISTS (SELECT 1 FROM information_schema.tables
        WHERE table_schema = 'operaciones' AND table_name = 'deudores') INTO v_deudores;
    SELECT EXISTS (SELECT 1 FROM information_schema.tables
        WHERE table_schema = 'operaciones' AND table_name = 'abonos_deuda') INTO v_abonos;
    SELECT EXISTS (SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'operaciones' AND table_name = 'clientes'
        AND column_name = 'email') INTO v_email;

    RAISE NOTICE '';
    RAISE NOTICE '╔═══════════════════════════════════════════════╗';
    RAISE NOTICE '║    MIGRACIÓN v4 — VERIFICACIÓN FINAL          ║';
    RAISE NOTICE '╠═══════════════════════════════════════════════╣';
    RAISE NOTICE '║ operaciones.deudores:      %', v_deudores;
    RAISE NOTICE '║ operaciones.abonos_deuda:  %', v_abonos;
    RAISE NOTICE '║ email en clientes:         %', v_email;
    RAISE NOTICE '╚═══════════════════════════════════════════════╝';
END;
$$;

COMMIT;
