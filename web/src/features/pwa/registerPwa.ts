import { registerSW } from 'virtual:pwa-register';
import { toaster } from '../../components/ui/toaster';
import { captureInstallPrompt, promptInstall } from './installPrompt';

// initPwa cablea las dos únicas piezas de la PWA con UI: ofrecer instalar y avisar
// de una versión nueva. Sin runtime caching de /api: el servidor sigue siendo la
// única fuente de verdad (los precios se recalculan en el backend).
export function initPwa(): void {
  // "Instalar app" propio: Chrome dispara beforeinstallprompt solo si es instalable
  // y no está ya instalada, así que este toast no aparece una vez instalada. El evento
  // también queda disponible en installPrompt.ts para el botón manual en Configuración.
  window.addEventListener('beforeinstallprompt', (e) => {
    e.preventDefault();
    captureInstallPrompt(e);
    toaster.create({
      title: 'Instalar El Gato Bobah',
      description: 'Agrégala a la tablet para abrirla como app.',
      type: 'info',
      closable: true,
      action: { label: 'Instalar', onClick: () => void promptInstall() },
    });
  });

  // registerType:'prompt' → el SW nuevo espera; updateSW(true) hace skipWaiting + reload.
  // No recargamos solos: el operador toca "Actualizar" entre pedidos.
  const updateSW = registerSW({
    onNeedRefresh() {
      toaster.create({
        title: 'Nueva versión disponible',
        type: 'info',
        duration: Number.POSITIVE_INFINITY, // que no se auto-cierre: puede tardar en verlo
        action: { label: 'Actualizar', onClick: () => void updateSW(true) },
      });
    },
  });
}
