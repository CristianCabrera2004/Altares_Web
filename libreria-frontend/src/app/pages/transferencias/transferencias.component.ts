// src/app/pages/transferencias/transferencias.component.ts
import { Component, inject, signal, OnInit, computed } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { FormBuilder, Validators, ReactiveFormsModule, FormControl } from '@angular/forms';
import { CommonModule } from '@angular/common';
import { environment } from '../../../environments/environment';
import { AuthService } from '../../core/services/auth.service';

interface Tienda {
  id_tienda: number;
  nombre: string;
  direccion?: string;
  telefono?: string;
  estado: string;
}

interface Producto {
  id_producto: number;
  nombre: string;
  id_categoria: number;
  nombre_categoria?: string;
  tasa_iva?: number;
  stock_actual: number;
  precio_venta: number;
  estado: string;
  codigo_barras?: string;
}

interface TransferenciaProductoDetalle {
  id_producto: number;
  nombre_producto: string;
  cantidad: number;
  stock_origen?: number;
}

interface Transferencia {
  id_transferencia: number;
  id_tienda_origen: number;
  tienda_origen_nombre: string;
  id_tienda_destino: number;
  tienda_destino_nombre: string;
  id_usuario: number;
  usuario_nombre: string;
  fecha: string;
  observacion: string;
  estado: string;
  requiere_confirmacion_destino: boolean;
  parcial: boolean;
  productos: TransferenciaProductoDetalle[];
}

interface TransferItem {
  producto: Producto;
  cantidad: number;
}

@Component({
  selector: 'app-transferencias',
  imports: [ReactiveFormsModule, CommonModule],
  templateUrl: './transferencias.component.html',
  styleUrl: './transferencias.component.css'
})
export class TransferenciasComponent implements OnInit {
  private readonly http = inject(HttpClient);
  private readonly fb   = inject(FormBuilder);
  readonly auth         = inject(AuthService);

  private readonly apiTiendasActivas = `${environment.apiUrl}/tiendas/activas`;
  private readonly apiProductos      = `${environment.apiUrl}/productos`;
  private readonly apiTransferencias = `${environment.apiUrl}/inventario/transferencias`;

  // ─── Roles e Identidades ──────────────────────────────────────────────────
  readonly isAdmin      = signal(this.auth.isAdmin());
  readonly tiendaUsuario = signal(this.auth.getIdTienda() ?? 1);
  readonly nombreTiendaUsuario = signal(this.auth.getNombreTienda() ?? 'Sucursal Actual');

  // ─── Estados de Carga e Historial ─────────────────────────────────────────
  readonly tiendas          = signal<Tienda[]>([]);
  readonly productos        = signal<Producto[]>([]);
  readonly transferencias   = signal<Transferencia[]>([]);
  readonly cargando         = signal(true);
  readonly guardando        = signal(false);
  readonly successMsg       = signal('');
  readonly errorMsg         = signal('');
  readonly tabActiva        = signal<'crear' | 'historial'>(this.auth.isAdmin() ? 'historial' : 'crear');
  readonly filtroHistorial  = signal<'todas' | 'enviadas' | 'recibidas'>('todas');
  readonly busquedaFiltro   = signal('');
  readonly filtroOrigen     = signal<number | null>(null);
  readonly filtroDestino    = signal<number | null>(null);

  // ─── Tipo de Operación (Para No Admins) ───────────────────────────────────
  readonly tipoOperacion    = signal<'solicitar' | 'enviar'>('solicitar');

  // ─── Formulario y Selección de Productos ──────────────────────────────────
  readonly itemsATransferir = signal<TransferItem[]>([]);
  readonly busquedaProducto = signal('');
  readonly mostrarSugerencias = signal(false);

  // Formulario reactivo principal: id_tienda_origen es obligatorio
  readonly form = this.fb.group({
    id_tienda_origen:  [null as number | null, Validators.required],
    id_tienda_destino: [null as number | null],
    observacion:       ['']
  });

