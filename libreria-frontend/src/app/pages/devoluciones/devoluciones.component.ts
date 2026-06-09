// src/app/pages/devoluciones/devoluciones.component.ts
// ─────────────────────────────────────────────────────────────────────────────
// Página independiente de Devoluciones (HU-04)
//
// Soporta 4 flujos combinando Tipo (Devolución/Cambio) y Estado (Buen/Mal estado)
// ─────────────────────────────────────────────────────────────────────────────
import { Component, inject, signal, OnInit, computed } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { FormBuilder, Validators, ReactiveFormsModule } from '@angular/forms';
import { CommonModule } from '@angular/common';
import { environment } from '../../../environments/environment';
import { AuthService } from '../../core/services/auth.service';

interface Producto {
  id_producto: number;
  nombre: string;
  stock_actual: number;
  precio_venta: number;
  codigo_barras?: string;
}

interface Devolucion {
  id_devolucion: number;
  id_venta: number | null;
  id_producto: number;
  nombre_producto: string;
  id_usuario: number;
  cantidad_devuelta: number;
  motivo: string;
  tipo: string;
  en_mal_estado: boolean;
  id_producto_cambio: number | null;
  nombre_producto_cambio: string | null;
  cantidad_cambio: number | null;
  diferencia_precio: number;
  fecha_devolucion: string;
}

@Component({
  selector: 'app-devoluciones',
  imports: [ReactiveFormsModule, CommonModule],
  templateUrl: './devoluciones.component.html',
  styleUrl: './devoluciones.component.css'
})
export class DevolucionesComponent implements OnInit {
  private readonly http = inject(HttpClient);
  private readonly fb   = inject(FormBuilder);
  private readonly auth = inject(AuthService);

  private readonly apiProductos    = `${environment.apiUrl}/productos`;
  private readonly apiDevoluciones = `${environment.apiUrl}/devoluciones`;

  // ─── Estado ───────────────────────────────────────────────────────────────
  readonly devoluciones    = signal<Devolucion[]>([]);
  readonly productos       = signal<Producto[]>([]);
  readonly cargando        = signal(true);
  readonly guardando       = signal(false);
  readonly successMsg      = signal('');
  readonly errorMsg        = signal('');
  readonly busqueda        = signal('');

  // ─── Autocomplete 1: Producto Original ────────────────────────────────────
  readonly busquedaProducto   = signal('');
  readonly productoSeleccionado = signal<Producto | null>(null);
  readonly mostrarSugerencias = signal(false);

  // ─── Autocomplete 2: Producto de Cambio ───────────────────────────────────
  readonly busquedaCambio   = signal('');
  readonly productoCambioSeleccionado = signal<Producto | null>(null);
  readonly mostrarSugerenciasCambio = signal(false);

  // ─── Formulario ───────────────────────────────────────────────────────────
  readonly form = this.fb.group({
    id_producto:       [null as number | null, Validators.required],
    id_venta:          [null as number | null],
    cantidad_devuelta: [1, [Validators.required, Validators.min(1)]],
    motivo:            ['', Validators.required],
    flujo:             ['DEV_SIMPLE', Validators.required],
    id_producto_cambio:[null as number | null],
    cantidad_cambio:   [1, [Validators.min(1)]]
  });

  // ─── Computed: Sugerencias ────────────────────────────────────────────────
  readonly sugerencias = computed(() => {
    const q = this.busquedaProducto().toLowerCase().trim();
    if (q.length < 1) return [];
    return this.productos().filter(p =>
      p.nombre.toLowerCase().includes(q) ||
      (p.codigo_barras ?? '').toLowerCase().includes(q)
    ).slice(0, 8);
  });

  readonly sugerenciasCambio = computed(() => {
    const q = this.busquedaCambio().toLowerCase().trim();
    if (q.length < 1) return [];
    return this.productos().filter(p =>
      p.nombre.toLowerCase().includes(q) ||
      (p.codigo_barras ?? '').toLowerCase().includes(q)
    ).slice(0, 8);
  });

  // ─── Computed: Historial y UI ─────────────────────────────────────────────
  readonly devolucionesFiltradas = computed(() => {
    const q = this.busqueda().toLowerCase().trim();
    if (!q) return this.devoluciones();
    return this.devoluciones().filter(d =>
      d.nombre_producto.toLowerCase().includes(q) ||
      d.motivo.toLowerCase().includes(q) ||
      (d.nombre_producto_cambio && d.nombre_producto_cambio.toLowerCase().includes(q))
    );
  });

  readonly stockDisponible = computed(() => this.productoSeleccionado()?.stock_actual ?? null);
  readonly stockDisponibleCambio = computed(() => this.productoCambioSeleccionado()?.stock_actual ?? null);

  readonly esCambio = computed(() => {
    const f = this.form.get('flujo')?.value;
    return f === 'CAMBIO_SIMPLE' || f === 'CAMBIO_DANO';
  });

  readonly diferenciaCalc = computed(() => {
    if (!this.esCambio()) return null;
    const pOrig = this.productoSeleccionado();
    const pCamb = this.productoCambioSeleccionado();
    const cOrig = this.form.get('cantidad_devuelta')?.value || 0;
    const cCamb = this.form.get('cantidad_cambio')?.value || 0;

    if (!pOrig || !pCamb || cOrig <= 0 || cCamb <= 0) return null;

    const totalOrig = pOrig.precio_venta * cOrig;
    const totalCamb = pCamb.precio_venta * cCamb;
    return {
      totalOrig,
      totalCamb,
      diff: totalCamb - totalOrig // Positivo = cliente paga
    };
  });

