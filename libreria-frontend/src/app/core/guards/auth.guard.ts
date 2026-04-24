// src/app/core/guards/auth.guard.ts
// ─────────────────────────────────────────────────────────────────────────────
// HT-06 CA 61: Guardia de Ruta (Route Guard)
//
// Protege todas las rutas internas de la aplicación.
// Si el usuario NO tiene un JWT válido (ausente o expirado) en localStorage,
// es redirigido automáticamente al /login sin importar la ruta que solicitó.
//
// Uso en app.routes.ts:
//   canActivate: [authGuard]
//
// Implementado como función (Functional Guard — Angular 15+).
// No requiere clase ni NgModule. Se inyecta con inject().
// ─────────────────────────────────────────────────────────────────────────────
import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { AuthService } from '../services/auth.service';

/**
 * authGuard: Guardia de activación de ruta.
 *
 * - Si isAuthenticated() == true  → permite el acceso a la ruta
 * - Si isAuthenticated() == false → redirige a /login (CA 61)
 *
 * isAuthenticated() verifica:
 *  1. Que exista un token en localStorage
 *  2. Que el campo `exp` del payload JWT sea mayor a la hora actual
 */
export const authGuard: CanActivateFn = (_route, _state) => {
  const authService = inject(AuthService);
  const router = inject(Router);

  if (authService.isAuthenticated()) {
    // Token válido y no expirado → acceso concedido
    return true;
  }

  // Sin JWT válido → redirigir al login (CA 61)
  // createUrlTree genera una redirección reactiva sin efectos secundarios
  return router.createUrlTree(['/login']);
};
