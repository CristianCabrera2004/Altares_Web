// src/app/pages/dashboard/dashboard.component.ts
import { Component, inject, signal, OnInit, AfterViewInit, ViewChild, ElementRef } from '@angular/core';
import { CommonModule } from '@angular/common';
import { AuthService } from '../../core/services/auth.service';
import { PredictionService, Prediccion, PredictionResponse } from '../../core/services/prediction.service';
import { InvoiceService, InvoiceSummary } from '../../core/services/invoice.service';
import { DashboardService } from '../../core/services/dashboard.service';
import { Chart, registerables } from 'chart.js';

Chart.register(...registerables);

@Component({
  selector: 'app-dashboard',
  standalone: true,
  imports: [CommonModule],
  templateUrl: './dashboard.component.html',
  styleUrl: './dashboard.component.css'
})
export class DashboardComponent implements OnInit, AfterViewInit {
  private readonly authService = inject(AuthService);
  private readonly predictionService = inject(PredictionService);
  private readonly invoiceService = inject(InvoiceService);
  private readonly dashboardService = inject(DashboardService);

  readonly nombreUsuario = signal('');
  readonly rolUsuario = signal('');
  readonly fechaActual = signal('');
  
  readonly nombre = signal('');
  readonly isAdmin = signal(false);
  readonly horaActual = signal('');

  // HU-03: Predicciones
  readonly predicciones = signal<Prediccion[]>([]);
  readonly loadingPredicciones = signal(true);
  readonly errorPredicciones = signal('');
  /** HU-03 CA#14 — Mensaje cuando el histórico es < 14 días */
  readonly advertenciaPredicciones = signal('');
  readonly diasConDatos = signal(0);
  readonly horizonteProyeccion = signal<7 | 15>(15);

  // HU-06 CA#26: KPI Stock Bajo (dato real)
  readonly stockBajoCount = signal<number | null>(null);

  // HU-02: Cierre de Caja
  readonly modalInvoiceVisible = signal(false);
  readonly invoiceData = signal<InvoiceSummary | null>(null);
  readonly procesandoCierre = signal(false);
  readonly errorInvoice = signal('');

  @ViewChild('salesChart') salesChartRef!: ElementRef;
  private chartInstance: Chart | null = null;

  // Período activo del histórico de ventas
  readonly periodoGrafica  = signal<'7' | '15' | '30' | '365' | '0'>('15');
  readonly graficaCargando = signal(false);
  readonly graficaError    = signal('');
  readonly periodos = [
    { valor: '7',   etiqueta: 'Semana' },
    { valor: '15',  etiqueta: '15 días' },
    { valor: '30',  etiqueta: 'Mes' },
    { valor: '365', etiqueta: 'Año' },
    { valor: '0',   etiqueta: 'General' }
  ] as const;

  ngOnInit(): void {
    const nombre = this.authService.getNombre();
    if (nombre) {
      this.nombreUsuario.set(nombre);
      this.nombre.set(nombre);
      this.rolUsuario.set(this.authService.getRol() ?? '');
    }

    this.isAdmin.set(this.authService.isAdmin());
    this.actualizarHora();
    setInterval(() => this.actualizarHora(), 60000);

    const opciones: Intl.DateTimeFormatOptions = { 
      weekday: 'long', year: 'numeric', month: 'long', day: 'numeric' 
    };
    this.fechaActual.set(new Date().toLocaleDateString('es-ES', opciones));

    this.cargarPredicciones();
    this.cargarStockBajo(); // HU-06 CA#26
  }

  private actualizarHora(): void {
    const ahora = new Date();
    this.horaActual.set(ahora.toLocaleTimeString('es-EC', { hour: '2-digit', minute: '2-digit' }));
  }

  ngAfterViewInit(): void {
    this.cargarGrafica();
  }

  /** HU-06 CA#26 — Carga conteo real de productos bajo stock mínimo */
  cargarStockBajo(): void {
    this.dashboardService.getStockBajoCount().subscribe({
      next: (count) => this.stockBajoCount.set(count),
      error: () => this.stockBajoCount.set(null)
    });
  }

