// src/app/app.config.ts
// ─────────────────────────────────────────────────────────────────────────────
// Configuración central de la aplicación Angular (standalone, sin NgModules).
//
// Registra los providers globales:
//  - provideRouter(routes, withPreloading)  → HT-06 CA 60: Router completo
//  - provideHttpClient(withInterceptors)     → HT-06 CA 62: Interceptor JWT
//
// El interceptor authInterceptor se inyecta aquí a nivel global, por lo que
// TODAS las peticiones HTTP de la aplicación incluirán automáticamente el JWT
// en la cabecera Authorization: Bearer <token> (CA 62).
// ─────────────────────────────────────────────────────────────────────────────
import { ApplicationConfig, provideBrowserGlobalErrorListeners } from '@angular/core';
import { provideRouter, withPreloading, PreloadAllModules } from '@angular/router';
import { provideHttpClient, withInterceptors } from '@angular/common/http';

import { routes } from './app.routes';
import { authInterceptor } from './core/interceptors/auth.interceptor';

export const appConfig: ApplicationConfig = {
  providers: [
    // Manejo global de errores del navegador
    provideBrowserGlobalErrorListeners(),

    // HT-06 CA 60: Proveedor del Router con todas las rutas definidas.
    // withPreloadingStrategy: pre-carga los lazy chunks tras el primer render
    // → mejora la navegación sin sacrificar el tiempo de carga inicial (CA 59).
    provideRouter(routes, withPreloading(PreloadAllModules)),

    // HT-06 CA 62: Cliente HTTP con el interceptor JWT registrado globalmente.
    // authInterceptor se ejecuta en CADA petición HTTP:
    //   - Lee el token de localStorage
    //   - Clona la request y añade: Authorization: Bearer <token>
    provideHttpClient(
      withInterceptors([authInterceptor])
    )
  ]
};
