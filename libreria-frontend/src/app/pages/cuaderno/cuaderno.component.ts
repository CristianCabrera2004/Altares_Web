// src/app/pages/cuaderno/cuaderno.component.ts
// ─────────────────────────────────────────────────────────────────────────────
// HU-01: Registro de ventas diarias del cuaderno.
//
// CA 1: Búsqueda de productos por nombre (texto) o por id_producto (código numérico).
// CA 2: Cantidad vendida editable por ítem con validación de stock.
// CA 3: Cálculo automático de total con IVA diferenciado (0% papelería / 15% resto).
// CA 4: Al guardar → POST /api/ventas/cuaderno → transacción atómica en BD.
// CA 5: Modal de verificación con arqueo de caja antes de confirmar.
// ─────────────────────────────────────────────────────────────────────────────
import { Component, inject, signal, computed, OnInit } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { AuthService } from '../../core/services/auth.service';
import { environment } from '../../../environments/environment';
import { jsPDF } from 'jspdf';
import autoTable from 'jspdf-autotable';

// ── Interfaces ───────────────────────────────────────────────────────────────
// Espeja el struct Producto del backend (incluye tasa_iva del JOIN con categorias)
export interface ProductoCatalogo {
  id_producto:      number;
  nombre:           string;
  id_categoria:     number;
  nombre_categoria: string;
  tasa_iva:         number;  // 0 ó 15 (HU-01 CA 3)
  stock_actual:     number;
  stock_alerta_min: number;
  precio_venta:     number;  // en centavos
  estado:           string;
}

// Una línea del cuaderno en el estado local del frontend
interface ItemCuaderno {
  producto: ProductoCatalogo;
  cantidad: number;
}

// Respuesta al guardar el cuaderno
interface RespuestaCuaderno {
  mensaje:        string;
  id_venta:       number;
  total:          number;
  items_cargados: number;
}

@Component({
  selector: 'app-cuaderno',
  imports: [],
  templateUrl: './cuaderno.component.html',
  styleUrl: './cuaderno.component.css'
})
export class CuadernoComponent implements OnInit {
  private readonly http        = inject(HttpClient);
  private readonly authService = inject(AuthService);

  // ── Estado del catálogo ─────────────────────────────────────────────────
  readonly catalogo         = signal<ProductoCatalogo[]>([]);
  readonly cargandoCatalogo = signal(true);
  readonly errorCatalogo    = signal('');

  // ── Búsqueda (CA 1) ─────────────────────────────────────────────────────
  readonly termino = signal('');
  readonly filtroStock = signal<'todos' | 'con_stock' | 'agotados'>('todos');
  
  // ── Ordenamiento ────────────────────────────────────────────────────────
  readonly sortField = signal<keyof ProductoCatalogo>('nombre');
  readonly sortDir   = signal<'asc' | 'desc'>('asc');

  // ── Ítems del cuaderno (CA 2) ────────────────────────────────────────────
  readonly items = signal<ItemCuaderno[]>([]);

  // ── Estado UI ────────────────────────────────────────────────────────────
  readonly modalVisible     = signal(false);
  readonly efectivoCaja     = signal(0);   // en centavos
  readonly guardando        = signal(false);
  readonly errorMsg         = signal('');
  readonly guardadoExitoso  = signal(false);
  readonly resumen          = signal<{ id_venta: number; total: number; items: number } | null>(null);

  // ── Datos de Facturación ─────────────────────────────────────────────────
  readonly tipoCliente      = signal<'final' | 'datos'>('final');
  readonly tipoFactura      = signal<'fisica' | 'digital'>('fisica');
  readonly clienteIdentificacion = signal('');
  readonly clienteNombre    = signal('');
  readonly clienteDireccion = signal('');
  readonly clienteTelefono  = signal('');
  readonly clienteCorreo    = signal('');

  // ── Fecha para el encabezado ─────────────────────────────────────────────
  readonly fechaHoy = new Date().toLocaleDateString('es-EC', {
    weekday: 'long', year: 'numeric', month: 'long', day: 'numeric'
  });

