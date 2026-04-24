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

  toggleSidebar(): void {
    this.sidebarOpen.update(v => !v);
  }

  logout(): void {
    this.authService.logout();
  }
}
