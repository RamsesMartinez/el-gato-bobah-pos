import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import '@fontsource-variable/inter';
import { Provider } from './components/ui/provider';
import { Toaster } from './components/ui/toaster';
import { App } from './App';
import { PantallaQueNoSeCae } from './app/PantallaQueNoSeCae';
import { registrarLimpiezaDeTenant } from './stores/session';
import { usePosStore } from './stores/pos';
import { reiniciarCaptura } from './features/pos/useCuenta';
import { initPwa } from './shared/pwa/registerPwa';
import './index.css';

const queryClient = new QueryClient();

// Aislamiento por empresa dentro del dispositivo. La caché del menú vive bajo ['menu'] sin empresa
// y con gcTime infinito: sin esto, entrar con otra empresa en la misma tablet deja el POS pintando
// el catálogo anterior. Las cuentas viven en el servidor (spec 030); de la tableta solo se olvida
// cuál estaba abierta y lo que tenía en vuelo.
registrarLimpiezaDeTenant(() => {
  queryClient.clear();
  usePosStore.getState().cuentaNueva();
  reiniciarCaptura();
});

createRoot(document.getElementById('root') as HTMLElement).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <Provider defaultTheme="light" enableSystem>
        {/* La barrera envuelve a <App/> y NO al árbol entero: por dentro del Provider, para que la
            pantalla de salida se pinte con el tema y los tokens de siempre; y sin envolver al
            <Toaster/>, que es lo único que puede seguir hablando si App se cayó. */}
        <PantallaQueNoSeCae>
          <App />
        </PantallaQueNoSeCae>
        <Toaster />
      </Provider>
    </QueryClientProvider>
  </StrictMode>,
);

// Registra el service worker (no-op en dev) y ofrece instalar / actualizar vía toaster.
initPwa();
