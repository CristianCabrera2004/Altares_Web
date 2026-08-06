import { Component, inject, signal, OnInit, computed } from '@angular/core';
import { HttpClient, HttpHeaders } from '@angular/common/http';
import { environment } from '../../../environments/environment';
import { DatePipe } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { ScannerComponent } from '../../shared/components/scanner/scanner.component';

interface Producto {
  id_producto: number;
  nombre: string;
  id_categoria: number;
  nombre_categoria: string;
  tasa_iva: number;
  tipo_iva: string;
  stock_actual: number;
  stock_alerta_min: number;
  precio_venta: number;
  estado: string;
  codigos_barras?: string[];
}

interface IngresoItem {
  producto: Producto;
  cantidad: number;
  costo_unitario: number;
}

interface Proveedor {
  id_proveedor: number;
  nombre_proveedor: string;
}

@Component({
  selector: 'app-ingresos',
  imports: [DatePipe, FormsModule, ScannerComponent],
  templateUrl: './ingresos.component.html',
  styleUrl: './ingresos.component.css',
  standalone: true
})
export class IngresosComponent implements OnInit {
  private readonly http = inject(HttpClient);
  private readonly apiProductos = `${environment.apiUrl}/productos`;
  private readonly apiBuscar = `${environment.apiUrl}/productos/buscar`;
  private readonly apiIngreso = `${environment.apiUrl}/inventario/ingreso-multiple`;

  fechaHoy = new Date();

  // Estado del catálogo
  readonly catalogo = signal<Producto[]>([]);
  readonly cargandoCatalogo = signal(true);
  readonly errorCatalogo = signal('');

  // Búsqueda
  readonly termino = signal('');
  readonly sortField = signal<'nombre' | 'precio_venta' | 'stock_actual'>('nombre');
  readonly sortDir = signal<'asc' | 'desc'>('asc');

  // Estado del cuaderno (lista de ingresos)
  readonly items = signal<IngresoItem[]>([]);
  readonly idsEnCuaderno = computed(() => {
    const set = new Set<number>();
    for (const i of this.items()) set.add(i.producto.id_producto);
    return set;
  });

  // ── Escáner ──
  abrirScanner() {
    this.scannerVisible.set(true);
  }

  cerrarScanner() {
    this.scannerVisible.set(false);
  }

  onScanSuccess(decodedText: string) {
    const text = decodedText.trim().toLowerCase();
    const found = this.catalogo().find(p => 
      p.id_producto.toString() === text || 
      p.nombre.toLowerCase() === text || 
      p.nombre.toLowerCase().includes(text) ||
      (p.codigos_barras && p.codigos_barras.some(cb => cb.toLowerCase() === text))
    );

    if (found) {
      this.agregarItem(found);
    } else {
      // Si no, buscar en la API por código de barras
      this.http.get<Producto>(`${this.apiBuscar}?q=${encodeURIComponent(text)}`).subscribe({
        next: (p) => {
          if (p) {
            this.agregarItem(p);
          } else {
            alert(`Producto con código "${decodedText}" no encontrado.`);
          }
        },
        error: () => {
          alert(`Producto con código "${decodedText}" no encontrado en el catálogo activo.`);
        }
      });
    }
  }

  // ── Modal de Confirmación ──
  readonly modalVisible = signal(false);
  readonly guardando = signal(false);
  readonly guardadoExitoso = signal(false);
  readonly errorMsg = signal('');
  readonly scannerVisible = signal(false);
  readonly resumen = signal<{ items: number, totalCosto: number } | null>(null);

  // Observación (opcional)
  readonly observacion = signal('');
  proveedorId = 0;
  numeroFactura = '';
  fechaCompra = this.fechaHoy.toISOString().split('T')[0]; // Por defecto hoy
  
  // Creación rápida de proveedor
  mostrarNuevoProveedor = false;
  nuevoProveedorNombre = '';
  creandoProveedor = false;
  readonly proveedores = signal<Proveedor[]>([]);

