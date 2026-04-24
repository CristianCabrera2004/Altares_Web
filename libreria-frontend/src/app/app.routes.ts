// src/app/app.routes.ts
// ─────────────────────────────────────────────────────────────────────────────
// HT-06 CA 60: Login, Dashboard, Inventario, Reportes.
// HU-05 CA 21: Ruta /usuarios protegida con adminGuard (solo admin_libreria).
// HU-05 CA 22: Ruta /reportes protegida con adminGuard (oculta para Operador).
// HU-05 CA 61: authGuard en el shell padre protege todas las rutas hijas.
// ─────────────────────────────────────────────────────────────────────────────
import { Routes } from '@angular/router';
import { authGuard }  from './core/guards/auth.guard';
import { adminGuard } from './core/guards/admin.guard';
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
      // Disponible para ambos roles (Admin y Operador)
      {
        path: 'dashboard',
        loadComponent: () =>
          import('./pages/dashboard/dashboard.component').then(m => m.DashboardComponent),
        title: 'Dashboard · Los Altares'
      },
      // HU-01: Cuaderno de ventas diarias (Operador + Admin)
      {
        path: 'cuaderno',
        loadComponent: () =>
          import('./pages/cuaderno/cuaderno.component').then(m => m.CuadernoComponent),
        title: 'Cuaderno del Día · Los Altares'
      },
      // CA 22: Inventario visible para Operador y Administrador
      {
        path: 'inventario',
        loadComponent: () =>
          import('./pages/inventario/inventario.component').then(m => m.InventarioComponent),
        title: 'Inventario · Los Altares'
      },

      // CA 22: Reportes solo para Administrador (adminGuard bloquea a Operadores)
      {
        path: 'reportes',
        canActivate: [adminGuard],
        loadComponent: () =>
          import('./pages/reportes/reportes.component').then(m => m.ReportesComponent),
        title: 'Reportes · Los Altares'
      },

      // CA 21: Módulo de Usuarios — exclusivo para Administrador
      // adminGuard redirige a /dashboard si el rol es 'operador_caja'
      {
        path: 'usuarios',
        canActivate: [adminGuard],
        loadComponent: () =>
          import('./pages/usuarios/usuarios.component').then(m => m.UsuariosComponent),
        title: 'Gestión de Usuarios · Los Altares'
      }
    ]
  },

  // Cualquier ruta desconocida → Login
  { path: '**', redirectTo: 'login' }
];
