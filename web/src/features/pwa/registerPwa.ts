import { registerSW } from 'virtual:pwa-register';
import { toaster } from '../../components/ui/toaster';
import { useAppUpdate } from '../../stores/appUpdate';
import { captureInstallPrompt, promptInstall } from './installPrompt';

// Marca (por pestaña) de que ya recargamos una vez por un cambio de service worker. Rompe el
// bucle de recargas: varias pestañas abiertas + un SW nuevo activándose disparan un
// `controllerchange` "externo" en las demás, que sin guard se recargan en cascada una y otra vez
// (síntoma en prod: /login en bucle, canceladas). sessionStorage sobrevive la recarga y se
// limpia al cerrar la pestaña.
const RELOADED_KEY = 'egb:sw-reloaded';

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

  // Recarga controlada por NOSOTROS y a lo sumo UNA vez por pestaña ante un cambio de controlador
  // (guard anti-bucle). Con updateSW(false) el plugin hace skipWaiting SIN recargar; la recarga
  // única la hacemos aquí cuando el SW nuevo toma control. En la primera instalación no hay
  // controllerchange (sin clientsClaim: el SW espera al siguiente navigate), así que no recarga de más.
  if ('serviceWorker' in navigator) {
    navigator.serviceWorker.addEventListener('controllerchange', () => {
      if (sessionStorage.getItem(RELOADED_KEY)) return; // ya recargamos una vez: no entrar en bucle
      sessionStorage.setItem(RELOADED_KEY, '1');
      window.location.reload();
    });
  }

  // registerType:'prompt' → el SW nuevo espera; el operador toca "Actualizar" entre pedidos.
  // updateSW(false): activa el SW nuevo (skipWaiting) SIN recarga automática del plugin; la
  // recarga la dispara el guard de arriba, una sola vez.
  const updateSW = registerSW({
    onNeedRefresh() {
      // Indicador persistente en el pie de sistema (además del toast): si el operador cierra el
      // toast, el aviso de "actualizar" sigue visible hasta que aplique la versión nueva.
      useAppUpdate.getState().markNeedRefresh(() => void updateSW(false));
      toaster.create({
        title: 'Nueva versión disponible',
        type: 'info',
        duration: Number.POSITIVE_INFINITY, // que no se auto-cierre: puede tardar en verlo
        action: { label: 'Actualizar', onClick: () => void updateSW(false) },
      });
    },
  });
}
