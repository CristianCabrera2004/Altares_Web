// src/app/pages/inventario/inventario.component.ts
// ─────────────────────────────────────────────────────────────────────────────
// HT-02 + HU-04 — Catálogo de Productos, Bajas y Devoluciones
//
// Módulos:
//   - Ingresar producto (barcode scan / crear nuevo)
//   - Baja de merma (Caducidad | Daño | Pérdida) → POST /api/inventario/baja
//   - Devolución de producto → POST /api/devoluciones
//   - Búsqueda por código de barras → GET /api/productos/buscar
// ─────────────────────────────────────────────────────────────────────────────
import { Component, inject, signal, OnInit, computed } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { FormBuilder, FormControl, Validators, ReactiveFormsModule } from '@angular/forms';
import { environment } from '../../../environments/environment';
import { AuthService } from '../../core/services/auth.service';

interface Producto {
  id_producto: number;
  nombre: string;
  id_categoria: number;
  nombre_categoria: string;
  tasa_iva: number;
  stock_actual: number;
  stock_alerta_min: number;
  precio_venta: number;
  estado: string;
  codigo_barras?: string;
}

interface Categoria {
  id_categoria: number;
  nombre: string;
  tasa_iva: number;
}

interface ProductoResponse {
  accion: 'producto_creado' | 'stock_incrementado';
  mensaje: string;
  producto: Producto;
}

@Component({
  selector: 'app-inventario',
  imports: [ReactiveFormsModule],
  templateUrl: './inventario.component.html',
  styleUrl: './inventario.component.css'
})
export class InventarioComponent implements OnInit {
  private readonly http   = inject(HttpClient);
  private readonly fb     = inject(FormBuilder);
  private readonly auth   = inject(AuthService);

  private readonly apiProductos  = `${environment.apiUrl}/productos`;
  private readonly apiBuscar     = `${environment.apiUrl}/productos/buscar`;
  private readonly apiCategorias = `${environment.apiUrl}/categorias`;
  private readonly apiBaja       = `${environment.apiUrl}/inventario/baja`;
  private readonly apiDevoluciones = `${environment.apiUrl}/devoluciones`;

  // ─── Estado general ───────────────────────────────────────────────────────
  readonly productos    = signal<Producto[]>([]);
  readonly categorias   = signal<Categoria[]>([]);
  readonly cargando     = signal(true);
  readonly errorMsg     = signal('');
  readonly successMsg   = signal('');
  readonly busqueda     = signal('');

  // ─── Modal Ingresar Producto ──────────────────────────────────────────────
  readonly mostrarModalIngreso    = signal(false);
  readonly guardandoIngreso       = signal(false);
  readonly modoModal              = signal<'crear' | 'actualizar_stock'>('crear');
  readonly productoEncontrado     = signal<Producto | null>(null);
  readonly buscandoCodigo         = signal(false);
  readonly cantidadAgregar        = new FormControl<number>(1, [Validators.required, Validators.min(1)]);

  // ─── Modal Baja de Merma ──────────────────────────────────────────────────
  readonly mostrarModalBaja = signal(false);
  readonly productoBaja     = signal<Producto | null>(null);
  readonly guardandoBaja    = signal(false);

  readonly formBaja = this.fb.group({
    motivo:       ['', Validators.required],
    cantidad_baja:[1,  [Validators.required, Validators.min(1)]]
  });

  readonly motivos = ['Caducidad', 'Daño', 'Pérdida'] as const;

  // ─── Modal Devolución ─────────────────────────────────────────────────────
  readonly mostrarModalDevolucion = signal(false);
  readonly productoDevolucion     = signal<Producto | null>(null);
  readonly guardandoDevolucion    = signal(false);

  readonly formDevolucion = this.fb.group({
    id_venta:          [null as number | null],
    cantidad_devuelta: [1, [Validators.required, Validators.min(1)]],
    motivo:            ['', Validators.required]
  });

  // ─── Formulario Nuevo Producto ────────────────────────────────────────────
  readonly form = this.fb.group({
    codigo_barras:   [''],
    nombre:          ['', [Validators.required, Validators.minLength(2)]],
    id_categoria:    [null as number | null, Validators.required],
    precio_venta:    [null as number | null, [Validators.required, Validators.min(0)]],
    stock_actual:    [0,  [Validators.required, Validators.min(0)]],
    stock_alerta_min:[5,  [Validators.required, Validators.min(0)]]
  });

  // ─── Productos filtrados ──────────────────────────────────────────────────
  readonly productosFiltrados = computed(() => {
    const q = this.busqueda().toLowerCase().trim();
    if (!q) return this.productos();
    return this.productos().filter(p =>
      p.nombre.toLowerCase().includes(q) ||
      p.nombre_categoria.toLowerCase().includes(q)
    );
  });

  ngOnInit(): void {
    this.cargarProductos();
    this.cargarCategorias();
  }

  // ─── Cargar datos ─────────────────────────────────────────────────────────
  cargarProductos(): void {
    this.cargando.set(true);
    this.errorMsg.set('');
    this.http.get<Producto[]>(this.apiProductos).subscribe({
      next: (data) => { this.productos.set(data); this.cargando.set(false); },
      error: (err) => {
        this.errorMsg.set(err?.error?.error ?? 'Error al cargar los productos.');
        this.cargando.set(false);
      }
    });
  }

