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

export interface FacturaResponse {
  id_factura: number;
  id_venta: number;
  id_tipo_factura: number;
  nombre_tipo_factura: string;
  id_cliente?: number;
  cliente_identificacion: string;
  cliente_nombre: string;
  cliente_direccion?: string;
  cliente_telefono?: string;
  cliente_email?: string;
  archivo_pdf?: string;
  fecha_emision: string;
  subtotal: number;
  total_iva: number;
  total: number;
  items?: any[];
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

  getFacturas(fecha?: string): Observable<FacturaResponse[]> {
    let url = `${environment.apiUrl}/facturas`;
    if (fecha) {
      url += `?fecha=${fecha}`;
    }
    return this.http.get<FacturaResponse[]>(url);
  }

  getFacturaById(idFactura: number): Observable<FacturaResponse> {
    return this.http.get<FacturaResponse>(`${environment.apiUrl}/facturas?id=${idFactura}`);
  }

  getFacturaDiaria(fecha?: string): Observable<FacturaResponse> {
    let url = `${environment.apiUrl}/reportes/factura-diaria`;
    if (fecha) {
      url += `?fecha=${fecha}`;
    }
    return this.http.get<FacturaResponse>(url);
  }
}