  // ─── Modal de Detalles de Transferencia ───────────────────────────────────
  readonly mostrarModalDetalles = signal(false);
  readonly transferenciaSeleccionada = signal<Transferencia | null>(null);
  readonly productosEdicion = signal<TransferenciaProductoDetalle[]>([]);

  // ─── Computeds para Lógica de UI ──────────────────────────────────────────
  
  // Tiendas origen permitidas (excluye la tienda destino logueada)
  readonly tiendasOrigen = computed(() => {
    const list = this.tiendas();
    if (this.isAdmin()) {
      const dest = this.form.get('id_tienda_destino')?.value;
      return dest ? list.filter(t => t.id_tienda !== Number(dest)) : list;
    } else {
      if (this.tipoOperacion() === 'solicitar') {
        // Yo soy destino, origen puede ser cualquier otra
        return list.filter(t => t.id_tienda !== this.tiendaUsuario());
      } else {
        // Yo soy origen, la lista de origen es solo mi tienda
        return list.filter(t => t.id_tienda === this.tiendaUsuario());
      }
    }
  });

  // Tiendas destino permitidas (excluye la tienda origen seleccionada)
  readonly tiendasDestino = computed(() => {
    const list = this.tiendas();
    if (this.isAdmin()) {
      const orig = this.form.get('id_tienda_origen')?.value;
      return orig ? list.filter(t => t.id_tienda !== Number(orig)) : list;
    } else {
      if (this.tipoOperacion() === 'solicitar') {
        // Yo soy destino, la lista destino es solo mi tienda
        return list.filter(t => t.id_tienda === this.tiendaUsuario());
      } else {
        // Yo soy origen, destino puede ser cualquier otra
        return list.filter(t => t.id_tienda !== this.tiendaUsuario());
      }
    }
  });

  // Sugerencias de productos filtradas para autocompletado
  readonly sugerencias = computed(() => {
    const q = this.busquedaProducto().toLowerCase().trim();
    if (q.length < 1) return [];

    // Excluir productos ya agregados
    const idsAgregados = new Set(this.itemsATransferir().map(i => i.producto.id_producto));

    return this.productos().filter(p =>
      !idsAgregados.has(p.id_producto) &&
      p.estado === 'activo' &&
      (p.nombre.toLowerCase().includes(q) || (p.codigo_barras ?? '').toLowerCase().includes(q))
    ).slice(0, 8);
  });

  // Cantidad de pedidos pendientes que requieren acción de ESTA sucursal (como origen)
  readonly pedidosPendientesOrigen = computed(() => {
    if (this.isAdmin()) return 0;
    const tiendaId = this.tiendaUsuario();
    return this.transferencias().filter(t =>
      t.estado === 'Pendiente' &&
      !t.requiere_confirmacion_destino &&
      t.id_tienda_origen === tiendaId
    ).length;
  });

  // Cantidad de pedidos parciales que requieren confirmación de ESTA sucursal (como destino)
  readonly pedidosPendientesDestino = computed(() => {
    if (this.isAdmin()) return 0;
    const tiendaId = this.tiendaUsuario();
    return this.transferencias().filter(t =>
      t.estado === 'Pendiente' &&
      t.requiere_confirmacion_destino &&
      t.id_tienda_destino === tiendaId
    ).length;
  });

  // Total de notificaciones pendientes
  readonly totalNotificaciones = computed(() =>
    this.pedidosPendientesOrigen() + this.pedidosPendientesDestino()
  );

