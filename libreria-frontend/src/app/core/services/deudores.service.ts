// src/app/core/services/deudores.service.ts
// ─────────────────────────────────────────────────────────────────────────────
// Servicio para el módulo de Deudores/Fiados (Anexo 4).
// ─────────────────────────────────────────────────────────────────────────────
import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { environment } from '../../../environments/environment';
import { Observable } from 'rxjs';

export interface Deudor {
  id_deuda: number;
  id_usuario: number;
  nombre_usuario?: string;
  id_tienda: number;
  nombre_deudor: string;
  telefono?: string;
  tipo_deuda: 'dinero' | 'producto';
  monto_deuda: number;
  monto_abonado: number;
  detalle_producto?: string;
  motivo?: string;
  estado: 'pendiente' | 'parcial' | 'pagado';
  fecha_registro: string;
  fecha_pago?: string;
}

export interface Abono {
  id_abono: number;
  monto_abono: number;
  observacion?: string;
  fecha_abono: string;
}

export interface AbonoResponse {
  mensaje: string;
  id_abono: number;
  monto_abonado: number;
  saldo_restante: number;
  estado: string;
}

@Injectable({ providedIn: 'root' })
export class DeudoresService {
  private readonly http = inject(HttpClient);
  private readonly apiUrl = `${environment.apiUrl}/deudores`;

  getAll(estado?: string): Observable<Deudor[]> {
    const params = estado && estado !== 'todos' ? `?estado=${estado}` : '';
    return this.http.get<Deudor[]>(`${this.apiUrl}${params}`);
  }

  crear(deuda: Partial<Deudor>): Observable<{ mensaje: string; deuda: Deudor }> {
    return this.http.post<{ mensaje: string; deuda: Deudor }>(this.apiUrl, deuda);
  }

  actualizar(id: number, deuda: Partial<Deudor>): Observable<{ mensaje: string }> {
    return this.http.put<{ mensaje: string }>(`${this.apiUrl}?id=${id}`, deuda);
  }

  marcarPagado(id: number): Observable<{ mensaje: string }> {
    return this.http.delete<{ mensaje: string }>(`${this.apiUrl}?id=${id}`);
  }

  registrarAbono(abono: { id_deuda: number; monto_abono: number; observacion?: string }): Observable<AbonoResponse> {
    return this.http.post<AbonoResponse>(`${this.apiUrl}/abono`, abono);
  }

  getAbonos(idDeuda: number): Observable<Abono[]> {
    return this.http.get<Abono[]>(`${this.apiUrl}/abonos?id_deuda=${idDeuda}`);
  }
}
