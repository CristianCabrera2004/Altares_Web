import { Component, inject, signal, computed, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { HttpClient } from '@angular/common/http';
import { environment } from '../../../environments/environment';
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
  private readonly http = inject(HttpClient);

  deudores = signal<Deudor[]>([]);
  loading = signal(false);
  error = signal('');
  success = signal('');
  filtroEstado = signal<string>('todos');
  searchQuery = signal('');

  // ─── Modal de Confirmación ──────────────────────────────────────────────
  readonly confirmModalVisible = signal(false);
  readonly confirmModalMessage = signal('');
  private confirmAction: (() => void) | null = null;

  // Selector de productos para deudas tipo "producto"
  readonly productos = signal<any[]>([]);
  readonly selectedProducts = signal<{ id_producto: number; nombre: string; precio_venta: number; tasa_iva: number; cantidad: number }[]>([]);
  readonly busquedaProducto = signal('');
  readonly mostrarSugerencias = signal(false);

  readonly sugerencias = computed(() => {
    const q = this.busquedaProducto().toLowerCase().trim();
    if (q.length < 1) return [];

    const idsAgregados = new Set(this.selectedProducts().map(i => i.id_producto));

    return this.productos().filter(p =>
      !idsAgregados.has(p.id_producto) &&
      p.estado === 'activo' &&
      p.nombre.toLowerCase().includes(q)
    ).slice(0, 8);
  });

  readonly totalDeudaProductos = computed(() => {
    return this.selectedProducts().reduce((acc, curr) => acc + (curr.precio_venta * curr.cantidad), 0);
  });

  // Modal de detalles de deuda
  readonly showDetailModal = signal(false);
  readonly selectedDetailDeuda = signal<Deudor | null>(null);

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
    this.cargarProductos();
  }

  loadDeudores(): void {
    this.loading.set(true);
    this.svc.getAll().subscribe({
      next: (data) => { this.deudores.set(data); this.loading.set(false); },
      error: () => { this.error.set('Error al cargar deudores.'); this.loading.set(false); }
    });
  }

  cargarProductos(): void {
    this.http.get<any[]>(`${environment.apiUrl}/productos`).subscribe({
      next: (data) => this.productos.set(data ?? []),
      error: (err) => console.error('Error al cargar productos', err)
    });
  }

  openCreate(): void {
    this.form.set({
      nombre_deudor: '', telefono: '', tipo_deuda: 'dinero',
      monto_deuda: 0, detalle_producto: '', motivo: ''
    });
    this.selectedProducts.set([]);
    this.busquedaProducto.set('');
    this.mostrarSugerencias.set(false);
    this.showModal.set(true);
    this.error.set('');
  }

  selectProduct(prod: any): void {
    this.selectedProducts.update(list => [
      ...list,
      {
        id_producto: prod.id_producto,
        nombre: prod.nombre,
        precio_venta: prod.precio_venta,
        tasa_iva: prod.tasa_iva || 0,
        cantidad: 1
      }
    ]);
    this.busquedaProducto.set('');
    this.mostrarSugerencias.set(false);
  }

  removeProduct(idProducto: number): void {
    this.selectedProducts.update(list => list.filter(p => p.id_producto !== idProducto));
  }

  updateQuantity(idProducto: number, quantity: number): void {
    if (quantity < 1) return;
    this.selectedProducts.update(list =>
      list.map(p => p.id_producto === idProducto ? { ...p, cantidad: quantity } : p)
    );
  }

  closeModal(): void { this.showModal.set(false); }

  save(): void {
    const f = { ...this.form() };
    if (!f.nombre_deudor || !f.tipo_deuda) {
      this.error.set('Nombre del deudor y tipo de deuda son obligatorios.');
      return;
    }
    if (f.tipo_deuda === 'producto') {
      const selected = this.selectedProducts();
      if (selected.length === 0) {
        this.error.set('Debe seleccionar al menos un producto para la deuda.');
        return;
      }
      f.monto_deuda = this.totalDeudaProductos();
      const desc = selected.map(item => `${item.cantidad}x ${item.nombre} ($${(item.precio_venta / 100).toFixed(2)})`).join(', ');
      f.detalle_producto = JSON.stringify({
        descripcion: desc,
        items: selected.map(item => ({
          id_producto: item.id_producto,
          nombre: item.nombre,
          cantidad: item.cantidad,
          precio_unitario: item.precio_venta,
          iva_aplicado: item.tasa_iva
        }))
      });
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
    this.confirmModalMessage.set('¿Marcar esta deuda como pagada?');
    this.confirmAction = () => {
      this.svc.marcarPagado(id).subscribe({
        next: () => {
          this.success.set('Deuda marcada como pagada.');
          this.loadDeudores();
          this.confirmModalVisible.set(false);
          setTimeout(() => this.success.set(''), 4000);
        },
        error: (e) => {
          this.error.set(e.error?.error || 'Error.');
          this.confirmModalVisible.set(false);
        }
      });
    };
    this.confirmModalVisible.set(true);
  }

  // ─── Acciones del Modal de Confirmación ─────────────────────────────────────
  confirmarAccion(): void {
    if (this.confirmAction) {
      this.confirmAction();
    }
  }

  cancelarConfirmacion(): void {
    this.confirmModalVisible.set(false);
    this.confirmAction = null;
  }

  formatMoney(centavos: number): string {
    return '$' + (centavos / 100).toFixed(2);
  }

  getDetalleText(detalle: string | undefined): string {
    if (!detalle) return '';
    if (detalle.startsWith('{') || detalle.startsWith('[')) {
      try {
        const parsed = JSON.parse(detalle);
        return parsed.descripcion || detalle;
      } catch (e) {
        return detalle;
      }
    }
    return detalle;
  }

  openDetail(d: Deudor): void {
    this.selectedDetailDeuda.set(d);
    this.showDetailModal.set(true);
  }

  parsedItems(detalle: string | undefined): any[] {
    if (!detalle) return [];
    if (detalle.startsWith('{') || detalle.startsWith('[')) {
      try {
        const parsed = JSON.parse(detalle);
        return parsed.items || [];
      } catch (e) {
        return [];
      }
    }
    return [];
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
