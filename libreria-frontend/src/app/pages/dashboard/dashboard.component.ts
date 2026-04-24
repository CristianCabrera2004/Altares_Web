// src/app/pages/dashboard/dashboard.component.ts
import { Component, inject, signal, OnInit } from '@angular/core';
import { AuthService } from '../../core/services/auth.service';

@Component({
  selector: 'app-dashboard',
  imports: [],
  templateUrl: './dashboard.component.html',
  styleUrl: './dashboard.component.css'
})
export class DashboardComponent implements OnInit {
  private readonly authService = inject(AuthService);

  readonly nombre = signal('');
  readonly isAdmin = signal(false);
  readonly horaActual = signal('');

  ngOnInit(): void {
    this.nombre.set(this.authService.getNombre() ?? 'Usuario');
    this.isAdmin.set(this.authService.isAdmin());
    this.actualizarHora();
    setInterval(() => this.actualizarHora(), 60000);
  }

  private actualizarHora(): void {
    const ahora = new Date();
    this.horaActual.set(ahora.toLocaleTimeString('es-EC', { hour: '2-digit', minute: '2-digit' }));
  }
}
