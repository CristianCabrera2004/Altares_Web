// src/app/app.routes.ts
// ─────────────────────────────────────────────────────────────────────────────
// Distribución de roles:
//   operador       → Dashboard, Cuaderno, Inventario, Reportes, Predicciones
//   admin_libreria → Gestión de Usuarios + Auditoría (SOLO esto)
//
// Guards:
//   authGuard     → Valida JWT válido (shell padre)
//   operadorGuard → Permite solo 'operador'; redirige admin a /usuarios
//   adminGuard    → Permite solo 'admin_libreria'; redirige operador a /dashboard
// ─────────────────────────────────────────────────────────────────────────────
import { Routes } from '@angular/router';
import { authGuard }     from './core/guards/auth.guard';
import { adminGuard }    from './core/guards/admin.guard';
import { operadorGuard } from './core/guards/operador.guard';
import { LayoutComponent } from './shared/layout/layout.component';

export const routes: Routes = [
  // Raíz → redirigir al login
  { path: '', redirectTo: 'login', pathMatch: 'full' },

  // ── Ruta pública: Login ───────────────────────────────────────────────────
  {
    path: 'login',
    loadComponent: () =>
      import('./pages/login/login.component').then(m => m.LoginComponent),
    title: 'Iniciar Sesión · Los Altares'
  },

  // ── Shell protegido por authGuard ─────────────────────────────────────────
  // CA 61: Si no hay JWT válido, el guard redirige a /login antes de renderizar.
  {
    path: '',
    component: LayoutComponent,
    canActivate: [authGuard],
    children: [

      // ── RUTAS DEL OPERADOR ────────────────────────────────────────────────
      // operadorGuard bloquea al admin y lo redirige a /usuarios
      {
        path: 'dashboard',
        canActivate: [operadorGuard],
        loadComponent: () =>
          import('./pages/dashboard/dashboard.component').then(m => m.DashboardComponent),
        title: 'Dashboard · Los Altares'
      },
      {
        path: 'cuaderno',
        canActivate: [operadorGuard],
        loadComponent: () =>
          import('./pages/cuaderno/cuaderno.component').then(m => m.CuadernoComponent),
        title: 'Cuaderno del Día · Los Altares'
      },
      {
        path: 'inventario',
        canActivate: [operadorGuard],
        loadComponent: () =>
          import('./pages/inventario/inventario.component').then(m => m.InventarioComponent),
        title: 'Inventario · Los Altares'
      },
      {
        path: 'reportes',
        canActivate: [operadorGuard],
        loadComponent: () =>
          import('./pages/reportes/reportes.component').then(m => m.ReportesComponent),
        title: 'Reportes · Los Altares'
      },

      // ── RUTAS DEL ADMINISTRADOR ───────────────────────────────────────────
      // adminGuard bloquea al operador y lo redirige a /dashboard
      {
        path: 'usuarios',
        canActivate: [adminGuard],
        loadComponent: () =>
          import('./pages/usuarios/usuarios.component').then(m => m.UsuariosComponent),
        title: 'Gestión de Usuarios · Los Altares'
      },
      {
        path: 'usuarios/auditoria',
        canActivate: [adminGuard],
        loadComponent: () =>
          import('./pages/usuarios/auditoria/auditoria.component').then(m => m.AuditoriaComponent),
        title: 'Auditoría · Los Altares'
      }
    ]
  },

  // Cualquier ruta desconocida → Login
  { path: '**', redirectTo: 'login' }
];
