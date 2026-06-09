-- ============================================================
-- SCRIPT MAESTRO FINAL - BASE DE DATOS: LIBRERÍA LOS ALTARES
-- Motor: PostgreSQL 15+
-- Arquitectura: Esquemas, RBAC, Pgcrypto Nativo, Precios en INT
--               y SOPORTE MULTITIENDA (inventario y ventas por tienda)
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
CREATE SCHEMA configuracion;
CREATE SCHEMA seguridad;
CREATE SCHEMA inventario;
CREATE SCHEMA operaciones;

-- ==========================================
-- 1.5 ESQUEMA: CONFIGURACIÓN (Tiendas)
-- ==========================================

CREATE TABLE configuracion.tiendas (
    id_tienda   SERIAL PRIMARY KEY,
    nombre      VARCHAR(150) NOT NULL,
    direccion   TEXT,
    telefono    VARCHAR(20),
    estado      VARCHAR(20) NOT NULL DEFAULT 'activa'
);

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
    -- Cada operador de caja se asigna a una tienda fija.
    -- Administradores pueden tener NULL (acceso global).
    id_tienda INT REFERENCES configuracion.tiendas(id_tienda),
    fecha_creacion TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    ultima_sesion TIMESTAMP,
    two_factor_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    two_factor_secret VARCHAR(100) DEFAULT NULL
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

-- Catálogo centralizado de productos (compartido entre tiendas).
-- El stock ya NO está aquí — se maneja en inventario.stock_tiendas.
CREATE TABLE inventario.productos (
    id_producto SERIAL PRIMARY KEY,
    nombre VARCHAR(200) NOT NULL,
    id_categoria INT NOT NULL REFERENCES inventario.categorias(id_categoria),
    precio_venta INT NOT NULL, -- Manejado en INT (centavos)
    estado VARCHAR(20) NOT NULL DEFAULT 'activo'
);

CREATE TABLE inventario.codigos_barras (
    id_codigo SERIAL PRIMARY KEY,
    id_producto INT NOT NULL REFERENCES inventario.productos(id_producto) ON DELETE CASCADE,
    codigo VARCHAR(50) UNIQUE NOT NULL
);

-- ==========================================
-- 3.5 INVENTARIO POR TIENDA (stock separado)
-- ==========================================

-- Cada fila representa el stock de un producto en UNA tienda específica.
-- Se reemplaza el antiguo stock_actual / stock_alerta_min de inventario.productos.
CREATE TABLE inventario.stock_tiendas (
    id_stock_tienda SERIAL PRIMARY KEY,
    id_tienda       INT NOT NULL REFERENCES configuracion.tiendas(id_tienda),
    id_producto     INT NOT NULL REFERENCES inventario.productos(id_producto),
    stock_actual    INT NOT NULL DEFAULT 0,
    stock_alerta_min INT NOT NULL DEFAULT 5,
    UNIQUE (id_tienda, id_producto)
);

-- ==========================================
-- 3.6 INVENTARIO TRANSACCIONAL (por tienda)
-- ==========================================

CREATE TABLE inventario.ingreso_inventario (
    id_ingreso SERIAL PRIMARY KEY,
    id_producto INT NOT NULL REFERENCES inventario.productos(id_producto),
    id_proveedor INT REFERENCES inventario.proveedores(id_proveedor),
    id_usuario INT NOT NULL REFERENCES seguridad.usuarios(id_usuario),
    id_tienda INT NOT NULL REFERENCES configuracion.tiendas(id_tienda),
    cantidad_ingresada INT NOT NULL,
    costo_unitario INT NOT NULL,
    fecha_ingreso TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    observacion TEXT
);

