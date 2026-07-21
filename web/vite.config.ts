import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react-swc';
import path from 'node:path';

// Puertos configurables por env (los fija scripts/start.sh tras preguntar/detectar libres);
// defaults 3000 (web) y 8080 (API). El dev server hace proxy de /api al backend para evitar CORS.
const FRONTEND_PORT = Number(process.env.FRONTEND_PORT) || 3000;
const BACKEND_PORT = Number(process.env.BACKEND_PORT) || 8080;

export default defineConfig({
  plugins: [react()],
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
