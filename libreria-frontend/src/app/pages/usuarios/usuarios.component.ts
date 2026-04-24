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

interface Usuario {
  id_usuario: number;
  nombre: string;
  email: string;
  rol: string;
  estado: string;
  fecha_creacion: string;
  ultima_sesion?: string;
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
  private readonly api  = `${environment.apiUrl}/usuarios`;

  readonly usuarios     = signal<Usuario[]>([]);
  readonly cargando     = signal(true);
  readonly errorMsg     = signal('');
  readonly successMsg   = signal('');
  readonly mostrarForm  = signal(false);
  readonly guardando    = signal(false);

  // Usuario seleccionado para edición inline de rol
  readonly editandoId   = signal<number | null>(null);
  readonly rolEditado   = signal('');

  // Formulario de nuevo usuario
  readonly form = this.fb.group({
    nombre:   ['', [Validators.required, Validators.minLength(3)]],
    email:    ['', [Validators.required, Validators.email]],
    password: ['', [Validators.required, Validators.minLength(8)]],
    rol:      ['operador_caja', Validators.required]
  });

  ngOnInit(): void {
    this.cargarUsuarios();
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
    if (this.form.invalid || this.guardando()) return;
    this.guardando.set(true);
    this.errorMsg.set('');
    this.successMsg.set('');

    this.http.post<{ mensaje: string; id_usuario: number }>(this.api, this.form.value).subscribe({
      next: (res) => {
        this.successMsg.set(`✓ ${res.mensaje}`);
        this.mostrarForm.set(false);
        this.form.reset({ rol: 'operador_caja' });
        this.guardando.set(false);
        this.cargarUsuarios();
      },
      error: (err) => {
        this.errorMsg.set(err?.error?.error ?? 'Error al crear el usuario.');
        this.guardando.set(false);
      }
    });
  }

  // ─── Edición Inline de Rol ────────────────────────────────────────────────
  iniciarEdicion(u: Usuario): void {
    this.editandoId.set(u.id_usuario);
    this.rolEditado.set(u.rol);
  }
  cancelarEdicion(): void { this.editandoId.set(null); }

  guardarRol(u: Usuario): void {
    const nuevoRol = this.rolEditado();
    if (nuevoRol === u.rol) { this.cancelarEdicion(); return; }
    this.errorMsg.set('');
    this.http.put<{ mensaje: string }>(`${this.api}?id=${u.id_usuario}`, { rol: nuevoRol }).subscribe({
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
    const confirmar = `¿${nuevoEstado === 'inactivo' ? 'Desactivar' : 'Reactivar'} al usuario "${u.nombre}"?`;
    if (!confirm(confirmar)) return;
    this.errorMsg.set('');
    this.http.put<{ mensaje: string }>(`${this.api}?id=${u.id_usuario}`, { estado: nuevoEstado }).subscribe({
      next: (res) => {
        this.successMsg.set(`✓ ${res.mensaje}`);
        this.cargarUsuarios();
      },
      error: (err) => {
        this.errorMsg.set(err?.error?.error ?? 'Error al cambiar el estado.');
      }
    });
  }

  // ─── Helpers ──────────────────────────────────────────────────────────────
  rolLabel(rol: string): string {
    return rol === 'admin_libreria' ? 'Administrador' : 'Operador';
  }
  toggleForm(): void {
    this.mostrarForm.update(v => !v);
    if (!this.mostrarForm()) this.form.reset({ rol: 'operador_caja' });
    this.errorMsg.set('');
    this.successMsg.set('');
  }
}