CREATE TABLE inventario.bajas_inventario (
    id_baja SERIAL PRIMARY KEY,
    id_producto INT NOT NULL REFERENCES inventario.productos(id_producto),
    id_usuario INT NOT NULL REFERENCES seguridad.usuarios(id_usuario),
    id_tienda INT NOT NULL REFERENCES configuracion.tiendas(id_tienda),
    cantidad_baja INT NOT NULL,
    motivo VARCHAR(50) NOT NULL,
    fecha_baja TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE inventario.pronosticos_demanda (
    id_pronostico SERIAL PRIMARY KEY,
    id_producto INT NOT NULL REFERENCES inventario.productos(id_producto),
    id_usuario INT NOT NULL REFERENCES seguridad.usuarios(id_usuario),
    id_tienda INT NOT NULL REFERENCES configuracion.tiendas(id_tienda),
    fecha_proyeccion_inicio DATE NOT NULL,
    fecha_proyeccion_fin DATE NOT NULL,
    cantidad_estimada INT NOT NULL,
    margen_error DECIMAL(5,4),
    fecha_generacion TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE inventario.movimientos_stock (
    id_movimiento SERIAL PRIMARY KEY,
    id_producto INT NOT NULL REFERENCES inventario.productos(id_producto),
    id_usuario INT NOT NULL REFERENCES seguridad.usuarios(id_usuario),
    id_tienda INT NOT NULL REFERENCES configuracion.tiendas(id_tienda),
    tipo_movimiento VARCHAR(20) NOT NULL,
    cantidad INT NOT NULL,
    stock_resultante INT NOT NULL,
    referencia_id INT,
    fecha_movimiento TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- ==========================================
-- 4. ESQUEMA: OPERACIONES
-- ==========================================

-- Catálogo de clientes (HU-Clientes): cedula_ruc es UNIQUE para evitar duplicados.
-- Un único registro '9999999999999' representa las ventas anónimas (Consumidor Final).
CREATE TABLE operaciones.clientes (
    id_cliente  SERIAL PRIMARY KEY,
    cedula_ruc  VARCHAR(15) NOT NULL,
    nombre      VARCHAR(150) NOT NULL,
    direccion   TEXT,
    telefono    VARCHAR(20),
    email       VARCHAR(150),
    CONSTRAINT uq_clientes_cedula_ruc UNIQUE (cedula_ruc)
);

CREATE TABLE operaciones.cierres_diarios (
    id_cierre         SERIAL PRIMARY KEY,
    id_usuario        INT NOT NULL REFERENCES seguridad.usuarios(id_usuario),
    id_tienda         INT NOT NULL REFERENCES configuracion.tiendas(id_tienda),
    fecha_cierre      DATE NOT NULL,
    total_recaudado   INT NOT NULL DEFAULT 0,
    estado            VARCHAR(20) NOT NULL DEFAULT 'cuadrado',
    -- Timestamp exacto del cierre: permite registrar múltiples cierres por día
    fecha_hora_cierre TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE operaciones.ventas (
    id_venta SERIAL PRIMARY KEY,
    id_usuario INT NOT NULL REFERENCES seguridad.usuarios(id_usuario),
    id_tienda INT NOT NULL REFERENCES configuracion.tiendas(id_tienda),
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
    id_venta INT REFERENCES operaciones.ventas(id_venta),
    id_producto INT NOT NULL REFERENCES inventario.productos(id_producto),
    id_usuario INT NOT NULL REFERENCES seguridad.usuarios(id_usuario),
    id_tienda INT NOT NULL REFERENCES configuracion.tiendas(id_tienda),
    cantidad_devuelta INT NOT NULL,
    motivo TEXT,
    tipo VARCHAR(20) NOT NULL DEFAULT 'DEVOLUCION',
    en_mal_estado BOOLEAN NOT NULL DEFAULT FALSE,
    id_producto_cambio INT REFERENCES inventario.productos(id_producto),
    cantidad_cambio INT,
    diferencia_precio INT DEFAULT 0,
    fecha_devolucion TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE operaciones.tipo_factura (
    id_tipo_factura SERIAL PRIMARY KEY,
    nombre VARCHAR(100) NOT NULL,
    descripcion TEXT
);

CREATE TABLE operaciones.facturas (
    id_factura              SERIAL PRIMARY KEY,
    id_venta                INT NOT NULL REFERENCES operaciones.ventas(id_venta),
    id_tipo_factura         INT NOT NULL REFERENCES operaciones.tipo_factura(id_tipo_factura),
    -- FK al catálogo de clientes (NULL = Consumidor Final sin datos registrados)
    id_cliente              INT REFERENCES operaciones.clientes(id_cliente) ON DELETE SET NULL,
    cliente_identificacion  VARCHAR(20) NOT NULL DEFAULT '9999999999999',
    cliente_nombre          VARCHAR(150) NOT NULL DEFAULT 'Consumidor Final',
    archivo_pdf             VARCHAR(255),
    fecha_emision           TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE operaciones.deudores (
    id_deuda         SERIAL PRIMARY KEY,
    id_usuario       INT NOT NULL REFERENCES seguridad.usuarios(id_usuario),
    id_tienda        INT NOT NULL REFERENCES configuracion.tiendas(id_tienda),
    nombre_deudor    VARCHAR(200) NOT NULL,
    telefono         VARCHAR(20),
    tipo_deuda       VARCHAR(20) NOT NULL, -- 'dinero' o 'producto'
    monto_deuda      INT DEFAULT 0,        -- centavos (si dinero)
    monto_abonado    INT DEFAULT 0,        -- total abonado
    detalle_producto TEXT,                   -- descripción (si producto)
    motivo           TEXT,
    estado           VARCHAR(20) DEFAULT 'pendiente', -- 'pendiente', 'parcial', 'pagado'
    fecha_registro   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    fecha_pago       TIMESTAMP
);

CREATE TABLE operaciones.abonos_deuda (
    id_abono    SERIAL PRIMARY KEY,
    id_deuda    INT NOT NULL REFERENCES operaciones.deudores(id_deuda) ON DELETE CASCADE,
    monto_abono INT NOT NULL,
    observacion TEXT,
    fecha_abono TIMESTAMP DEFAULT CURRENT_TIMESTAMP
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

GRANT ALL ON SCHEMA configuracion, seguridad, inventario, operaciones TO admin_libreria;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA configuracion, seguridad, inventario, operaciones TO admin_libreria;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA configuracion, seguridad, inventario, operaciones TO admin_libreria;

GRANT USAGE ON SCHEMA configuracion, seguridad, inventario, operaciones TO operador_caja;

GRANT SELECT ON configuracion.tiendas TO operador_caja;

GRANT SELECT, INSERT, UPDATE ON ALL TABLES IN SCHEMA inventario TO operador_caja;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA inventario TO operador_caja;
REVOKE DELETE ON ALL TABLES IN SCHEMA inventario FROM operador_caja;

GRANT SELECT, INSERT, UPDATE ON ALL TABLES IN SCHEMA operaciones TO operador_caja;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA operaciones TO operador_caja;
REVOKE DELETE ON ALL TABLES IN SCHEMA operaciones FROM operador_caja;

-- Permisos explícitos sobre la nueva tabla de clientes
GRANT SELECT, INSERT, UPDATE ON operaciones.clientes TO operador_caja;
GRANT USAGE, SELECT ON SEQUENCE operaciones.clientes_id_cliente_seq TO operador_caja;

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

-- Crear las dos tiendas
INSERT INTO configuracion.tiendas (nombre, direccion) VALUES
    ('Los Altares - Sucursal Principal', 'Dirección Sucursal Principal'),
    ('Los Altares - Sucursal 2', 'Dirección Sucursal 2');

-- Administrador global (sin tienda fija → acceso a ambas)
INSERT INTO seguridad.usuarios (nombre, email, contrasena_hash, rol, id_tienda)
VALUES (
    'Administrador Principal',
    'admin@losaltares.com',
    crypt('Admin123!', gen_salt('bf', 10)),
    'admin_libreria',
    NULL
);

INSERT INTO operaciones.tipo_factura (nombre, descripcion)
VALUES
    ('Consumidor Final', 'Ventas menores sin datos requeridos'),
    ('Factura con Datos', 'Venta con RUC o Cédula registrada'),
    ('Factura Electrónica', 'Venta electrónica con datos de cliente enviada por correo');

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

-- Búsqueda predictiva y exacta en catálogo de clientes
CREATE INDEX idx_clientes_nombre ON operaciones.clientes USING gin (nombre gin_trgm_ops);
CREATE INDEX idx_clientes_cedula ON operaciones.clientes(cedula_ruc);

-- Índice FK en facturas → clientes para JOINs rápidos
CREATE INDEX idx_facturas_id_cliente ON operaciones.facturas(id_cliente);

-- Índice de stock por tienda (consultas frecuentes de inventario por tienda)
CREATE INDEX idx_stock_tiendas_tienda_producto ON inventario.stock_tiendas(id_tienda, id_producto);

-- Índice de movimientos por tienda
CREATE INDEX idx_movimientos_tienda ON inventario.movimientos_stock(id_tienda);

-- Índice de ventas por tienda
CREATE INDEX idx_ventas_tienda ON operaciones.ventas(id_tienda);

-- ==========================================
-- 8. VISTA MATERIALIZADA DE VENTAS DIARIAS
-- ==========================================

CREATE MATERIALIZED VIEW operaciones.ventas_diarias_mv AS
SELECT
    m.id_producto,
    m.id_tienda,
    DATE(m.fecha_movimiento) as fecha,
    SUM(ABS(m.cantidad)) as demanda_diaria
FROM inventario.movimientos_stock m
WHERE m.tipo_movimiento = 'VENTA'
GROUP BY m.id_producto, m.id_tienda, DATE(m.fecha_movimiento);
