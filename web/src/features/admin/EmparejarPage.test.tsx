import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../../components/ui/provider';
import { EmparejarPage } from './EmparejarPage';
import * as api from '../../api/plataformas';
import { ApiError } from '../../api/client';

vi.mock('../../api/plataformas', async () => {
  const real = await vi.importActual<typeof api>('../../api/plataformas');
  return {
    ...real,
    tablero: vi.fn(),
    candidatos: vi.fn(),
    guardarPareja: vi.fn(),
    borrarPareja: vi.fn(),
    confirmarLote: vi.fn(),
    marcarSoloEnPlataforma: vi.fn(),
    quitarSoloEnPlataforma: vi.fn(),
  };
});

const renglon = (r: Partial<api.RenglonDelTablero> & { externalId: string }): api.RenglonDelTablero => ({
  kind: 'platillo',
  name: r.externalId,
  price: '99.00',
  available: true,
  group: 'unpaired',
  link: null,
  proposal: null,
  ...r,
});

const datos = (): api.TableroDeEmparejamiento => ({
  platformName: 'Uber Eats',
  storeLabel: 'Sucursal Centro',
  readAt: '2026-10-03T16:42:00Z',
  counts: { unpaired: 2, toReview: 2, done: 1, excluded: 0 },
  items: [
    renglon({ externalId: 'crepa', name: 'Crepa de Nutella con Fresa', price: '149.00' }),
    renglon({ externalId: 'queso', name: 'Dedos de queso', price: '82.80' }),
    renglon({
      externalId: 'chai', name: 'Chai Latte', group: 'toReview',
      proposal: { localKind: 'producto', localId: 41, localName: 'Chai Latte' },
    }),
    renglon({
      externalId: 'capu', name: 'Capuccino', group: 'toReview',
      proposal: { localKind: 'producto', localId: 42, localName: 'Capuccino' },
    }),
    renglon({
      externalId: 'mango', name: 'Chamoyada de Mango', group: 'done',
      link: { localKind: 'producto', localId: 7, localName: 'Chamoyada', isCapturePrice: true, confirmedAt: '2026-10-03T16:00:00Z' },
    }),
  ],
  priceChanges: [{ name: 'Capuccino', old: '80.00', new: '85.05' }],
});

const montar = () =>
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <Provider>
        <EmparejarPage conexionId={1} />
      </Provider>
    </QueryClientProvider>,
  );

beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(api.tablero).mockResolvedValue(datos());
  vi.mocked(api.candidatos).mockResolvedValue([
    { localKind: 'producto', id: 50, name: 'Arma tu Crepa', linkedCount: 2 },
    { localKind: 'producto', id: 51, name: 'Crepa Nutella', linkedCount: 0 },
  ]);
  vi.mocked(api.guardarPareja).mockResolvedValue(undefined as never);
  vi.mocked(api.confirmarLote).mockResolvedValue({ confirmed: ['chai', 'capu'], skipped: [] });
  vi.mocked(api.marcarSoloEnPlataforma).mockResolvedValue(undefined as never);
});

