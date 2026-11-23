import { beforeEach, describe, expect, test, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../../components/ui/provider';
import type { AccountItem } from '../../types/pos';
import { TodasLasCuentasSheet } from './TodasLasCuentasSheet';
import { antiguedad } from './estadosDeCuenta';
import { cuenta } from './__fixtures__/cuentas';

const api = vi.hoisted(() => ({ liveAccounts: vi.fn(), businessSettings: vi.fn() }));
vi.mock('../../api/pos', () => ({ posApi: api }));

const VARIAS: AccountItem[] = [
  cuenta({ key: 'd:a', kind: 'draft', draftId: 'a', orderId: null, number: null, folioName: 'Levkoy', state: 'capturing', group: 'capturing', total: '74.00', outstanding: '74.00', lineCount: 2 }),
  cuenta({ key: 'o:1', orderId: 1, folioName: 'Khao Manee', group: 'in_kitchen' }),
  cuenta({ key: 'o:6', orderId: 6, number: 6, folioName: 'Siamés', state: 'paid_in_kitchen', group: 'in_kitchen', outstanding: '0.00', paid: '63.00', total: '63.00' }),
  cuenta({ key: 'o:5', orderId: 5, number: 5, folioName: 'Mesa de afuera', state: 'partly_paid', group: 'delivered_owes', outstanding: '130.00', paid: '110.00', total: '240.00' }),
  cuenta({ key: 'o:14', orderId: 14, number: 14, folioName: 'Manx', state: 'delivered_owes', group: 'previous_days', outstanding: '88.00', total: '88.00', businessDate: '2026-10-07', openedAt: '2026-10-08T03:05:00Z' }),
];

function montar(items: AccountItem[] = VARIAS) {
  api.liveAccounts.mockResolvedValue({ items, outstanding: '605.00', serverTime: '2026-10-08T15:42:00Z' });
  api.businessSettings.mockResolvedValue({ timezone: 'America/Mexico_City' });
  const onElegir = vi.fn();
  const onClose = vi.fn();
  const onDeliver = vi.fn();
  const onCharge = vi.fn();
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <Provider>
        <TodasLasCuentasSheet isOpen seleccionada={null} onElegir={onElegir} onClose={onClose}
          onDeliver={onDeliver} onCharge={onCharge} />
      </Provider>
    </QueryClientProvider>,
  );
  return { onElegir, onClose, onDeliver, onCharge };
}

beforeEach(() => vi.clearAllMocks());

describe('la hoja «+N» con todas las cuentas', () => {
  // Las de días anteriores solo vienen con olderDebts: la fila cada 30 s mira 90 días, esta hoja
  // mira todo (FR-009, research R-7).
  test('al abrirse pide también las deudas de días anteriores', async () => {
    montar();
    await waitFor(() => expect(api.liveAccounts).toHaveBeenCalledWith(true));
  });

  test('agrupa en orden y con su conteo', async () => {
    montar();
    const grupos = await screen.findAllByRole('heading', { level: 3 });
    expect(grupos.map((g) => g.textContent)).toEqual([
      'Capturando · aún no en cocina (1)', 'En cocina (2)', 'Entregadas que deben (1)', 'De días anteriores (1)',
    ]);
  });

  test('un grupo vacío no se pinta', async () => {
    montar(VARIAS.filter((c) => c.group !== 'previous_days'));
    await screen.findByText('Levkoy');
    expect(screen.queryByText(/De días anteriores/)).toBeNull();
  });

  test('el encabezado dice cuántas viven y cuánto se debe', async () => {
    montar();
    expect(await screen.findByText(/5 vivas · por cobrar \$605/)).toBeInTheDocument();
  });

  // Spec 089: el renglón dice lo que le falta para quitarse, no el estado interno.
  test('cada renglón dice lo que le falta, y mide al menos 56 px', async () => {
    montar();
    const fila = await screen.findByRole('button', { name: /^Mesa de afuera/ });
    expect(within(fila).getByText('Falta cobrar $130 y entregar')).toBeInTheDocument();
    expect(within(fila).queryByText('Pago parcial')).toBeNull();
    expect(parseInt(getComputedStyle(fila).minHeight || '0', 10)).toBeGreaterThanOrEqual(56);
  });

  test('una deuda de otro día dice su fecha', async () => {
    montar();
    const fila = await screen.findByRole('button', { name: /^Manx/ });
    expect(within(fila).getByText(/7 oct/)).toBeInTheDocument();
  });

  // Con el teclado abierto la lista se queda en ~150 px: el buscador solo cuando hace falta.
  test('sin más de 8 cuentas no hay buscador', async () => {
    montar();
    await screen.findByText('Levkoy');
    expect(screen.queryByRole('textbox', { name: /Buscar cuenta/ })).toBeNull();
  });

  test('con más de 8 hay buscador y filtra por nombre o número', async () => {
    const muchas = Array.from({ length: 9 }, (_, i) => cuenta({ key: `o:${i + 1}`, orderId: i + 1, number: i + 1, folioName: `Gato ${i + 1}` }));
    muchas.push(cuenta({ key: 'o:77', orderId: 77, number: 77, folioName: 'Persa' }));
    montar(muchas);
    const buscar = await screen.findByRole('textbox', { name: /Buscar cuenta/ });
    fireEvent.change(buscar, { target: { value: 'pers' } });
    expect(screen.getByRole('button', { name: /^Persa/ })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /^Gato 1\b/ })).toBeNull();
  });

  test('tocar una la elige y cierra la hoja', async () => {
    const { onElegir, onClose } = montar();
    fireEvent.click(await screen.findByRole('button', { name: /^Siamés/ }));
    expect(onElegir).toHaveBeenCalledWith(expect.objectContaining({ key: 'o:6' }));
    expect(onClose).toHaveBeenCalled();
  });
});

