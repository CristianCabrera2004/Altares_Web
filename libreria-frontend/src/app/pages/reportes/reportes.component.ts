import { Component, inject, signal, OnInit, computed } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { ReportesService, ReporteItem } from '../../core/services/reportes.service';

@Component({
  selector: 'app-reportes',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './reportes.component.html',
  styleUrl: './reportes.component.css'
})
export class ReportesComponent implements OnInit {
  private readonly reportesService = inject(ReportesService);

  readonly startDate = signal('');
  readonly endDate = signal('');
  readonly categoria = signal('Todas');
  
  readonly categorias = ['Todas', 'Papelería', 'Bazar', 'Arte y Diseño', 'Tecnología'];
  
  readonly items = signal<ReporteItem[]>([]);
  readonly loading = signal(false);
  readonly errorMsg = signal('');

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
  }

  generarReporte(): void {
    if (!this.startDate() || !this.endDate()) return;
    
    this.loading.set(true);
    this.errorMsg.set('');
    
    this.reportesService.getVentas(this.startDate(), this.endDate(), this.categoria()).subscribe({
      next: (data) => {
        this.items.set(data);
        this.loading.set(false);
      },
      error: (err) => {
        this.errorMsg.set('Error al cargar los reportes de ventas.');
        this.loading.set(false);
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
      csv += `"${i.id_producto}","${i.fecha_venta}","${i.producto}","${i.categoria}","${i.cantidad}","${pu}","${total}"\n`;
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

  // CA 30: Exportar a PDF (Impresión nativa)
  imprimirPDF(): void {
    window.print();
  }

  formatCurrency(centavos: number): string {
    return `$${(Math.abs(centavos) / 100).toFixed(2)}`;
  }
}
