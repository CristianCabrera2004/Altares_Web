import { Component, inject, signal, OnInit } from '@angular/core';
import { FormBuilder, Validators, ReactiveFormsModule } from '@angular/forms';
import { TiendasService, Tienda } from '../../core/services/tiendas.service';

@Component({
  selector: 'app-tiendas',
  standalone: true,
  imports: [ReactiveFormsModule],
  templateUrl: './tiendas.html',
  styleUrl: './tiendas.css',
})
export class Tiendas implements OnInit {
  private readonly tiendasService = inject(TiendasService);
  private readonly fb   = inject(FormBuilder);

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

  readonly form = this.fb.group({
    nombre:    ['', [Validators.required, Validators.minLength(3)]],
    direccion: [''],
    telefono:  ['']
  });

  // Modal de Confirmación
  readonly confirmModalVisible = signal(false);
  readonly confirmModalMessage = signal('');
  private confirmAction: (() => void) | null = null;

  ngOnInit(): void {
    this.cargarTiendas();
  }

  // ─── Cargar Lista de Tiendas ────────────────────────────────────────────────
  cargarTiendas(): void {
    this.cargando.set(true);
    this.errorMsg.set('');
    this.tiendasService.getTiendas().subscribe({
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

    this.tiendasService.crearTienda(this.form.value as Partial<Tienda>).subscribe({
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

    this.tiendasService.actualizarTienda(t.id_tienda, payload).subscribe({
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
    this.confirmModalMessage.set(`¿${nuevoEstado === 'inactiva' ? 'Desactivar' : 'Reactivar'} la tienda "${t.nombre}"?`);
    
    this.confirmAction = () => {
      this.errorMsg.set('');
      this.successMsg.set('');

      if (nuevoEstado === 'inactiva') {
        this.tiendasService.desactivarTienda(t.id_tienda).subscribe({
          next: (res) => {
            this.successMsg.set(`✓ ${res.mensaje}`);
            this.cargarTiendas();
            this.confirmModalVisible.set(false);
          },
          error: (err) => {
            this.errorMsg.set(err?.error?.error ?? 'Error al desactivar la tienda.');
            this.confirmModalVisible.set(false);
          }
        });
      } else {
        const payload = {
          nombre: t.nombre,
          direccion: t.direccion,
          telefono: t.telefono,
          estado: 'activa'
        };
        this.tiendasService.actualizarTienda(t.id_tienda, payload).subscribe({
          next: (res) => {
            this.successMsg.set(`✓ ${res.mensaje}`);
            this.cargarTiendas();
            this.confirmModalVisible.set(false);
          },
          error: (err) => {
            this.errorMsg.set(err?.error?.error ?? 'Error al reactivar la tienda.');
            this.confirmModalVisible.set(false);
          }
        });
      }
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

  // ─── Helpers ────────────────────────────────────────────────────────────────
  toggleForm(): void {
    this.mostrarForm.update(v => !v);
    if (!this.mostrarForm()) this.form.reset();
    this.errorMsg.set('');
    this.successMsg.set('');
  }
}
