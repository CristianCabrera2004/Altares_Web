import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { environment } from '../../../environments/environment';
import { Observable } from 'rxjs';

export interface CajaResponse {
  saldo_caja: number;
}

export interface CajaUpdateResponse {
  mensaje: string;
}

@Injectable({
  providedIn: 'root'
})
export class CajaService {
  private readonly http = inject(HttpClient);
  private apiUrl = `${environment.apiUrl}/caja`;

  getSaldoCaja(): Observable<CajaResponse> {
    return this.http.get<CajaResponse>(this.apiUrl);
  }

  updateSaldoCaja(saldo: number): Observable<CajaUpdateResponse> {
    return this.http.put<CajaUpdateResponse>(this.apiUrl, { saldo_caja: saldo });
  }
}
