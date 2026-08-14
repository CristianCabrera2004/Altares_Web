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
  metodo_pago: string;
  items?: any[];
}

export interface CompraDetail {
  producto: string;
  cantidad: number;
  precio_unitario: number;
  subtotal: number;
}

export interface FacturaCompra {
  id_factura: number;
  numero_factura: string;
  fecha_compra: string;
  id_proveedor?: number;
  proveedor: string;
  id_usuario: number;
  total: number;
  fecha_registro: string;
  items?: CompraDetail[];
}

@Injectable({
  providedIn: 'root'
})
export class ReportesService {
  private readonly http = inject(HttpClient);
  
  getVentas(startDate: string, endDate: string, categoria?: string, metodoPago?: string): Observable<ReporteItem[]> {
    let params = new HttpParams()
      .set('start_date', startDate)
      .set('end_date', endDate);
      
    if (categoria && categoria !== 'Todas') {
      params = params.set('categoria', categoria);
    }
    
    if (metodoPago && metodoPago !== 'Todos') {
      params = params.set('metodo_pago', metodoPago.toLowerCase());
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

  reenviarRecibo(idFactura: number, pdfBase64: string): Observable<any> {
    return this.http.post<any>(`${environment.apiUrl}/facturas/reenviar`, {
      id_factura: idFactura,
      pdf_base64: pdfBase64
    });
  }

  getFacturaDiaria(fecha?: string): Observable<FacturaResponse> {
    let url = `${environment.apiUrl}/reportes/factura-diaria`;
    if (fecha) {
      url += `?fecha=${fecha}`;
    }
    return this.http.get<FacturaResponse>(url);
  }

  getCompras(fecha?: string): Observable<FacturaCompra[]> {
    let url = `${environment.apiUrl}/reportes/compras`;
    if (fecha) {
      url += `?fecha=${fecha}`;
    }
    return this.http.get<FacturaCompra[]>(url);
  }

  getCompraById(idFactura: number): Observable<FacturaCompra> {
    return this.http.get<FacturaCompra>(`${environment.apiUrl}/reportes/compras/${idFactura}`);
  }
}
