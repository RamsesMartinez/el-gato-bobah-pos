import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react-swc';
import { VitePWA } from 'vite-plugin-pwa';
import { execSync } from 'node:child_process';
import path from 'node:path';

// Puertos configurables por env (los fija scripts/start.sh tras preguntar/detectar libres);
// defaults 3000 (web) y 8080 (API). El dev server hace proxy de /api al backend para evitar CORS.
const FRONTEND_PORT = Number(process.env.FRONTEND_PORT) || 3000;
const BACKEND_PORT = Number(process.env.BACKEND_PORT) || 8080;

// Versión horneada en el build para el pie de "Detalles del sistema". CI la pasa por
// VITE_APP_VERSION (SHA corto); en local cae al SHA de git, y a "dev" si no hay repo.
function gitSha(): string {
  try {
    return execSync('git rev-parse --short HEAD').toString().trim();
  } catch {
    return 'dev';
  }
}
const APP_VERSION = process.env.VITE_APP_VERSION || gitSha();
const BUILT_AT = new Date().toISOString();

export default defineConfig({
  define: {
    __APP_VERSION__: JSON.stringify(APP_VERSION),
    __APP_BUILT_AT__: JSON.stringify(BUILT_AT),
  },
  plugins: [
    react(),
    VitePWA({
      registerType: 'prompt', // POS: nunca recargar solo a media venta; el operador decide (ver registerPwa.ts)
      injectRegister: false, // registramos a mano en registerPwa.ts para poder mostrar el aviso con nuestro toaster
      manifest: false, // public/manifest.json es la fuente de verdad; el plugin solo genera el service worker
      workbox: {
        // Precachea el shell (incl. fuente e íconos) para que la app abra aunque parpadee la red.
        globPatterns: ['**/*.{js,css,html,ico,png,svg,webp,woff,woff2}'],
        navigateFallback: 'index.html',
        navigateFallbackDenylist: [/^\/api/], // el SW nunca sirve el shell a llamadas de API
        cleanupOutdatedCaches: true,
      },
    }),
  ],
  resolve: {
    alias: { '@': path.resolve(__dirname, './src') },
  },
  server: {
    port: FRONTEND_PORT,
    strictPort: true, // no saltar a otro puerto en silencio: start.sh ya garantizó que está libre
    host: true, // expone en LAN para probar en la tablet
    proxy: {
      '/api': { target: `http://localhost:${BACKEND_PORT}`, changeOrigin: true },
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: './src/setupTests.ts',
  },
});
