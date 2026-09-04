import { Component, ElementRef, EventEmitter, Input, OnChanges, OnDestroy, OnInit, Output, SimpleChanges, ViewChild } from '@angular/core';
import { CommonModule } from '@angular/common';
import { Html5Qrcode } from 'html5-qrcode';

@Component({
  selector: 'app-scanner',
  standalone: true,
  imports: [CommonModule],
  templateUrl: './scanner.component.html',
  styleUrls: ['./scanner.component.css']
})
export class ScannerComponent implements OnInit, OnChanges, OnDestroy {
  @Input() visible = false;
  @Output() close = new EventEmitter<void>();
  @Output() scanSuccess = new EventEmitter<string>();

  private html5QrCode: Html5Qrcode | null = null;
  hasCameras = false;
  cameraError = '';
  cameras: { id: string; label: string }[] = [];
  selectedCameraId = '';

  ngOnInit(): void {
    // Verificar cámaras disponibles
    Html5Qrcode.getCameras()
      .then(devices => {
        if (devices && devices.length) {
          this.hasCameras = true;
          this.cameras = devices;
          // Preferir cámara trasera si existe
          const backCam = devices.find(d => d.label.toLowerCase().includes('back') || d.label.toLowerCase().includes('trasera'));
          this.selectedCameraId = backCam ? backCam.id : devices[0].id;
          
          if (this.visible) {
             this.startScanner();
          }
        } else {
          this.hasCameras = false;
          this.cameraError = 'No se detectaron cámaras en el dispositivo.';
        }
      })
      .catch(err => {
        this.cameraError = 'Error al acceder a la cámara. Asegúrate de dar permisos al navegador.';
        console.error(err);
      });
  }

  ngOnChanges(changes: SimpleChanges): void {
    if (changes['visible']) {
      if (this.visible && this.hasCameras) {
        setTimeout(() => this.startScanner(), 100);
      } else if (!this.visible) {
        this.stopScanner();
      }
    }
  }

  ngOnDestroy(): void {
    this.stopScanner();
  }

  startScanner(): void {
    if (!this.selectedCameraId) return;

    // Esperar a que el DOM renderice el div#reader
    setTimeout(() => {
      const readerEl = document.getElementById('reader');
      if (!readerEl) return;

      // Limpiar instancia previa si existe
      if (this.html5QrCode) {
        try { this.html5QrCode.clear(); } catch(e) {}
        this.html5QrCode = null;
      }

      this.html5QrCode = new Html5Qrcode("reader");

      this.html5QrCode.start(
        this.selectedCameraId,
        {
          fps: 10,
          qrbox: { width: 200, height: 200 },
          aspectRatio: 1.0
        },
        (decodedText) => {
          // Beep simple con Web Audio API (sin necesitar archivo externo)
          try {
            const ctx = new AudioContext();
            const osc = ctx.createOscillator();
            const gain = ctx.createGain();
            osc.connect(gain);
            gain.connect(ctx.destination);
            osc.frequency.value = 1000;
            gain.gain.setValueAtTime(0.3, ctx.currentTime);
            gain.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + 0.1);
            osc.start(ctx.currentTime);
            osc.stop(ctx.currentTime + 0.1);
          } catch (e) {}

          this.stopScanner();
          this.scanSuccess.emit(decodedText);
        },
        (_errorMessage) => {
          // Errores constantes de escaneo (cuando no ve código), se ignoran
        }
      ).catch(err => {
        this.cameraError = 'No se pudo iniciar el escáner. Verifica los permisos de cámara.';
        console.error(err);
      });
    }, 200);
  }

  stopScanner(): void {
    if (this.html5QrCode && this.html5QrCode.isScanning) {
      this.html5QrCode.stop().then(() => {
        this.html5QrCode?.clear();
        this.html5QrCode = null;
      }).catch(err => console.error("Failed to stop scanning.", err));
    }
  }

  onCameraChange(event: Event): void {
    const target = event.target as HTMLSelectElement;
    this.selectedCameraId = target.value;
    
    if (this.html5QrCode && this.html5QrCode.isScanning) {
      this.stopScanner();
      setTimeout(() => this.startScanner(), 500);
    }
  }

  closeModal(): void {
    this.stopScanner();
    this.close.emit();
  }
}