  // ── Computed: resultados de búsqueda (CA 1) ──────────────────────────────
  // Filtra por nombre (contains) o por id_producto (exacto).
  readonly resultados = computed<ProductoCatalogo[]>(() => {
    const t = this.termino().trim().toLowerCase();
    let lista = this.catalogo();

    if (t) {
      lista = lista.filter(p =>
        p.nombre.toLowerCase().includes(t) ||
        p.id_producto.toString() === t
      );
    }

    const stock = this.filtroStock();
    if (stock === 'con_stock') {
      lista = lista.filter(p => p.stock_actual > 0);
    } else if (stock === 'agotados') {
      lista = lista.filter(p => p.stock_actual === 0);
    }

    // Ordenar
    const campo = this.sortField();
    const dir = this.sortDir();
    lista = [...lista].sort((a, b) => {
      let va: string | number = a[campo];
      let vb: string | number = b[campo];
      if (typeof va === 'string') va = va.toLowerCase();
      if (typeof vb === 'string') vb = vb.toLowerCase();
      if (va < vb) return dir === 'asc' ? -1 : 1;
      if (va > vb) return dir === 'asc' ?  1 : -1;
      return 0;
    });

    // Sin filtros: primeros 20; con filtro: hasta 50
    return lista.slice(0, (t || stock !== 'todos') ? 50 : 20);
  });

  // IDs que ya están en el cuaderno (para marcar visualmente en los resultados)
  readonly idsEnCuaderno = computed(
    () => new Set(this.items().map(i => i.producto.id_producto))
  );

  // ── Computed: totales con IVA diferenciado (CA 3) ───────────────────────
  readonly totales = computed(() => {
    let base0  = 0;  // base de ítems con tasa_iva = 0 (papelería)
    let base15 = 0;  // base de ítems con tasa_iva = 15
    for (const item of this.items()) {
      const lineBase = item.producto.precio_venta * item.cantidad;
      if (item.producto.tasa_iva === 0) {
        base0  += lineBase;
      } else {
        base15 += lineBase;
      }
    }
    // Redondeo bancario en centavos
    const iva15  = Math.round(base15 * 15 / 100);
    const total  = base0 + base15 + iva15;
    return { base0, base15, iva15, total, cantidadItems: this.items().length };
  });

  // Diferencia para el arqueo de caja (CA 5)
  readonly diferencia = computed(
    () => this.efectivoCaja() - this.totales().total
  );

  // ────────────────────────────────────────────────────────────────────────
  ngOnInit(): void {
    this.cargarCatalogo();
  }

  // ── Carga del catálogo activo ────────────────────────────────────────────
  cargarCatalogo(): void {
    this.cargandoCatalogo.set(true);
    this.errorCatalogo.set('');
    this.http.get<ProductoCatalogo[]>(`${environment.apiUrl}/productos`).subscribe({
      next: data => {
        this.catalogo.set(data);
        this.cargandoCatalogo.set(false);
      },
      error: err => {
        this.errorCatalogo.set(err?.error?.error ?? 'Error al cargar el catálogo.');
        this.cargandoCatalogo.set(false);
      }
    });
  }

  // ── Ordenamiento ─────────────────────────────────────────────────────────
  setSort(campo: keyof ProductoCatalogo): void {
    if (this.sortField() === campo) {
      this.sortDir.set(this.sortDir() === 'asc' ? 'desc' : 'asc');
    } else {
      this.sortField.set(campo);
      this.sortDir.set('asc'); // Por defecto al cambiar de campo
    }
  }

  // ── Gestión de ítems (CA 2) ──────────────────────────────────────────────

  /** Agrega un producto al cuaderno o incrementa su cantidad si ya existe. */
  agregarItem(producto: ProductoCatalogo): void {
    const yaExiste = this.items().some(i => i.producto.id_producto === producto.id_producto);
    if (yaExiste) {
      this.items.update(items =>
        items.map(i =>
          i.producto.id_producto === producto.id_producto
            ? { ...i, cantidad: Math.min(i.cantidad + 1, i.producto.stock_actual) }
            : i
        )
      );
    } else {
      this.items.update(items => [...items, { producto, cantidad: 1 }]);
    }
    this.termino.set('');
  }

