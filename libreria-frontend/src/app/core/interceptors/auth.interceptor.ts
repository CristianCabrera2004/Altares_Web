// src/app/core/interceptors/auth.interceptor.ts
// ─────────────────────────────────────────────────────────────────────────────
// HT-06 CA 62: Intercepta peticiones HTTP y adjunta el JWT.
// HU-05 CA 23: Maneja respuestas HTTP 401 (token expirado/inválido):
//              → Elimina el token del localStorage
//              → Redirige automáticamente al /login
//
// IMPORTANTE — Anti-loop: El handleo de 401 se omite en las rutas
// /auth/login y /auth/logout para evitar bucles infinitos donde el
// interceptor reintenta redirigir sobre su propia petición.
// ─────────────────────────────────────────────────────────────────────────────
import { HttpInterceptorFn, HttpErrorResponse } from '@angular/common/http';
import { inject } from '@angular/core';
import { Router } from '@angular/router';
import { catchError, throwError } from 'rxjs';
import { AuthService } from '../services/auth.service';

export const authInterceptor: HttpInterceptorFn = (req, next) => {
  const authService = inject(AuthService);
  const router      = inject(Router);
  const token       = authService.getToken();

  // ── CA 62: Adjuntar Bearer token en cada petición ──────────────────────
  const authenticatedReq = token
    ? req.clone({ setHeaders: { Authorization: `Bearer ${token}` } })
    : req;

  // ── CA 23: Capturar respuestas HTTP 401 (sesión inválida/expirada) ─────
  return next(authenticatedReq).pipe(
    catchError((error: HttpErrorResponse) => {
      // Omitir el handler en las rutas de auth propias para evitar bucles
      const isAuthRoute =
        req.url.includes('/auth/login') ||
        req.url.includes('/auth/logout');

      if (error.status === 401 && !isAuthRoute) {
        // Token expirado o inválido confirmado por el servidor (CA 23):
        //  1. Eliminar el JWT del localStorage (no llamar al API logout para evitar loop)
        authService.removeToken();
        //  2. Redirigir al login con indicador de sesión expirada
        router.navigate(['/login'], {
          queryParams: { expired: '1' }
        });
      }

      // Re-lanzar el error para que los componentes puedan manejarlo si lo necesitan
      return throwError(() => error);
    })
  );
};