  // Historial filtrado dinámicamente según rol y filtros seleccionados
  readonly historialFiltrado = computed(() => {
    let list = this.transferencias();
    const q = this.busquedaFiltro().toLowerCase().trim();
    const subFiltro = this.filtroHistorial();
    const tiendaId = this.tiendaUsuario();

    // Filtro por rol / tipo de transferencia (Enviada/Recibida)
    if (!this.isAdmin()) {
      if (subFiltro === 'enviadas') {
        list = list.filter(t => t.id_tienda_origen === tiendaId);
      } else if (subFiltro === 'recibidas') {
        list = list.filter(t => t.id_tienda_destino === tiendaId);
      }
    } else {
      const origenId = this.filtroOrigen();
      const destinoId = this.filtroDestino();
      if (origenId !== null) {
        list = list.filter(t => t.id_tienda_origen === origenId);
      }
      if (destinoId !== null) {
        list = list.filter(t => t.id_tienda_destino === destinoId);
      }
    }

    // Filtro por buscador de texto
    if (q) {
      list = list.filter(t =>
        t.tienda_origen_nombre.toLowerCase().includes(q) ||
        t.tienda_destino_nombre.toLowerCase().includes(q) ||
        t.usuario_nombre.toLowerCase().includes(q) ||
        t.observacion.toLowerCase().includes(q) ||
        t.productos.some(p => p.nombre_producto.toLowerCase().includes(q))
      );
    }

    return list;
  });

  ngOnInit(): void {
    this.cargarTiendas();
    this.cargarHistorial();

    if (!this.isAdmin()) {
      this.aplicarTipoOperacion('solicitar');
    }

    // Escuchar cambios en id_tienda_origen para cargar el catálogo correspondiente
    this.form.get('id_tienda_origen')?.valueChanges.subscribe(val => {
      if (val) {
        this.cargarProductosDeTienda(Number(val));
        this.itemsATransferir.set([]);
      } else {
        this.productos.set([]);
        this.itemsATransferir.set([]);
      }
    });
  }

  aplicarTipoOperacion(tipo: 'solicitar' | 'enviar'): void {
    this.tipoOperacion.set(tipo);
    this.itemsATransferir.set([]);
    this.productos.set([]);
    if (tipo === 'solicitar') {
      this.form.patchValue({ 
        id_tienda_destino: this.tiendaUsuario(),
        id_tienda_origen: null 
      });
    } else {
      this.form.patchValue({ 
        id_tienda_origen: this.tiendaUsuario(),
        id_tienda_destino: null 
      });
      this.cargarProductosDeTienda(this.tiendaUsuario());
    }
  }

  mostrarNotificacion(msg: string): void {
    this.errorMsg.set(msg);
    setTimeout(() => {
      if (this.errorMsg() === msg) {
        this.errorMsg.set('');
      }
    }, 4000);
  }

  // ─── Carga de datos ───────────────────────────────────────────────────────
  cargarTiendas(): void {
    this.http.get<Tienda[]>(this.apiTiendasActivas).subscribe({
      next: (data) => this.tiendas.set(data),
      error: () => this.errorMsg.set('Error al cargar la lista de sucursales.')
    });
  }

  cargarProductosDeTienda(idTienda: number): void {
    const url = `${this.apiProductos}?tienda=${idTienda}`;
    this.http.get<Producto[]>(url).subscribe({
      next: (data) => this.productos.set(data),
      error: () => this.errorMsg.set('Error al cargar el inventario de la sucursal origen.')
    });
  }

  cargarHistorial(): void {
    this.cargando.set(true);
    this.http.get<Transferencia[]>(this.apiTransferencias).subscribe({
      next: (data) => {
        this.transferencias.set(data);
        this.cargando.set(false);
      },
      error: (err) => {
        this.errorMsg.set(err?.error?.error ?? 'Error al cargar el historial de transferencias.');
        this.cargando.set(false);
      }
    });
  }

  // ─── Pestañas y Navegación ────────────────────────────────────────────────
  setTab(tab: 'crear' | 'historial'): void {
    this.tabActiva.set(tab);
    this.errorMsg.set('');
    this.successMsg.set('');
    if (tab === 'historial') {
      this.cargarHistorial();
    }
  }

  // ─── Selección de Productos y Autocomplete ────────────────────────────────
  onBusquedaProductoInput(valor: string): void {
    this.busquedaProducto.set(valor);
    this.mostrarSugerencias.set(true);
  }

