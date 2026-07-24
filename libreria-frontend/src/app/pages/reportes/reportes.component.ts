import { Component, inject, signal, OnInit, computed } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { ReportesService, ReporteItem, FacturaResponse, FacturaCompra } from '../../core/services/reportes.service';
import { PdfService, ItemRecibo } from '../../core/services/pdf.service';
import { jsPDF } from 'jspdf';
import autoTable from 'jspdf-autotable';

@Component({
  selector: 'app-reportes',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './reportes.component.html',
  styleUrl: './reportes.component.css'
})
export class ReportesComponent implements OnInit {
  private readonly reportesService = inject(ReportesService);
  private readonly pdfService = inject(PdfService);

  readonly tab = signal<'ventas' | 'facturas' | 'compras'>('ventas');

  readonly startDate = signal('');
  readonly endDate = signal('');
  readonly categoria = signal('Todas');
  
  readonly categorias = ['Todas', 'Papelería', 'Bazar', 'Arte y Diseño', 'Tecnología'];
  
  readonly items = signal<ReporteItem[]>([]);
  readonly facturas = signal<FacturaResponse[]>([]);
  readonly compras = signal<FacturaCompra[]>([]);
  readonly loading = signal(false);
  readonly errorMsg = signal('');
  readonly loadingPdfId = signal<number | null>(null);
  readonly loadingGlobalPdf = signal(false);

  readonly fechaFiltroFacturas = signal<string>(new Date().toISOString().split('T')[0]);
  readonly fechaFiltroCompras = signal<string>('');
  
  readonly compraDetalle = signal<FacturaCompra | null>(null);

  readonly totalGlobal = computed(() => {
    return this.items().reduce((acc, curr) => acc + curr.total, 0);
  });

  ngOnInit(): void {
    const hoy = new Date();
    const hace30 = new Date();
    hace30.setDate(hoy.getDate() - 30);
    
    this.endDate.set(hoy.toISOString().split('T')[0]);
    this.startDate.set(hace30.toISOString().split('T')[0]);
    
    this.generarReporte();
    this.cargarFacturas();
  }

  generarReporte(): void {
    if (!this.startDate() || !this.endDate()) return;
    
    this.loading.set(true);
    this.errorMsg.set('');
    
    this.reportesService.getVentas(this.startDate(), this.endDate(), this.categoria()).subscribe({
      next: (data) => {
        this.items.set(data || []);
        this.loading.set(false);
      },
      error: (err) => {
        this.errorMsg.set('Error al cargar los reportes de ventas.');
        this.loading.set(false);
      }
    });
  }

  setTab(newTab: 'ventas' | 'facturas' | 'compras') {
    this.tab.set(newTab);
    if (newTab === 'facturas' && this.facturas().length === 0) {
      this.cargarFacturas();
    }
    if (newTab === 'compras' && this.compras().length === 0) {
      this.cargarCompras();
    }
  }

  cargarFacturas(): void {
    this.loading.set(true);
    this.errorMsg.set('');
    this.reportesService.getFacturas(this.fechaFiltroFacturas()).subscribe({
      next: (data) => {
        this.facturas.set(data || []);
        this.loading.set(false);
      },
      error: () => {
        this.errorMsg.set('Error al cargar el historial de recibos.');
        this.loading.set(false);
      }
    });
  }

  cargarCompras(): void {
    this.loading.set(true);
    this.errorMsg.set('');
    this.compraDetalle.set(null);
    this.reportesService.getCompras(this.fechaFiltroCompras()).subscribe({
      next: (data) => {
        this.compras.set(data || []);
        this.loading.set(false);
      },
      error: () => {
        this.errorMsg.set('Error al cargar el historial de compras.');
        this.loading.set(false);
      }
    });
  }

  verDetalleCompra(c: FacturaCompra): void {
    this.loading.set(true);
    this.errorMsg.set('');
    this.reportesService.getCompraById(c.id_factura).subscribe({
      next: (data) => {
        this.compraDetalle.set(data);
        this.loading.set(false);
      },
      error: () => {
        this.errorMsg.set('Error al cargar el detalle de la compra.');
        this.loading.set(false);
      }
    });
  }

  descargarFactura(f: FacturaResponse): void {
    this.loadingPdfId.set(f.id_factura);
    this.errorMsg.set('');
    this.reportesService.getFacturaById(f.id_factura).subscribe({
      next: (data) => {
        const items = data.items || [];
        const mappedItems: ItemRecibo[] = items.map(i => ({
          cantidad: i.cantidad,
          producto: { nombre: i.producto, precio_venta: i.precio_unitario, tasa_iva: i.iva_aplicado }
        }));
        const doc = this.pdfService.generarPdfRecibo(
          data.id_venta,
          data.fecha_emision,
          data.cliente_nombre,
          data.cliente_identificacion,
          mappedItems,
          data.cliente_direccion || ''
        );
        const name = data.id_tipo_factura === 3 
          ? `Recibo_Electronico_${data.id_venta.toString().padStart(6, '0')}.pdf`
          : `Recibo_${data.id_venta.toString().padStart(6, '0')}.pdf`;
        doc.save(name);
        this.loadingPdfId.set(null);
      },
      error: () => {
        this.errorMsg.set('No se pudo descargar el recibo.');
        this.loadingPdfId.set(null);
      }
    });
  }

