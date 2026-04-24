// src/environments/environment.ts
// ─────────────────────────────────────────────────────────────────────────────
// Variables de entorno para el modo DESARROLLO.
// La URL base apunta al backend Go corriendo en local.
// ─────────────────────────────────────────────────────────────────────────────
export const environment = {
  production: false,
  apiUrl: 'http://localhost:8080/api'
};
