-- ============================================================
-- SCRIPT MAESTRO FINAL - BASE DE DATOS: LIBRERÍA LOS ALTARES
-- Motor: PostgreSQL 15+
-- Arquitectura: Esquemas, RBAC, Pgcrypto Nativo y Precios en INT
-- ============================================================

-- Paso 1: Crear base de datos (Ejecutar primero si no existe, luego conectarse)
-- CREATE DATABASE libreria_los_altares;
-- \c libreria_los_altares;

-- ==========================================
-- 1. CONFIGURACIÓN INICIAL DE SEGURIDAD
-- ==========================================

-- Bloquear el esquema público por defecto
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
REVOKE ALL ON SCHEMA public FROM PUBLIC;

-- Habilitar extensión para encriptación nativa de contraseñas (Bcrypt)
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Habilitar extensión para búsquedas de texto predictivas (Autocompletado ágil en el frontend)
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Crear esquemas lógicos
CREATE SCHEMA seguridad;
CREATE SCHEMA inventario;
CREATE SCHEMA operaciones;

-- ==========================================
-- 2. ESQUEMA: SEGURIDAD (Acceso Restringido)
-- ==========================================

CREATE TABLE seguridad.usuarios (
    id_usuario SERIAL PRIMARY KEY,
    nombre VARCHAR(100) NOT NULL,
    email VARCHAR(150) UNIQUE NOT NULL,
    contrasena_hash VARCHAR(255) NOT NULL,
    rol VARCHAR(20) NOT NULL,
    estado VARCHAR(20) NOT NULL DEFAULT 'activo',
    fecha_creacion TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    ultima_sesion TIMESTAMP
);

CREATE TABLE seguridad.sesiones (
    id_sesion SERIAL PRIMARY KEY,
    id_usuario INT NOT NULL REFERENCES seguridad.usuarios(id_usuario) ON DELETE CASCADE,
    token_jwt TEXT NOT NULL,
    fecha_inicio TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    fecha_expiracion TIMESTAMP NOT NULL,
    ip_origen VARCHAR(45),
    activa BOOLEAN DEFAULT TRUE
);

