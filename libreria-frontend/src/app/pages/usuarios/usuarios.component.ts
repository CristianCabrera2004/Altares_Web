// src/app/pages/usuarios/usuarios.component.ts
// ─────────────────────────────────────────────────────────────────────────────
// HU-05 CA 21: Módulo de gestión de usuarios — exclusivo para Administrador.
//
// Funcionalidades:
//   - Listar todos los usuarios (GET /api/usuarios)
//   - Crear nuevo usuario con rol y contraseña (POST /api/usuarios)
//   - Cambiar rol de un usuario (PUT /api/usuarios?id=X)
//   - Activar/Desactivar usuarios - baja lógica (PUT /api/usuarios?id=X)
//
// El interceptor authInterceptor añade automáticamente el JWT Bearer
// a todas estas peticiones (CA 62). Si el token expira, redirige a /login (CA 23).
// ─────────────────────────────────────────────────────────────────────────────
import { Component, inject, signal, OnInit } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { FormBuilder, Validators, ReactiveFormsModule } from '@angular/forms';
import { environment } from '../../../environments/environment';
import { AuthService } from '../../core/services/auth.service';
import { debounceTime, distinctUntilChanged, switchMap, catchError, filter } from 'rxjs/operators';
import { of } from 'rxjs';

interface Usuario {
  id_usuario: number;
  nombre: string;
  email: string;
  rol: string;
  estado: string;
  id_tienda?: number;
  fecha_creacion: string;
  ultima_sesion?: string;
}

interface Tienda {
  id_tienda: number;
  nombre: string;
  direccion?: string;
  telefono?: string;
  estado?: string;
}

@Component({
  selector: 'app-usuarios',
  imports: [ReactiveFormsModule],
  templateUrl: './usuarios.component.html',
  styleUrl: './usuarios.component.css'
})
export class UsuariosComponent implements OnInit {
  private readonly http = inject(HttpClient);
  private readonly fb   = inject(FormBuilder);
  private readonly authService = inject(AuthService);
  private readonly api  = `${environment.apiUrl}/usuarios`;

  readonly usuarios     = signal<Usuario[]>([]);
  readonly cargando     = signal(true);
  readonly errorMsg     = signal('');
  readonly successMsg   = signal('');
  readonly mostrarForm  = signal(false);
  readonly guardando    = signal(false);

  // Modal de Confirmación
  readonly confirmModalVisible = signal(false);
  readonly confirmModalMessage = signal('');
  private confirmAction: (() => void) | null = null;
  readonly emailExiste  = signal(false); // Estado para validar email duplicado

  // Señales de 2FA para el usuario activo
  readonly twoFactorEnabled = signal(false);
  readonly show2faSetup     = signal(false);
  readonly totpSecret       = signal('');
  readonly totpQrUri        = signal('');
  readonly totpCode         = signal('');
  readonly loading2fa       = signal(false);

  // Señales para desactivación de 2FA (formulario inline con código + contraseña)
  readonly showDisable2faForm = signal(false);
  readonly disable2faCode     = signal('');
  readonly disable2faPassword = signal('');

  // Tiendas
  readonly tiendas      = signal<Tienda[]>([]);
  readonly apiTiendas   = `${environment.apiUrl}/tiendas`;

  // Usuario seleccionado para edición inline de rol/tienda
  readonly editandoId   = signal<number | null>(null);
  readonly rolEditado   = signal('');
  readonly tiendaEditada = signal<number | null>(null);

  // Formulario de nuevo usuario
  readonly form = this.fb.group({
    nombre:   ['', [Validators.required, Validators.minLength(3)]],
    email:    ['', [Validators.required, Validators.email]],
    password: ['', [Validators.required, Validators.minLength(8)]],
    rol:      ['operador_caja', Validators.required],
    id_tienda: [0]
  });

  ngOnInit(): void {
    this.cargarUsuarios();
    this.cargarTiendas();
    this.cargarPerfil();
    this.setupEmailValidation();
  }

