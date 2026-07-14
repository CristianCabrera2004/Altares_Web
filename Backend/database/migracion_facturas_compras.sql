CREATE TABLE IF NOT EXISTS inventario.facturas_compra (
    id_factura SERIAL PRIMARY KEY,
    numero_factura VARCHAR(100) NOT NULL,
    fecha_compra DATE NOT NULL,
    id_proveedor INT REFERENCES inventario.proveedores(id_proveedor),
    id_tienda INT NOT NULL REFERENCES configuracion.tiendas(id_tienda),
    id_usuario INT NOT NULL REFERENCES seguridad.usuarios(id_usuario),
    total INT NOT NULL DEFAULT 0,
    fecha_registro TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

ALTER TABLE inventario.ingreso_inventario ADD COLUMN IF NOT EXISTS id_factura_compra INT REFERENCES inventario.facturas_compra(id_factura) ON DELETE CASCADE;
ALTER TABLE inventario.ingreso_inventario ADD COLUMN IF NOT EXISTS subtotal INT DEFAULT 0;
