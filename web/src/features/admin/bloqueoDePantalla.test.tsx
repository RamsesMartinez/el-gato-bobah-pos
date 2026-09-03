import { vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ChakraProvider, defaultSystem } from '@chakra-ui/react';
import type { ReactNode } from 'react';

const businessSettings = vi.hoisted(() => vi.fn());
const updateTicketSettings = vi.hoisted(() => vi.fn());
vi.mock('../../api/pos', () => ({
  posApi: {
    businessSettings,
    updateTicketSettings,
    updateBusinessInfo: vi.fn(),
    ticketLogo: vi.fn(),
    uploadTicketLogo: vi.fn(),
    deleteTicketLogo: vi.fn(),
  },
}));
vi.mock('../../components/ui/toaster', () => ({ toaster: { create: vi.fn() } }));

import { PrintSettingsPage } from './PrintSettingsPage';

const ajustes = (over: Record<string, unknown> = {}) => ({
  deliveryFee: '20', businessName: 'Gato', address: '', phone: '', headerNote: '', footerNote: '',
  autoPrintOnClose: false, timezone: 'America/Mexico_City', printFreeModifiers: true,
  printKitchenTicket: true, kitchenCanCharge: false, pinOnlyUnlock: false,
  lockAfterSeconds: 0, sessionHours: 8, hasLogo: false, logoUpdatedAt: null,
  corteDeVista: 'medianoche', folioScheme: 'razas', ...over,
});

function pinta(nodo: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <ChakraProvider value={defaultSystem}>
      <QueryClientProvider client={qc}>{nodo}</QueryClientProvider>
    </ChakraProvider>,
  );
}

beforeEach(() => {
  businessSettings.mockResolvedValue(ajustes());
  updateTicketSettings.mockImplementation(async (v: Record<string, unknown>) => ({ ...ajustes(), ...v }));
});
afterEach(() => { vi.clearAllMocks(); });

const interruptor = () => screen.getByLabelText('Bloqueo de la pantalla');

// EL BLOQUEO NACE APAGADO, Y SE APAGA CON UN INTERRUPTOR.
//
// Apagarlo ya se podía —el campo aceptaba 0 y la letra chica decía "0 = no se bloquea"— pero eso no
// es un interruptor: nadie adivina que un tiempo de cero es la forma de apagar una protección, y el
// negocio se quedaba pidiendo PIN cada tres minutos a media venta.
test('con el bloqueo apagado no se pregunta cada cuánto', async () => {
  pinta(<PrintSettingsPage />);

  await waitFor(() => expect(interruptor()).not.toBeChecked());
  // El campo de segundos no se pinta: una pregunta que no hace nada es alto gastado.
  expect(screen.queryByLabelText('Se bloquea a los')).toBeNull();
  // Y se dice qué sigue protegiendo, para que apagarlo no se lea como "sin ninguna barrera".
  expect(screen.getByText(/La sesión sigue caducando/)).toBeInTheDocument();
});

test('encenderlo pregunta el tiempo y guarda uno mayor que cero', async () => {
  const u = userEvent.setup();
  pinta(<PrintSettingsPage />);

  await waitFor(() => expect(interruptor()).not.toBeChecked());
  await u.click(interruptor());

  expect(await screen.findByLabelText('Se bloquea a los')).toHaveValue(180);
  await u.click(screen.getByRole('button', { name: /Guardar tiempos/ }));
  await waitFor(() => expect(updateTicketSettings).toHaveBeenCalledWith(
    expect.objectContaining({ lockAfterSeconds: 180 }),
  ));
});

// APAGARLO GUARDA CERO, que es lo que el servidor entiende por "no se bloquea". Sin esto el
// interruptor se vería apagado y la tableta se seguiría bloqueando.
test('apagarlo guarda cero', async () => {
  const u = userEvent.setup();
  businessSettings.mockResolvedValue(ajustes({ lockAfterSeconds: 300 }));
  pinta(<PrintSettingsPage />);

  await waitFor(() => expect(interruptor()).toBeChecked());
  await u.click(interruptor());
  await u.click(screen.getByRole('button', { name: /Guardar tiempos/ }));

  await waitFor(() => expect(updateTicketSettings).toHaveBeenCalledWith(
    expect.objectContaining({ lockAfterSeconds: 0 }),
  ));
});

// APAGAR Y VOLVER A ENCENDER NO PIERDE EL TIEMPO QUE EL NEGOCIO ELIGIÓ.
//
// Es el borde que un interruptor sobre un número tiene siempre: apagar escribe 0 y con eso se borra
// el valor. Quien tenía 600 segundos y lo apaga por una tarde volvería con 180 sin haberlo pedido.
test('apagar y encender devuelve el tiempo que ya estaba, no el default', async () => {
  const u = userEvent.setup();
  businessSettings.mockResolvedValue(ajustes({ lockAfterSeconds: 600 }));
  pinta(<PrintSettingsPage />);

  await waitFor(() => expect(interruptor()).toBeChecked());
  await u.click(interruptor());
  await u.click(interruptor());

  expect(await screen.findByLabelText('Se bloquea a los')).toHaveValue(600);
});