  readonly resultados = computed(() => {
    let raw = [...this.catalogo()];
    
    // Filtro búsqueda predictiva (local si no se ha enterado)
    const t = this.termino().toLowerCase().trim();
    if (t) {
      raw = raw.filter(p => 
        p.nombre.toLowerCase().includes(t) || 
        (p.codigos_barras && p.codigos_barras.some(cb => cb.toLowerCase().includes(t))) ||
        p.id_producto.toString() === t
      );
    }

    // Ordenamiento
    const field = this.sortField();
    const dir = this.sortDir() === 'asc' ? 1 : -1;
    raw.sort((a, b) => {
      if (field === 'nombre') return a.nombre.localeCompare(b.nombre) * dir;
      if (field === 'precio_venta') return (a.precio_venta - b.precio_venta) * dir;
      if (field === 'stock_actual') return (a.stock_actual - b.stock_actual) * dir;
      return 0;
    });

    return raw.slice(0, 20); // Top 20 para no sobrecargar
  });

  readonly totales = computed(() => {
    let total = 0;
    for (const item of this.items()) {
      total += item.cantidad * item.costo_unitario;
    }
    return { total };
  });

  ngOnInit(): void {
    this.cargarCatalogo();
    this.cargarProveedores();
  }

  cargarProveedores(): void {
    this.http.get<Proveedor[]>(`${environment.apiUrl}/proveedores`).subscribe({
      next: (data) => this.proveedores.set(data ?? []),
      error: () => {}
    });
  }

  crearProveedorRapido(): void {
    if (!this.nuevoProveedorNombre.trim()) return;
    this.creandoProveedor = true;
    
    const token = localStorage.getItem('jwt_token');
    const headers = new HttpHeaders().set('Authorization', `Bearer ${token}`);
    
    const payload = {
      nombre_proveedor: this.nuevoProveedorNombre.trim(),
      identificacion: 'PROV-' + Date.now(),
      email: '',
      telefono: '',
      direccion: ''
    };

    this.http.post<any>(`${environment.apiUrl}/proveedores`, payload, { headers }).subscribe({
      next: (res) => {
        this.creandoProveedor = false;
        this.mostrarNuevoProveedor = false;
        this.nuevoProveedorNombre = '';
        
        // Recargar proveedores y seleccionar el nuevo
        this.http.get<Proveedor[]>(`${environment.apiUrl}/proveedores`).subscribe(data => {
          this.proveedores.set(data ?? []);
          // Assuming the backend returns the created ID or we can find it by name
          const prov = (data ?? []).find(p => p.nombre_proveedor.toLowerCase() === payload.nombre_proveedor.toLowerCase());
          if (prov) {
            this.proveedorId = prov.id_proveedor;
          }
        });
      },
      error: () => {
        this.creandoProveedor = false;
        alert('Error al crear proveedor');
      }
    });
  }

  cargarCatalogo() {
    this.cargandoCatalogo.set(true);
    this.errorCatalogo.set('');
    this.http.get<Producto[]>(this.apiProductos).subscribe({
      next: (data) => {
        this.catalogo.set(data.filter(p => p.estado === 'activo'));
        this.cargandoCatalogo.set(false);
      },
      error: () => {
        this.errorCatalogo.set('Error al cargar el catálogo.');
        this.cargandoCatalogo.set(false);
      }
    });
  }

  // ── Acciones del Cuaderno ──

  agregarItem(p: Producto) {
    if (this.idsEnCuaderno().has(p.id_producto)) return;
    this.items.update(list => [...list, { producto: p, cantidad: 1, costo_unitario: 0 }]);
  }

  eliminarItem(id_producto: number) {
    this.items.update(list => list.filter(i => i.producto.id_producto !== id_producto));
  }

