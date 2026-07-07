# -*- coding: utf-8 -*-
import psycopg2
import sys

conn = psycopg2.connect(
    dbname='backend_cinder_shape_4030',
    user='backend_cinder_shape_4030',
    password='SIQJEop2oAAUbcs',
    host='localhost',
    port='5433',
    client_encoding='UTF8'
)
cursor = conn.cursor()

# Actualizar por ID exacto (1, 2 y 3 fueron los insertados en init.sql)
cursor.execute("UPDATE inventario.categorias SET nombre='Papelería', detalle='Cuadernos, esferos, útiles escolares' WHERE id_categoria = 1;")
cursor.execute("UPDATE inventario.categorias SET nombre='Novedades y Regalos', detalle='Mercadería general y envolturas' WHERE id_categoria = 3;")

# Actualizar tipos de factura por nombre parcial pero sin tildes en la búsqueda
cursor.execute("UPDATE operaciones.tipo_factura SET nombre='Factura con Datos', descripcion='Venta con RUC o Cédula registrada' WHERE id_tipo_factura = 2;")
cursor.execute("UPDATE operaciones.tipo_factura SET nombre='Factura Electrónica', descripcion='Venta electrónica con datos de cliente enviada por correo' WHERE id_tipo_factura = 3;")

conn.commit()
cursor.close()
conn.close()
print('Corregido por ID exitosamente.')