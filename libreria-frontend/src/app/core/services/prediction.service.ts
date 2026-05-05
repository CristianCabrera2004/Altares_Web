import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { environment } from '../../../environments/environment';
import { Observable } from 'rxjs';

export interface Prediccion {
  id_producto: number;
  nombre: string;
  cantidad_proyectada: number;
  margen_error: number;
}

/** HU-03 CA#14 \u2014 Respuesta envuelta del motor anal\u00edtico */
export interface PredictionResponse {
  advertencia?: string;   // Presente cuando el hist\u00f3rico < 14 d\u00edas
  dias_con_datos: number;
  predicciones: Prediccion[];
}

@Injectable({
  providedIn: 'root'
})
export class PredictionService {
  private readonly http = inject(HttpClient);
  private readonly apiUrl = `${environment.apiUrl}/predicciones`;

  getPredicciones(dias: number = 15): Observable<PredictionResponse> {
    return this.http.get<PredictionResponse>(`${this.apiUrl}?dias=${dias}`);
  }
}
