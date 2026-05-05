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
  private readonly apiUrl = `${environment.apiUrl}/ventas/factura-cierre`;

  generarCierre(): Observable<InvoiceSummary> {
    // Es un endpoint POST porque representa el acto formal de "Cierre" (podría mutar estado en el futuro)
    // Actualmente nuestro backend lo soporta tanto como GET como POST.
    return this.http.post<InvoiceSummary>(this.apiUrl, {});
  }
}