describe('EmparejarPage (diseño B)', () => {
  it('muestra los grupos con su conteo, el aviso de precios y el precio con dos decimales', async () => {
    montar();
    expect(await screen.findByRole('button', { name: /Sin pareja 2/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Por revisar 2/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Listos 1/ })).toBeInTheDocument();
    expect(screen.getByText(/Precios: los pone Uber/)).toBeInTheDocument();
    // «$82.8» se lee como un error al compararlo con lo que publica la plataforma.
    expect(screen.getByText('$82.80')).toBeInTheDocument();
    expect(screen.getByText(/1 precio cambió/)).toBeInTheDocument();
  });

  it('no usa select nativo y todo control tappable mide al menos 44 px', async () => {
    const { container } = montar();
    await screen.findByRole('button', { name: /Sin pareja 2/ });
    expect(container.querySelector('select')).toBeNull();
    for (const b of screen.getAllByRole('button')) {
      const alto = getComputedStyle(b).minHeight;
      expect(parseInt(alto || '0', 10), b.textContent ?? '').toBeGreaterThanOrEqual(44);
    }
  });

  // `overflowY` SIN ALTO NO HACE SCROLL: con 33 platillos sin pareja, a 600 px solo se verían 7 y
  // el resto no existiría para quien opera.
  it('la lista tiene su propio scroll con alto acotado', async () => {
    montar();
    const lista = await screen.findByTestId('lista-de-la-tienda');
    const estilo = getComputedStyle(lista);
    expect(estilo.overflowY).toBe('auto');
    expect(estilo.minHeight).toBe('0px');
  });

  it('un platillo sin pareja ofrece candidatos, y un producto ya ligado sigue apareciendo', async () => {
    const user = userEvent.setup();
    montar();
    await user.click(within(await screen.findByTestId('lista-de-la-tienda')).getByText('Crepa de Nutella con Fresa'));
    const panel = await screen.findByRole('region', { name: 'Decidir' });
    expect(await within(panel).findByText('Arma tu Crepa')).toBeInTheDocument();
    expect(within(panel).getByText(/ya ligado a 2 platillos/)).toBeInTheDocument();
  });

  it('el segundo platillo al mismo producto pide el precio de captura y no liga sin elegir', async () => {
    const user = userEvent.setup();
    vi.mocked(api.guardarPareja).mockRejectedValueOnce(
      new ApiError(422, 'CAPTURE_PRICE_REQUIRED', 'elige', 'r1'),
    );
    montar();
    await user.click(within(await screen.findByTestId('lista-de-la-tienda')).getByText('Crepa de Nutella con Fresa'));
    await user.click(await screen.findByText('Arma tu Crepa'));
    const grupo = await screen.findByRole('radiogroup', { name: /Precio para capturar a mano/ });
    await user.click(within(grupo).getByRole('radio', { name: /Crepa de Nutella con Fresa/ }));
    await user.click(screen.getByRole('button', { name: /Ligar/ }));
    await waitFor(() =>
      expect(api.guardarPareja).toHaveBeenLastCalledWith(1, 'crepa', expect.objectContaining({ localId: 50, capturePrice: true })),
    );
  });

  it('en lote confirma todas las marcadas con un toque, y desmarcar deja fuera', async () => {
    const user = userEvent.setup();
    montar();
    await user.click(await screen.findByRole('button', { name: /Por revisar 2/ }));
    await user.click(screen.getByRole('button', { name: 'En lote' }));
    await user.click(screen.getByRole('checkbox', { name: /Capuccino/ }));
    await user.click(screen.getByRole('button', { name: /Confirmar 1/ }));
    await waitFor(() => expect(api.confirmarLote).toHaveBeenCalledWith(1, ['chai']));
  });

  it('uno por uno confirma la propuesta y avanza sola', async () => {
    const user = userEvent.setup();
    montar();
    await user.click(await screen.findByRole('button', { name: /Por revisar 2/ }));
    await user.click(screen.getByRole('button', { name: /Es el mismo/ }));
    await waitFor(() =>
      expect(api.guardarPareja).toHaveBeenCalledWith(1, 'chai', expect.objectContaining({ localId: 41, localKind: 'producto' })),
    );
  });

  it('«Solo existe en Uber» guarda la decisión', async () => {
    const user = userEvent.setup();
    montar();
    await user.click(within(await screen.findByTestId('lista-de-la-tienda')).getByText('Dedos de queso'));
    await user.click(screen.getByRole('button', { name: 'Solo existe en Uber' }));
    await waitFor(() => expect(api.marcarSoloEnPlataforma).toHaveBeenCalledWith(1, 'queso'));
  });

  it('una pareja lista muestra que el precio lo pone Uber y se puede quitar', async () => {
    const user = userEvent.setup();
    vi.mocked(api.borrarPareja).mockResolvedValue(undefined as never);
    montar();
    await user.click(await screen.findByRole('button', { name: /Listos 1/ }));
    await user.click(within(screen.getByTestId('lista-de-la-tienda')).getByText('Chamoyada de Mango'));
    expect(await screen.findByText(/lo pone Uber/)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Quitar la pareja' }));
    await waitFor(() => expect(api.borrarPareja).toHaveBeenCalledWith(1, 'mango'));
  });
});
