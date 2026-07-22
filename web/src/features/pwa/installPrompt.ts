// beforeinstallprompt no está en la lib estándar de DOM; solo necesitamos prompt().
interface BeforeInstallPromptEvent extends Event {
  prompt: () => Promise<void>;
}

// El evento solo lo dispara Chrome una vez por carga de página, así que lo capturamos
// aquí para que tanto el toast automático (registerPwa.ts) como el botón manual en
// Configuración > Negocio puedan reusarlo.
let deferred: BeforeInstallPromptEvent | null = null;
const listeners = new Set<(available: boolean) => void>();

export function captureInstallPrompt(e: Event): void {
  deferred = e as BeforeInstallPromptEvent;
  listeners.forEach((l) => l(true));
}

export function onInstallAvailable(listener: (available: boolean) => void): () => void {
  listeners.add(listener);
  if (deferred) listener(true);
  return () => listeners.delete(listener);
}

export async function promptInstall(): Promise<void> {
  if (!deferred) return;
  await deferred.prompt();
  deferred = null;
}

export function isInstalled(): boolean {
  return (
    window.matchMedia('(display-mode: fullscreen)').matches ||
    window.matchMedia('(display-mode: standalone)').matches
  );
}
