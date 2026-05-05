import pandas as pd
import logging
from sqlalchemy import create_engine
import psycopg2
from datetime import datetime

# --- CONFIGURACIÓN DEL LOG (CA 58) ---
logging.basicConfig(
    filename='errores_migracion.log',
    level=logging.ERROR,
    format='%(asctime)s - %(levelname)s - %(message)s',
    encoding='utf-8'
)

# --- CONFIGURACIÓN DE BASE DE DATOS ---
# Sysag Origen (de donde extraemos)
SYSAG_DB_USER = 'postgres'
SYSAG_DB_PASS = 'cace2004'  
SYSAG_DB_HOST = 'localhost'
SYSAG_DB_PORT = '5432'
SYSAG_DB_NAME = 'Tesis_DB'

# Altares Destino (donde insertamos) 
# Si estás usando la misma base de datos para todo, déjalo igual.
# Si creaste "libreria_los_altares", pon ese nombre.
DEST_DB_USER = 'postgres'
DEST_DB_PASS = 'cace2004'
DEST_DB_HOST = 'localhost'
DEST_DB_PORT = '5432'
DEST_DB_NAME = 'libreria_los_altares_V2' 

sysag_conn_str = f'postgresql+psycopg2://{SYSAG_DB_USER}:{SYSAG_DB_PASS}@{SYSAG_DB_HOST}:{SYSAG_DB_PORT}/{SYSAG_DB_NAME}'

def extraer_sysag():
    print("1. Conectando a Sysag y extrayendo histórico de ventas (CA 55)...")
    try:
        engine = create_engine(sysag_conn_str, connect_args={'client_encoding': 'utf8'})
        query = """
        SELECT 
            cab.fec_venta as fecha,
            cab.cod_venta,
            cab.nom_cliente,
            det.cod_item,
            det.nombre as nombre_producto,
            det.cantidad,
            det.pvp as precio_unitario
        FROM 
            public.vent_ventas_detalle as det
        JOIN 
            public.vent_ventas_principal as cab ON det.cod_venta = cab.cod_venta
        WHERE
            cab.estado = 'GRABADO'
        ORDER BY 
            cab.fec_venta ASC;
        """
        df = pd.read_sql(query, engine)
        print(f"   -> {len(df)} registros extraídos con éxito.")
        return df
    except Exception as e:
        print("   -> ERROR FATAL extrayendo datos:", e)
        exit(1)

def transformar_datos(df):
    print("2. Limpiando y transformando datos (CA 56)...")
    
    # A. Eliminar duplicados exactos si hubieran
    df = df.drop_duplicates()
    
    # B. Asegurar tipos correctos
    df['cantidad'] = pd.to_numeric(df['cantidad'], errors='coerce').fillna(0).astype(int)
    df['precio_unitario'] = pd.to_numeric(df['precio_unitario'], errors='coerce').fillna(0.0)
    
    # C. LA SOLUCIÓN AL ERROR DE DECIMALES REPORTADO:
    # Convertimos el flotante (ej. 0.899999) multiplicándolo por 100, redondeando 
    # matemáticamente a .90 y forzando a número entero (centavos) para la BD.
    df['precio_unitario_cents'] = (df['precio_unitario'] * 100).round().astype(int)
    
    # D. Calcular el total en base a los enteros (adiós errores de coma flotante)
    df['total_venta_cents'] = df['cantidad'] * df['precio_unitario_cents']
    
    # E. Estandarizar la fecha a tipo datetime de Pandas
    df['fecha'] = pd.to_datetime(df['fecha'])
    
    return df