  cargarCategorias(): void {
    this.http.get<Categoria[]>(this.apiCategorias).subscribe({
      next: (data) => this.categorias.set(data),
      error: () => {}
    });
  }

  // ═══════════════════════════════════════════════════════════
  // MODAL: INGRESAR PRODUCTO
  // ═══════════════════════════════════════════════════════════
  abrirModalIngreso(): void {
    this.form.reset({ stock_actual: 0, stock_alerta_min: 5, codigo_barras: '' });
    this.cantidadAgregar.setValue(1);
    this.modoModal.set('crear');
    this.productoEncontrado.set(null);
    this.errorMsg.set('');
    this.mostrarModalIngreso.set(true);
    setTimeout(() => (document.getElementById('input-codigo-barras') as HTMLInputElement)?.focus(), 150);
  }

  cerrarModalIngreso(): void {
    this.mostrarModalIngreso.set(false);
    this.form.reset({ stock_actual: 0, stock_alerta_min: 5, codigo_barras: '' });
    this.cantidadAgregar.setValue(1);
    this.modoModal.set('crear');
    this.productoEncontrado.set(null);
    this.errorMsg.set('');
  }

  buscarPorCodigo(): void {
    const codigo = (this.form.get('codigo_barras')?.value ?? '').trim();
    if (!codigo) return;
    this.buscandoCodigo.set(true);
    this.errorMsg.set('');
    this.productoEncontrado.set(null);

    this.http.get<Producto>(`${this.apiBuscar}?codigo=${encodeURIComponent(codigo)}`).subscribe({
      next: (producto) => {
        this.productoEncontrado.set(producto);
        this.modoModal.set('actualizar_stock');
        this.cantidadAgregar.setValue(1);
        this.buscandoCodigo.set(false);
      },
      error: (err) => {
        if (err.status === 404) { this.modoModal.set('crear'); this.errorMsg.set(''); }
        else this.errorMsg.set(err?.error?.error ?? 'Error al verificar el código de barras.');
        this.buscandoCodigo.set(false);
      }
    });
  }

  limpiarCodigo(): void {
    this.form.get('codigo_barras')?.setValue('');
    this.productoEncontrado.set(null);
    this.modoModal.set('crear');
    this.errorMsg.set('');
    setTimeout(() => (document.getElementById('input-codigo-barras') as HTMLInputElement)?.focus(), 50);
  }

  guardarIngreso(): void {
    if (this.guardandoIngreso()) return;
    if (this.modoModal() === 'actualizar_stock') { this.incrementarStock(); return; }
    this.crearProducto();
  }

  private crearProducto(): void {
    if (this.form.invalid) { this.form.markAllAsTouched(); return; }
    this.guardandoIngreso.set(true);
    this.errorMsg.set('');
    const raw = this.form.value;
    const payload = {
      codigo_barras:    (raw.codigo_barras ?? '').trim(),
      nombre:           raw.nombre,
      id_categoria:     Number(raw.id_categoria),
      precio_venta:     Math.round((raw.precio_venta ?? 0) * 100),
      stock_actual:     raw.stock_actual ?? 0,
      stock_alerta_min: raw.stock_alerta_min ?? 5
    };
    this.http.post<ProductoResponse>(this.apiProductos, payload).subscribe({
      next: (res) => {
        const msg = res.accion === 'stock_incrementado'
          ? `✓ Stock de "${res.producto.nombre}" actualizado. Stock actual: ${res.producto.stock_actual}`
          : `✓ Producto "${res.producto.nombre}" creado exitosamente.`;
        this.successMsg.set(msg);
        this.guardandoIngreso.set(false);
        this.cerrarModalIngreso();
        this.cargarProductos();
        setTimeout(() => this.successMsg.set(''), 5000);
      },
      error: (err) => {
        this.errorMsg.set(err?.error?.error ?? 'Error al crear el producto.');
        this.guardandoIngreso.set(false);
      }
    });
  }

  private incrementarStock(): void {
    if (this.cantidadAgregar.invalid) return;
    const producto = this.productoEncontrado();
    if (!producto) return;
    this.guardandoIngreso.set(true);
    this.errorMsg.set('');
    const payload = {
      codigo_barras: producto.codigo_barras ?? this.form.get('codigo_barras')?.value ?? '',
      stock_actual:  this.cantidadAgregar.value ?? 1
    };
    this.http.post<ProductoResponse>(this.apiProductos, payload).subscribe({
      next: (res) => {
        this.successMsg.set(`✓ +${payload.stock_actual} unidades a "${res.producto.nombre}". Stock: ${res.producto.stock_actual}`);
        this.guardandoIngreso.set(false);
        this.cerrarModalIngreso();
        this.cargarProductos();
        setTimeout(() => this.successMsg.set(''), 5000);
      },
      error: (err) => {
        this.errorMsg.set(err?.error?.error ?? 'Error al actualizar el stock.');
        this.guardandoIngreso.set(false);
      }
    });
  }

