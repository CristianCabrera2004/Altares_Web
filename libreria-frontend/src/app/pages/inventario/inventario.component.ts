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
import { Router } from '@angular/router';
import { environment } from '../../../environments/environment';
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

interface Categoria {
  id_categoria: number;
  nombre: string;
  tasa_iva: number;
}

interface Proveedor {
  id_proveedor: number;
  nombre_proveedor: string;
}

interface Tienda {
  id_tienda: number;
  nombre: string;
  estado: string;
}

interface ProductoResponse {
  accion: 'producto_creado' | 'stock_incrementado';
  mensaje: string;
  producto: Producto;
}

@Component({
  selector: 'app-inventario',
  imports: [ReactiveFormsModule, ScannerComponent],
  templateUrl: './inventario.component.html',
  styleUrl: './inventario.component.css',
  standalone: true
})
export class InventarioComponent implements OnInit {
  private readonly http   = inject(HttpClient);
  private readonly fb     = inject(FormBuilder);
  private readonly router = inject(Router);

  private readonly apiProductos  = `${environment.apiUrl}/productos`;
  private readonly apiBuscar     = `${environment.apiUrl}/productos/buscar`;
  private readonly apiCategorias = `${environment.apiUrl}/categorias`;
  private readonly apiTiendas    = `${environment.apiUrl}/tiendas/activas`;
  private readonly apiTransfer   = `${environment.apiUrl}/inventario/transferencias`;

  // ─── Estado general ───────────────────────────────────────────────────────
  readonly productos    = signal<Producto[]>([]);
  readonly categorias   = signal<Categoria[]>([]);
  readonly proveedores  = signal<Proveedor[]>([]);
  readonly tiendas      = signal<Tienda[]>([]);
  readonly catalogo = signal<Producto[]>([]);
  readonly cargando = signal(true);
  readonly guardando = signal(false);
  readonly errorMsg = signal('');
  readonly scannerVisible = signal(false);
  readonly successMsg   = signal('');
  readonly busqueda     = signal('');
  readonly proveedorIngresoId = signal<number>(0);

  // ─── Modal de Confirmación ──────────────────────────────────────────────
  readonly confirmModalVisible = signal(false);
  readonly confirmModalMessage = signal('');
  private confirmAction: (() => void) | null = null;

  // ─── Filtros de ordenamiento ──────────────────────────────────────────────
  readonly sortField = signal<'nombre' | 'precio_venta' | 'stock_actual'>('nombre');
  readonly sortDir   = signal<'asc' | 'desc'>('asc');
  readonly filtroCat = signal<number | null>(null);  // null = todas las categorías

  // ─── Modal Ingresar Producto ──────────────────────────────────────────────
  readonly mostrarModalIngreso    = signal(false);
  readonly guardandoIngreso       = signal(false);
  readonly modoModal              = signal<'crear' | 'actualizar_stock'>('crear');
  readonly productoEncontrado     = signal<Producto | null>(null);
  readonly buscandoCodigo         = signal(false);
  readonly cantidadAgregar        = new FormControl<number>(1, [Validators.required, Validators.min(1)]);

  // ─── Formulario Nuevo Producto ────────────────────────────────────────────
  readonly form = this.fb.group({
    codigos_barras:   [''],
    nombre:          ['', [Validators.required, Validators.minLength(2)]],
    id_categoria:    [null as number | null, Validators.required],
    precio_venta:    [null as number | null, [Validators.required, Validators.min(0)]],
    tipo_iva:        ['grabado', Validators.required],
    stock_actual:    [0,  [Validators.required, Validators.min(0)]],
    stock_alerta_min:[5,  [Validators.required, Validators.min(0)]]
  });

  // ─── Modal Editar Producto ────────────────────────────────────────────────
  readonly mostrarModalEditar = signal(false);
  readonly productoEditar     = signal<Producto | null>(null);
  readonly guardandoEdicion   = signal(false);

  // ─── Modal Transferir ─────────────────────────────────────────────────────
  readonly modalTransferVisible = signal(false);
  readonly prodTransfer         = signal<Producto | null>(null);
  readonly tipoTransfer         = signal<'solicitar' | 'enviar'>('enviar');
  readonly transferTienda       = signal<number | null>(null);
  readonly transferObs          = signal('');
  readonly transferQty          = new FormControl<number>(1, [Validators.required, Validators.min(1)]);

