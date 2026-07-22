/// <reference types="vite/client" />
/// <reference types="vite-plugin-pwa/client" />

// @fontsource-* solo exporta CSS (side-effect), sin tipos propios.
declare module '@fontsource-variable/inter';

interface ImportMetaEnv {
  // Vacío en dev (Vite hace proxy de /api → :8080). En prod, mismo origen tras Caddy.
  readonly VITE_API_URL?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