  setupEmailValidation(): void {
    this.form.get('email')?.valueChanges.pipe(
      debounceTime(500),
      distinctUntilChanged(),
      filter(val => !!val && this.form.get('email')!.valid),
      switchMap(email => 
        this.http.get<{existe: boolean}>(`${this.api}/verificar-email?email=${encodeURIComponent(email!)}`).pipe(
          catchError(() => of({ existe: false }))
        )
      )
    ).subscribe(res => {
      this.emailExiste.set(res.existe);
    });
  }

  cargarPerfil(): void {
    this.http.get<{ two_factor_enabled: boolean }>(`${environment.apiUrl}/auth/perfil`).subscribe({
      next: (perfil) => {
        this.twoFactorEnabled.set(perfil.two_factor_enabled);
      },
      error: () => console.error("Error al cargar perfil de usuario")
    });
  }

  // ─── Lógica de 2FA ────────────────────────────────────────────────────────
  iniciar2faSetup(): void {
    this.loading2fa.set(true);
    this.errorMsg.set('');
    this.successMsg.set('');

    this.authService.get2faSetup().subscribe({
      next: (data) => {
        this.totpSecret.set(data.secret);
        this.totpQrUri.set(data.qr_uri);
        this.show2faSetup.set(true);
        this.loading2fa.set(false);
      },
      error: (err) => {
        this.errorMsg.set(err?.error?.error ?? 'Error al iniciar configuración 2FA.');
        this.loading2fa.set(false);
      }
    });
  }

  confirmar2fa(): void {
    const code = this.totpCode().trim();
    if (code.length !== 6 || this.loading2fa()) return;

    this.loading2fa.set(true);
    this.errorMsg.set('');

    this.authService.enable2fa(code).subscribe({
      next: (res) => {
        this.successMsg.set(`✓ ${res.mensaje}`);
        this.twoFactorEnabled.set(true);
        this.show2faSetup.set(false);
        this.totpCode.set('');
        this.loading2fa.set(false);
      },
      error: (err) => {
        this.errorMsg.set(err?.error?.error ?? 'Código incorrecto. Vuelva a intentar.');
        this.loading2fa.set(false);
      }
    });
  }

  // ─── Desactivación de 2FA (requiere código TOTP + contraseña) ─────────────
  mostrarFormDesactivar(): void {
    this.showDisable2faForm.update(v => !v);
    this.disable2faCode.set('');
    this.disable2faPassword.set('');
    this.errorMsg.set('');
  }

  desactivar2fa(): void {
    const code = this.disable2faCode().trim();
    const password = this.disable2faPassword();
    if (!code || code.length !== 6 || !password) return;

    this.loading2fa.set(true);
    this.errorMsg.set('');
    this.successMsg.set('');

    this.authService.disable2fa(code, password).subscribe({
      next: (res) => {
        this.successMsg.set(`✓ ${res.mensaje}`);
        this.twoFactorEnabled.set(false);
        this.showDisable2faForm.set(false);
        this.disable2faCode.set('');
        this.disable2faPassword.set('');
        this.loading2fa.set(false);
      },
      error: (err) => {
        this.errorMsg.set(err?.error?.error ?? 'Error al desactivar 2FA. Verifique el código y la contraseña.');
        this.loading2fa.set(false);
      }
    });
  }

  cargarTiendas(): void {
    this.http.get<Tienda[]>(this.apiTiendas).subscribe({
      next: (data) => this.tiendas.set(data),
      error: () => console.error("Error al cargar tiendas")
    });
  }

  // ─── Cargar Lista ─────────────────────────────────────────────────────────
  cargarUsuarios(): void {
    this.cargando.set(true);
    this.errorMsg.set('');
    this.http.get<Usuario[]>(this.api).subscribe({
      next: (data) => {
        this.usuarios.set(data);
        this.cargando.set(false);
      },
      error: (err) => {
        this.errorMsg.set(err?.error?.error ?? 'Error al cargar los usuarios.');
        this.cargando.set(false);
      }
    });
  }

