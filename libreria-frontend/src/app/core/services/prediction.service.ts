import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { environment } from '../../../environments/environment';
import { Observable } from 'rxjs';

export interface Prediccion {
  id_producto: number;
  nombre: string;
  id_categoria: number;
  nombre_categoria: string;
  cantidad_proyectada: number;
  margen_error: number;
  stock_actual: number;
  stock_alerta_min: number;
  cantidad_a_comprar: number;
}

/** HU-03 CA#14 — Respuesta envuelta del motor analítico */
export interface PredictionResponse {
  advertencia?: string;   // Presente cuando el histórico < 14 días
  dias_con_datos: number;
  predicciones: Prediccion[];
}

@Injectable({
  providedIn: 'root'
})
export class PredictionService {
  private readonly http = inject(HttpClient);
  private readonly apiUrl = `${environment.apiUrl}/predicciones`;

  getPredicciones(horizonte: string | number = 'mensual'): Observable<PredictionResponse> {
    const param = typeof horizonte === 'number' ? `dias=${horizonte}` : `horizonte=${horizonte}`;
    return this.http.get<PredictionResponse>(`${this.apiUrl}?${param}`);
  }

  getListaCompras(horizonte: string = 'mensual'): Observable<PredictionResponse> {
    return this.http.get<PredictionResponse>(`${this.apiUrl}/lista-compras?horizonte=${horizonte}`);
  }
}