  seleccionarProducto(p: Producto): void {
    if (p.stock_actual <= 0) {
      this.mostrarNotificacion(`El producto "${p.nombre}" no tiene stock disponible en la tienda origen.`);
      return;
    }

    const currentItems = this.itemsATransferir();
    this.itemsATransferir.set([...currentItems, { producto: p, cantidad: 1 }]);
    this.busquedaProducto.set('');
    this.mostrarSugerencias.set(false);
  }

  ocultarSugerencias(): void {
    setTimeout(() => this.mostrarSugerencias.set(false), 200);
  }

  removerItem(index: number): void {
    const current = [...this.itemsATransferir()];
    current.splice(index, 1);
    this.itemsATransferir.set(current);
  }

  actualizarCantidad(index: number, cantidadStr: string): void {
    const cant = Number(cantidadStr);
    const current = [...this.itemsATransferir()];
    const item = current[index];

    if (isNaN(cant) || cant <= 0) {
      item.cantidad = 1;
    } else if (cant > item.producto.stock_actual) {
      this.mostrarNotificacion(`Cantidad excede el stock disponible (${item.producto.stock_actual} unidades).`);
      item.cantidad = item.producto.stock_actual;
    } else {
      item.cantidad = Math.floor(cant);
    }
    this.itemsATransferir.set(current);
  }

  // ─── Operaciones del Formulario ───────────────────────────────────────────
  realizarTransferencia(): void {
    if (this.form.invalid || this.guardando()) {
      this.form.markAllAsTouched();
      return;
    }

    const items = this.itemsATransferir();
    if (items.length === 0) {
      this.errorMsg.set('Debe agregar al menos un producto a la transferencia.');
      return;
    }

    // Validar cantidad contra stock antes de enviar
    for (const item of items) {
      if (item.cantidad <= 0) {
        this.errorMsg.set(`La cantidad para el producto "${item.producto.nombre}" debe ser mayor a 0.`);
        return;
      }
      if (item.cantidad > item.producto.stock_actual) {
        this.errorMsg.set(`El producto "${item.producto.nombre}" excede el stock disponible.`);
        return;
      }
    }

    this.guardando.set(true);
    this.errorMsg.set('');
    this.successMsg.set('');

    const raw = this.form.value;
    const payload = {
      id_tienda_origen:  Number(raw.id_tienda_origen),
      id_tienda_destino: Number(raw.id_tienda_destino),
      observacion:       raw.observacion ?? '',
      productos:         items.map(i => ({
        id_producto: i.producto.id_producto,
        cantidad:    i.cantidad
      }))
    };

    this.http.post<{ mensaje: string; id_transferencia: number }>(
      this.apiTransferencias,
      payload
    ).subscribe({
      next: (res) => {
        this.successMsg.set(`✓ ${res.mensaje} (ID: ${res.id_transferencia})`);
        this.guardando.set(false);
        this.itemsATransferir.set([]);
        this.form.reset({ id_tienda_origen: null, id_tienda_destino: null, observacion: '' });
        if (!this.isAdmin()) {
          this.aplicarTipoOperacion(this.tipoOperacion());
        }
        this.busquedaProducto.set('');
        this.productos.set([]);

        // Redirigir a pestaña historial
        setTimeout(() => {
          this.setTab('historial');
        }, 1500);
      },
      error: (err) => {
        this.errorMsg.set(err?.error?.error ?? 'Error al procesar la transferencia de inventario.');
        this.guardando.set(false);
      }
    });
  }

  // ─── Modal de detalles ───────────────────────────────────────────────────
  verDetalles(t: Transferencia): void {
    this.transferenciaSeleccionada.set(t);
    this.productosEdicion.set(JSON.parse(JSON.stringify(t.productos)));
    this.mostrarModalDetalles.set(true);
  }

  cerrarModalDetalles(): void {
    this.mostrarModalDetalles.set(false);
    this.transferenciaSeleccionada.set(null);
    this.productosEdicion.set([]);
  }

