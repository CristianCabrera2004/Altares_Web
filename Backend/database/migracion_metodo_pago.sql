ALTER TABLE operaciones.ventas ADD COLUMN IF NOT EXISTS metodo_pago VARCHAR(50) NOT NULL DEFAULT 'efectivo';
