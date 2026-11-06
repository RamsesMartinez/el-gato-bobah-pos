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
// de una versión nueva (el aviso lo pinta el riel, ver UpdateRailButton). Sin runtime
// caching de /api: el servidor sigue siendo la única fuente de verdad (los precios se
// recalculan en el backend).
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
  // updateSW(false) manda skipWaiting al SW que espera; al tomar control, recargan el plugin (en
  // esta pestaña) y el guard de arriba (una sola vez por pestaña).
  //
  // Si no hay ninguno esperando es que otra pestaña ya lo aplicó y aquí el guard pudo no recargar
  // (ya había recargado una vez en esta sesión): mandar skipWaiting no haría nada y el botón se
  // quedaría puesto. Recargar ya carga la versión nueva, que es la que controla.
  //
  // El aviso es SOLO el botón del riel (spec 064), no un toast: flotando arriba al centro tapaba
  // botones de la pantalla —«Agregar lugar» en Negocio a 1024×600— y no se cerraba solo.
  const updateSW = registerSW({
    onNeedRefresh() {
      useAppUpdate.getState().markNeedRefresh(() => void (async () => {
        const registration = await navigator.serviceWorker.getRegistration();
        if (registration?.waiting) await updateSW(false);
        else window.location.reload();
      })());
    },
  });

  // En desarrollo no hay service worker y una versión nueva de verdad exige dos builds servidos.
  // La e2e del aviso (spec 064) lo prende con esto; Vite lo quita del build de producción.
  if (import.meta.env.DEV) {
    window.__egbSimularVersionNueva = () => useAppUpdate.getState().markNeedRefresh(() => window.location.reload());
  }
}

declare global {
  interface Window { __egbSimularVersionNueva?: () => void }
}
