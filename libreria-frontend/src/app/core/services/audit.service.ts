import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { environment } from '../../../environments/environment';
import { Observable } from 'rxjs';

export interface AuditLog {
  id_log: number;
  nombre_usuario: string;
  accion: string;
  tabla_afectada: string;
  id_registro_afectado: number | null;
  valor_anterior: string;
  valor_nuevo: string;
  ip_origen: string;
  fecha: string;
}

@Injectable({
  providedIn: 'root'
})
export class AuditService {
  private readonly http = inject(HttpClient);
  
  getLogs(): Observable<AuditLog[]> {
    return this.http.get<AuditLog[]>(`${environment.apiUrl}/auditoria`);
  }
}
