import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../../components/ui/provider';
import { AvisoDePlataforma } from './AvisoDePlataforma';
import * as api from '../../api/pedidosDePlataforma';
import { posApi } from '../../api/pos';
import type { PedidoDePlataforma } from '../../api/pedidosDePlataforma';

const pendientes = vi.hoisted(() => ({ current: [] as PedidoDePlataforma[] }));
vi.mock('./usePedidosDePlataforma', () => ({
  CLAVE_PENDIENTES: ['pedidos-de-plataforma', 'pendientes'],
  usePedidosDePlataforma: () => ({ data: pendientes.current }),
}));
vi.mock('../../api/pedidosDePlataforma', async () => {
  const real = await vi.importActual<typeof api>('../../api/pedidosDePlataforma');
  return { ...real, aceptarPedidoDePlataforma: vi.fn() };
});
vi.mock('../../api/pos', async () => {
  const real = await vi.importActual<typeof import('../../api/pos')>('../../api/pos');
  return { ...real, posApi: { ...real.posApi, order: vi.fn() } };
});

const printHtmlOffscreen = vi.hoisted(() => vi.fn((_html: string) => Promise.resolve(true)));
vi.mock('../../utils/printReceipt', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../utils/printReceipt')>()),
  printHtmlOffscreen,
}));
const comanda = vi.hoisted(() => ({ encendida: true }));
vi.mock('../../shared/tickets/ticketBusinessInfo', () => ({
  useTicketBusinessInfo: () => ({
    data: { businessName: 'El Gato Bobah', timezone: 'America/Monterrey' },
    isLoading: false,
    autoPrintOnClose: false,
    printKitchenTicket: comanda.encendida,
  }),
}));

const enMinutos = (m: number) => new Date(Date.now() + m * 60000).toISOString();
const pedido = (id: number, displayId: string): PedidoDePlataforma => ({
  id, platformName: 'Uber Eats', displayId, placedAt: null, decideBefore: enMinutos(10),
  serviceType: 'domicilio', customerName: 'Ana', total: '342.00',
  lines: [{ externalName: 'Crepa de Nutella', quantity: '2', unitPrice: '129', productId: 8, matched: true }],
});

const detalle = {
  id: 91, number: 12, folioName: '', status: 'en_preparacion', serviceType: 'domicilio', customerName: 'Ana',
  subtotal: '342', discount: '0', deliveryFee: '0', total: '342', currency: 'MXN', paid: true,
  openedAt: '2026-09-23T18:00:00Z',
  lines: [{ id: 1, productName: 'Crepa de Nutella', quantity: '2', unitPrice: '171', lineTotal: '342', modifiers: [] }],
};

const montar = () =>
  render(
    <QueryClientProvider client={new QueryClient()}>
      <Provider>
        <AvisoDePlataforma />
      </Provider>
    </QueryClientProvider>,
  );

beforeEach(() => {
  pendientes.current = [pedido(1, 'AAA1')];
  comanda.encendida = true;
  printHtmlOffscreen.mockClear();
  vi.mocked(api.aceptarPedidoDePlataforma).mockReset().mockResolvedValue({ id: 91, number: 12, platformRef: 'ped-1' });
  vi.mocked(posApi.order).mockReset().mockResolvedValue(detalle as never);
});

describe('AvisoDePlataforma', () => {
  // ACEPTAR SACA LA COMANDA. Sin papel, cocina no se entera: el pedido está «aceptado» en la
  // plataforma, el cliente ya pagó, y nadie lo está haciendo.
  it('al aceptar imprime la comanda del pedido que se creó', async () => {
    montar();
    await userEvent.click(screen.getByRole('button', { name: /Aceptar/ }));
    await waitFor(() => expect(printHtmlOffscreen).toHaveBeenCalledTimes(1));
    // La del pedido que quedó en el POS, no la del aviso: es la que cocina ve en Pedidos.
    expect(posApi.order).toHaveBeenCalledWith(91);
    expect(printHtmlOffscreen.mock.calls[0][0]).toContain('Crepa de Nutella');
  });

  it('con la comanda apagada en el negocio, aceptar no imprime', async () => {
    comanda.encendida = false;
    montar();
    await userEvent.click(screen.getByRole('button', { name: /Aceptar/ }));
    await waitFor(() => expect(api.aceptarPedidoDePlataforma).toHaveBeenCalled());
    await new Promise((r) => setTimeout(r, 20));
    expect(printHtmlOffscreen).not.toHaveBeenCalled();
  });

  // Si la plataforma no confirmó, el pedido NO quedó aceptado: una comanda de un pedido que no
  // existe manda a cocina a preparar algo que nadie va a recoger.
  it('si aceptar falla no sale ninguna comanda', async () => {
    vi.mocked(api.aceptarPedidoDePlataforma).mockRejectedValue(new Error('409'));
    montar();
    await userEvent.click(screen.getByRole('button', { name: /Aceptar/ }));
    await new Promise((r) => setTimeout(r, 20));
    expect(posApi.order).not.toHaveBeenCalled();
    expect(printHtmlOffscreen).not.toHaveBeenCalled();
  });

  // EL CONTADOR ABRE LA LISTA COMPLETA. Antes el «+2 más» se pintaba y no hacía nada: con dos
  // pendientes, para ver el segundo había que aceptar el primero.
  it('el contador abre la lista con todos, y cada uno se acepta desde ahí', async () => {
    pendientes.current = [pedido(1, 'AAA1'), pedido(2, 'BBB2'), pedido(3, 'CCC3')];
    montar();
    await userEvent.click(screen.getByRole('button', { name: /\+2 más/ }));
    const hoja = await screen.findByRole('dialog');
    for (const folio of ['AAA1', 'BBB2', 'CCC3']) {
      expect(within(hoja).getByText(new RegExp(folio))).toBeInTheDocument();
    }
    // La franja se quita mientras la lista está abierta: va por encima de todo, y taparía la hoja.
    expect(screen.queryByTestId('aviso-de-pedido-entrante')).not.toBeInTheDocument();

    await userEvent.click(within(hoja).getByRole('button', { name: /Aceptar BBB2/ }));
    expect(api.aceptarPedidoDePlataforma).toHaveBeenCalledWith(2);
  });
});
