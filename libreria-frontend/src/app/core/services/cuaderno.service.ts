import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { environment } from '../../../environments/environment';
import { Observable } from 'rxjs';

export interface ProductoCatalogo {
  id_producto:      number;
  nombre:           string;
  id_categoria:     number;
  nombre_categoria: string;
  tasa_iva:         number;
  stock_actual:     number;
  stock_alerta_min: number;
  precio_venta:     number;
  estado:           string;
}

export interface RespuestaCuaderno {
  mensaje:        string;
  id_venta:       number;
  total:          number;
  items_cargados: number;
}

@Injectable({
  providedIn: 'root'
})
export class CuadernoService {
  private readonly http = inject(HttpClient);
  private readonly apiUrl = environment.apiUrl;

  getProductosActivos(): Observable<ProductoCatalogo[]> {
    return this.http.get<ProductoCatalogo[]>(`${this.apiUrl}/productos?estado=activo`);
  }

  buscarProductoPorCodigo(codigo: string): Observable<ProductoCatalogo> {
    return this.http.get<ProductoCatalogo>(`${this.apiUrl}/productos/buscar?codigo=${encodeURIComponent(codigo)}`);
  }

  buscarCliente(cedulaRuc: string): Observable<any[]> {
    return this.http.get<any[]>(`${this.apiUrl}/clientes/buscar?q=${encodeURIComponent(cedulaRuc)}`);
  }

  crearCliente(cliente: any): Observable<any> {
    return this.http.post<any>(`${this.apiUrl}/clientes`, cliente);
  }

  actualizarCliente(idCliente: number, cliente: any): Observable<any> {
    return this.http.put<any>(`${this.apiUrl}/clientes?id=${idCliente}`, cliente);
  }

  guardarCuaderno(payload: any): Observable<RespuestaCuaderno> {
    return this.http.post<RespuestaCuaderno>(`${this.apiUrl}/ventas/cuaderno`, payload);
  }

  crearFactura(payload: any): Observable<any> {
    return this.http.post<any>(`${this.apiUrl}/facturas`, payload);
  }
}
