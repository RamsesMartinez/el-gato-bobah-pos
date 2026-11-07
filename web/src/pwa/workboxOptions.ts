// Opciones de workbox del service worker del POS (las usa vite.config.ts).
//
// EL SHELL SE LLAMA «/», NO «index.html». Las navegaciones (entrar directo a /pedidos, recargar) las
// sirve el service worker con el shell de su caché; si esa entrada falta —el cambio de versión entre
// el SW viejo y el nuevo, o un caché desalojado— la pide a la red. Cloudflare Pages responde
// /index.html con 308 a «/», y una respuesta redirigida no se puede usar para una navegación: la
// tableta quedaba en blanco (net::ERR_FAILED). «/» nunca redirige.

type Entry = { url: string; revision: string | null; size: number };

// shellFromRoot publica la entrada index.html del precache como «/», con su misma revisión.
export async function shellFromRoot(entries: Entry[]): Promise<{ manifest: Entry[]; warnings: string[] }> {
  return { manifest: entries.map((e) => (e.url === 'index.html' ? { ...e, url: '/' } : e)), warnings: [] };
}

export const workboxOptions = {
  // Precachea el shell (incl. fuente e íconos) para que la app abra aunque parpadee la red.
  globPatterns: ['**/*.{js,css,html,ico,png,svg,webp,woff,woff2}'],
  manifestTransforms: [shellFromRoot],
  navigateFallback: '/',
  navigateFallbackDenylist: [/^\/api/], // el SW nunca sirve el shell a llamadas de API
  cleanupOutdatedCaches: true,
};
