// src/app/pages/bajas/bajas.component.ts
// ─────────────────────────────────────────────────────────────────────────────
// Página independiente de Bajas por Merma (HU-04)
//
// Selector de producto con búsqueda por nombre O código de barras.
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
  codigo_barras?: string;
}

interface MovimientoBaja {
  id_movimiento: number;
  id_producto: number;
  nombre_producto: string;
  tipo_movimiento: string;
  cantidad: number;
  stock_resultante: number;
  referencia_id: number | null;
  fecha_movimiento: string;
}

@Component({
  selector: 'app-bajas',
  imports: [ReactiveFormsModule, CommonModule],
  templateUrl: './bajas.component.html',
  styleUrl: './bajas.component.css'
})
export class BajasComponent implements OnInit {
  private readonly http = inject(HttpClient);
  private readonly fb   = inject(FormBuilder);
  private readonly auth = inject(AuthService);

  private readonly apiProductos   = `${environment.apiUrl}/productos`;
  private readonly apiBaja        = `${environment.apiUrl}/inventario/baja`;
  private readonly apiMovimientos = `${environment.apiUrl}/inventario/movimientos`;

  readonly motivos = ['Caducidad', 'Daño', 'Pérdida'] as const;

  // ─── Estado ───────────────────────────────────────────────────────────────
  readonly bajas      = signal<MovimientoBaja[]>([]);
  readonly productos  = signal<Producto[]>([]);
  readonly cargando   = signal(true);
  readonly guardando  = signal(false);
  readonly successMsg = signal('');
  readonly errorMsg   = signal('');
  readonly busqueda   = signal('');

  // ─── Búsqueda de producto en el formulario ────────────────────────────────
  readonly busquedaProducto    = signal('');
  readonly productoSeleccionado = signal<Producto | null>(null);
  readonly mostrarSugerencias   = signal(false);

  // ─── Formulario ───────────────────────────────────────────────────────────
  readonly form = this.fb.group({
    id_producto:  [null as number | null, Validators.required],
    motivo:       ['', Validators.required],
    cantidad_baja:[1, [Validators.required, Validators.min(1)]]
  });

  // ─── Computed: sugerencias de producto filtradas ──────────────────────────
  readonly sugerencias = computed(() => {
    const q = this.busquedaProducto().toLowerCase().trim();
    if (q.length < 1) return [];
    return this.productos().filter(p =>
      p.nombre.toLowerCase().includes(q) ||
      (p.codigo_barras ?? '').toLowerCase().includes(q)
    ).slice(0, 8);
  });

  // ─── Computed: historial filtrado ─────────────────────────────────────────
  readonly bajasFiltradas = computed(() => {
    const q = this.busqueda().toLowerCase().trim();
    if (!q) return this.bajas();
    return this.bajas().filter(b =>
      b.nombre_producto.toLowerCase().includes(q) ||
      b.tipo_movimiento.toLowerCase().includes(q)
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

    this.http.get<MovimientoBaja[]>(this.apiMovimientos).subscribe({
      next: (data) => {
        this.bajas.set(data.filter(m => m.tipo_movimiento === 'BAJA_MERMA'));
        this.cargando.set(false);
      },
      error: (err) => {
        this.errorMsg.set(err?.error?.error ?? 'Error al cargar el historial de bajas.');
        this.cargando.set(false);
      }
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
    setTimeout(() => this.mostrarSugerencias.set(false), 180);
  }

  // ─── Registrar ────────────────────────────────────────────────────────────
  registrar(): void {
    if (this.form.invalid || this.guardando()) {
      this.form.markAllAsTouched();
      return;
    }

    const raw        = this.form.value;
    const idProducto = Number(raw.id_producto);
    const cantidad   = raw.cantidad_baja ?? 0;
    const stock      = this.stockDisponible() ?? 0;

    if (cantidad > stock) {
      this.errorMsg.set(`Stock insuficiente. Disponible: ${stock} unidades.`);
      return;
    }

    this.guardando.set(true);
    this.errorMsg.set('');

    const payload = {
      id_producto:   idProducto,
      id_usuario:    this.auth.getIdUsuario() ?? 1,
      cantidad_baja: cantidad,
      motivo:        raw.motivo
    };

    this.http.post<{ mensaje: string; stock_nuevo: number; motivo: string }>(
      this.apiBaja, payload
    ).subscribe({
      next: (res) => {
        const prod = this.productoSeleccionado();
        this.successMsg.set(`✓ Baja registrada (${res.motivo}). Nuevo stock de "${prod?.nombre ?? ''}": ${res.stock_nuevo} u.`);
        this.guardando.set(false);
        this.form.reset({ id_producto: null, motivo: '', cantidad_baja: 1 });
        this.busquedaProducto.set('');
        this.productoSeleccionado.set(null);
        this.cargarDatos();
        setTimeout(() => this.successMsg.set(''), 6000);
      },
      error: (err) => {
        this.errorMsg.set(err?.error?.error ?? 'Error al registrar la baja.');
        this.guardando.set(false);
      }
    });
  }

  setBusqueda(value: string): void { this.busqueda.set(value); }

  formatFecha(fecha: string): string {
    return fecha ? fecha.replace('T', ' ').slice(0, 16) : '—';
  }

  cantidadAbs(n: number): number { return Math.abs(n); }
}
