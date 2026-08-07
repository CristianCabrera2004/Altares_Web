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
import { CuadernoService, ProductoCatalogo, RespuestaCuaderno } from '../../core/services/cuaderno.service';
import { AuthService } from '../../core/services/auth.service';
import { environment } from '../../../environments/environment';
import { PdfService } from '../../core/services/pdf.service';
import { jsPDF } from 'jspdf';
import { ScannerComponent } from '../../shared/components/scanner/scanner.component';

// Una línea del cuaderno en el estado local del frontend
interface ItemCuaderno {
  producto: ProductoCatalogo;
  cantidad: number;
}

@Component({
  selector: 'app-cuaderno',
  standalone: true,
  imports: [ScannerComponent],
  templateUrl: './cuaderno.component.html',
  styleUrl: './cuaderno.component.css'
})
export class CuadernoComponent implements OnInit {
  private readonly cuadernoService = inject(CuadernoService);
  private readonly authService = inject(AuthService);
  private readonly pdfService = inject(PdfService);

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

  // ── Modales ──────────────────────────────────────────────────────────────
  readonly modalVisible = signal(false);
  readonly reciboVisible = signal(false);
  readonly scannerVisible = signal(false);
  readonly efectivoCaja = signal(0);   // en centavos
  readonly guardando = signal(false);
  readonly errorMsg         = signal('');
  readonly guardadoExitoso  = signal(false);
  readonly resumen          = signal<{ id_venta: number; total: number; items: number } | null>(null);

  // ── Códigos de Barras Desconocidos ───────────────────────────────────────
  readonly barcodeDesconocido = signal('');
  readonly modoBarcode = signal<'opciones' | 'enlazar' | 'crear'>('opciones');
  readonly terminoEnlace = signal('');
  readonly enlazando = signal(false);
  readonly errorEnlace = signal('');
  readonly nuevoProducto = signal({ nombre: '', id_categoria: 1, precio_venta: 0 });
  readonly creandoProducto = signal(false);

  // ── Datos de Facturación ─────────────────────────────────────────────────
  readonly tipoCliente      = signal<'final' | 'datos'>('final');
  readonly tipoFactura      = signal<'fisica' | 'digital'>('fisica');
  readonly clienteIdentificacion = signal('');
  readonly clienteNombre    = signal('');
  readonly clienteDireccion = signal('');
  readonly clienteTelefono  = signal('');
  readonly clienteCorreo    = signal('');
  readonly metodoPago       = signal<'efectivo' | 'transferencia'>('efectivo');

  // ── Estado de búsqueda de cliente ──────────────────────────────────────────
  readonly buscandoCliente   = signal(false);
  readonly clienteEncontrado = signal(false);
  readonly clienteNuevoMsg   = signal('');

  // ── Fecha para el encabezado ─────────────────────────────────────────────
  readonly fechaHoy = new Date().toLocaleDateString('es-EC', {
    weekday: 'long', year: 'numeric', month: 'long', day: 'numeric'
  });