  readonly formEditar = this.fb.group({
    nombre:          ['', [Validators.required, Validators.minLength(2)]],
    id_categoria:    [null as number | null, Validators.required],
    precio_venta:    [null as number | null, [Validators.required, Validators.min(0)]],
    tipo_iva:        ['grabado', Validators.required],
    stock_actual:    [0,  [Validators.required, Validators.min(0)]],
    stock_alerta_min:[5,  [Validators.required, Validators.min(0)]],
    estado:          ['activo', Validators.required]
  });

  // Estado para la edición dinámica de códigos de barras
  readonly nuevoCodigoEdicion = signal('');
  readonly guardandoCodigoEdicion = signal(false);

  // ─── Productos filtrados y ordenados ─────────────────────────────────────
  readonly productosFiltrados = computed(() => {
    const q      = this.busqueda().toLowerCase().trim();
    const campo  = this.sortField();
    const dir    = this.sortDir();
    const catId  = this.filtroCat();

    let lista = this.productos();

    // Filtrar por búsqueda de texto
    if (q) lista = lista.filter(p =>
      p.nombre.toLowerCase().includes(q) ||
      p.nombre_categoria.toLowerCase().includes(q) ||
      (p.codigos_barras && p.codigos_barras.some(c => c.includes(q)))
    );

    // Filtrar por categoría
    if (catId !== null) lista = lista.filter(p => p.id_categoria === catId);

    // Ordenar
    lista = [...lista].sort((a, b) => {
      let va: string | number = a[campo];
      let vb: string | number = b[campo];
      if (typeof va === 'string') va = va.toLowerCase();
      if (typeof vb === 'string') vb = vb.toLowerCase();
      if (va < vb) return dir === 'asc' ? -1 : 1;
      if (va > vb) return dir === 'asc' ?  1 : -1;
      return 0;
    });

    return lista;
  });

  // ─── Paginación Virtual / Scroll Infinito ──────────────────────────────
  readonly displayedCount = signal(50);

  readonly productosMostrados = computed(() => {
    return this.productosFiltrados().slice(0, this.displayedCount());
  });

  onTableScroll(event: Event) {
    const target = event.target as HTMLElement;
    // Si estamos a 100px del final, cargamos más
    if (target.scrollHeight - target.scrollTop <= target.clientHeight + 100) {
      if (this.displayedCount() < this.productosFiltrados().length) {
        this.displayedCount.update(c => c + 50);
      }
    }
  }

  readonly totalDineroInventario = computed(() => {
    return this.productosFiltrados().reduce((sum, p) => sum + (p.stock_actual > 0 ? (p.precio_venta * p.stock_actual) : 0), 0);
  });

  exportarExcel() {
    const data = this.productosFiltrados()
      .filter(p => p.stock_actual > 0)
      .map(p => ({
        ID: p.id_producto,
        Nombre: p.nombre,
        Categoria: p.nombre_categoria,
        Stock: p.stock_actual,
        'Precio Venta': p.precio_venta / 100,
        'Total ($)': (p.stock_actual * p.precio_venta) / 100
      }));

    if (data.length === 0) {
      alert('No hay productos con stock para exportar.');
      return;
    }

    import('xlsx').then(XLSX => {
      const ws = XLSX.utils.json_to_sheet(data);
      const wb = XLSX.utils.book_new();
      XLSX.utils.book_append_sheet(wb, ws, 'Inventario');
      XLSX.writeFile(wb, `Inventario_${new Date().toISOString().split('T')[0]}.xlsx`);
    });
  }

