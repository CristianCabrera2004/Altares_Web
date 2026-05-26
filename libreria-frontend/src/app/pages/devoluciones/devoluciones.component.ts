// src/app/pages/devoluciones/devoluciones.component.ts
// ─────────────────────────────────────────────────────────────────────────────
// Página independiente de Devoluciones (HU-04)
//
// Selector de producto con búsqueda por nombre O código de barras.
// ─────────────────────────────────────────────────────────────────────────────
import { Component, inject, signal, OnInit, computed } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { FormBuilder, FormControl, Validators, ReactiveFormsModule } from '@angular/forms';
import { CommonModule } from '@angular/common';
import { environment } from '../../../environments/environment';
import { AuthService } from '../../core/services/auth.service';

interface Producto {
  id_producto: number;
  nombre: string;
  stock_actual: number;
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
  readonly productos        = signal<Producto[]>([]);
  readonly cargando         = signal(true);
  readonly guardando        = signal(false);
  readonly successMsg       = signal('');
  readonly errorMsg         = signal('');
  readonly busqueda         = signal('');

  // ─── Búsqueda de producto en el formulario ────────────────────────────────
  readonly busquedaProducto   = signal('');           // texto del input de búsqueda
  readonly productoSeleccionado = signal<Producto | null>(null);
  readonly mostrarSugerencias  = signal(false);

  // ─── Formulario ───────────────────────────────────────────────────────────
  readonly form = this.fb.group({
    id_producto:       [null as number | null, Validators.required],
    id_venta:          [null as number | null],
    cantidad_devuelta: [1, [Validators.required, Validators.min(1)]],
    motivo:            ['', Validators.required]
  });

  // ─── Computed: sugerencias de producto filtradas ──────────────────────────
  readonly sugerencias = computed(() => {
    const q = this.busquedaProducto().toLowerCase().trim();
    if (q.length < 1) return [];
    return this.productos().filter(p =>
      p.nombre.toLowerCase().includes(q) ||
      (p.codigo_barras ?? '').toLowerCase().includes(q)
    ).slice(0, 8); // máx 8 sugerencias
  });

  // ─── Computed: historial filtrado ─────────────────────────────────────────
  readonly devolucionesFiltradas = computed(() => {
    const q = this.busqueda().toLowerCase().trim();
    if (!q) return this.devoluciones();
    return this.devoluciones().filter(d =>
      d.nombre_producto.toLowerCase().includes(q) ||
      d.motivo.toLowerCase().includes(q)
    );
  });

  readonly stockDisponible = computed(() => {
    return this.productoSeleccionado()?.stock_actual ?? null;
  });

  ngOnInit(): void {
    this.cargarDatos();
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

  // ─── Autocomplete handlers ────────────────────────────────────────────────
  onBusquedaInput(valor: string): void {
    this.busquedaProducto.set(valor);
    this.mostrarSugerencias.set(true);
    // Si se borra el texto, limpiar selección
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

  ocultarSugerencias(): void {
    // Pequeño delay para permitir el click en sugerencia
    setTimeout(() => this.mostrarSugerencias.set(false), 180);
  }

  // ─── Registrar ────────────────────────────────────────────────────────────
  registrar(): void {
    if (this.form.invalid || this.guardando()) {
      this.form.markAllAsTouched();
      return;
    }

    this.guardando.set(true);
    this.errorMsg.set('');
    const raw = this.form.value;

    const payload = {
      id_producto:       Number(raw.id_producto),
      id_venta:          raw.id_venta ?? 0,
      id_usuario:        this.auth.getIdUsuario() ?? 1,
      cantidad_devuelta: raw.cantidad_devuelta ?? 1,
      motivo:            raw.motivo
    };

    this.http.post<{ mensaje: string; stock_nuevo: number; nombre_producto: string }>(
      this.apiDevoluciones, payload
    ).subscribe({
      next: (res) => {
        this.successMsg.set(`✓ Devolución registrada para "${res.nombre_producto}". Stock repuesto: ${res.stock_nuevo} u.`);
        this.guardando.set(false);
        this.form.reset({ id_producto: null, id_venta: null, cantidad_devuelta: 1, motivo: '' });
        this.busquedaProducto.set('');
        this.productoSeleccionado.set(null);
        this.cargarDatos();
        setTimeout(() => this.successMsg.set(''), 6000);
      },
      error: (err) => {
        this.errorMsg.set(err?.error?.error ?? 'Error al registrar la devolución.');
        this.guardando.set(false);
      }
    });
  }

  setBusqueda(value: string): void { this.busqueda.set(value); }

  formatFecha(fecha: string): string {
    return fecha ? fecha.replace('T', ' ').slice(0, 16) : '—';
  }
}
