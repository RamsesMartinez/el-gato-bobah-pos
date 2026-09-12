import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react-swc';
import path from 'node:path';

// EL BUILD DE LA CONSOLA DE PLATAFORMA (spec 016). Separado del POS a propósito, y no por gusto:
//
//  - `outDir: dist-consola`. Con el `dist/` del POS, el segundo build BORRA el primero: CI corre
//    los dos y subiría a Pages lo que quedó al final. El síntoma sería el POS sirviendo la consola.
//  - `publicDir: public-consola`. El `public/` del POS trae `manifest.json`, los íconos y el
//    `_headers` de la app del restaurante: con él, la consola quedaría instalable como PWA con el
//    nombre y el logo del negocio en la computadora de quien VENDE el sistema.
//  - Sin `VitePWA`. Una consola de escritorio no necesita service worker, y con uno cada cambio
//    esperaría a que alguien acepte una actualización — la trampa que el `_headers` del POS
//    documenta.
//
// El `root` es `consola/`, donde vive su `index.html`: así el archivo que Pages sirve en la raíz
// se llama `index.html` y no `consola.html`.
const FRONTEND_PORT = Number(process.env.CONSOLA_PORT) || 3100;
const BACKEND_PORT = Number(process.env.BACKEND_PORT) || 8080;

export default defineConfig({
  root: path.resolve(__dirname, 'consola'),
  publicDir: path.resolve(__dirname, 'public-consola'),
  plugins: [react()],
  resolve: {
    alias: { '@': path.resolve(__dirname, './src') },
  },
  build: {
    outDir: path.resolve(__dirname, 'dist-consola'),
    emptyOutDir: true,
  },
  server: {
    port: FRONTEND_PORT,
    strictPort: true,
    // Mismo proxy que el POS: en desarrollo la consola habla con el backend por el mismo origen y
    // no hace falta tocar CORS para trabajar en local.
    proxy: {
      '/api': { target: `http://localhost:${BACKEND_PORT}`, changeOrigin: true },
    },
  },
});
