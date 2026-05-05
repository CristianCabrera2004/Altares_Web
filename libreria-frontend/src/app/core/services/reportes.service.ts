import { Injectable, inject } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { environment } from '../../../environments/environment';
import { Observable } from 'rxjs';

export interface ReporteItem {
  fecha_venta: string;
  id_producto: number;
  producto: string;
  categoria: string;
  cantidad: number;
  precio_unitario: number;
  total: number;
}

@Injectable({
  providedIn: 'root'
})
export class ReportesService {
  private readonly http = inject(HttpClient);
  
  getVentas(startDate: string, endDate: string, categoria?: string): Observable<ReporteItem[]> {
    let params = new HttpParams()
      .set('start_date', startDate)
      .set('end_date', endDate);
      
    if (categoria && categoria !== 'Todas') {
      params = params.set('categoria', categoria);
    }
    
    return this.http.get<ReporteItem[]>(`${environment.apiUrl}/reportes/ventas`, { params });
  }
}
