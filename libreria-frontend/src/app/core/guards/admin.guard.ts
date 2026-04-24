// src/app/core/guards/admin.guard.ts
// ─────────────────────────────────────────────────────────────────────────────
// HU-05 CA 21: Guardia de ruta exclusiva para el rol 'admin_libreria'.
//
// Usado en la ruta /usuarios para impedir que un Operador acceda
// tecleando la URL directamente en el navegador, aunque el sidebar
// ya oculte el enlace visualmente.
//
// Flujo:
//  - Sin JWT válido          → redirige a /login
//  - JWT válido pero Operador → redirige a /dashboard (acceso denegado silencioso)
//  - JWT válido y Admin       → permite el acceso
// ─────────────────────────────────────────────────────────────────────────────
import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { AuthService } from '../services/auth.service';

export const adminGuard: CanActivateFn = (_route, _state) => {
  const authService = inject(AuthService);
  const router      = inject(Router);

  // Sin sesión → login
  if (!authService.isAuthenticated()) {
    return router.createUrlTree(['/login']);
  }

  // Sesión activa pero no es Admin → dashboard (CA 21 / CA 22)
  if (!authService.isAdmin()) {
    return router.createUrlTree(['/dashboard']);
  }

  // Administrador confirmado → acceso concedido
  return true;
};