  // ═══════════════════════════════════════════════════════════
  // MODAL: BAJA DE MERMA (HU-04)
  // ═══════════════════════════════════════════════════════════
  abrirModalBaja(p: Producto): void {
    this.productoBaja.set(p);
    this.formBaja.reset({ motivo: '', cantidad_baja: 1 });
    this.errorMsg.set('');
    this.mostrarModalBaja.set(true);
  }

  cerrarModalBaja(): void {
    this.mostrarModalBaja.set(false);
    this.productoBaja.set(null);
    this.formBaja.reset();
    this.errorMsg.set('');
  }

  registrarBaja(): void {
    if (this.formBaja.invalid || this.guardandoBaja()) return;
    const p = this.productoBaja();
    if (!p) return;

    const cantidad = this.formBaja.get('cantidad_baja')?.value ?? 0;
    if (cantidad > p.stock_actual) {
      this.errorMsg.set(`Stock insuficiente. Stock disponible: ${p.stock_actual} unidades.`);
      return;
    }

    this.guardandoBaja.set(true);
    this.errorMsg.set('');
    const payload = {
      id_producto:  p.id_producto,
      id_usuario:   this.auth.getIdUsuario() ?? 1,
      cantidad_baja: cantidad,
      motivo:       this.formBaja.get('motivo')?.value
    };

    this.http.post<{ mensaje: string; stock_nuevo: number; motivo: string }>(this.apiBaja, payload).subscribe({
      next: (res) => {
        this.successMsg.set(`✓ Baja registrada (${payload.motivo}). Nuevo stock de "${p.nombre}": ${res.stock_nuevo} u.`);
        this.guardandoBaja.set(false);
        this.cerrarModalBaja();
        this.cargarProductos();
        setTimeout(() => this.successMsg.set(''), 6000);
      },
      error: (err) => {
        this.errorMsg.set(err?.error?.error ?? 'Error al registrar la baja.');
        this.guardandoBaja.set(false);
      }
    });
  }

  // ═══════════════════════════════════════════════════════════
  // MODAL: DEVOLUCIÓN
  // ═══════════════════════════════════════════════════════════
  abrirModalDevolucion(p: Producto): void {
    this.productoDevolucion.set(p);
    this.formDevolucion.reset({ id_venta: null, cantidad_devuelta: 1, motivo: '' });
    this.errorMsg.set('');
    this.mostrarModalDevolucion.set(true);
  }

  cerrarModalDevolucion(): void {
    this.mostrarModalDevolucion.set(false);
    this.productoDevolucion.set(null);
    this.formDevolucion.reset();
    this.errorMsg.set('');
  }

  registrarDevolucion(): void {
    if (this.formDevolucion.invalid || this.guardandoDevolucion()) return;
    const p = this.productoDevolucion();
    if (!p) return;

    this.guardandoDevolucion.set(true);
    this.errorMsg.set('');
    const raw = this.formDevolucion.value;
    const payload = {
      id_producto:       p.id_producto,
      id_venta:          raw.id_venta ?? 0,
      id_usuario:        this.auth.getIdUsuario() ?? 1,
      cantidad_devuelta: raw.cantidad_devuelta ?? 1,
      motivo:            raw.motivo
    };

    this.http.post<{ mensaje: string; stock_nuevo: number }>(this.apiDevoluciones, payload).subscribe({
      next: (res) => {
        this.successMsg.set(`✓ Devolución registrada para "${p.nombre}". Stock repuesto: ${res.stock_nuevo} u.`);
        this.guardandoDevolucion.set(false);
        this.cerrarModalDevolucion();
        this.cargarProductos();
        setTimeout(() => this.successMsg.set(''), 6000);
      },
      error: (err) => {
        this.errorMsg.set(err?.error?.error ?? 'Error al registrar la devolución.');
        this.guardandoDevolucion.set(false);
      }
    });
  }

  // ─── Baja lógica (desactivar del catálogo) ───────────────────────────────
  darDeBaja(p: Producto): void {
    if (!confirm(`¿Desactivar "${p.nombre}" del catálogo?\nEl producto quedará inactivo pero sus registros se conservan.`)) return;
    this.errorMsg.set('');
    this.http.delete<{ mensaje: string }>(`${this.apiProductos}?id=${p.id_producto}`).subscribe({
      next: (res) => {
        this.successMsg.set(`✓ ${res.mensaje}`);
        this.cargarProductos();
        setTimeout(() => this.successMsg.set(''), 4000);
      },
      error: (err) => this.errorMsg.set(err?.error?.error ?? 'Error al dar de baja el producto.')
    });
  }

  // ─── Helpers ──────────────────────────────────────────────────────────────
  formatPrecio(centavos: number): string { return '$' + (centavos / 100).toFixed(2); }

  stockNivel(p: Producto): 'ok' | 'alerta' | 'critico' {
    if (p.stock_actual === 0) return 'critico';
    if (p.stock_actual <= p.stock_alerta_min) return 'alerta';
    return 'ok';
  }

  setBusqueda(value: string): void { this.busqueda.set(value); }
}
