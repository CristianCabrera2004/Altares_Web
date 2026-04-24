// src/app/core/services/auth.service.ts
// ─────────────────────────────────────────────────────────────────────────────
// Servicio central de autenticación.
// Gestiona el ciclo de vida del JWT en localStorage y expone métodos para:
//   - Guardar / leer / eliminar el token
//   - Verificar si el token existe y no ha expirado
//   - Extraer el rol y nombre del usuario desde el payload del JWT
//   - Llamar a POST /api/auth/login y POST /api/auth/logout
//
// Usado por: AuthGuard (CA 61) y AuthInterceptor (CA 62).
// ─────────────────────────────────────────────────────────────────────────────
import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Router } from '@angular/router';
import { Observable, tap } from 'rxjs';
import { environment } from '../../../environments/environment';

// Estructura del payload decodificado del JWT emitido por el backend Go
interface JwtPayload {
  id_usuario: number;
  nombre: string;
  email: string;
  rol: string;
  exp: number;   // Unix timestamp de expiración
  iat: number;   // Unix timestamp de emisión
}

// Respuesta del endpoint POST /api/auth/login
export interface LoginResponse {
  token: string;
  rol: string;
  nombre: string;
  id_usuario: number;
  expires_at: string;
}

// Credenciales enviadas al backend
export interface LoginCredentials {
  email: string;
  password: string;
}

@Injectable({ providedIn: 'root' })
export class AuthService {
  private readonly http = inject(HttpClient);
  private readonly router = inject(Router);

  // Clave bajo la que se almacena el JWT en localStorage
  private readonly TOKEN_KEY = 'jwt_token';

  // ─── Gestión del Token ────────────────────────────────────────────────────

  /** Guarda el JWT en localStorage tras un login exitoso. */
  setToken(token: string): void {
    localStorage.setItem(this.TOKEN_KEY, token);
  }

  /** Lee el JWT raw del localStorage. Devuelve null si no existe. */
  getToken(): string | null {
    return localStorage.getItem(this.TOKEN_KEY);
  }

  /** Elimina el JWT del localStorage. */
  removeToken(): void {
    localStorage.removeItem(this.TOKEN_KEY);
  }

  // ─── Validación del Token (CA 61) ─────────────────────────────────────────

  /**
   * Verifica si el usuario tiene un JWT válido y no expirado en el navegador.
   * Utilizado por AuthGuard para proteger rutas (CA 61).
   *
   * Proceso:
   *  1. Obtiene el token del localStorage
   *  2. Decodifica el payload (parte central del JWT en Base64)
   *  3. Compara exp (Unix timestamp) con la hora actual
   */
  isAuthenticated(): boolean {
    const token = this.getToken();
    if (!token) return false;

    try {
      const payload = this.decodePayload(token);
      // exp está en segundos, Date.now() en milisegundos
      return payload.exp > Date.now() / 1000;
    } catch {
      // Token malformado → no autenticado
      return false;
    }
  }

  // ─── Datos del Usuario Autenticado ────────────────────────────────────────

  /** Extrae el rol del usuario desde el payload del JWT. */
  getRol(): string | null {
    return this.getPayload()?.rol ?? null;
  }

  /** Extrae el nombre del usuario desde el payload del JWT. */
  getNombre(): string | null {
    return this.getPayload()?.nombre ?? null;
  }

  /** Extrae el email del usuario desde el payload del JWT. */
  getEmail(): string | null {
    return this.getPayload()?.email ?? null;
  }

  /** Extrae el id_usuario del payload del JWT. */
  getIdUsuario(): number | null {
    return this.getPayload()?.id_usuario ?? null;
  }

  /** Devuelve true si el rol del usuario es 'admin_libreria'. */
  isAdmin(): boolean {
    return this.getRol() === 'admin_libreria';
  }

  // ─── Operaciones de Autenticación ─────────────────────────────────────────

  /**
   * Envía las credenciales al backend Go (POST /api/auth/login).
   * CA 51/52: El backend verifica BCrypt y emite un JWT válido por 8 horas.
   * Guarda el token automáticamente si la respuesta es exitosa.
   */
  login(credentials: LoginCredentials): Observable<LoginResponse> {
    return this.http.post<LoginResponse>(`${environment.apiUrl}/auth/login`, credentials).pipe(
      tap((response) => {
        this.setToken(response.token);
      })
    );
  }

  /**
   * Cierra la sesión del usuario:
   *  1. Llama a POST /api/auth/logout (invalida la sesión en BD)
   *  2. Elimina el JWT del localStorage
   *  3. Redirige al login
   */
  logout(): void {
    // Intentar llamar al endpoint de logout (no crítico si falla)
    const token = this.getToken();
    if (token) {
      this.http.post(`${environment.apiUrl}/auth/logout`, {}).subscribe({
        error: () => {} // Silenciar error si el servidor no está disponible
      });
    }
    this.removeToken();
    this.router.navigate(['/login']);
  }

  // ─── Helpers Privados ─────────────────────────────────────────────────────

  /** Decodifica el payload del JWT (Base64 → objeto). */
  private decodePayload(token: string): JwtPayload {
    const base64Url = token.split('.')[1];
    // Convertir Base64Url a Base64 estándar
    const base64 = base64Url.replace(/-/g, '+').replace(/_/g, '/');
    const jsonPayload = decodeURIComponent(
      atob(base64)
        .split('')
        .map((c) => '%' + ('00' + c.charCodeAt(0).toString(16)).slice(-2))
        .join('')
    );
    return JSON.parse(jsonPayload);
  }

  /** Devuelve el payload completo o null si el token no existe/es inválido. */
  private getPayload(): JwtPayload | null {
    const token = this.getToken();
    if (!token) return null;
    try {
      return this.decodePayload(token);
    } catch {
      return null;
    }
  }
}
