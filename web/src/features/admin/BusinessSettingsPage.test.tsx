import { vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ChakraProvider, defaultSystem } from '@chakra-ui/react';
import type { ReactNode } from 'react';

const businessSettings = vi.hoisted(() => vi.fn());
const updateCorteDeVista = vi.hoisted(() => vi.fn());
const updateTimezone = vi.hoisted(() => vi.fn());
vi.mock('../../api/pos', () => ({
  posApi: {
    businessSettings,
    updateCorteDeVista,
    updateTimezone,
    updateBusinessSettings: vi.fn(),
    updateBusinessInfo: vi.fn(),
    businessLogo: vi.fn(),
    uploadBusinessLogo: vi.fn(),
    deleteBusinessLogo: vi.fn(),
  },
}));
vi.mock('../../components/ui/toaster', () => ({ toaster: { create: vi.fn() } }));

import { BusinessSettingsPage } from './BusinessSettingsPage';

const ajustes = {
  deliveryFee: '20', businessName: 'Gato', address: '', phone: '', headerNote: '', footerNote: '',
  autoPrintOnClose: false, timezone: 'America/Mexico_City', printFreeModifiers: true,
  printKitchenTicket: true, kitchenCanCharge: false, pinOnlyUnlock: false, lockAfterSeconds: 180,
  sessionHours: 8, hasLogo: false, logoUpdatedAt: null, corteDeVista: 'medianoche',
};

function pinta(nodo: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <ChakraProvider value={defaultSystem}>
      <QueryClientProvider client={qc}>{nodo}</QueryClientProvider>
    </ChakraProvider>,
  );
}

beforeEach(() => { businessSettings.mockResolvedValue(ajustes); });
afterEach(() => { vi.clearAllMocks(); });

// El corte se elige con `Picker`, nunca con un `<select>` nativo: en una tableta de 7 pulgadas el
// desplegable del sistema tapa la pantalla con renglones de 20 px y no se acierta con el dedo.
test('el corte de vista se elige sin desplegables del sistema', async () => {
  const { container } = pinta(<BusinessSettingsPage />);
  await screen.findByText(/Pedidos entregados en pantalla/i);
  expect(container.querySelector('select'), 'un <select> nativo en 7 pulgadas no se puede tocar').toBeNull();
});

// CAMBIAR LA ZONA AVISA, Y DICE LAS DOS COSAS.
//
// Todas las horas de todas las pantallas se mueven de golpe. Sin decirlo antes, se lee como que los
// datos se corrompieron — y la segunda frase es la que desactiva ese miedo: el dinero no se movió.
test('al cambiar la zona se avisa qué cambia y qué NO', async () => {
  pinta(<BusinessSettingsPage />);
  await screen.findByText(/Zona horaria/i);

  // Sin cambio no hay aviso: un mensaje permanente es ruido que se deja de leer.
  expect(screen.queryByText(/no cambian de día/i)).toBeNull();

  fireEvent.click(screen.getByRole('button', { name: /Centro|Ciudad de México/i }));
  fireEvent.click(await screen.findByText(/Tijuana/i));

  await waitFor(() => {
    expect(screen.getByText(/las horas que muestran las pantallas y los tickets cambian/i)).toBeInTheDocument();
    expect(screen.getByText(/no cambian de día ni de corte/i)).toBeInTheDocument();
  });
});
