import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import '@fontsource-variable/inter';
import { Provider } from './components/ui/provider';
import { Toaster } from './components/ui/toaster';
import { App } from './App';
import { registrarLimpiezaDeTenant } from './stores/session';
import { useTicketStore } from './stores/ticket';
import { initPwa } from './shared/pwa/registerPwa';
import './index.css';

const queryClient = new QueryClient();

// Aislamiento por empresa dentro del dispositivo. La caché del menú vive bajo ['menu'] sin empresa
// y con gcTime infinito, y el carrito se persiste en localStorage: sin esto, entrar con otra
// empresa en la misma tablet deja el POS pintando el catálogo anterior y el ticket viejo esperando.
// El backend rechaza esa venta, pero el operador se entera hasta el cobro.
registrarLimpiezaDeTenant(() => {
  queryClient.clear();
  useTicketStore.getState().descartarTodo();
});

createRoot(document.getElementById('root') as HTMLElement).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <Provider defaultTheme="light" enableSystem>
        <App />
        <Toaster />
      </Provider>
    </QueryClientProvider>
  </StrictMode>,
);

// Registra el service worker (no-op en dev) y ofrece instalar / actualizar vía toaster.
initPwa();