def cargar_datos(df):
    print("3. Conectando al nuevo esquema y cargando datos (CA 57)...")
    try:
        conn = psycopg2.connect(
            dbname=DEST_DB_NAME,
            user=DEST_DB_USER,
            password=DEST_DB_PASS,
            host=DEST_DB_HOST,
            port=DEST_DB_PORT
        )
        # Apagamos autocommit para controlar nosotros el Begin/Commit atómico
        conn.autocommit = False
        cursor = conn.cursor()
    except Exception as e:
        print("   -> ERROR de conexión a la BD Destino:", e)
        return

    # ID de usuario administrador por defecto para registros históricos
    ID_USUARIO_ADMIN = 1

    # --- PASO A. MIGRAMOS PRODUCTOS AL CATÁLOGO ---
    productos_unicos = df[['cod_item', 'nombre_producto', 'precio_unitario_cents']].drop_duplicates('cod_item')
    mapa_productos = {} # Mapeo cod_item original -> id_producto nuevo
    
    print("   -> A. Sincronizando catálogo de productos...")
    for _, row in productos_unicos.iterrows():
        cod_item = str(row['cod_item']).strip()
        nombre = str(row['nombre_producto']).strip()
        precio = int(row['precio_unitario_cents'])
        
        try:
            # Buscar si el producto ya existe (mediante el código de barras)
            cursor.execute("""
                SELECT p.id_producto 
                FROM inventario.codigos_barras cb 
                JOIN inventario.productos p ON cb.id_producto = p.id_producto 
                WHERE cb.codigo = %s
            """, (cod_item,))
            res = cursor.fetchone()
            
            if res:
                # Ya existe, lo guardamos en mapa
                mapa_productos[cod_item] = res[0]
            else:
                # No existe, lo insertamos. Asumimos categoría 1 (Papelería/General)
                cursor.execute("""
                    INSERT INTO inventario.productos 
                    (nombre, id_categoria, stock_actual, stock_alerta_min, precio_venta, estado)
                    VALUES (%s, 1, 0, 5, %s, 'activo') RETURNING id_producto
                """, (nombre, precio))
                nuevo_id = cursor.fetchone()[0]
                
                # Le asignamos su código de barras original
                cursor.execute("""
                    INSERT INTO inventario.codigos_barras (id_producto, codigo) 
                    VALUES (%s, %s)
                """, (nuevo_id, cod_item))
                
                mapa_productos[cod_item] = nuevo_id
            conn.commit()
        except Exception as e:
            conn.rollback()
            logging.error(f"ERROR creando producto [{cod_item}] {nombre}: {e}")

    # --- PASO B. MIGRAMOS VENTAS HISTÓRICAS ---
    # Agrupamos el DataFrame original para procesar cada venta como un solo documento
    ventas_agrupadas = df.groupby(['cod_venta', 'fecha', 'nom_cliente'])
    print(f"   -> B. Procesando {len(ventas_agrupadas)} transacciones de venta consolidadas...")
    
    ventas_exitosas = 0
    ventas_fallidas = 0

    for (cod_venta, fecha, cliente), grupo in ventas_agrupadas:
        try:
            # Sumar el subtotal de todos los productos en esa venta (en centavos)
            subtotal_venta = int(grupo['total_venta_cents'].sum())
            
            # 1. Insertar Venta Principal
            cursor.execute("""
                INSERT INTO operaciones.ventas (id_usuario, fecha_venta, subtotal, total_iva, total, estado)
                VALUES (%s, %s, %s, 0, %s, 'completada') RETURNING id_venta
            """, (ID_USUARIO_ADMIN, fecha, subtotal_venta, subtotal_venta))
            id_venta_nueva = cursor.fetchone()[0]
            
            # 2. Insertar Detalles y Movimientos para Predicción
            for _, fila in grupo.iterrows():
                cod_item = str(fila['cod_item']).strip()
                if cod_item not in mapa_productos:
                    continue # Saltamos productos que dieron error de inserción previo
                    
                id_producto = mapa_productos[cod_item]
                cantidad = int(fila['cantidad'])
                precio_u = int(fila['precio_unitario_cents'])
                subtotal_item = int(fila['total_venta_cents'])
                
                # Insertar en operaciones.detalle_ventas
                cursor.execute("""
                    INSERT INTO operaciones.detalle_ventas 
                    (id_venta, id_producto, cantidad, precio_unitario, iva_aplicado, subtotal)
                    VALUES (%s, %s, %s, %s, 0, %s)
                """, (id_venta_nueva, id_producto, cantidad, precio_u, subtotal_item))
                
                # Insertar en inventario.movimientos_stock 
                # (Crucial para que el Motor de Predicción tenga datos reales, CA 57)
                cursor.execute("""
                    INSERT INTO inventario.movimientos_stock 
                    (id_producto, id_usuario, tipo_movimiento, cantidad, stock_resultante, referencia_id, fecha_movimiento)
                    VALUES (%s, %s, 'VENTA', %s, 0, %s, %s)
                """, (id_producto, ID_USUARIO_ADMIN, -cantidad, id_venta_nueva, fecha))

            # Transacción completa de la venta: Todo o nada
            conn.commit()
            ventas_exitosas += 1
        except Exception as e:
            # Si un item falla, deshace toda la venta para mantener integridad
            conn.rollback()
            ventas_fallidas += 1
            # CA 58: Log de errores detallado
            logging.error(f"Error procesando venta Sysag [{cod_venta}]: {e}")
            
    cursor.close()
    conn.close()
    
    print("-" * 50)
    print("🏁 RESUMEN DE MIGRACIÓN")
    print(f"   ✅ Ventas insertadas exitosamente: {ventas_exitosas}")
    if ventas_fallidas > 0:
        print(f"   ⚠️ Ventas con error: {ventas_fallidas} (Detalles en errores_migracion.log)")
    else:
         print(f"   ⚠️ Ventas con error: 0")
    print("-" * 50)

if __name__ == "__main__":
    print("=" * 50)
    print(" INICIANDO PROCESO ETL: SYSAG -> LOS ALTARES")
    print("=" * 50)
    df_raw = extraer_sysag()
    df_clean = transformar_datos(df_raw)
    cargar_datos(df_clean)
    print("🚀 MIGRACIÓN FINALIZADA CORRECTAMENTE.")