  // ─── Crear Usuario ────────────────────────────────────────────────────────
  crearUsuario(): void {
    if (this.form.invalid || this.guardando() || this.emailExiste()) return;
    this.guardando.set(true);
    this.errorMsg.set('');
    this.successMsg.set('');

    const payload = {
      ...this.form.value,
      id_tienda: Number(this.form.value.id_tienda ?? 0)
    };

    this.http.post<{ mensaje: string; id_usuario: number }>(this.api, payload).subscribe({
      next: (res) => {
        this.successMsg.set(`✓ ${res.mensaje}`);
        this.mostrarForm.set(false);
        this.form.reset({ rol: 'operador_caja', id_tienda: 0 });
        this.guardando.set(false);
        this.cargarUsuarios();
      },
      error: (err) => {
        this.errorMsg.set(err?.error?.error ?? 'Error al crear el usuario.');
        this.guardando.set(false);
      }
    });
  }

  // ─── Edición Inline de Rol/Tienda ─────────────────────────────────────────
  iniciarEdicion(u: Usuario): void {
    this.editandoId.set(u.id_usuario);
    this.rolEditado.set(u.rol);
    this.tiendaEditada.set(u.id_tienda ?? 0);
  }
  cancelarEdicion(): void { this.editandoId.set(null); }

  guardarEdicion(u: Usuario): void {
    const nuevoRol = this.rolEditado();
    const nuevaTienda = Number(this.tiendaEditada() ?? 0);
    
    if (nuevoRol === u.rol && (nuevaTienda === u.id_tienda || (nuevaTienda === 0 && u.id_tienda === undefined))) { 
      this.cancelarEdicion(); 
      return; 
    }
    
    this.errorMsg.set('');
    const payload = { rol: nuevoRol, id_tienda: nuevaTienda };
    this.http.put<{ mensaje: string }>(`${this.api}?id=${u.id_usuario}`, payload).subscribe({
      next: (res) => {
        this.successMsg.set(`✓ ${res.mensaje}`);
        this.cancelarEdicion();
        this.cargarUsuarios();
      },
      error: (err) => {
        this.errorMsg.set(err?.error?.error ?? 'Error al actualizar el rol.');
        this.cancelarEdicion();
      }
    });
  }

  // ─── Toggle Estado (Activar/Desactivar) ──────────────────────────────────
  toggleEstado(u: Usuario): void {
    const nuevoEstado = u.estado === 'activo' ? 'inactivo' : 'activo';
    this.confirmModalMessage.set(`¿${nuevoEstado === 'inactivo' ? 'Desactivar' : 'Reactivar'} al usuario "${u.nombre}"?`);
    this.confirmAction = () => {
      this.errorMsg.set('');
      this.http.put<{ mensaje: string }>(`${this.api}?id=${u.id_usuario}`, { estado: nuevoEstado }).subscribe({
        next: (res) => {
          this.successMsg.set(`✓ ${res.mensaje}`);
          this.cargarUsuarios();
          this.confirmModalVisible.set(false);
        },
        error: (err) => {
          this.errorMsg.set(err?.error?.error ?? 'Error al cambiar el estado.');
          this.confirmModalVisible.set(false);
        }
      });
    };
    this.confirmModalVisible.set(true);
  }

  // ─── Acciones del Modal de Confirmación ─────────────────────────────────────
  confirmarAccion(): void {
    if (this.confirmAction) {
      this.confirmAction();
    }
  }

  cancelarConfirmacion(): void {
    this.confirmModalVisible.set(false);
    this.confirmAction = null;
  }
  // ─── Helpers ──────────────────────────────────────────────────────────────
  rolLabel(rol: string): string {
    return rol === 'admin_libreria' ? 'Administrador' : 'Operador';
  }
  tiendaLabel(idTienda: number | undefined): string {
    if (!idTienda) return 'Global';
    const t = this.tiendas().find(x => x.id_tienda === idTienda);
    return t ? t.nombre : `Tienda ${idTienda}`;
  }
  toggleForm(): void {
    this.mostrarForm.update(v => !v);
    if (!this.mostrarForm()) this.form.reset({ rol: 'operador_caja', id_tienda: 0 });
    this.errorMsg.set('');
    this.successMsg.set('');
  }
}
