import { Routes } from '@angular/router';
import { LoginComponent } from './pages/login/login';
// Nota: Si inventario te da error similar, revisa si su clase se llama Inventario o InventarioComponent
import { Inventario } from './pages/inventario/inventario'; 

export const routes: Routes = [
  // Ruta pública
  { path: 'login', component: LoginComponent },
  
  // Ruta protegida
  { path: 'inventario', component: Inventario },
  
  // Redirecciones
  { path: '', redirectTo: '/login', pathMatch: 'full' },
  { path: '**', redirectTo: '/login' }
];