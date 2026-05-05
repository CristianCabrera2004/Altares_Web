import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { environment } from '../../../environments/environment';
import { Observable, map } from 'rxjs';

export interface GraficaData {
  fecha: string;
  total: number;
}

interface ProductoStock {
  stock_actual: number;
  stock_alerta_min: number;
}

@Injectable({
  providedIn: 'root'
})
export class DashboardService {
  private readonly http = inject(HttpClient);

  getGraficaVentas(periodo: '7' | '15' | '30' | '365' | '0' = '15'): Observable<GraficaData[]> {
    return this.http.get<GraficaData[]>(`${environment.apiUrl}/dashboard/grafica?periodo=${periodo}`);
  }

  /** HU-06 CA#26 — Cuenta productos con stock <= stock_alerta_min */
  getStockBajoCount(): Observable<number> {
    return this.http.get<ProductoStock[]>(`${environment.apiUrl}/productos`).pipe(
      map(productos => productos.filter(p => p.stock_actual <= p.stock_alerta_min).length)
    );
  }
}