  ngOnInit(): void {
    this.cargarDatos();

    // Resetear campos de cambio si se vuelve a DEVOLUCION
    this.form.get('flujo')?.valueChanges.subscribe(flujo => {
      if (flujo === 'DEV_SIMPLE' || flujo === 'DEV_DANO') {
        this.productoCambioSeleccionado.set(null);
        this.busquedaCambio.set('');
        this.form.patchValue({ id_producto_cambio: null, cantidad_cambio: 1 });
      }
    });
  }

  cargarDatos(): void {
    this.cargando.set(true);
    this.errorMsg.set('');

    this.http.get<Devolucion[]>(this.apiDevoluciones).subscribe({
      next: (data) => { this.devoluciones.set(data); this.cargando.set(false); },
      error: (err)  => { this.errorMsg.set(err?.error?.error ?? 'Error al cargar devoluciones.'); this.cargando.set(false); }
    });

    this.http.get<Producto[]>(this.apiProductos).subscribe({
      next: (data) => this.productos.set(data),
      error: () => {}
    });
  }

  // ─── Autocomplete Handlers 1 ──────────────────────────────────────────────
  onBusquedaInput(valor: string): void {
    this.busquedaProducto.set(valor);
    this.mostrarSugerencias.set(true);
    if (!valor.trim()) {
      this.productoSeleccionado.set(null);
      this.form.patchValue({ id_producto: null });
    }
  }

  seleccionarProducto(p: Producto): void {
    this.productoSeleccionado.set(p);
    this.busquedaProducto.set(p.nombre);
    this.mostrarSugerencias.set(false);
    this.form.patchValue({ id_producto: p.id_producto });
  }

  ocultarSugerencias(): void { setTimeout(() => this.mostrarSugerencias.set(false), 180); }

  // ─── Autocomplete Handlers 2 (Cambio) ─────────────────────────────────────
  onBusquedaCambioInput(valor: string): void {
    this.busquedaCambio.set(valor);
    this.mostrarSugerenciasCambio.set(true);
    if (!valor.trim()) {
      this.productoCambioSeleccionado.set(null);
      this.form.patchValue({ id_producto_cambio: null });
    }
  }

  seleccionarProductoCambio(p: Producto): void {
    this.productoCambioSeleccionado.set(p);
    this.busquedaCambio.set(p.nombre);
    this.mostrarSugerenciasCambio.set(false);
    this.form.patchValue({ id_producto_cambio: p.id_producto });
  }

  ocultarSugerenciasCambio(): void { setTimeout(() => this.mostrarSugerenciasCambio.set(false), 180); }

  // ─── Registrar ────────────────────────────────────────────────────────────
  registrar(): void {
    // Validaciones personalizadas
    if (this.esCambio() && !this.form.get('id_producto_cambio')?.value) {
      this.errorMsg.set('Seleccione un producto de cambio.');
      return;
    }
    if (this.esCambio() && this.form.get('cantidad_cambio')?.value! > (this.stockDisponibleCambio() || 0)) {
       this.errorMsg.set('No hay suficiente stock para el producto de cambio seleccionado.');
       return;
    }

    if (this.form.invalid || this.guardando()) {
      this.form.markAllAsTouched();
      return;
    }

    this.guardando.set(true);
    this.errorMsg.set('');
    const raw = this.form.value;

    let tipo = 'DEVOLUCION';
    let en_mal_estado = false;
    if (raw.flujo === 'DEV_DANO') en_mal_estado = true;
    if (raw.flujo === 'CAMBIO_SIMPLE') tipo = 'CAMBIO';
    if (raw.flujo === 'CAMBIO_DANO') { tipo = 'CAMBIO'; en_mal_estado = true; }

    const payload: any = {
      id_producto:       Number(raw.id_producto),
      id_venta:          raw.id_venta ?? 0,
      id_usuario:        this.auth.getIdUsuario() ?? 1,
      cantidad_devuelta: raw.cantidad_devuelta ?? 1,
      motivo:            raw.motivo,
      tipo:              tipo,
      en_mal_estado:     en_mal_estado
    };

    if (tipo === 'CAMBIO') {
      payload.id_producto_cambio = Number(raw.id_producto_cambio);
      payload.cantidad_cambio = raw.cantidad_cambio;
    }

    this.http.post<any>(this.apiDevoluciones, payload).subscribe({
      next: (res) => {
        this.successMsg.set(`✓ ${res.mensaje}`);
        this.guardando.set(false);
        this.form.reset({ flujo: 'DEV_SIMPLE', cantidad_devuelta: 1, cantidad_cambio: 1, motivo: '' });
        this.busquedaProducto.set('');
        this.productoSeleccionado.set(null);
        this.busquedaCambio.set('');
        this.productoCambioSeleccionado.set(null);
        this.cargarDatos();
        setTimeout(() => this.successMsg.set(''), 6000);
      },
      error: (err) => {
        this.errorMsg.set(err?.error?.error ?? 'Error al registrar la operación.');
        this.guardando.set(false);
      }
    });
  }

  // ─── Utils ────────────────────────────────────────────────────────────────
  setBusqueda(value: string): void { this.busqueda.set(value); }

  formatFecha(fecha: string): string {
    return fecha ? fecha.replace('T', ' ').slice(0, 16) : '—';
  }

  formatPrecio(centavos: number): string {
    return `$${(Math.abs(centavos) / 100).toFixed(2)}`;
  }
}
