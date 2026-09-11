import { vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ChakraProvider, defaultSystem } from '@chakra-ui/react';
import type { ReactNode } from 'react';

const businessSettings = vi.hoisted(() => vi.fn());
const paymentMethods = vi.hoisted(() => vi.fn());
const updatePaymentMethod = vi.hoisted(() => vi.fn());
const updateArqueoCiego = vi.hoisted(() => vi.fn());
vi.mock('../../api/backoffice', () => ({ backofficeApi: { updatePaymentMethod } }));
const updateCorteDeVista = vi.hoisted(() => vi.fn());
const updateTimezone = vi.hoisted(() => vi.fn());
vi.mock('../../api/pos', () => ({
  posApi: {
    businessSettings,
    paymentMethods,
    updateCorteDeVista,
    updateTimezone,
    updateArqueoCiego,
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
  sessionHours: 8, hasLogo: false, logoUpdatedAt: null, corteDeVista: 'medianoche', blindCashCount: false,
};

function pinta(nodo: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <ChakraProvider value={defaultSystem}>
      <QueryClientProvider client={qc}>{nodo}</QueryClientProvider>
    </ChakraProvider>,
  );
}

const metodos = [
  { id: 1, name: 'Efectivo', kind: 'efectivo', affectsCashDrawer: true, autoDeclare: false, isActive: true, deliveryPlatformId: null },
  { id: 8, name: 'Didi efectivo', kind: 'plataforma', affectsCashDrawer: true, autoDeclare: false, isActive: true, deliveryPlatformId: 1 },
  { id: 2, name: 'Tarjeta débito', kind: 'tarjeta', affectsCashDrawer: false, autoDeclare: true, isActive: true, deliveryPlatformId: null },
];

beforeEach(() => {
  businessSettings.mockResolvedValue(ajustes);
  paymentMethods.mockResolvedValue({ items: metodos });
  updatePaymentMethod.mockResolvedValue(metodos[0]);
  updateArqueoCiego.mockResolvedValue(ajustes);
});
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


// LOS TRES INTERRUPTORES DE UN MÉTODO (spec 015, US2).
//
// Hasta aquí las dos columnas existían en la base y ningún endpoint las escribía: apagar un método
// o decidir si su efectivo llega al cajón exigía entrar a la base de datos.
describe('los métodos de cobro', () => {
  test('cada método trae sus tres interruptores, con su etiqueta en el encabezado', async () => {
    pinta(<BusinessSettingsPage />);
    await screen.findByText('Métodos de cobro');
    // Las etiquetas viven UNA vez, en el encabezado: en 520 px de ancho no caben repetidas por
    // renglón, que es lo que el plan daba por bueno midiendo contra 1024.
    expect(screen.getByRole('columnheader', { name: 'Va al cajón' })).toBeInTheDocument();
    for (const nombre of ['Efectivo', 'Didi efectivo', 'Tarjeta débito']) {
      expect(screen.getByLabelText(`${nombre} activo`)).toBeInTheDocument();
      expect(screen.getByLabelText(`${nombre} va al cajón`)).toBeInTheDocument();
      expect(screen.getByLabelText(`${nombre} automático`)).toBeInTheDocument();
    }
  });

  // EL CASO QUE IMPORTA: apagar uno NO puede tocar los otros dos.
  //
  // Es el mismo defecto que el handler evita con punteros, visto desde la pantalla: si esta manda
  // los tres valores en cada cambio, desactivar un método le apaga de paso el del cajón y saca su
  // dinero del arqueo.
  test('apagar «Activo» manda solo ese interruptor', async () => {
    pinta(<BusinessSettingsPage />);
    await screen.findByText('Métodos de cobro');

    fireEvent.click(screen.getByLabelText('Didi efectivo activo'));
    await waitFor(() => expect(updatePaymentMethod).toHaveBeenCalled());
    expect(updatePaymentMethod).toHaveBeenCalledWith(8, { isActive: false });
  });

  test('apagar «Va al cajón» manda solo ese interruptor', async () => {
    pinta(<BusinessSettingsPage />);
    await screen.findByText('Métodos de cobro');

    fireEvent.click(screen.getByLabelText('Didi efectivo va al cajón'));
    await waitFor(() => expect(updatePaymentMethod).toHaveBeenCalled());
    expect(updatePaymentMethod).toHaveBeenCalledWith(8, { affectsCashDrawer: false });
  });

  test('no hay desplegables del sistema en el bloque de métodos', async () => {
    const { container } = pinta(<BusinessSettingsPage />);
    await screen.findByText('Métodos de cobro');
    expect(container.querySelector('select')).toBeNull();
  });
});


// EL ARQUEO CIEGO (spec 015, US3).
//
// Encenderlo enmienda FR-005 de la spec 003 —"la diferencia se ve antes de confirmar"— y por eso es
// un interruptor del negocio y no una preferencia de quien opera: las dos reglas protegen cosas
// distintas, y quién decide cuál aplica es el dueño.
describe('el arqueo ciego', () => {
  test('se enciende desde Ajustes, y el texto dice qué cambia para quien cuenta', async () => {
    pinta(<BusinessSettingsPage />);
    await screen.findByText('Contar sin ver lo esperado');
    expect(screen.getByText(/La diferencia aparece al confirmar/i)).toBeInTheDocument();

    fireEvent.click(screen.getByLabelText('Contar sin ver lo esperado'));
    await waitFor(() => expect(updateArqueoCiego).toHaveBeenCalledWith(true));
  });
});