  // ── Modal de Confirmación ────────────────────────────────────────────────
  readonly confirmModalVisible = signal(false);
  readonly confirmModalMessage = signal('');
  private confirmAction: (() => void) | null = null;

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
      let va: any = (a as any)[campo];
      let vb: any = (b as any)[campo];
      if (typeof va === 'string') va = va.toLowerCase();
      if (typeof vb === 'string') vb = vb.toLowerCase();
      if (va < vb) return dir === 'asc' ? -1 : 1;
      if (va > vb) return dir === 'asc' ?  1 : -1;
      return 0;
    });

    // Sin filtros: primeros 20; con filtro: hasta 50
    return lista.slice(0, (t || stock !== 'todos') ? 50 : 20);
  });

  // Computed: catálogo filtrado para enlazar código de barras
  readonly resultadosEnlace = computed<ProductoCatalogo[]>(() => {
    const t = this.terminoEnlace().trim().toLowerCase();
    if (!t) return [];
    return this.catalogo()
      .filter(p => p.nombre.toLowerCase().includes(t) || p.id_producto.toString() === t)
      .slice(0, 10); // Máximo 10 resultados rápidos
  });

  // IDs que ya están en el cuaderno (para marcar visualmente en los resultados)
  readonly idsEnCuaderno = computed(
    () => new Set(this.items().map(i => i.producto.id_producto))
  );

  // ── Computed: totales con IVA extraído del precio (CA 3) ───────────────────────
  readonly totales = computed(() => {
    let base0  = 0;  // base de ítems con tasa_iva = 0 (papelería)
    let total15 = 0; // total con iva de ítems con tasa_iva = 15
    for (const item of this.items()) {
      const lineTotal = item.producto.precio_venta * item.cantidad;
      if (item.producto.tasa_iva === 0) {
        base0  += lineTotal;
      } else {
        total15 += lineTotal;
      }
    }
    
    // El precio de venta ya incluye el IVA, así que extraemos el IVA del total
    const base15 = Math.round(total15 / 1.15);
    const iva15  = total15 - base15;
    const total  = base0 + total15;
    
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

  // ── Búsqueda de cliente por cédula ────────────────────────────────────────
  private timeoutBusqueda: any;

  buscarClientePorCedula(): void {
    const ci = this.clienteIdentificacion().trim();
    
    // Solo buscar si tiene longitud válida (10 para cédula, 13 para RUC)
    if (ci.length !== 10 && ci.length !== 13) {
      this.clienteEncontrado.set(false);
      return;
    }
    
    clearTimeout(this.timeoutBusqueda);
    this.timeoutBusqueda = setTimeout(() => {
      this.buscandoCliente.set(true);
      this.clienteEncontrado.set(false);
      this.clienteNuevoMsg.set('');

      this.cuadernoService.buscarCliente(ci).subscribe({
        next: (clientes) => {
          const exacto = clientes.find(c => c.cedula_ruc === ci);
          if (exacto) {
            this.clienteNombre.set(exacto.nombre);
            this.clienteDireccion.set(exacto.direccion || '');
            this.clienteTelefono.set(exacto.telefono || '');
            this.clienteCorreo.set(exacto.email || '');
            this.clienteEncontrado.set(true);
          } else {
            // No existe, limpiar campos para que el usuario los llene
            this.clienteEncontrado.set(false);
          }
          this.buscandoCliente.set(false);
        },
        error: () => {
          this.buscandoCliente.set(false);
          this.clienteEncontrado.set(false);
        }
      });
    }, 400);
  }

  // ── Carga del catálogo activo ────────────────────────────────────────────
  cargarCatalogo(): void {
    this.cargandoCatalogo.set(true);
    this.errorCatalogo.set('');
    this.cuadernoService.getProductosActivos().subscribe({
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
    this.cuadernoService.buscarProductoPorCodigo(t).subscribe({
      next: (producto) => {
        this.agregarItem(producto);
      },
      error: () => {
        // Abrir modal de código desconocido
        this.barcodeDesconocido.set(t);
        this.modoBarcode.set('opciones');
        this.terminoEnlace.set('');
        this.errorEnlace.set('');
      }
    });
  }

  // ─── Funciones para Código de Barras Desconocido ──────────────────────────
  cerrarModalBarcode(): void {
    this.barcodeDesconocido.set('');
    this.termino.set(''); // Limpiar el input principal
  }

  confirmarEnlace(idProducto: number): void {
    const codigo = this.barcodeDesconocido();
    if (!codigo) return;

    this.enlazando.set(true);
    this.errorEnlace.set('');
    this.cuadernoService.enlazarCodigoBarras(idProducto, codigo).subscribe({
      next: () => {
        this.enlazando.set(false);
        // Recargar catálogo para tener el código en memoria
        this.cargarCatalogo();
        // Agregar el producto al carrito automáticamente
        const p = this.catalogo().find(x => x.id_producto === idProducto);
        if (p) this.agregarItem(p);
        this.cerrarModalBarcode();
      },
      error: (err) => {
        this.enlazando.set(false);
        this.errorEnlace.set(err?.error?.error || 'Error al enlazar código.');
      }
    });
  }

  confirmarCreacionRapida(): void {
    const p = this.nuevoProducto();
    const codigo = this.barcodeDesconocido();
    if (!p.nombre || p.precio_venta <= 0) {
      this.errorEnlace.set('Completa el nombre y un precio válido.');
      return;
    }

    this.creandoProducto.set(true);
    this.errorEnlace.set('');
    
    // Preparar payload de creación
    const payload = {
      nombre: p.nombre,
      id_categoria: p.id_categoria,
      precio_venta: Math.round(p.precio_venta * 100), // a centavos
      stock_alerta_min: 0,
      estado: 'activo',
      codigos_barras: [codigo]
    };

    this.cuadernoService.crearProducto(payload).subscribe({
      next: (res) => {
        this.creandoProducto.set(false);
        this.cargarCatalogo();
        if (res.producto) {
          this.agregarItem(res.producto);
        }
        this.cerrarModalBarcode();
        // Reset form
        this.nuevoProducto.set({ nombre: '', id_categoria: 1, precio_venta: 0 });
      },
      error: (err) => {
        this.creandoProducto.set(false);
        this.errorEnlace.set(err?.error?.error || 'Error al crear producto.');
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

  /** Limpia todos los datos del cliente para permitir ingresar uno nuevo */
  limpiarDatosCliente(): void {
    this.clienteIdentificacion.set('');
    this.clienteNombre.set('');
    this.clienteDireccion.set('');
    this.clienteTelefono.set('');
    this.clienteCorreo.set('');
    this.clienteEncontrado.set(false);
    this.clienteNuevoMsg.set('');
  }

  limpiarCuaderno(): void {
    if (this.items().length === 0) return;
    this.items.set([]);
    this.termino.set('');
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
  
  actualizarTerminoEnlace(val: string) {
    this.terminoEnlace.set(val);
  }

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
      // Buscar en backend si no está en el catálogo en memoria local
      this.cuadernoService.buscarProductoPorCodigo(text).subscribe({
        next: (producto) => {
          this.agregarItem(producto);
        },
        error: () => {
          this.barcodeDesconocido.set(text);
          this.modoBarcode.set('opciones');
          this.terminoEnlace.set('');
          this.errorEnlace.set('');
        }
      });
    }
  }

  // ── Funciones auxiliares ───────────────────────────────────────────────────

  /** Total de una línea incluyendo el IVA correspondiente. (El precio de venta ya incluye IVA) */
  totalLinea(item: ItemCuaderno): number {
    return item.producto.precio_venta * item.cantidad;
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
      metodo_pago: this.metodoPago(),
      items: this.items().map(item => ({
        id_producto:     item.producto.id_producto,
        cantidad:        item.cantidad,
        precio_unitario: item.producto.precio_venta,    // base sin IVA
        iva_aplicado:    item.producto.tasa_iva          // 0 ó 15
      }))
    };

    // 1. Registrar la venta en el backend
    this.cuadernoService.guardarCuaderno(payload).subscribe({
      next: (ventaRes) => {
        const idVenta = ventaRes.id_venta;

        const itemsParaRecibo: any[] = this.items().map(i => ({ cantidad: i.cantidad, producto: { nombre: i.producto.nombre, precio_venta: i.producto.precio_venta, tasa_iva: i.producto.tasa_iva } }));

        // 2. Gestionar el cliente y la creación de la factura
        if (this.tipoCliente() === 'final') {
          // Factura a Consumidor Final: Registramos la venta y creamos el recibo para descarga automática
          this.crearFacturaBackend(idVenta, 2, null, null, ventaRes, undefined, itemsParaRecibo);
        } else {
          // Factura con datos. Buscar si existe el cliente o crearlo
          const ruc = this.clienteIdentificacion().trim();
          this.cuadernoService.buscarCliente(ruc).subscribe({
            next: (clientes) => {
              const exactClient = clientes.find(c => c.cedula_ruc === ruc);
              if (exactClient) {
                // Cliente existe. Verificar si hay nuevos datos (ej. correo)
                const formNombre = this.clienteNombre() || exactClient.nombre;
                const formDireccion = this.clienteDireccion() || exactClient.direccion || 'Dirección de la Tienda';
                const formTelefono = this.clienteTelefono() || exactClient.telefono || 'N/A';
                const formEmail = this.clienteCorreo() || exactClient.email || '';

                const necesitaActualizar = 
                  (formNombre !== exactClient.nombre) ||
                  (this.clienteDireccion() && formDireccion !== exactClient.direccion) ||
                  (this.clienteTelefono() && formTelefono !== exactClient.telefono) ||
                  (this.clienteCorreo() && formEmail !== exactClient.email);

                if (necesitaActualizar) {
                  this.cuadernoService.actualizarCliente(exactClient.id_cliente, {
                    cedula_ruc: ruc,
                    nombre: formNombre,
                    direccion: formDireccion,
                    telefono: formTelefono,
                    email: formEmail
                  }).subscribe({
                    next: () => {
                      this.procederConFacturaConDatos(idVenta, exactClient.id_cliente, ventaRes, itemsParaRecibo);
                    },
                    error: (err) => {
                      console.warn("No se pudo actualizar la info del cliente", err);
                      this.procederConFacturaConDatos(idVenta, exactClient.id_cliente, ventaRes, itemsParaRecibo);
                    }
                  });
                } else {
                  this.procederConFacturaConDatos(idVenta, exactClient.id_cliente, ventaRes, itemsParaRecibo);
                }
              } else {
                // Cliente no existe, crearlo
                this.cuadernoService.crearCliente({
                  cedula_ruc: ruc,
                  nombre: this.clienteNombre(),
                  direccion: this.clienteDireccion() || 'Dirección de la Tienda',
                  telefono: this.clienteTelefono() || 'N/A',
                  email: this.clienteCorreo() || ''
                }).subscribe({
                  next: (newClientRes) => {
                    this.clienteNuevoMsg.set(`Nuevo cliente "${this.clienteNombre()}" registrado automáticamente.`);
                    setTimeout(() => this.clienteNuevoMsg.set(''), 5000);
                    this.procederConFacturaConDatos(idVenta, newClientRes.cliente.id_cliente, ventaRes, itemsParaRecibo);
                  },
                  error: (err) => {
                    this.errorMsg.set('Venta guardada, pero falló al crear el cliente para la factura: ' + (err?.error?.error ?? ''));
                    this.guardando.set(false);
                  }
                });
              }
            },
            error: () => {
              // Si falla la búsqueda, intentar crear el cliente directamente
              this.cuadernoService.crearCliente({
                cedula_ruc: ruc,
                nombre: this.clienteNombre(),
                direccion: this.clienteDireccion() || 'Dirección de la Tienda',
                telefono: this.clienteTelefono() || 'N/A',
                email: this.clienteCorreo() || ''
              }).subscribe({
                next: (newClientRes) => {
                  this.clienteNuevoMsg.set(`Nuevo cliente "${this.clienteNombre()}" registrado automáticamente.`);
                  setTimeout(() => this.clienteNuevoMsg.set(''), 5000);
                  this.procederConFacturaConDatos(idVenta, newClientRes.cliente.id_cliente, ventaRes, itemsParaRecibo);
                },
                error: (err) => {
                  this.errorMsg.set('Venta guardada, pero falló al registrar el cliente: ' + (err?.error?.error ?? ''));
                  this.guardando.set(false);
                }
              });
            }
          });
        }
      },
      error: err => {
        this.errorMsg.set(err?.error?.error ?? 'Error al guardar el cuaderno. Ningún stock fue modificado.');
        this.guardando.set(false);
      }
    });
  }

  procederConFacturaConDatos(idVenta: number, idCliente: number, ventaRes: RespuestaCuaderno, itemsParaRecibo?: any[]): void {
    const tipoFacturaId = this.tipoFactura() === 'digital' ? 3 : 2; // 3: Electrónica, 2: Física con Datos
    let pdfBase64: string | null = null;
    
    // Generar PDF
    const items = itemsParaRecibo || this.items().map(i => ({ cantidad: i.cantidad, producto: { nombre: i.producto.nombre, precio_venta: i.producto.precio_venta, tasa_iva: i.producto.tasa_iva } }));
    const doc = this.pdfService.generarPdfRecibo(
      idVenta,
      null,
      this.clienteNombre(),
      this.clienteIdentificacion(),
      items,
      this.clienteDireccion(),
      this.clienteCorreo()
    );
    
    if (this.tipoFactura() === 'digital' || (this.clienteCorreo() && this.clienteCorreo().trim() !== '')) {
      const pdfDataUri = doc.output('datauristring');
      pdfBase64 = pdfDataUri.split(',')[1];
    }

    this.crearFacturaBackend(idVenta, tipoFacturaId, idCliente, pdfBase64, ventaRes, doc);
  }

  crearFacturaBackend(idVenta: number, idTipoFactura: number, idCliente: number | null, pdfBase64: string | null, ventaRes: RespuestaCuaderno, doc?: jsPDF, itemsParaRecibo?: any[]): void {
    const payload = {
      id_venta: idVenta,
      id_tipo_factura: idTipoFactura,
      id_cliente: idCliente,
      pdf_base64: pdfBase64 || undefined
    };

    this.cuadernoService.crearFactura(payload).subscribe({
      next: () => {
        this.resumen.set({
          id_venta: idVenta,
          total:    this.totales().total,
          items:    ventaRes.items_cargados
        });
        this.guardadoExitoso.set(true);
        this.modalVisible.set(false);
        this.guardando.set(false);

        // Descargar PDF
        const items = itemsParaRecibo || this.items().map(i => ({ cantidad: i.cantidad, producto: { nombre: i.producto.nombre, precio_venta: i.producto.precio_venta, tasa_iva: i.producto.tasa_iva } }));
        const docToSave = doc || this.pdfService.generarPdfRecibo(
          idVenta,
          null,
          this.tipoCliente() === 'datos' ? this.clienteNombre() : 'Consumidor Final',
          this.tipoCliente() === 'datos' ? this.clienteIdentificacion() : '9999999999999',
          items,
          this.tipoCliente() === 'datos' ? this.clienteDireccion() : '',
          this.tipoCliente() === 'datos' ? this.clienteCorreo() : ''
        );
        const docName = idTipoFactura === 3 
          ? `Recibo_Electronico_${idVenta.toString().padStart(6, '0')}.pdf`
          : `Recibo_${idVenta.toString().padStart(6, '0')}.pdf`;
        docToSave.save(docName);

        this.items.set([]);
      },
      error: (err) => {
        this.errorMsg.set('Venta registrada, pero falló al crear el comprobante de factura: ' + (err?.error?.error ?? ''));
        this.guardando.set(false);
      }
    });
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
    this.clienteEncontrado.set(false);
    this.clienteNuevoMsg.set('');
  }
}