CREATE TABLE seguridad.logs_auditoria (
    id_log SERIAL PRIMARY KEY,
    id_usuario INT NOT NULL REFERENCES seguridad.usuarios(id_usuario),
    accion VARCHAR(50) NOT NULL,
    tabla_afectada VARCHAR(100) NOT NULL,
    id_registro_afectado INT,
    valor_anterior TEXT,
    valor_nuevo TEXT,
    ip_origen VARCHAR(45),
    fecha TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- ==========================================
-- 3. ESQUEMA: INVENTARIO
-- ==========================================

CREATE TABLE inventario.categorias (
    id_categoria SERIAL PRIMARY KEY,
    nombre VARCHAR(100) NOT NULL,
    detalle TEXT,
    tasa_iva INT NOT NULL DEFAULT 0
);

CREATE TABLE inventario.proveedores (
    id_proveedor SERIAL PRIMARY KEY,
    identificacion VARCHAR(20) UNIQUE NOT NULL,
    nombre_proveedor VARCHAR(150) NOT NULL,
    contacto VARCHAR(100),
    email VARCHAR(150),
    telefono VARCHAR(20)
);

CREATE TABLE inventario.productos (
    id_producto SERIAL PRIMARY KEY,
    nombre VARCHAR(200) NOT NULL,
    id_categoria INT NOT NULL REFERENCES inventario.categorias(id_categoria),
    stock_actual INT NOT NULL DEFAULT 0,
    stock_alerta_min INT NOT NULL DEFAULT 5,
    precio_venta INT NOT NULL, -- Manejado en INT (centavos)
    estado VARCHAR(20) NOT NULL DEFAULT 'activo'
);

CREATE TABLE inventario.codigos_barras (
    id_codigo SERIAL PRIMARY KEY,
    id_producto INT NOT NULL REFERENCES inventario.productos(id_producto) ON DELETE CASCADE,
    codigo VARCHAR(50) UNIQUE NOT NULL
);

CREATE TABLE inventario.ingreso_inventario (
    id_ingreso SERIAL PRIMARY KEY,
    id_producto INT NOT NULL REFERENCES inventario.productos(id_producto),
    id_proveedor INT REFERENCES inventario.proveedores(id_proveedor),
    id_usuario INT NOT NULL REFERENCES seguridad.usuarios(id_usuario),
    cantidad_ingresada INT NOT NULL,
    costo_unitario INT NOT NULL,
    fecha_ingreso TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    observacion TEXT
);

CREATE TABLE inventario.bajas_inventario (
    id_baja SERIAL PRIMARY KEY,
    id_producto INT NOT NULL REFERENCES inventario.productos(id_producto),
    id_usuario INT NOT NULL REFERENCES seguridad.usuarios(id_usuario),
    cantidad_baja INT NOT NULL,
    motivo VARCHAR(50) NOT NULL,
    fecha_baja TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE inventario.pronosticos_demanda (
    id_pronostico SERIAL PRIMARY KEY,
    id_producto INT NOT NULL REFERENCES inventario.productos(id_producto),
    id_usuario INT NOT NULL REFERENCES seguridad.usuarios(id_usuario),
    fecha_proyeccion_inicio DATE NOT NULL,
    fecha_proyeccion_fin DATE NOT NULL,
    cantidad_estimada INT NOT NULL,
    margen_error DECIMAL(5,4),
    fecha_generacion TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- ==========================================
-- 4. ESQUEMA: OPERACIONES
-- ==========================================

CREATE TABLE operaciones.cierres_diarios (
    id_cierre SERIAL PRIMARY KEY,
    id_usuario INT NOT NULL REFERENCES seguridad.usuarios(id_usuario),
    fecha_cierre DATE NOT NULL,
    total_recaudado INT NOT NULL DEFAULT 0,
    estado VARCHAR(20) NOT NULL DEFAULT 'cuadrado'
);

CREATE TABLE operaciones.ventas (
    id_venta SERIAL PRIMARY KEY,
    id_usuario INT NOT NULL REFERENCES seguridad.usuarios(id_usuario),
    fecha_venta TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    subtotal INT NOT NULL,
    total_iva INT NOT NULL DEFAULT 0,
    total INT NOT NULL,
    estado VARCHAR(20) NOT NULL DEFAULT 'completada'
);

CREATE TABLE operaciones.detalle_ventas (
    id_detalle SERIAL PRIMARY KEY,
    id_venta INT NOT NULL REFERENCES operaciones.ventas(id_venta) ON DELETE CASCADE,
    id_producto INT NOT NULL REFERENCES inventario.productos(id_producto),
    cantidad INT NOT NULL,
    precio_unitario INT NOT NULL,
    iva_aplicado INT NOT NULL DEFAULT 0,
    subtotal INT NOT NULL
);

CREATE TABLE operaciones.devoluciones (
    id_devolucion SERIAL PRIMARY KEY,
    id_venta INT NOT NULL REFERENCES operaciones.ventas(id_venta),
    id_producto INT NOT NULL REFERENCES inventario.productos(id_producto),
    id_usuario INT NOT NULL REFERENCES seguridad.usuarios(id_usuario),
    cantidad_devuelta INT NOT NULL,
    motivo TEXT,
    fecha_devolucion TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE operaciones.tipo_factura (
    id_tipo_factura SERIAL PRIMARY KEY,
    nombre VARCHAR(100) NOT NULL,
    descripcion TEXT
);

CREATE TABLE operaciones.facturas (
    id_factura SERIAL PRIMARY KEY,
    id_venta INT NOT NULL REFERENCES operaciones.ventas(id_venta),
    id_tipo_factura INT NOT NULL REFERENCES operaciones.tipo_factura(id_tipo_factura),
    cliente_identificacion VARCHAR(20) NOT NULL DEFAULT '9999999999999',
    cliente_nombre VARCHAR(150) NOT NULL DEFAULT 'Consumidor Final',
    archivo_pdf VARCHAR(255),
    fecha_emision TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE inventario.movimientos_stock (
    id_movimiento SERIAL PRIMARY KEY,
    id_producto INT NOT NULL REFERENCES inventario.productos(id_producto),
    id_usuario INT NOT NULL REFERENCES seguridad.usuarios(id_usuario),
    tipo_movimiento VARCHAR(20) NOT NULL,
    cantidad INT NOT NULL,
    stock_resultante INT NOT NULL,
    referencia_id INT,
    fecha_movimiento TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- ==========================================
-- 5. CREACIÓN DE ROLES Y USUARIOS
-- ==========================================

DO $$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'admin_libreria') THEN
    CREATE ROLE admin_libreria;
  END IF;
  IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'operador_caja') THEN
    CREATE ROLE operador_caja;
  END IF;
END
$$;

GRANT ALL ON SCHEMA seguridad, inventario, operaciones TO admin_libreria;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA seguridad, inventario, operaciones TO admin_libreria;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA seguridad, inventario, operaciones TO admin_libreria;

GRANT USAGE ON SCHEMA seguridad, inventario, operaciones TO operador_caja;

GRANT SELECT, INSERT, UPDATE ON ALL TABLES IN SCHEMA inventario TO operador_caja;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA inventario TO operador_caja;
REVOKE DELETE ON ALL TABLES IN SCHEMA inventario FROM operador_caja;

GRANT SELECT, INSERT, UPDATE ON ALL TABLES IN SCHEMA operaciones TO operador_caja;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA operaciones TO operador_caja;
REVOKE DELETE ON ALL TABLES IN SCHEMA operaciones FROM operador_caja;

GRANT SELECT ON seguridad.usuarios TO operador_caja;
GRANT SELECT, INSERT, UPDATE ON seguridad.sesiones TO operador_caja;
GRANT USAGE, SELECT ON SEQUENCE seguridad.sesiones_id_sesion_seq TO operador_caja;
GRANT INSERT ON seguridad.logs_auditoria TO operador_caja;
GRANT USAGE, SELECT ON SEQUENCE seguridad.logs_auditoria_id_log_seq TO operador_caja;

DROP USER IF EXISTS app_backend_go;
CREATE USER app_backend_go WITH PASSWORD 'SuperSecretaApp2026!';
GRANT admin_libreria TO app_backend_go;

-- ==========================================
-- 6. INSERCIÓN DE DATOS INICIALES (SEEDING)
-- ==========================================

INSERT INTO seguridad.usuarios (nombre, email, contrasena_hash, rol)
VALUES (
    'Administrador Principal',
    'admin@losaltares.com',
    crypt('Admin123!', gen_salt('bf', 10)),
    'admin_libreria'
);

INSERT INTO operaciones.tipo_factura (nombre, descripcion)
VALUES
    ('Consumidor Final', 'Ventas menores sin datos requeridos'),
    ('Factura con Datos', 'Venta con RUC o Cédula registrada'),
    ('Cierre de Jornada (Cuaderno)', 'Registro masivo de ventas del día');

INSERT INTO inventario.categorias (nombre, detalle, tasa_iva)
VALUES
    ('Papelería', 'Cuadernos, esferos, útiles escolares', 0),
    ('Golosinas y Snacks', 'Chocolates, galletas, bebidas', 15),
    ('Novedades y Regalos', 'Mercadería general y envolturas', 15);

-- ==========================================
-- 7. ÍNDICES PARA RENDIMIENTO < 200ms (CA 46)
-- ==========================================

-- Búsqueda exacta por código de barras
CREATE INDEX idx_codigo_barras ON inventario.codigos_barras(codigo);

-- Búsqueda predictiva por nombre de producto (requiere pg_trgm)
CREATE INDEX idx_productos_nombre ON inventario.productos USING gin (nombre gin_trgm_ops);

-- Consultas del motor de pronóstico por fechas
CREATE INDEX idx_movimientos_fecha ON inventario.movimientos_stock(fecha_movimiento);

-- Índice de productos activos (consultas frecuentes del catálogo)
CREATE INDEX idx_productos_estado ON inventario.productos(estado);

-- ==========================================
-- 8. VISTA MATERIALIZADA DE VENTAS DIARIAS
-- ==========================================

CREATE MATERIALIZED VIEW operaciones.ventas_diarias_mv AS
SELECT
    id_producto,
    DATE(fecha_movimiento) as fecha,
    SUM(ABS(cantidad)) as demanda_diaria
FROM inventario.movimientos_stock
WHERE tipo_movimiento = 'VENTA'
GROUP BY id_producto, DATE(fecha_movimiento);
