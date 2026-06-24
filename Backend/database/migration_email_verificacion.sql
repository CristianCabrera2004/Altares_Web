-- ============================================================
-- MIGRACIÓN: Verificación de Email al Primer Login
-- Ejecutar sobre la BD existente (libreria_los_altares_V2)
-- ============================================================

-- 1. Agregar columna para saber si el email ya fue verificado
ALTER TABLE seguridad.usuarios
    ADD COLUMN IF NOT EXISTS email_verificado BOOLEAN NOT NULL DEFAULT FALSE;

-- 2. Columna para almacenar el código temporal de 6 dígitos
ALTER TABLE seguridad.usuarios
    ADD COLUMN IF NOT EXISTS codigo_verificacion VARCHAR(6) DEFAULT NULL;

-- 3. Columna para la expiración del código (15 min de vigencia)
ALTER TABLE seguridad.usuarios
    ADD COLUMN IF NOT EXISTS codigo_verificacion_expira TIMESTAMP DEFAULT NULL;

-- 4. Actualizar el administrador existente con el nuevo email, contraseña y marcarlo como verificado
UPDATE seguridad.usuarios
SET email = 'cristianalejandrocabreraestrad@gmail.com',
    nombre = 'Cristian Cabrera',
    contrasena_hash = crypt('102044CACEDracai', gen_salt('bf', 10)),
    email_verificado = TRUE
WHERE email = 'admin@losaltares.com' AND rol = 'admin_libreria';

-- Si el admin ya tiene el nuevo email (por si se ejecuta más de una vez)
UPDATE seguridad.usuarios
SET email_verificado = TRUE
WHERE email = 'cristianalejandrocabreraestrad@gmail.com';

-- Nota: Todos los usuarios nuevos creados a partir de ahora tendrán
-- email_verificado = FALSE por defecto y deberán verificar en su primer login.
