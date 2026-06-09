// src/app/core/services/clientes.service.ts
// ─────────────────────────────────────────────────────────────────────────────
// Servicio para el catálogo de clientes (Anexo 3).
// Comunica con /api/clientes y /api/clientes/buscar.
// ─────────────────────────────────────────────────────────────────────────────
import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { environment } from '../../../environments/environment';
import { Observable } from 'rxjs';

export interface Cliente {
  id_cliente: number;
  cedula_ruc: string;
  nombre: string;
  direccion?: string;
  telefono?: string;
  email?: string;
}

@Injectable({ providedIn: 'root' })
export class ClientesService {
  private readonly http = inject(HttpClient);
  private readonly apiUrl = `${environment.apiUrl}/clientes`;

  getAll(): Observable<Cliente[]> {
    return this.http.get<Cliente[]>(this.apiUrl);
  }

  getById(id: number): Observable<Cliente> {
    return this.http.get<Cliente>(`${this.apiUrl}?id=${id}`);
  }

  buscar(query: string): Observable<Cliente[]> {
    return this.http.get<Cliente[]>(`${this.apiUrl}/buscar?q=${encodeURIComponent(query)}`);
  }

  crear(cliente: Partial<Cliente>): Observable<{ mensaje: string; cliente: Cliente }> {
    return this.http.post<{ mensaje: string; cliente: Cliente }>(this.apiUrl, cliente);
  }

  actualizar(id: number, cliente: Partial<Cliente>): Observable<{ mensaje: string; cliente: Cliente }> {
    return this.http.put<{ mensaje: string; cliente: Cliente }>(`${this.apiUrl}?id=${id}`, cliente);
  }
}
