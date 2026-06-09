import { Component, inject, signal, OnInit } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { FormBuilder, Validators, ReactiveFormsModule } from '@angular/forms';
import { environment } from '../../../environments/environment';

interface Tienda {
  id_tienda: number;
  nombre: string;
  direccion: string;
  telefono: string;
  estado: string;
}

@Component({
  selector: 'app-tiendas',
  standalone: true,
  imports: [ReactiveFormsModule],
  templateUrl: './tiendas.html',
  styleUrl: './tiendas.css',
})
export class Tiendas implements OnInit {
  private readonly http = inject(HttpClient);
  private readonly fb   = inject(FormBuilder);
  private readonly api  = `${environment.apiUrl}/tiendas`;

  readonly tiendas      = signal<Tienda[]>([]);
  readonly cargando     = signal(true);
  readonly errorMsg     = signal('');
  readonly successMsg   = signal('');
  readonly mostrarForm  = signal(false);
  readonly guardando    = signal(false);

  // Tienda seleccionada para edición inline
  readonly editandoId       = signal<number | null>(null);
  readonly nombreEditado    = signal('');
  readonly direccionEditada = signal('');
  readonly telefonoEditado  = signal('');
  readonly estadoEditado    = signal('');

  // Formulario de nueva tienda
  readonly form = this.fb.group({
    nombre:    ['', [Validators.required, Validators.minLength(3)]],
    direccion: [''],
    telefono:  ['']
  });

  ngOnInit(): void {
    this.cargarTiendas();
  }

  // ─── Cargar Lista de Tiendas ────────────────────────────────────────────────
  cargarTiendas(): void {
    this.cargando.set(true);
    this.errorMsg.set('');
    this.http.get<Tienda[]>(this.api).subscribe({
      next: (data) => {
        this.tiendas.set(data);
        this.cargando.set(false);
      },
      error: (err) => {
        this.errorMsg.set(err?.error?.error ?? 'Error al cargar las tiendas/librerías.');
        this.cargando.set(false);
      }
    });
  }

  // ─── Crear Tienda ───────────────────────────────────────────────────────────
  crearTienda(): void {
    if (this.form.invalid || this.guardando()) return;
    this.guardando.set(true);
    this.errorMsg.set('');
    this.successMsg.set('');

    this.http.post<{ mensaje: string; id_tienda: number }>(this.api, this.form.value).subscribe({
      next: (res) => {
        this.successMsg.set(`✓ ${res.mensaje}`);
        this.mostrarForm.set(false);
        this.form.reset();
        this.guardando.set(false);
        this.cargarTiendas();
      },
      error: (err) => {
        this.errorMsg.set(err?.error?.error ?? 'Error al crear la tienda.');
        this.guardando.set(false);
      }
    });
  }

  // ─── Edición Inline ─────────────────────────────────────────────────────────
  iniciarEdicion(t: Tienda): void {
    this.editandoId.set(t.id_tienda);
    this.nombreEditado.set(t.nombre);
    this.direccionEditada.set(t.direccion);
    this.telefonoEditado.set(t.telefono);
    this.estadoEditado.set(t.estado);
  }

  cancelarEdicion(): void {
    this.editandoId.set(null);
  }

  guardarEdicion(t: Tienda): void {
    const nombre = this.nombreEditado().trim();
    if (!nombre || nombre.length < 3) {
      this.errorMsg.set('El nombre de la tienda debe tener al menos 3 caracteres.');
      return;
    }

    this.errorMsg.set('');
    this.successMsg.set('');

    const payload = {
      nombre,
      direccion: this.direccionEditada().trim(),
      telefono: this.telefonoEditado().trim(),
      estado: this.estadoEditado()
    };

    this.http.put<{ mensaje: string }>(`${this.api}?id=${t.id_tienda}`, payload).subscribe({
      next: (res) => {
        this.successMsg.set(`✓ ${res.mensaje}`);
        this.cancelarEdicion();
        this.cargarTiendas();
      },
      error: (err) => {
        this.errorMsg.set(err?.error?.error ?? 'Error al actualizar la tienda.');
        this.cancelarEdicion();
      }
    });
  }

  // ─── Activar / Desactivar Tienda ───────────────────────────────────────────
  toggleEstado(t: Tienda): void {
    const nuevoEstado = t.estado === 'activa' ? 'inactiva' : 'activa';
    const confirmar = `¿${nuevoEstado === 'inactiva' ? 'Desactivar' : 'Reactivar'} la tienda "${t.nombre}"?`;
    if (!confirm(confirmar)) return;

    this.errorMsg.set('');
    this.successMsg.set('');

    if (nuevoEstado === 'inactiva') {
      // Usamos el endpoint DELETE para la baja lógica
      this.http.delete<{ mensaje: string }>(`${this.api}?id=${t.id_tienda}`).subscribe({
        next: (res) => {
          this.successMsg.set(`✓ ${res.mensaje}`);
          this.cargarTiendas();
        },
        error: (err) => {
          this.errorMsg.set(err?.error?.error ?? 'Error al desactivar la tienda.');
        }
      });
    } else {
      // Para reactivar usamos PUT
      const payload = {
        nombre: t.nombre,
        direccion: t.direccion,
        telefono: t.telefono,
        estado: 'activa'
      };
      this.http.put<{ mensaje: string }>(`${this.api}?id=${t.id_tienda}`, payload).subscribe({
        next: (res) => {
          this.successMsg.set(`✓ ${res.mensaje}`);
          this.cargarTiendas();
        },
        error: (err) => {
          this.errorMsg.set(err?.error?.error ?? 'Error al reactivar la tienda.');
        }
      });
    }
  }

  // ─── Helpers ────────────────────────────────────────────────────────────────
  toggleForm(): void {
    this.mostrarForm.update(v => !v);
    if (!this.mostrarForm()) this.form.reset();
    this.errorMsg.set('');
    this.successMsg.set('');
  }
}