  exportarPDF() {
    const data = this.productosFiltrados().filter(p => p.stock_actual > 0);
    if (data.length === 0) {
      alert('No hay productos con stock para exportar.');
      return;
    }

    Promise.all([
      import('jspdf'),
      import('jspdf-autotable')
    ]).then(([jspdf, autoTable]) => {
      const doc = new jspdf.default();
      const rows = data.map(p => [
        p.id_producto.toString(),
        p.nombre,
        p.nombre_categoria,
        p.stock_actual.toString(),
        this.currency(p.precio_venta),
        this.currency(p.precio_venta * p.stock_actual)
      ]);

      const totalDinero = data.reduce((sum, p) => sum + (p.precio_venta * p.stock_actual), 0);

      doc.setFontSize(18);
      doc.text('Reporte de Inventario Activo', 14, 22);
      doc.setFontSize(11);
      doc.text(`Fecha: ${new Date().toLocaleDateString()}`, 14, 30);
      doc.text(`Total en Inventario: ${this.currency(totalDinero)}`, 14, 36);

      autoTable.default(doc, {
        head: [['ID', 'Producto', 'Categoría', 'Stock', 'P. Venta', 'Total']],
        body: rows,
        startY: 42,
        theme: 'grid',
        styles: { fontSize: 9 },
        headStyles: { fillColor: [79, 142, 247] }
      });

      doc.save(`Inventario_${new Date().toISOString().split('T')[0]}.pdf`);
    });
  }

  currency(val: number): string {
    return '$' + (val / 100).toFixed(2);
  }


  irATransferencias() {
    this.router.navigate(['/transferencias']);
  }

  ngOnInit(): void {
    this.cargarProductos();
    this.cargarCategorias();
    this.cargarTiendas();
    this.cargarProveedores();
  }