  actualizarCantidadEdicion(index: number, cantidadStr: string): void {
    const cant = Number(cantidadStr);
    const current = [...this.productosEdicion()];
    const item = current[index];
    const maxStock = item.stock_origen ?? 99999;

    if (isNaN(cant) || cant <= 0) {
      item.cantidad = 0;
    } else if (cant > maxStock) {
      this.mostrarNotificacion(`Cantidad excede el stock disponible en sucursal (${maxStock} unidades).`);
      item.cantidad = maxStock;
    } else {
      item.cantidad = Math.floor(cant);
    }
    this.productosEdicion.set(current);
  }

  removerItemEdicion(index: number): void {
    const current = [...this.productosEdicion()];
    current.splice(index, 1);
    this.productosEdicion.set(current);
  }

  // ─── Nuevas Operaciones de Aprobación ─────────────────────────────────────
  responderPedido(t: Transferencia, accion: 'aceptar' | 'rechazar', productosModificados?: TransferenciaProductoDetalle[]): void {
    this.guardando.set(true);
    this.errorMsg.set('');
    this.successMsg.set('');

    const payload = {
      id_transferencia: t.id_transferencia,
      accion: accion,
      productos: productosModificados ? productosModificados.map(p => ({
        id_producto: p.id_producto,
        cantidad: p.cantidad
      })) : undefined
    };

    this.http.post<{ mensaje: string }>(
      `${environment.apiUrl}/inventario/transferencias/responder`,
      payload
    ).subscribe({
      next: (res) => {
        this.successMsg.set(`✓ ${res.mensaje}`);
        this.guardando.set(false);
        this.cerrarModalDetalles();
        this.cargarHistorial();
      },
      error: (err) => {
        this.errorMsg.set(err?.error?.error ?? 'Error al responder el pedido.');
        this.guardando.set(false);
      }
    });
  }

  confirmarParcial(t: Transferencia, accion: 'aceptar' | 'rechazar'): void {
    this.guardando.set(true);
    this.errorMsg.set('');
    this.successMsg.set('');

    const payload = {
      id_transferencia: t.id_transferencia,
      accion: accion
    };

    this.http.post<{ mensaje: string }>(
      `${environment.apiUrl}/inventario/transferencias/confirmar-parcial`,
      payload
    ).subscribe({
      next: (res) => {
        this.successMsg.set(`✓ ${res.mensaje}`);
        this.guardando.set(false);
        this.cerrarModalDetalles();
        this.cargarHistorial();
      },
      error: (err) => {
        this.errorMsg.set(err?.error?.error ?? 'Error al confirmar la transferencia parcial.');
        this.guardando.set(false);
      }
    });
  }

  recibirTransferencia(t: Transferencia): void {
    this.guardando.set(true);
    this.errorMsg.set('');
    this.successMsg.set('');

    const payload = {
      id_transferencia: t.id_transferencia
    };

    this.http.post<{ mensaje: string }>(
      `${environment.apiUrl}/inventario/transferencias/recibir`,
      payload
    ).subscribe({
      next: (res) => {
        this.successMsg.set(`✓ ${res.mensaje}`);
        this.guardando.set(false);
        this.cerrarModalDetalles();
        this.cargarHistorial();
      },
      error: (err) => {
        this.errorMsg.set(err?.error?.error ?? 'Error al registrar la recepción.');
        this.guardando.set(false);
      }
    });
  }

  // ─── Helpers ──────────────────────────────────────────────────────────────
  formatFecha(fecha: string): string {
    return fecha ? fecha.replace('T', ' ').slice(0, 16) : '—';
  }

  setFiltroHistorial(valor: 'todas' | 'enviadas' | 'recibidas'): void {
    this.filtroHistorial.set(valor);
  }

  setFiltroOrigen(valor: string): void {
    this.filtroOrigen.set(valor === '' ? null : Number(valor));
  }

  setFiltroDestino(valor: string): void {
    this.filtroDestino.set(valor === '' ? null : Number(valor));
  }
}
