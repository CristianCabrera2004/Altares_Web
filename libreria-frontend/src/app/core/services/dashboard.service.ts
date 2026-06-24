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

  getGraficaVentas(periodo: '7' | '15' | '30' | '365' | '0' = '15', tiendaId?: number): Observable<GraficaData[]> {
    let url = `${environment.apiUrl}/dashboard/grafica?periodo=${periodo}`;
    if (tiendaId) {
      url += `&tienda=${tiendaId}`;
    }
    return this.http.get<GraficaData[]>(url);
  }

  /** HU-06 CA#26 — Cuenta productos con stock <= stock_alerta_min.
   *  Se filtra directamente en backend para evitar descargar el catálogo completo.
   *  El endpoint devuelve sólo los productos con stock bajo de la tienda activa.
   */
  getStockBajoCount(): Observable<number> {
    return this.http.get<ProductoStock[]>(`${environment.apiUrl}/productos?stock_bajo=true`).pipe(
      map(productos => productos.length)
    );
  }
}
