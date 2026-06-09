// src/app/pages/clientes/clientes.component.ts
import { Component, inject, signal, computed, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { ClientesService, Cliente } from '../../core/services/clientes.service';

@Component({
  selector: 'app-clientes',
  imports: [CommonModule, FormsModule],
  templateUrl: './clientes.component.html',
  styleUrl: './clientes.component.css'
})
export class ClientesComponent implements OnInit {
  private readonly svc = inject(ClientesService);

  clientes = signal<Cliente[]>([]);
  searchQuery = signal('');
  loading = signal(false);
  error = signal('');
  success = signal('');

  // Modal
  showModal = signal(false);
  editMode = signal(false);
  form = signal<Partial<Cliente>>({ cedula_ruc: '', nombre: '', direccion: '', telefono: '', email: '' });

  filtered = computed(() => {
    const q = this.searchQuery().toLowerCase();
    if (!q) return this.clientes();
    return this.clientes().filter(c =>
      c.nombre.toLowerCase().includes(q) ||
      c.cedula_ruc.includes(q) ||
      (c.email && c.email.toLowerCase().includes(q))
    );
  });

  ngOnInit(): void {
    this.loadClientes();
  }

  loadClientes(): void {
    this.loading.set(true);
    this.svc.getAll().subscribe({
      next: (data) => { this.clientes.set(data); this.loading.set(false); },
      error: () => { this.error.set('Error al cargar clientes.'); this.loading.set(false); }
    });
  }

  openCreate(): void {
    this.editMode.set(false);
    this.form.set({ cedula_ruc: '', nombre: '', direccion: '', telefono: '', email: '' });
    this.showModal.set(true);
    this.error.set('');
  }

  openEdit(c: Cliente): void {
    this.editMode.set(true);
    this.form.set({ ...c });
    this.showModal.set(true);
    this.error.set('');
  }

  closeModal(): void {
    this.showModal.set(false);
  }

  save(): void {
    const f = this.form();
    if (!f.cedula_ruc || !f.nombre) {
      this.error.set('Cédula/RUC y nombre son obligatorios.');
      return;
    }

    this.loading.set(true);
    if (this.editMode() && f.id_cliente) {
      this.svc.actualizar(f.id_cliente, f).subscribe({
        next: () => {
          this.success.set('Cliente actualizado.');
          this.closeModal();
          this.loadClientes();
        },
        error: (e) => {
          this.error.set(e.error?.error || 'Error al actualizar.');
          this.loading.set(false);
        }
      });
    } else {
      this.svc.crear(f).subscribe({
        next: () => {
          this.success.set('Cliente creado exitosamente.');
          this.closeModal();
          this.loadClientes();
        },
        error: (e) => {
          this.error.set(e.error?.error || 'Error al crear cliente.');
          this.loading.set(false);
        }
      });
    }

    setTimeout(() => this.success.set(''), 4000);
  }

  formatCedula(cedula: string): string {
    if (cedula === '9999999999999') return 'Consumidor Final';
    return cedula;
  }
}
