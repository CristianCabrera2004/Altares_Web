import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { environment } from '../../../environments/environment';
import { Observable } from 'rxjs';

export interface Configuracion {
  clave: string;
  valor: string;
}

@Injectable({
  providedIn: 'root'
})
export class ConfigService {
  private http = inject(HttpClient);
  private apiUrl = `${environment.apiUrl}/configuracion`;

  getConfiguracion(clave: string): Observable<Configuracion> {
    return this.http.get<Configuracion>(`${this.apiUrl}?clave=${clave}`);
  }

  updateConfiguracion(clave: string, valor: string): Observable<{mensaje: string}> {
    return this.http.put<{mensaje: string}>(this.apiUrl, { clave, valor });
  }
}