  /**
   * Se ejecuta al presionar Enter en el buscador (típicamente emitido por un lector de código de barras).
   * Intenta encontrar el producto localmente o mediante la API y lo agrega automáticamente.
   */
  onEnterScanner(): void {
    const t = this.termino().trim();
    if (!t) return;

    // 1. Búsqueda local (por ID o nombre exacto)
    const localMatch = this.resultados().find(p => 
      p.id_producto.toString() === t || p.nombre.toLowerCase() === t.toLowerCase()
    );

    if (localMatch) {
      this.agregarItem(localMatch);
      return;
    }

    // 2. Si no hay coincidencia local, busca por código de barras en el backend
    this.http.get<ProductoCatalogo>(`${environment.apiUrl}/productos/buscar?codigo=${encodeURIComponent(t)}`).subscribe({
      next: (producto) => {
        this.agregarItem(producto);
      },
      error: () => {
        this.errorCatalogo.set(`Producto no encontrado para el código: ${t}`);
        setTimeout(() => this.errorCatalogo.set(''), 4000);
      }
    });
  }

  /** Actualiza la cantidad de un ítem. 0 elimina el ítem. */
  setCantidad(idProducto: number, valor: number): void {
    const cant = Math.max(0, Math.floor(valor));
    if (cant === 0) {
      this.eliminarItem(idProducto);
      return;
    }
    const maxStock = this.items().find(i => i.producto.id_producto === idProducto)?.producto.stock_actual ?? 999;
    this.items.update(items =>
      items.map(i =>
        i.producto.id_producto === idProducto
          ? { ...i, cantidad: Math.min(cant, maxStock) }
          : i
      )
    );
  }

  eliminarItem(idProducto: number): void {
    this.items.update(items => items.filter(i => i.producto.id_producto !== idProducto));
  }

  limpiarCuaderno(): void {
    if (this.items().length === 0) return;
    if (!confirm('¿Limpiar todo el cuaderno? Los ítems no guardados se perderán.')) return;
    this.items.set([]);
  }

  // ── Helpers de cálculo ───────────────────────────────────────────────────

  /** Total de una línea incluyendo el IVA correspondiente. */
  totalLinea(item: ItemCuaderno): number {
    const base = item.producto.precio_venta * item.cantidad;
    return base + Math.round(base * item.producto.tasa_iva / 100);
  }

  /** Formatea centavos a string de moneda. */
  currency(centavos: number): string {
    const signo = centavos < 0 ? '-' : '';
    return `${signo}$${(Math.abs(centavos) / 100).toFixed(2)}`;
  }

  // ── Modal de arqueo (CA 5) ────────────────────────────────────────────────

  abrirModal(): void {
    if (this.items().length === 0) return;
    this.efectivoCaja.set(0);
    this.errorMsg.set('');
    this.modalVisible.set(true);
  }

  cerrarModal(): void {
    this.modalVisible.set(false);
    this.errorMsg.set('');
  }

  setEfectivo(event: Event): void {
    const val = parseFloat((event.target as HTMLInputElement).value) || 0;
    this.efectivoCaja.set(Math.round(val * 100));
  }

  // ── Guardar cuaderno (CA 4) ───────────────────────────────────────────────

