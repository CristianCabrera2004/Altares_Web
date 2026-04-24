// src/app/pages/login/login.component.ts
import { Component, inject, signal, OnInit } from '@angular/core';
import { FormBuilder, Validators, ReactiveFormsModule } from '@angular/forms';
import { Router, ActivatedRoute } from '@angular/router';
import { AuthService } from '../../core/services/auth.service';

@Component({
  selector: 'app-login',
  imports: [ReactiveFormsModule],
  templateUrl: './login.component.html',
  styleUrl: './login.component.css'
})
export class LoginComponent implements OnInit {
  private readonly fb = inject(FormBuilder);
  private readonly authService = inject(AuthService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);

  readonly loading = signal(false);
  readonly errorMsg = signal('');
  readonly showPassword = signal(false);
  readonly sessionExpired = signal(false);

  ngOnInit(): void {
    // Mostrar aviso si el interceptor redirigió por token expirado (CA 23)
    this.sessionExpired.set(this.route.snapshot.queryParams['expired'] === '1');
    // Si ya hay sesión activa → ir directo al dashboard
    if (this.authService.isAuthenticated()) {
      this.router.navigate(['/dashboard']);
    }
  }

  readonly form = this.fb.group({
    email: ['', [Validators.required, Validators.email]],
    password: ['', [Validators.required, Validators.minLength(6)]]
  });

  togglePassword(): void {
    this.showPassword.update(v => !v);
  }

  onSubmit(): void {
    if (this.form.invalid || this.loading()) return;
    this.errorMsg.set('');
    this.loading.set(true);

    this.authService.login(this.form.value as { email: string; password: string }).subscribe({
      next: () => {
        this.router.navigate(['/dashboard']);
      },
      error: (err) => {
        const msg = err?.error?.error ?? 'Error al conectar con el servidor.';
        this.errorMsg.set(msg);
        this.loading.set(false);
      }
    });
  }
}
