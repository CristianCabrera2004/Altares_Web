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
  readonly twoFactorRequired = signal(false);
  readonly emailVerificationRequired = signal(false);
  readonly emailHint = signal('');
  readonly reenvioMsg = signal('');

  ngOnInit(): void {
    // Mostrar aviso si el interceptor redirigió por token expirado (CA 23)
    this.sessionExpired.set(this.route.snapshot.queryParams['expired'] === '1');
    // Si ya hay sesión activa → redirigir según rol
    if (this.authService.isAuthenticated()) {
      this.redirectByRole();
    }
  }

  readonly form = this.fb.group({
    email: ['', [Validators.required, Validators.email]],
    password: ['', [Validators.required, Validators.minLength(6)]],
    two_factor_code: ['', []],
    verification_code: ['', []]
  });

  togglePassword(): void {
    this.showPassword.update(v => !v);
  }

  /** Redirige al home correspondiente según el rol del JWT */
  private redirectByRole(): void {
    const destino = this.authService.isAdmin() ? '/usuarios' : '/dashboard';
    this.router.navigate([destino]);
  }

  onSubmit(): void {
    if (this.form.invalid || this.loading()) return;
    this.errorMsg.set('');
    this.reenvioMsg.set('');
    this.loading.set(true);

    const payload = { ...this.form.value } as any;

    // Limpiar campos no necesarios según el flujo actual
    if (!this.twoFactorRequired()) {
      delete payload.two_factor_code;
    }
    if (!this.emailVerificationRequired()) {
      delete payload.verification_code;
    }

    this.authService.login(payload).subscribe({
      next: (res) => {
        if (res.email_verification_required) {
          // Primer login: requiere verificación de email
          this.emailVerificationRequired.set(true);
          this.emailHint.set(res.email_hint || '');
          this.form.get('verification_code')?.setValidators([
            Validators.required,
            Validators.pattern(/^\d{6}$/)
          ]);
          this.form.get('verification_code')?.updateValueAndValidity();
          this.loading.set(false);
        } else if (res.two_factor_required) {
          // 2FA activado
          this.twoFactorRequired.set(true);
          this.form.get('two_factor_code')?.setValidators([
            Validators.required,
            Validators.pattern(/^\d{6}$/)
          ]);
          this.form.get('two_factor_code')?.updateValueAndValidity();
          this.loading.set(false);
        } else {
          this.redirectByRole();
        }
      },
      error: (err) => {
        const msg = err?.error?.error ?? 'Error al conectar con el servidor.';
        this.errorMsg.set(msg);
        this.loading.set(false);
      }
    });
  }

  reenviarCodigo(): void {
    const email = this.form.get('email')?.value;
    if (!email) return;
    this.reenvioMsg.set('');
    this.errorMsg.set('');

    this.authService.reenviarCodigo(email).subscribe({
      next: (res) => {
        this.reenvioMsg.set(res.mensaje || 'Código reenviado exitosamente.');
      },
      error: () => {
        this.errorMsg.set('Error al reenviar el código.');
      }
    });
  }
}