  descargarFacturaGlobal(): void {
    this.loadingGlobalPdf.set(true);
    this.errorMsg.set('');
    
    // Podemos obtener el día de hoy, o permitir pasar una fecha (para empezar, usaremos hoy)
    const fechaHoy = new Date().toISOString().split('T')[0];
    
    this.reportesService.getFacturaDiaria(fechaHoy).subscribe({
      next: (data) => {
        const items = data.items || [];
        if (items.length === 0) {
          this.errorMsg.set('No hay ventas a Consumidor Final registradas el día de hoy.');
          this.loadingGlobalPdf.set(false);
          return;
        }
        
        const mappedItems: ItemRecibo[] = items.map(i => ({
          cantidad: i.cantidad,
          producto: { nombre: i.producto, precio_venta: i.precio_unitario, tasa_iva: i.iva_aplicado }
        }));
        
        const doc = this.pdfService.generarPdfRecibo(
          0, // 0 para que no salga un id_venta
          data.fecha_emision,
          data.cliente_nombre,
          data.cliente_identificacion,
          mappedItems,
          data.cliente_direccion || ''
        );
        
        const name = `Factura_Global_Diaria_${fechaHoy}.pdf`;
        doc.save(name);
        this.loadingGlobalPdf.set(false);
      },
      error: () => {
        this.errorMsg.set('Error al generar la Factura Global del Día.');
        this.loadingGlobalPdf.set(false);
      }
    });
  }

  // CA 31: Exportar a Excel (CSV nativo)
  exportarExcel(): void {
    const arr = this.items();
    if (arr.length === 0) return;
    
    let csv = 'ID,Fecha,Producto,Categoria,Cantidad,Precio Unitario,Total\n';
    
    arr.forEach(i => {
      // Precio y total en centavos, los dividimos para decimales
      const pu = (i.precio_unitario / 100).toFixed(2);
      const total = (i.total / 100).toFixed(2);
      csv += `"${i.id_producto}","${this.formatDate(i.fecha_venta)}","${i.producto}","${i.categoria}","${i.cantidad}","${pu}","${total}"\n`;
    });
    
    const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' });
    const link = document.createElement('a');
    const url = URL.createObjectURL(blob);
    
    link.setAttribute('href', url);
    link.setAttribute('download', `reporte_ventas_${this.startDate()}_a_${this.endDate()}.csv`);
    link.style.visibility = 'hidden';
    
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
  }

  // CA 30: Exportar a PDF usando jsPDF (A4, encabezado formal, descarga rápida < 3s)
  imprimirPDF(): void {
    const doc = new jsPDF({
      orientation: 'portrait',
      unit: 'mm',
      format: 'a4'
    });

    const fechaHoy = new Date().toLocaleString('es-EC');

    // Encabezado de la librería
    doc.setFontSize(16);
    doc.text('LIBRERÍA "LOS ALTARES"', 105, 15, { align: 'center' });
    
    doc.setFontSize(10);
    doc.text('RUC: 1234567890001', 14, 25);
    doc.text('Dirección: Av. Principal y Secundaria, Sangolquí', 14, 30);
    doc.text('Teléfono: (02) 233-4455', 14, 35);

    // Título del reporte
    doc.setFontSize(14);
    doc.text('REPORTE DE VENTAS', 105, 45, { align: 'center' });

    // Meta-datos del reporte
    doc.setFontSize(10);
    doc.text(`Fecha de Emisión: ${fechaHoy}`, 14, 55);
    doc.text(`Período: Desde ${this.startDate()} Hasta ${this.endDate()}`, 14, 60);
    doc.text(`Categoría Filtrada: ${this.categoria()}`, 14, 65);

    // Tabla de datos
    const tableData = this.items().map(item => [
      this.formatDate(item.fecha_venta),
      item.producto,
      item.categoria,
      item.cantidad.toString(),
      this.formatCurrency(item.total)
    ]);

    autoTable(doc, {
      startY: 70,
      head: [['Fecha', 'Descripción', 'Categoría', 'Cant.', 'Total']],
      body: tableData,
      theme: 'grid',
      headStyles: { fillColor: [79, 142, 247], textColor: [255, 255, 255] },
      columnStyles: {
        3: { halign: 'right' },
        4: { halign: 'right' }
      }
    });

    // Total Global
    const finalY = (doc as any).lastAutoTable.finalY + 10;
    doc.setFontSize(12);
    doc.setFont('helvetica', 'bold');
    doc.text(`Total del Período: ${this.formatCurrency(this.totalGlobal())}`, 196, finalY, { align: 'right' });

    // Guardado (CA 32: Inicia en menos de 3s)
    doc.save(`reporte_ventas_${this.startDate()}_a_${this.endDate()}.pdf`);
  }

  formatDate(isoString: string): string {
    if (!isoString) return '';
    if (isoString.length === 10) {
      const [year, month, day] = isoString.split('-');
      return `${day}/${month}/${year}`;
    }
    const date = new Date(isoString);
    const day = date.getDate().toString().padStart(2, '0');
    const month = (date.getMonth() + 1).toString().padStart(2, '0');
    const year = date.getFullYear();
    const hours = date.getHours().toString().padStart(2, '0');
    const minutes = date.getMinutes().toString().padStart(2, '0');
    return `${day}/${month}/${year} ${hours}:${minutes}`;
  }

  formatCurrency(centavos: number): string {
    return `$${(Math.abs(centavos) / 100).toFixed(2)}`;
  }
}
