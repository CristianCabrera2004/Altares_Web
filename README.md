# 📚 Sistema de Gestión — Librería Los Altares

Sistema web integral para la gestión de inventario, ventas, deudores y predicción de demanda de la Librería Los Altares. Arquitectura: **Angular 21** (frontend) + **Go 1.22** (API REST) + **PostgreSQL 15** (base de datos).

---

## 🚀 Instalación y Ejecución

### Requisitos previos

| Herramienta | Versión mínima |
|---|---|
| Go | 1.22+ |
| Node.js | 20+ |
| PostgreSQL | 15+ |
| Angular CLI | 18+ |

### 1. Base de Datos

```bash
# Crear la base de datos
psql -U postgres -c "CREATE DATABASE libreria_altares;"

# Ejecutar el schema completo (crea esquemas, tablas, roles y datos semilla)
psql -U postgres -d libreria_altares -f Backend/database/init.sql
```

### 2. Backend (Go)

```bash
cd Backend

# Copiar y configurar variables de entorno
cp .env.example .env
# Editar .env con tus credenciales de PostgreSQL

# Instalar dependencias
go mod tidy

# Ejecutar en modo desarrollo
go run main.go

# Compilar binario de producción
go build -o libreria-altares.exe .
```

**Variables de entorno requeridas (`.env`):**

```env
DB_HOST=localhost
DB_PORT=5432
DB_NAME=libreria_altares
DB_USER=app_backend_go
DB_PASSWORD=<tu_password>
JWT_SECRET=<secreto_aleatorio_seguro>
ALLOWED_ORIGIN=http://localhost:4200   # En producción: https://tu-dominio.com
```

> ⚠️ **Producción:** Configura `ALLOWED_ORIGIN` con el dominio real del frontend para evitar CORS abierto.

### 3. Frontend (Angular)

```bash
cd libreria-frontend

# Instalar dependencias
npm install

# Ejecutar en modo desarrollo (hot-reload)
ng serve

# Compilar para producción
ng build --configuration production
```

La aplicación quedará disponible en `http://localhost:4200`.

---

## 🗂️ Módulos del Sistema

| Módulo | Rol | Descripción |
|---|---|---|
| Dashboard | Operador | KPIs, gráfica de ventas, alertas de stock bajo |
| Facturación | Operador | Registro de ventas unitarias con cálculo de IVA |
| Cuaderno | Operador | Carga masiva de ventas del día en una sola transacción |
| Inventario | Operador | CRUD de productos, ingresos y bajas de stock |
| Devoluciones | Operador | Registro de devoluciones con reintegro de stock |
| Bajas por Merma | Operador | Registro de pérdidas o productos dañados |
| Clientes | Operador | Gestión de clientes con autocompletado (pg_trgm) |
| Deudores/Fiados | Operador | Control de créditos con abonos y saldo pendiente |
| Transferencias | Operador | Solicitudes y envíos de stock entre sucursales |
| Reportes | Operador | Ventas por rango de fechas, exportación PDF |
| Predicciones | Operador | Motor AR(1) con lista de compras sugerida |
| Usuarios | Admin | CRUD de usuarios y gestión de contraseñas |
| Tiendas | Admin | Gestión de sucursales activas/inactivas |
| Auditoría | Admin | Log inmutable de acciones críticas del sistema |

---

## 🔐 Seguridad

- **Contraseñas:** BCrypt (factor 10) — NIST 800-63B
- **Sesiones:** JWT HS256, 8h, verificación en BD — RFC 7519
- **Autorización:** RBAC en API Go y en PostgreSQL — NIST INCITS 359
- **2FA:** TOTP compatible con Google Authenticator — RFC 6238
- **Auditoría:** Log de eventos críticos en `seguridad.logs_auditoria`

---

## 📁 Estructura del Proyecto

```
Código/
├── Backend/
│   ├── main.go                  # Punto de entrada, rutas y CORS
│   ├── database/
│   │   └── init.sql             # Schema completo de la BD
│   ├── handlers/                # 17 handlers REST
│   ├── middleware/              # JWT, RBAC
│   └── utils/                   # TOTP, auditoría
├── libreria-frontend/
│   └── src/app/
│       ├── core/                # Guards, interceptores, servicios
│       ├── pages/               # 13 módulos de página
│       └── shared/              # Layout, componentes comunes
├── Scripts_ETL/
│   └── migracion_sysag.py      # Migración de datos históricos
└── documentacion_tesis.tex     # Documentación técnica completa
```

---

## 📖 Documentación

El archivo [`documentacion_tesis.tex`](./documentacion_tesis.tex) contiene la documentación técnica completa del proyecto (compilable en Overleaf):

- Planteamiento del problema y árbol de causas/efectos
- Objetivos general y específicos
- Marco teórico (AR(1), JWT, BCrypt, RBAC, TOTP)
- Metodología Scrum adaptada con sprints
- Diagramas ER, casos de uso, secuencia y arquitectura
- Análisis de resultados y verificación de criterios de aceptación

---

## ⚠️ Deudas Técnicas Conocidas

1. Configurar HTTPS/TLS en producción
2. Agregar rate limiting en `/api/auth/login`
3. Programar REFRESH automático de la vista materializada `ventas_diarias_mv`
4. Implementar tests unitarios Go (`go test`) y Angular (Vitest)