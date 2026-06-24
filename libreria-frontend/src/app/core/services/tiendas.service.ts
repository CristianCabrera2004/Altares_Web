import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { environment } from '../../../environments/environment';
import { Observable } from 'rxjs';

export interface Tienda {
  id_tienda: number;
  nombre: string;
  direccion: string;
  telefono: string;
  estado: string;
}

@Injectable({
  providedIn: 'root'
})
export class TiendasService {
  private readonly http = inject(HttpClient);
  private readonly api  = `${environment.apiUrl}/tiendas`;

  getTiendas(): Observable<Tienda[]> {
    return this.http.get<Tienda[]>(this.api);
  }

  crearTienda(tienda: Partial<Tienda>): Observable<{ mensaje: string; id_tienda: number }> {
    return this.http.post<{ mensaje: string; id_tienda: number }>(this.api, tienda);
  }

  actualizarTienda(id: number, tienda: Partial<Tienda>): Observable<{ mensaje: string }> {
    return this.http.put<{ mensaje: string }>(`${this.api}?id=${id}`, tienda);
  }

  desactivarTienda(id: number): Observable<{ mensaje: string }> {
    return this.http.delete<{ mensaje: string }>(`${this.api}?id=${id}`);
  }
}
