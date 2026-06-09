import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { environment } from '../../../environments/environment';
import { Observable } from 'rxjs';

export interface InvoiceDetail {
  producto: string;
  cantidad: number;
  precio_unitario: number;
  iva_aplicado: number;
  subtotal: number;
}

export interface InvoiceSummary {
  fecha_emision: string;
  ruc_cliente: string;
  nombre_cliente: string;
  subtotal_base: number;
  total_iva_15: number;
  total_iva_0: number;
  total_global: number;
  detalles: InvoiceDetail[];
  xml_sri_mock: string;
}

@Injectable({
  providedIn: 'root'
})
export class InvoiceService {
  private readonly http = inject(HttpClient);
  private readonly apiUrlCierre = `${environment.apiUrl}/ventas/factura-cierre`;
  private readonly apiUrlFactura = `${environment.apiUrl}/facturas`;

  generarCierre(): Observable<InvoiceSummary> {
    return this.http.post<InvoiceSummary>(this.apiUrlCierre, {});
  }

  crearFactura(payload: { id_venta: number; id_tipo_factura: number; id_cliente?: number | null; pdf_base64?: string }): Observable<any> {
    return this.http.post<any>(this.apiUrlFactura, payload);
  }

  getFacturaByVenta(idVenta: number): Observable<any> {
    return this.http.get<any>(`${this.apiUrlFactura}?venta=${idVenta}`);
  }

  getFacturaById(idFactura: number): Observable<any> {
    return this.http.get<any>(`${this.apiUrlFactura}?id=${idFactura}`);
  }

  listarFacturas(): Observable<any[]> {
    return this.http.get<any[]>(this.apiUrlFactura);
  }
}
