// src/app/shared/layout/layout.component.ts
// Shell principal con sidebar y router-outlet para las vistas internas.
import { Component, inject, signal } from '@angular/core';
import { RouterOutlet, RouterLink, RouterLinkActive } from '@angular/router';
import { UpperCasePipe } from '@angular/common';
import { AuthService } from '../../core/services/auth.service';

@Component({
  selector: 'app-layout',
  imports: [RouterOutlet, RouterLink, RouterLinkActive, UpperCasePipe],
  templateUrl: './layout.component.html',
  styleUrl: './layout.component.css'
})
export class LayoutComponent {
  private readonly authService = inject(AuthService);

  readonly nombre = signal(this.authService.getNombre() ?? 'Usuario');
  readonly rol = signal(this.authService.getRol() ?? '');
  readonly isAdmin = signal(this.authService.isAdmin());
  readonly sidebarOpen = signal(true);

  // Inicializa el tema desde localStorage para persistir entre recargas
  readonly isLightMode = signal(
    typeof localStorage !== 'undefined' && localStorage.getItem('theme') === 'light'
  );

  constructor() {
    // Aplica la clase antes del primer render para evitar flash de tema incorrecto
    if (typeof document !== 'undefined' && this.isLightMode()) {
      document.body.classList.add('light-mode');
    }
  }

  toggleSidebar(): void {
    this.sidebarOpen.update(v => !v);
  }

  toggleTheme(): void {
    this.isLightMode.update(v => !v);
    if (typeof document !== 'undefined') {
      document.body.classList.toggle('light-mode', this.isLightMode());
      localStorage.setItem('theme', this.isLightMode() ? 'light' : 'dark');
    }
  }

  logout(): void {
    this.authService.logout();
  }
}
