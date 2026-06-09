// src/app/pages/predicciones/predicciones.component.ts
import { Component, inject, signal, computed, OnInit, effect } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { PredictionService, Prediccion } from '../../core/services/prediction.service';
import { AuthService } from '../../core/services/auth.service';
import { jsPDF } from 'jspdf';
import autoTable from 'jspdf-autotable';

@Component({
  selector: 'app-predicciones',
  imports: [CommonModule, FormsModule],
  templateUrl: './predicciones.component.html',
  styleUrl: './predicciones.component.css'
})
export class PrediccionesComponent implements OnInit {
  private readonly svc = inject(PredictionService);
  private readonly authService = inject(AuthService);

  horizonte = signal<string>('mensual'); // 'semanal', 'mensual', 'anual'
  loading = signal(false);
  advertencia = signal('');
  error = signal('');
  diasConDatos = signal(0);
  predicciones = signal<Prediccion[]>([]);
  nombreTienda = signal(this.authService.getNombreTienda() ?? 'Todas las tiendas');

  // Filtros locales
  searchQuery = signal('');
  soloConCompra = signal(false); // Filtro local para ver solo lo que necesita comprarse
  categoriaSeleccionada = signal<string>('');
  stockFiltro = signal<string>('todos'); // 'todos', 'sin_stock', 'stock_bajo', 'stock_suficiente'

  constructor() {
    // Recargar automáticamente cuando cambia el horizonte
    effect(() => {
      this.loadData();
    });
  }

  ngOnInit(): void {}

  loadData(): void {
    this.loading.set(true);
    this.error.set('');
    this.advertencia.set('');
    
    this.svc.getPredicciones(this.horizonte()).subscribe({
      next: (res) => {
        this.diasConDatos.set(res.dias_con_datos);
        if (res.advertencia) {
          this.advertencia.set(res.advertencia);
          this.predicciones.set([]);
        } else {
          this.predicciones.set(res.predicciones || []);
        }
        this.loading.set(false);
      },
      error: () => {
        this.error.set('Error al conectar con el servidor para obtener predicciones.');
        this.loading.set(false);
      }
    });
  }

  // Extraer categorías únicas de las predicciones para el select
  categorias = computed(() => {
    const unique = new Set<string>();
    this.predicciones().forEach(p => {
      if (p.nombre_categoria) {
        unique.add(p.nombre_categoria);
      }
    });
    return Array.from(unique).sort();
  });

  // Filtrado reactivo de predicciones
  filtered = computed(() => {
    let list = this.predicciones();
    const q = this.searchQuery().trim().toLowerCase();
    
    if (q) {
      list = list.filter(p => p.nombre.toLowerCase().includes(q));
    }
    
    const cat = this.categoriaSeleccionada();
    if (cat) {
      list = list.filter(p => p.nombre_categoria === cat);
    }
    
    const stock = this.stockFiltro();
    if (stock === 'sin_stock') {
      list = list.filter(p => p.stock_actual === 0);
    } else if (stock === 'stock_bajo') {
      list = list.filter(p => p.stock_actual <= p.stock_alerta_min);
    } else if (stock === 'stock_suficiente') {
      list = list.filter(p => p.stock_actual > p.stock_alerta_min);
    }
    
    if (this.soloConCompra()) {
      list = list.filter(p => p.cantidad_a_comprar > 0);
    }
    
    return list;
  });

  // Lista de compras filtrada (solo productos que requieren abastecimiento)
  listaCompras = computed(() => {
    return this.predicciones().filter(p => p.cantidad_a_comprar > 0);
  });

  getHorizonteLabel(): string {
    switch (this.horizonte()) {
      case 'semanal': return 'Semanal (7 días)';
      case 'mensual': return 'Mensual (30 días)';
      case 'anual': return 'Anual (365 días)';
      default: return this.horizonte();
    }
  }

  descargarListaCompras(): void {
    const items = this.listaCompras();
    if (items.length === 0) {
      alert('No hay productos que requieran compras bajo esta proyección.');
      return;
    }

    const doc = new jsPDF({
      orientation: 'portrait',
      unit: 'mm',
      format: 'a4'
    });

    const fecha = new Date().toLocaleDateString('es-EC');
    const horizonteText = this.getHorizonteLabel().toUpperCase();

    // Título y Cabecera del PDF
    doc.setFontSize(18);
    doc.text('SUGERENCIA DE COMPRAS / REABASTECIMIENTO', 14, 15);
    
    doc.setFontSize(10);
    doc.text(`Librería "Los Altares" — ${this.nombreTienda()}`, 14, 22);
    doc.text(`Fecha: ${fecha}`, 14, 27);
    doc.text(`Horizonte de Proyección: ${horizonteText}`, 14, 32);
    doc.text(`Historial utilizado: ${this.diasConDatos()} días con ventas registradas`, 14, 37);

    // Tabla de compras
    const tableData = items.map((p, index) => [
      (index + 1).toString(),
      p.nombre,
      p.stock_actual.toString(),
      p.cantidad_proyectada.toString(),
      p.cantidad_a_comprar.toString()
    ]);

    autoTable(doc, {
      startY: 43,
      head: [['#', 'Producto / Artículo', 'Stock Actual', 'Proyección Demanda', 'Sugerido a Comprar']],
      body: tableData,
      theme: 'grid',
      styles: { fontSize: 9 },
      headStyles: { fillColor: [79, 142, 247], textColor: [255, 255, 255] },
      columnStyles: {
        0: { cellWidth: 10 },
        1: { cellWidth: 90 },
        2: { cellWidth: 25, halign: 'center' },
        3: { cellWidth: 35, halign: 'center' },
        4: { cellWidth: 30, halign: 'center' }
      },
      margin: { left: 14, right: 14 }
    });

    const finalY = (doc as any).lastAutoTable.finalY || 50;
    doc.setFontSize(9);
    doc.text(`* Total de artículos sugeridos para abastecimiento: ${items.length}`, 14, finalY + 10);
    doc.text('Generado automáticamente por el motor de predicciones analíticas ARIMA.', 14, finalY + 15);

    doc.save(`Lista_Compras_${this.horizonte()}_${new Date().toISOString().slice(0, 10)}.pdf`);
  }

  imprimir(): void {
    const items = this.filtered();
    if (items.length === 0) return;
    window.print();
  }
}