describe('antigüedad', () => {
  const ahora = Date.parse('2026-10-08T15:42:00Z');
  test.each([
    ['2026-10-08T15:41:40Z', 'hace un momento'],
    ['2026-10-08T15:00:00Z', 'hace 42 min'],
    ['2026-10-08T12:30:00Z', 'hace 3 h'],
  ])('%s → %s', (desde, esperado) => {
    expect(antiguedad(desde, ahora)).toBe(esperado);
  });
});

// «1 productos» (validación como usuario nuevo).
test('una cuenta con un solo producto dice «1 producto»', async () => {
  montar([cuenta({ key: 'd:b', kind: 'draft', draftId: 'b', orderId: null, number: null, folioName: 'Persa', state: 'capturing', group: 'capturing', lineCount: 1 })]);
  expect(await screen.findByText(/· 1 producto$/)).toBeInTheDocument();
});

// Pedidos 614 y 623: entregados y pagados con una «Nuevo» viva. No están en cocina ni deben nada:
// mostrar su total a la derecha se lee como dinero por cobrar.
test('una cerrada con algo nuevo no dice «en cocina» ni muestra su total como por cobrar', async () => {
  montar([cuenta({ key: 'o:614', orderId: 614, number: 614, folioName: 'Ragdoll', state: 'closed_with_new', group: 'capturing',
    closedWithPending: true, outstanding: '0.00', paid: '93.00', total: '93.00' })]);
  const fila = await screen.findByText('Ragdoll');
  const renglon = fila.closest('button') ?? fila.parentElement!.parentElement!;
  expect(within(renglon as HTMLElement).queryByText(/en cocina/)).toBeNull();
  expect(within(renglon as HTMLElement).queryByText('$93')).toBeNull();
  expect(within(renglon as HTMLElement).getByText('Nuevo sin enviar')).toBeInTheDocument();
});

// Spec 089, US2: lo que le falta se resuelve desde la hoja, sin elegir la cuenta primero.
describe('entregar y cobrar desde la hoja', () => {
  const acciones = async (nombre: string) => {
    const renglon = (await screen.findByRole('button', { name: new RegExp(`^${nombre}`) })).closest('[data-renglon]') as HTMLElement;
    return {
      entregar: within(renglon).queryByRole('button', { name: `Entregar ${nombre}` }),
      cobrar: within(renglon).queryByRole('button', { name: new RegExp(`^Cobrar .* ${nombre}$`) }),
    };
  };

  test('la pagada sin entregar ofrece solo «Entregar»', async () => {
    montar();
    const a = await acciones('Siamés');
    expect(a.entregar).not.toBeNull();
    expect(a.cobrar).toBeNull();
  });

  test('la entregada que debe ofrece solo «Cobrar» con lo que falta', async () => {
    montar();
    const a = await acciones('Manx');
    expect(a.entregar).toBeNull();
    expect(a.cobrar).toHaveTextContent('Cobrar $88');
  });

  test('la que falta cobrar y entregar ofrece las dos', async () => {
    montar();
    const a = await acciones('Khao Manee');
    expect(a.entregar).not.toBeNull();
    expect(a.cobrar).not.toBeNull();
  });

  test('una cuenta en captura no ofrece ninguna: todavía no es pedido', async () => {
    montar();
    const a = await acciones('Levkoy');
    expect(a.entregar).toBeNull();
    expect(a.cobrar).toBeNull();
  });

  test('tocar «Entregar» o «Cobrar» resuelve esa cuenta sin elegirla', async () => {
    const { onDeliver, onCharge, onElegir } = montar();
    const s = await acciones('Siamés');
    fireEvent.click(s.entregar!);
    expect(onDeliver).toHaveBeenCalledWith(expect.objectContaining({ key: 'o:6' }));
    const m = await acciones('Manx');
    fireEvent.click(m.cobrar!);
    expect(onCharge).toHaveBeenCalledWith(expect.objectContaining({ key: 'o:14' }));
    expect(onElegir).not.toHaveBeenCalled();
  });

  test('los botones miden al menos 44 px', async () => {
    montar();
    const a = await acciones('Khao Manee');
    for (const b of [a.entregar!, a.cobrar!]) expect(parseInt(getComputedStyle(b).minHeight || '0', 10)).toBeGreaterThanOrEqual(44);
  });
});
