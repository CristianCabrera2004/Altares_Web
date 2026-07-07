import { Component, inject, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { ConfigService } from '../../core/services/config.service';

@Component({
  selector: 'app-ajustes',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './ajustes.component.html',
  styleUrls: ['./ajustes.component.css']
})
export class AjustesComponent implements OnInit {
  private configService = inject(ConfigService);

  ivaGrabado: number | null = null;
  cargando = false;
  guardando = false;
  mensaje = '';
  error = '';

  ngOnInit(): void {
    this.cargarConfiguracion();
  }

  cargarConfiguracion(): void {
    this.cargando = true;
    this.error = '';
    this.configService.getConfiguracion('tasa_iva_grabado').subscribe({
      next: (config) => {
        this.ivaGrabado = parseInt(config.valor, 10);
        this.cargando = false;
      },
      error: (err) => {
        console.error('Error cargando config:', err);
        this.error = 'No se pudo cargar la configuración del sistema.';
        this.cargando = false;
      }
    });
  }

  guardarIva(): void {
    if (this.ivaGrabado === null || this.ivaGrabado < 0 || this.ivaGrabado > 100) {
      this.error = 'Por favor ingrese un valor válido entre 0 y 100.';
      return;
    }

    this.guardando = true;
    this.error = '';
    this.mensaje = '';

    this.configService.updateConfiguracion('tasa_iva_grabado', this.ivaGrabado.toString()).subscribe({
      next: (res) => {
        this.mensaje = res.mensaje;
        this.guardando = false;
        setTimeout(() => this.mensaje = '', 3000);
      },
      error: (err) => {
        console.error('Error guardando config:', err);
        this.error = 'No se pudo guardar la configuración.';
        this.guardando = false;
      }
    });
  }
}
