import { Injectable } from '@angular/core';
import { jsPDF } from 'jspdf';
import autoTable from 'jspdf-autotable';

export interface ItemRecibo {
  cantidad: number;
  producto: {
    nombre: string;
    precio_venta: number; // en centavos, base + iva si aplica
    tasa_iva: number; // 0 o 15
  };
}

@Injectable({
  providedIn: 'root'
})
export class PdfService {

  /** Total de una línea incluyendo el IVA correspondiente. (El precio de venta ya incluye IVA) */
  private totalLinea(item: ItemRecibo): number {
    return item.producto.precio_venta * item.cantidad;
  }

  /** Formatea centavos a string de moneda. */
  private currency(centavos: number): string {
    const signo = centavos < 0 ? '-' : '';
    return `${signo}$${(Math.abs(centavos) / 100).toFixed(2)}`;
  }

  /**
   * Genera un recibo en PDF en tamaño A5
   */
  generarPdfRecibo(
    idVenta: number,
    fechaEmision: string | null,
    clienteNombre: string,
    clienteIdentificacion: string,
    itemsFactura: ItemRecibo[],
    clienteDireccion?: string,
    clienteCorreo?: string
  ): jsPDF {
    const doc = new jsPDF({
      orientation: 'portrait',
      unit: 'mm',
      format: 'a5'
    });

    const numRecibo = idVenta.toString().padStart(6, '0');
    // Si no viene fecha, usamos la actual
    const fecha = fechaEmision ? new Date(fechaEmision).toLocaleString('es-EC') : new Date().toLocaleString('es-EC');

    // Header - Nuevo Formato "Recibo"
    doc.setFontSize(14);
    doc.setFont('helvetica', 'bold');
    doc.text('Libreria y Papelería "Los Altares"', 74, 15, { align: 'center' });
    
    doc.setFontSize(9);
    doc.setFont('helvetica', 'normal');
    doc.text('RUC: 0604928325001', 74, 20, { align: 'center' });
    doc.text('Dirección: Quimiag Centro', 74, 25, { align: 'center' });
    doc.text('Teléfono: 098 321 9219', 74, 30, { align: 'center' });
    
    doc.setFontSize(10);
    doc.text(`Recibo N.- ${numRecibo}`, 10, 40);
    doc.text(`Fecha: ${fecha}`, 10, 45);
    doc.text(`Cliente: ${clienteNombre}`, 10, 50);
    doc.text(`RUC/CI: ${clienteIdentificacion}`, 10, 55);

    let startY = 60;
    if (clienteDireccion && clienteDireccion.trim() !== '') {
      doc.text(`Dirección: ${clienteDireccion}`, 10, startY);
      startY += 6;
    }
    
    if (clienteCorreo && clienteCorreo.trim() !== '') {
      doc.text(`Correo: ${clienteCorreo}`, 10, startY);
      startY += 6;
    }

    let subtotal = 0;
    let totalIva = 0;

    const tableData = itemsFactura.map(item => {
      const totalLinea = this.totalLinea(item);
      
      let lineaBase = totalLinea;
      let ivaLinea = 0;
      if (item.producto.tasa_iva > 0) {
        lineaBase = Math.round(totalLinea / (1 + item.producto.tasa_iva / 100));
        ivaLinea = totalLinea - lineaBase;
      }
      
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
      startY: startY,
      head: [['Cant', 'Descripción', 'V. Unit', 'Total']],
      body: tableData,
      theme: 'grid',
      styles: { fontSize: 8 },
      headStyles: { fillColor: [79, 142, 247] },
      margin: { left: 10, right: 10 }
    });

    // Summary
    const finalY = (doc as any).lastAutoTable.finalY || 60;
    
    doc.setFontSize(9);
    doc.text(`Subtotal: ${this.currency(subtotal)}`, 85, finalY + 10);
    doc.text(`IVA: ${this.currency(totalIva)}`, 85, finalY + 15);
    doc.setFontSize(11);
    doc.setFont('helvetica', 'bold');
    doc.text(`TOTAL A PAGAR: ${this.currency(subtotal + totalIva)}`, 85, finalY + 22);

    doc.setFontSize(10);
    doc.setFont('helvetica', 'normal');
    doc.text('¡Gracias por su compra!', 74, finalY + 35, { align: 'center' });

    return doc;
  }
}
