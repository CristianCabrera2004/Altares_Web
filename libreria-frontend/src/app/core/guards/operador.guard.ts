// src/app/core/guards/operador.guard.ts
// ─────────────────────────────────────────────────────────────────────────────
// Guardia de ruta exclusiva para el rol 'operador'.
//
// Protege las rutas funcionales (Dashboard, Inventario, Cuaderno, Reportes).
// El Administrador NO tiene acceso a estas vistas — su función es gestionar
// usuarios y revisar auditoría.
//
// Flujo:
//  - Sin JWT válido       → redirige a /login
//  - JWT válido y Admin   → redirige a /usuarios (su sección)
//  - JWT válido y Operador → permite el acceso
// ─────────────────────────────────────────────────────────────────────────────
import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { AuthService } from '../services/auth.service';

export const operadorGuard: CanActivateFn = (_route, _state) => {
  const authService = inject(AuthService);
  const router      = inject(Router);

  // Sin sesión → login
  if (!authService.isAuthenticated()) {
    return router.createUrlTree(['/login']);
  }

  // Administrador intenta acceder a zona de operador → su home
  if (authService.getRol() !== 'operador_caja') {
    return router.createUrlTree(['/usuarios']);
  }

  // Operador confirmado → acceso concedido
  return true;
};