  cargarTiendas(): void {
    this.http.get<Tienda[]>(this.apiTiendas).subscribe({
      next: (data) => {
        // Excluir la tienda actual (1) idealmente
        this.tiendas.set(data.filter(t => t.id_tienda !== 1));
      }
    });
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

  cargarProveedores(): void {
    this.http.get<Proveedor[]>(`${environment.apiUrl}/proveedores`).subscribe({
      next: (data) => this.proveedores.set(data ?? []),
      error: () => {}
    });
  }

  // ═══════════════════════════════════════════════════════════
  // MODAL: INGRESAR PRODUCTO
  // ═══════════════════════════════════════════════════════════
  abrirModalIngreso(): void {
    this.form.reset({ stock_actual: 0, stock_alerta_min: 5, codigos_barras: '' });
    this.cantidadAgregar.setValue(1);
    this.modoModal.set('crear');
    this.productoEncontrado.set(null);
    this.errorMsg.set('');
    this.mostrarModalIngreso.set(true);
    setTimeout(() => (document.getElementById('input-codigo-barras') as HTMLInputElement)?.focus(), 150);
  }

  cerrarModalIngreso(): void {
    this.mostrarModalIngreso.set(false);
    this.form.reset({ stock_actual: 0, stock_alerta_min: 5, codigos_barras: '' });
    this.cantidadAgregar.setValue(1);
    this.modoModal.set('crear');
    this.productoEncontrado.set(null);
    this.errorMsg.set('');
  }

  buscarPorCodigo(): void {
    const codigo = (this.form.get('codigos_barras')?.value ?? '').trim();
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
    this.form.get('codigos_barras')?.setValue('');
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
    const codigosArray = (raw.codigos_barras ?? '').split(',').map((c: string) => c.trim()).filter((c: string) => c);
    const payload = {
      codigos_barras:   codigosArray,
      nombre:           raw.nombre,
      id_categoria:     Number(raw.id_categoria),
      precio_venta:     Math.round((raw.precio_venta ?? 0) * 100),
      tipo_iva:         raw.tipo_iva ?? '0%',
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
    const codigos = producto.codigos_barras && producto.codigos_barras.length > 0 ? producto.codigos_barras : [(this.form.get('codigos_barras')?.value ?? '').trim()];
    const payload = {
      codigos_barras: codigos,
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
  // MODAL: EDITAR PRODUCTO

  // ═══════════════════════════════════════════════════════════
  abrirModalEditar(p: Producto): void {
    // Clonamos el producto para que los arrays no mute en la lista principal antes de guardar
    this.productoEditar.set(JSON.parse(JSON.stringify(p)));
    this.formEditar.patchValue({
      nombre:          p.nombre,
      id_categoria:    p.id_categoria,
      precio_venta:    p.precio_venta / 100,
      tipo_iva:        p.tipo_iva,
      stock_actual:    p.stock_actual,
      stock_alerta_min: p.stock_alerta_min,
      estado:          p.estado
    });
    this.errorMsg.set('');
    this.nuevoCodigoEdicion.set('');
    this.mostrarModalEditar.set(true);
  }

  cerrarModalEditar(): void {
    this.mostrarModalEditar.set(false);
    this.productoEditar.set(null);
    this.formEditar.reset({ estado: 'activo' });
    this.errorMsg.set('');
  }

  guardarEdicion(): void {
    if (this.formEditar.invalid || this.guardandoEdicion()) {
      this.formEditar.markAllAsTouched();
      return;
    }
    const p = this.productoEditar();
    if (!p) return;

    this.guardandoEdicion.set(true);
    this.errorMsg.set('');
    const raw = this.formEditar.value;
    
    // Al guardar, enviamos los códigos actuales (que ya fueron guardados instantáneamente) 
    // solo por compatibilidad con la API PUT de actualizar producto global.
    const codigosArray = p.codigos_barras || [];
    
    const payload = {
      codigos_barras:   codigosArray,
      nombre:           raw.nombre,
      id_categoria:     Number(raw.id_categoria),
      precio_venta:     Math.round((raw.precio_venta ?? 0) * 100),
      tipo_iva:         raw.tipo_iva ?? 'grabado',
      stock_actual:     raw.stock_actual ?? 0,
      stock_alerta_min: raw.stock_alerta_min ?? 5,
      estado:           raw.estado ?? 'activo'
    };

    this.http.put<{ mensaje: string; producto: Producto }>(`${this.apiProductos}?id=${p.id_producto}`, payload).subscribe({
      next: (res) => {
        this.successMsg.set(`✓ ${res.mensaje}`);
        this.guardandoEdicion.set(false);
        this.cerrarModalEditar();
        this.cargarProductos();
        setTimeout(() => this.successMsg.set(''), 5000);
      },
      error: (err) => {
        this.errorMsg.set(err?.error?.error ?? 'Error al actualizar el producto.');
        this.guardandoEdicion.set(false);
      }
    });
  }

  // ─── Gestión dinámica de Códigos de Barras en Modal Edición ─────────────
  agregarCodigoBarraEnEdicion(): void {
    const codigo = this.nuevoCodigoEdicion().trim();
    const p = this.productoEditar();
    if (!codigo || !p || this.guardandoCodigoEdicion()) return;

    this.guardandoCodigoEdicion.set(true);
    this.errorMsg.set('');
    
    this.http.post(`${this.apiProductos}/${p.id_producto}/codigos-barras`, { codigo }).subscribe({
      next: () => {
        this.guardandoCodigoEdicion.set(false);
        this.nuevoCodigoEdicion.set('');
        // Actualizar UI localmente
        this.productoEditar.update(prod => {
          if (!prod) return prod;
          const current = prod.codigos_barras || [];
          if (!current.includes(codigo)) {
            prod.codigos_barras = [...current, codigo];
          }
          return { ...prod };
        });
        // También actualizar el catálogo principal para que se refleje inmediatamente en el grid
        this.cargarProductos(); 
      },
      error: (err) => {
        this.errorMsg.set(err?.error?.error ?? 'Error al añadir código de barras.');
        this.guardandoCodigoEdicion.set(false);
      }
    });
  }

  eliminarCodigoBarraEnEdicion(codigo: string): void {
    const p = this.productoEditar();
    if (!p || this.guardandoCodigoEdicion()) return;

    this.guardandoCodigoEdicion.set(true);
    this.errorMsg.set('');
    
    this.http.delete(`${this.apiProductos}/${p.id_producto}/codigos-barras?codigo=${encodeURIComponent(codigo)}`).subscribe({
      next: () => {
        this.guardandoCodigoEdicion.set(false);
        // Actualizar UI localmente
        this.productoEditar.update(prod => {
          if (!prod) return prod;
          prod.codigos_barras = (prod.codigos_barras || []).filter(c => c !== codigo);
          return { ...prod };
        });
        this.cargarProductos();
      },
      error: (err) => {
        this.errorMsg.set(err?.error?.error ?? 'Error al eliminar código de barras.');
        this.guardandoCodigoEdicion.set(false);
      }
    });
  }

  // ─── Baja lógica (desactivar del catálogo) ───────────────────────────────
  darDeBaja(p: Producto): void {
    this.confirmModalMessage.set(`¿Desactivar "${p.nombre}" del catálogo?\nEl producto quedará inactivo pero sus registros se conservan.`);
    this.confirmAction = () => {
      this.errorMsg.set('');
      this.http.delete<{ mensaje: string }>(`${this.apiProductos}?id=${p.id_producto}`).subscribe({
        next: (res) => {
          this.successMsg.set(`✓ ${res.mensaje}`);
          this.cargarProductos();
          this.confirmModalVisible.set(false);
          setTimeout(() => this.successMsg.set(''), 4000);
        },
        error: (err) => {
          this.errorMsg.set(err?.error?.error ?? 'Error al dar de baja el producto.');
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

  // ── Escáner ──
  abrirScanner() {
    this.scannerVisible.set(true);
  }

  cerrarScanner() {
    this.scannerVisible.set(false);
  }

  onScanSuccess(decodedText: string) {
    // Para el inventario, simplemente establecemos el texto en el buscador
    this.setBusqueda(decodedText.trim().toLowerCase());
  }

  // ── Ingreso Rápido (Modal) ────────────────────────────────────────────────
  // ─── Helpers ──────────────────────────────────────────────────────────────
  formatPrecio(centavos: number): string { return '$' + (centavos / 100).toFixed(2); }

  stockNivel(p: Producto): 'ok' | 'alerta' | 'critico' {
    if (p.stock_actual === 0) return 'critico';
    if (p.stock_actual <= p.stock_alerta_min) return 'alerta';
    return 'ok';
  }

  setBusqueda(value: string): void {
    this.displayedCount.set(50);
    this.busqueda.set(value);
  }

  setSort(campo: 'nombre' | 'precio_venta' | 'stock_actual'): void {
    this.displayedCount.set(50);
    if (this.sortField() === campo) {
      // Mismo campo → invertir dirección
      this.sortDir.update(d => d === 'asc' ? 'desc' : 'asc');
    } else {
      this.sortField.set(campo);
      this.sortDir.set('asc');
    }
  }

  setFiltroCat(valor: string): void {
    this.displayedCount.set(50);
    this.filtroCat.set(valor === '' ? null : Number(valor));
  }

  sortIcon(campo: 'nombre' | 'precio_venta' | 'stock_actual'): string {
    if (this.sortField() !== campo) return '↕';
    return this.sortDir() === 'asc' ? '↑' : '↓';
  }

  // ─── Modal Transferir ─────────────────────────────────────────────────────

  abrirModalTransferencia(p: Producto) {
    this.prodTransfer.set(p);
    this.transferQty.setValue(1);
    this.transferQty.setValidators([Validators.required, Validators.min(1), Validators.max(this.tipoTransfer() === 'enviar' ? p.stock_actual : 9999)]);
    this.transferQty.updateValueAndValidity();
    this.transferTienda.set(null);
    this.transferObs.set('');
    this.modalTransferVisible.set(true);
  }

  cerrarModalTransferencia() {
    this.modalTransferVisible.set(false);
    this.prodTransfer.set(null);
  }

  guardarTransferencia() {
    if (this.transferQty.invalid || !this.transferTienda()) return;

    const p = this.prodTransfer();
    if (!p) return;

    this.guardando.set(true);
    
    const token = localStorage.getItem('jwt_token');
    let id_usuario = 1;
    if (token) {
      try {
        const payload = JSON.parse(atob(token.split('.')[1]));
        id_usuario = payload.id_usuario;
      } catch(e) {}
    }

    const payload = {
      id_tienda_origen: this.tipoTransfer() === 'enviar' ? 1 : this.transferTienda(),
      id_tienda_destino: this.tipoTransfer() === 'enviar' ? this.transferTienda() : 1,
      id_usuario: id_usuario,
      observacion: this.transferObs(),
      productos: [{
        id_producto: p.id_producto,
        cantidad: this.transferQty.value
      }]
    };

    const headers = { Authorization: `Bearer ${token}` };

    this.http.post(this.apiTransfer, payload, { headers }).subscribe({
      next: () => {
        this.guardando.set(false);
        this.successMsg.set(`Transferencia registrada.`);
        this.cerrarModalTransferencia();
        this.cargarProductos();
        setTimeout(() => this.successMsg.set(''), 3000);
      },
      error: (err: any) => {
        this.guardando.set(false);
        this.errorMsg.set(err.error?.error || 'Error al procesar la transferencia');
        setTimeout(() => this.errorMsg.set(''), 4000);
      }
    });
  }
}