  guardarCuaderno(): void {
    if (this.guardando() || this.items().length === 0) return;
    this.guardando.set(true);
    this.errorMsg.set('');

    const idUsuario = this.authService.getIdUsuario() ?? 1;

    const payload = {
      id_usuario: idUsuario,
      cliente_identificacion: this.tipoCliente() === 'datos' ? this.clienteIdentificacion() : '9999999999999',
      cliente_nombre: this.tipoCliente() === 'datos' ? this.clienteNombre() : 'Consumidor Final',
      items: this.items().map(item => ({
        id_producto:     item.producto.id_producto,
        cantidad:        item.cantidad,
        precio_unitario: item.producto.precio_venta,    // base sin IVA
        iva_aplicado:    item.producto.tasa_iva          // 0 ó 15
      }))
    };

    this.http
      .post<RespuestaCuaderno>(`${environment.apiUrl}/ventas/cuaderno`, payload)
      .subscribe({
        next: res => {
          this.resumen.set({
            id_venta: res.id_venta,
            total:    this.totales().total, // total local con IVA calculado
            items:    res.items_cargados
          });
          this.guardadoExitoso.set(true);
          this.modalVisible.set(false);
          this.guardando.set(false);

          if (this.tipoFactura() === 'fisica') {
            this.generarYDescargarFacturaFisica(res.id_venta, payload.cliente_nombre, payload.cliente_identificacion, this.items());
          }

          this.items.set([]);             // Limpiar cuaderno tras guardar
        },
        error: err => {
          this.errorMsg.set(err?.error?.error ?? 'Error al guardar el cuaderno. Ningún stock fue modificado.');
          this.guardando.set(false);
        }
      });
  }

  generarYDescargarFacturaFisica(idVenta: number, clienteNombre: string, clienteIdentificacion: string, itemsFactura: ItemCuaderno[]): void {
    const doc = new jsPDF({
      orientation: 'portrait',
      unit: 'mm',
      format: 'a5'
    });

    const numFactura = idVenta.toString().padStart(6, '0');
    const fecha = new Date().toLocaleString('es-EC');

    // Header
    doc.setFontSize(16);
    doc.text('LIBRERÍA "LOS ALTARES"', 74, 15, { align: 'center' });
    
    doc.setFontSize(10);
    doc.text(`Factura Nro: ${numFactura}`, 10, 25);
    doc.text(`Fecha: ${fecha}`, 10, 30);
    doc.text(`Cliente: ${clienteNombre}`, 10, 35);
    doc.text(`RUC/CI: ${clienteIdentificacion}`, 10, 40);

    let subtotal = 0;
    let totalIva = 0;

    const tableData = itemsFactura.map(item => {
      const totalLinea = this.totalLinea(item);
      const lineaBase = item.producto.precio_venta * item.cantidad;
      const ivaLinea = totalLinea - lineaBase;
      
      subtotal += lineaBase;
      totalIva += ivaLinea;

      return [
        item.cantidad.toString(),
        item.producto.nombre,
        this.currency(item.producto.precio_venta),
        this.currency(totalLinea)
      ];
    });

    autoTable(doc, {
      startY: 45,
      head: [['Cant', 'Descripción', 'V. Unit', 'Total']],
      body: tableData,
      theme: 'grid',
      styles: { fontSize: 8 },
      headStyles: { fillColor: [79, 142, 247] },
      margin: { left: 10, right: 10 }
    });

    // Summary
    const finalY = (doc as any).lastAutoTable.finalY || 45;
    
    doc.setFontSize(9);
    doc.text(`Subtotal: ${this.currency(subtotal)}`, 85, finalY + 10);
    doc.text(`IVA: ${this.currency(totalIva)}`, 85, finalY + 15);
    doc.setFontSize(11);
    doc.text(`TOTAL A PAGAR: ${this.currency(subtotal + totalIva)}`, 85, finalY + 22);

    doc.setFontSize(10);
    doc.text('¡Gracias por su compra!', 74, finalY + 35, { align: 'center' });

    doc.save(`Factura_${numFactura}.pdf`);
  }

  nuevoCuaderno(): void {
    this.guardadoExitoso.set(false);
    this.resumen.set(null);
    this.termino.set('');
    this.tipoCliente.set('final');
    this.tipoFactura.set('fisica');
    this.clienteIdentificacion.set('');
    this.clienteNombre.set('');
    this.clienteDireccion.set('');
    this.clienteTelefono.set('');
    this.clienteCorreo.set('');
  }
}