  cargarGrafica(periodo?: '7' | '15' | '30' | '365' | '0'): void {
    const p = periodo ?? this.periodoGrafica();
    this.graficaCargando.set(true);
    this.graficaError.set('');

    this.dashboardService.getGraficaVentas(p).subscribe({
      next: (rawData) => {
        this.graficaCargando.set(false);
        const data = rawData ?? [];

        const labels = data.map(d => {
          const parts = d.fecha.split('-');
          return `${parts[2]}/${parts[1]}`;
        });
        const values = data.map(d => d.total / 100);

        if (!this.salesChartRef?.nativeElement) return;
        const ctx = this.salesChartRef.nativeElement.getContext('2d');

        if (this.chartInstance) {
          // Actualizar datos sin destruir el chart (más fluido)
          this.chartInstance.data.labels = labels;
          this.chartInstance.data.datasets[0].data = values;
          this.chartInstance.update();
          return;
        }

        // Primera vez: crear el chart
        this.chartInstance = new Chart(ctx, {
          type: 'line',
          data: {
            labels,
            datasets: [{
              label: 'Ventas (USD)',
              data: values,
              borderColor: '#4F8EF7',
              backgroundColor: 'rgba(79, 142, 247, 0.1)',
              borderWidth: 3,
              pointBackgroundColor: '#22c55e',
              pointBorderColor: '#16181f',
              pointBorderWidth: 2,
              pointRadius: 4,
              pointHoverRadius: 6,
              fill: true,
              tension: 0.4
            }]
          },
          options: {
            responsive: true,
            maintainAspectRatio: false,
            plugins: {
              legend: { display: false },
              tooltip: {
                backgroundColor: '#1e2130',
                titleColor: '#a0a8bb',
                bodyColor: '#f0f2f8',
                borderColor: '#2d3148',
                borderWidth: 1,
                padding: 12,
                displayColors: false,
                callbacks: {
                  label: function(context) {
                    let label = context.dataset.label || '';
                    if (label) { label += ': '; }
                    if (context.parsed.y !== null) {
                      label += new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(context.parsed.y);
                    }
                    return label;
                  }
                }
              }
            },
            scales: {
              x: {
                grid: { color: 'rgba(45, 49, 72, 0.4)' },
                ticks: { color: '#6b7280', font: { family: 'inherit', size: 11 } }
              },
              y: {
                grid: { color: 'rgba(45, 49, 72, 0.4)' },
                ticks: { color: '#6b7280', font: { family: 'inherit', size: 11 },
                  callback: function(value) { return '$' + value; }
                },
                beginAtZero: true
              }
            }
          }
        });
      },
      error: (err) => {
        this.graficaCargando.set(false);
        this.graficaError.set('No se pudieron cargar los datos de ventas. Verifica la conexión.');
        console.error('Error gráfica:', err);
      }
    });
  }

  cargarPredicciones(): void {
    this.loadingPredicciones.set(true);
    this.advertenciaPredicciones.set('');
    this.predictionService.getPredicciones(this.horizonteProyeccion()).subscribe({
      next: (res: PredictionResponse) => {
        this.diasConDatos.set(res.dias_con_datos);
        if (res.advertencia) {
          // CA#14: Historial insuficiente — mostrar advertencia en lugar de predicciones
          this.advertenciaPredicciones.set(res.advertencia);
          this.predicciones.set([]);
        } else {
          const top5 = res.predicciones
            .sort((a, b) => b.cantidad_proyectada - a.cantidad_proyectada)
            .slice(0, 5);
          this.predicciones.set(top5);
        }
        this.loadingPredicciones.set(false);
      },
      error: (err) => {
        console.error('Error al cargar predicciones', err);
        this.errorPredicciones.set('No se pudo cargar la sugerencia de compra.');
        this.loadingPredicciones.set(false);
      }
    });
  }

  cambiarHorizonteProyeccion(dias: 7 | 15): void {
    if (this.horizonteProyeccion() !== dias) {
      this.horizonteProyeccion.set(dias);
      this.cargarPredicciones();
    }
  }

  generarFacturaCierre(): void {
    this.procesandoCierre.set(true);
    this.errorInvoice.set('');
    
    this.invoiceService.generarCierre().subscribe({
      next: (res) => {
        this.invoiceData.set(res);
        this.procesandoCierre.set(false);
        this.modalInvoiceVisible.set(true);
      },
      error: (err) => {
        console.error('Error al generar cierre', err);
        this.errorInvoice.set(err?.error?.error ?? 'Error generando la factura de cierre.');
        this.procesandoCierre.set(false);
        // Mostramos el modal de todos modos para que el usuario vea el mensaje de error ("No hay ventas hoy")
        this.modalInvoiceVisible.set(true);
      }
    });
  }

  // CA 33: Exportar Sugerencia de Compra (HU-07 — formato enriquecido para proveedores)
  exportarSugerenciasExcel(): void {
    const arr = this.predicciones();
    if (arr.length === 0) return;

    const hoy = new Date().toISOString().split('T')[0];
    const horizonte = this.horizonteProyeccion();
    // Encabezado con datos de la librería (CA#30 estilo)
    let csv = '"LIBRERÍA LOS ALTARES"\n';
    csv += '"RUC: 1234567890001"\n';
    csv += '"Av. Principal y Secundaria, Sangolquí"\n';
    csv += `"Sugerencia de Compra generada: ${hoy}"\n`;
    csv += '"Motor Analítico — Modelo ARIMA (Autoregresivo de 2 años)"\n\n';
    csv += `ID Producto,Nombre,Cantidad Sugerida (Próx. ${horizonte} días),Margen de Error (%),Acción Sugerida\n`;

    arr.forEach(i => {
      const margen = (i.margen_error * 100).toFixed(0);
      csv += `"${i.id_producto}","${i.nombre}","${i.cantidad_proyectada}","±${margen}%","Solicitar a proveedor"\n`;
    });

    csv += `\n"Total productos en alerta: ${arr.length}"\n`;

    const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' });
    const link = document.createElement('a');
    const url = URL.createObjectURL(blob);
    link.setAttribute('href', url);
    link.setAttribute('download', `sugerencia_compras_${hoy}.csv`);
    link.style.visibility = 'hidden';
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
  }

  cerrarModalInvoice(): void {
    this.modalInvoiceVisible.set(false);
    this.invoiceData.set(null);
    this.errorInvoice.set('');
  }

  imprimirFactura(): void {
    window.print();
  }

  setPeriodo(valor: '7' | '15' | '30' | '365' | '0'): void {
    this.periodoGrafica.set(valor);
    this.cargarGrafica(valor);
  }

  formatCurrency(centavos: number): string {
    return `$${(Math.abs(centavos) / 100).toFixed(2)}`;
  }
}