  setCantidad(id_producto: number, qty: number) {
    if (qty < 1) qty = 1;
    this.items.update(list => list.map(i => i.producto.id_producto === id_producto ? { ...i, cantidad: qty } : i));
  }

  setCosto(id_producto: number, event: Event) {
    const target = event.target as HTMLInputElement;
    let costoDolares = parseFloat(target.value) || 0;
    if (costoDolares < 0) costoDolares = 0;
    const costoCentavos = Math.round(costoDolares * 100);
    this.items.update(list => list.map(i => i.producto.id_producto === id_producto ? { ...i, costo_unitario: costoCentavos } : i));
  }

  limpiarCuaderno() {
    this.items.set([]);
    this.observacion.set('');
  }

  setSort(field: 'nombre' | 'precio_venta' | 'stock_actual') {
    if (this.sortField() === field) {
      this.sortDir.set(this.sortDir() === 'asc' ? 'desc' : 'asc');
    } else {
      this.sortField.set(field);
      this.sortDir.set('asc');
    }
  }

  onEnterScanner() {
    const t = this.termino().trim();
    if (!t) return;
    // Si la búsqueda devuelve exactamente 1 resultado local:
    if (this.resultados().length === 1) {
      this.agregarItem(this.resultados()[0]);
      this.termino.set('');
      return;
    }

    // Si no, buscar en la API por código de barras
    this.http.get<Producto>(`${this.apiBuscar}?q=${encodeURIComponent(t)}`).subscribe({
      next: (p) => {
        if (p) {
          this.agregarItem(p);
          this.termino.set('');
        }
      },
      error: () => {}
    });
  }

  currency(centavos: number): string {
    return '$' + (centavos / 100).toFixed(2);
  }
  
  currencyVal(centavos: number): string {
    return (centavos / 100).toFixed(2);
  }

  // ── Guardado ──

  abrirModal() {
    this.errorMsg.set('');
    this.modalVisible.set(true);
  }
  cerrarModal() {
    this.modalVisible.set(false);
  }

  nuevoIngreso() {
    this.limpiarCuaderno();
    this.guardadoExitoso.set(false);
    this.resumen.set(null);
  }

  guardarIngreso() {
    if (this.items().length === 0) return;
    this.guardando.set(true);
    this.errorMsg.set('');

    const token = localStorage.getItem('jwt_token');
    if (!token) {
      this.errorMsg.set('No hay sesión activa.');
      this.guardando.set(false);
      return;
    }
    
    // Decodificar JWT para obtener id_usuario (simplificado, usualmente de Auth service, pero aquí sacamos del payload)
    let id_usuario = 0;
    try {
      const payload = JSON.parse(atob(token.split('.')[1]));
      id_usuario = payload.id_usuario;
    } catch(e) {
      this.errorMsg.set('Token inválido.');
      this.guardando.set(false);
      return;
    }

    const payload = {
      id_proveedor: this.proveedorId,
      numero_factura: this.numeroFactura,
      fecha_compra: this.fechaCompra,
      id_usuario: id_usuario,
      observacion: this.observacion(),
      items: this.items().map(i => ({
        id_producto: i.producto.id_producto,
        cantidad_ingresada: i.cantidad,
        costo_unitario: i.costo_unitario
      }))
    };

    const headers = new HttpHeaders().set('Authorization', `Bearer ${token}`);

    this.http.post<any>(this.apiIngreso, payload, { headers }).subscribe({
      next: (res) => {
        this.guardando.set(false);
        this.modalVisible.set(false);
        
        // Mostrar comprobante
        this.resumen.set({
          items: this.items().length,
          totalCosto: this.totales().total
        });
        this.guardadoExitoso.set(true);
        this.cargarCatalogo(); // recargar stock
      },
      error: (err) => {
        this.guardando.set(false);
        this.errorMsg.set(err.error?.error || 'Error al guardar el ingreso.');
      }
    });
  }
}
