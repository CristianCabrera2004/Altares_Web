import { Component, inject, signal, computed, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { AuditService, AuditLog } from '../../../core/services/audit.service';

@Component({
  selector: 'app-auditoria',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './auditoria.component.html',
  styleUrl: './auditoria.component.css'
})
export class AuditoriaComponent implements OnInit {
  private readonly auditService = inject(AuditService);

  readonly logs = signal<AuditLog[]>([]);
  readonly loading = signal(true);
  readonly errorMsg = signal('');
  
  // CA 36: Filtros
  readonly searchUsuario = signal('');
  readonly searchAccion = signal('');
  readonly searchFecha = signal('');

  readonly acciones = ['Todas', 'LOGIN', 'LOGOUT', 'CIERRE_CAJA', 'MODIFICACION_PRECIO'];

  readonly logsFiltrados = computed(() => {
    let filtrados = this.logs();
    
    if (this.searchUsuario().trim()) {
      const q = this.searchUsuario().toLowerCase();
      filtrados = filtrados.filter(l => l.nombre_usuario.toLowerCase().includes(q));
    }
    if (this.searchAccion() && this.searchAccion() !== 'Todas') {
      filtrados = filtrados.filter(l => l.accion === this.searchAccion());
    }
    if (this.searchFecha()) {
      filtrados = filtrados.filter(l => l.fecha.startsWith(this.searchFecha()));
    }
    
    return filtrados;
  });

  ngOnInit(): void {
    this.auditService.getLogs().subscribe({
      next: (data) => {
        this.logs.set(data);
        this.loading.set(false);
      },
      error: (err) => {
        this.errorMsg.set('No se pudieron cargar los logs de auditoría.');
        this.loading.set(false);
      }
    });
  }

  getBadgeClass(accion: string): string {
    switch (accion) {
      case 'LOGIN': return 'badge-green';
      case 'LOGOUT': return 'badge-gray';
      case 'CIERRE_CAJA': return 'badge-purple';
      case 'MODIFICACION_PRECIO': return 'badge-orange';
      default: return 'badge-blue';
    }
  }
}
