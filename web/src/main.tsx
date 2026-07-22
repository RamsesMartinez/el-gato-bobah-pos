import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import '@fontsource-variable/inter';
import { Provider } from './components/ui/provider';
import { Toaster } from './components/ui/toaster';
import { App } from './App';
import { initPwa } from './features/pwa/registerPwa';
import './index.css';

const queryClient = new QueryClient();

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
