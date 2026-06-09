// src/app/pages/deudores/deudores.component.ts
import { Component, inject, signal, computed, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { DeudoresService, Deudor, Abono } from '../../core/services/deudores.service';

@Component({
  selector: 'app-deudores',
  imports: [CommonModule, FormsModule],
  templateUrl: './deudores.component.html',
  styleUrl: './deudores.component.css'
})
export class DeudoresComponent implements OnInit {
  protected readonly Math = Math;
  private readonly svc = inject(DeudoresService);

  deudores = signal<Deudor[]>([]);
  loading = signal(false);
  error = signal('');
  success = signal('');
  filtroEstado = signal<string>('todos');
  searchQuery = signal('');

  // Modal de nueva deuda
  showModal = signal(false);
  form = signal<Partial<Deudor>>({
    nombre_deudor: '', telefono: '', tipo_deuda: 'dinero',
    monto_deuda: 0, detalle_producto: '', motivo: ''
  });

  // Modal de abono
  showAbonoModal = signal(false);
  abonoDeuda = signal<Deudor | null>(null);
  abonoMonto = signal(0);
  abonoObservacion = signal('');
  abonos = signal<Abono[]>([]);

  pendientes = computed(() => this.deudores().filter(d => d.estado !== 'pagado').length);
  totalDeuda = computed(() =>
    this.deudores()
      .filter(d => d.estado !== 'pagado')
      .reduce((sum, d) => sum + (d.monto_deuda - d.monto_abonado), 0)
  );

  filtered = computed(() => {
    let list = this.deudores();
    const estado = this.filtroEstado();
    if (estado !== 'todos') {
      list = list.filter(d => d.estado === estado);
    }
    const q = this.searchQuery().toLowerCase();
    if (q) {
      list = list.filter(d => d.nombre_deudor.toLowerCase().includes(q));
    }
    return list;
  });

  ngOnInit(): void {
    this.loadDeudores();
  }

  loadDeudores(): void {
    this.loading.set(true);
    this.svc.getAll().subscribe({
      next: (data) => { this.deudores.set(data); this.loading.set(false); },
      error: () => { this.error.set('Error al cargar deudores.'); this.loading.set(false); }
    });
  }

  openCreate(): void {
    this.form.set({
      nombre_deudor: '', telefono: '', tipo_deuda: 'dinero',
      monto_deuda: 0, detalle_producto: '', motivo: ''
    });
    this.showModal.set(true);
    this.error.set('');
  }

  closeModal(): void { this.showModal.set(false); }

  save(): void {
    const f = this.form();
    if (!f.nombre_deudor || !f.tipo_deuda) {
      this.error.set('Nombre del deudor y tipo de deuda son obligatorios.');
      return;
    }
    this.loading.set(true);
    this.svc.crear(f).subscribe({
      next: () => {
        this.success.set('Deuda registrada exitosamente.');
        this.closeModal();
        this.loadDeudores();
        setTimeout(() => this.success.set(''), 4000);
      },
      error: (e) => {
        this.error.set(e.error?.error || 'Error al registrar la deuda.');
        this.loading.set(false);
      }
    });
  }

  // ── Abonos ──
  openAbono(d: Deudor): void {
    this.abonoDeuda.set(d);
    this.abonoMonto.set(0);
    this.abonoObservacion.set('');
    this.showAbonoModal.set(true);
    this.error.set('');
    // Cargar historial de abonos
    this.svc.getAbonos(d.id_deuda).subscribe({
      next: (data) => this.abonos.set(data),
      error: () => this.abonos.set([])
    });
  }

  closeAbonoModal(): void { this.showAbonoModal.set(false); this.abonos.set([]); }

  registrarAbono(): void {
    const deuda = this.abonoDeuda();
    if (!deuda || this.abonoMonto() <= 0) {
      this.error.set('Ingrese un monto de abono válido.');
      return;
    }
    this.loading.set(true);
    this.svc.registrarAbono({
      id_deuda: deuda.id_deuda,
      monto_abono: this.abonoMonto(),
      observacion: this.abonoObservacion()
    }).subscribe({
      next: (res) => {
        this.success.set(`Abono registrado. Saldo restante: $${(res.saldo_restante / 100).toFixed(2)}`);
        this.closeAbonoModal();
        this.loadDeudores();
        setTimeout(() => this.success.set(''), 5000);
      },
      error: (e) => {
        this.error.set(e.error?.error || 'Error al registrar abono.');
        this.loading.set(false);
      }
    });
  }

  marcarPagado(id: number): void {
    if (!confirm('¿Marcar esta deuda como pagada?')) return;
    this.svc.marcarPagado(id).subscribe({
      next: () => {
        this.success.set('Deuda marcada como pagada.');
        this.loadDeudores();
        setTimeout(() => this.success.set(''), 4000);
      },
      error: (e) => this.error.set(e.error?.error || 'Error.')
    });
  }

  formatMoney(centavos: number): string {
    return '$' + (centavos / 100).toFixed(2);
  }

  getEstadoClass(estado: string): string {
    switch (estado) {
      case 'pendiente': return 'badge-warning';
      case 'parcial': return 'badge-info';
      case 'pagado': return 'badge-success';
      default: return 'badge-muted';
    }
  }

  getEstadoLabel(estado: string): string {
    switch (estado) {
      case 'pendiente': return 'Pendiente';
      case 'parcial': return 'Parcial';
      case 'pagado': return 'Pagado';
      default: return estado;
    }
  }
}
