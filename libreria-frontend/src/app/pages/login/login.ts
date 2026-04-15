import { Component, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { FormsModule } from '@angular/forms';
import { CommonModule } from '@angular/common';

@Component({
  selector: 'app-login',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './login.html',    // <-- Quita el ".component" de aquí
  styleUrl: './login.scss',       // <-- Y de aquí si es necesario
})
export class LoginComponent {
  http = inject(HttpClient);
  isLoginMode = true;
  email = ''; password = ''; nombre = ''; mensaje = '';

  toggleMode() { this.isLoginMode = !this.isLoginMode; this.mensaje = ''; }

  onSubmit() {
    const url = this.isLoginMode ? 'login' : 'register';
    const body = this.isLoginMode ? { email: this.email, password: this.password } 
                                  : { nombre: this.nombre, email: this.email, password: this.password };

    this.http.post(`http://127.0.0.1:8080/api/auth/${url}`, body).subscribe({
      next: (res: any) => {
        if (this.isLoginMode) {
          localStorage.setItem('token', res.token);
          this.mensaje = `Bienvenido ${res.usuario.nombre} (${res.usuario.rol})`;
        } else {
          alert(res.mensaje);
          this.toggleMode();
        }
      },
      error: (err) => this.mensaje = err.error?.error || 'Error de conexión'
    });
  }
}