import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../../components/ui/provider';
import { Toaster } from '../../components/ui/toaster';
import * as api from '../../api/pedidosEntrantes';
import { ApiError } from '../../api/client';
import type { PedidoEntrante } from '../../api/pedidosEntrantes';
import { ColumnaPorAceptar } from './ColumnaPorAceptar';
import { useColumnaPorAceptar } from './estado';
import * as hook from './useEntrantes';

vi.mock('./useEntrantes', () => ({ useEntrantes: vi.fn(), CLAVE_ENTRANTES: ['pedidos-entrantes'] }));
vi.mock('../../api/pedidosEntrantes', async () => {
  const real = await vi.importActual<typeof api>('../../api/pedidosEntrantes');
  return { ...real, aceptarEntrante: vi.fn(), rechazarEntrante: vi.fn() };
});
const comanda = vi.hoisted(() => vi.fn());
const montajes = vi.hoisted(() => vi.fn());
vi.mock('../tickets/AutoPrintTicket', async () => {
  const { useEffect } = await vi.importActual<typeof import('react')>('react');
  return {
    // Cuenta montajes: uno nuevo olvida lo ya impreso y saca otra comanda.
    KitchenTicket: (p: { order: unknown }) => {
      useEffect(() => { montajes(); }, []);
      comanda(p.order);
      return null;
    },
  };
});
vi.mock('../../api/pos', () => ({ posApi: { order: vi.fn(async (id: number) => ({ id, number: 14 })) } }));

const linea = (name: string, extra: Partial<api.RenglonEntrante> = {}): api.RenglonEntrante =>
  ({ name, quantity: '1', unitPrice: '50.00', matched: true, options: [], ...extra });
const pedido = (id: number, extra: Partial<PedidoEntrante> = {}): PedidoEntrante => ({
  source: 'platform', id, sourceName: 'Uber Eats', displayId: `F${id}`, customerName: `Cliente ${id}`, customerPhone: '',
  address: '', serviceType: 'domicilio', total: '312.00', placedAt: new Date().toISOString(),
  decideBefore: new Date(Date.now() + 8.5 * 60000).toISOString(),
  lines: [linea('Taro grande', { quantity: '2', options: [linea('Perlas')] }), linea('Crepa de Nutella'), linea('Matcha mediano')],
  ...extra,
});

function montar(pedidos: PedidoEntrante[]) {
  vi.mocked(hook.useEntrantes).mockReturnValue({ pedidos, refrescar: vi.fn() });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const arbol = () => (
    <QueryClientProvider client={qc}>
      <Provider><ColumnaPorAceptar /><Toaster /></Provider>
    </QueryClientProvider>
  );
  const r = render(arbol());
  return { ...r, con: (otros: PedidoEntrante[]) => {
    vi.mocked(hook.useEntrantes).mockReturnValue({ pedidos: otros, refrescar: vi.fn() });
    r.rerender(arbol());
  } };
}
const tarjeta = (nombre: string) => screen.getByText(nombre).closest('[data-entrante]') as HTMLElement;

describe('ColumnaPorAceptar (M-PorAceptar, B2)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useColumnaPorAceptar.setState({ visible: false });
  });

  it('sin pendientes no se pinta ni cuenta como visible', () => {
    montar([]);
    expect(screen.queryByText('Por aceptar')).not.toBeInTheDocument();
    expect(useColumnaPorAceptar.getState().visible).toBe(false);
  });

  it('con pendientes avisa que está a la vista (la franja se quita en Pedidos)', () => {
    montar([pedido(1)]);
    expect(screen.getByText('Por aceptar')).toBeInTheDocument();
    expect(useColumnaPorAceptar.getState().visible).toBe(true);
  });

  // EL MÁS URGENTE ABIERTO, LOS DEMÁS PLEGADOS (FR-028).
  it('el primero va completo y los demás plegados con su total, tiempo y botones', async () => {
    montar([pedido(1), pedido(2)]);
    const primero = tarjeta('Cliente 1');
    expect(within(primero).getByText('Taro grande')).toBeInTheDocument();
    expect(within(primero).getByText('Perlas')).toBeInTheDocument();
    const segundo = tarjeta('Cliente 2');
    expect(within(segundo).queryByText('Taro grande')).not.toBeInTheDocument();
    expect(within(segundo).getByText('$312')).toBeInTheDocument();
    expect(within(segundo).getByText(/quedan 8 min/)).toBeInTheDocument();
    expect(within(segundo).getByRole('button', { name: 'Aceptar' })).toBeInTheDocument();
    await userEvent.click(within(segundo).getByRole('button', { name: /4 productos/ }));
    expect(within(segundo).getByText('Taro grande')).toBeInTheDocument();
  });

  it('plegado avisa si algo no está en el menú', () => {
    montar([pedido(1), pedido(2, { lines: [linea('Waffle de temporada', { matched: false })] })]);
    expect(within(tarjeta('Cliente 2')).getByRole('button', { name: /1 producto · 1 no está en el menú/ })).toBeInTheDocument();
  });

  // Hasta 3 renglones abiertos: con más, los botones se iban de los primeros 300 px a 1024×600.
  it('abierto muestra hasta 3 renglones y «+N más»', async () => {
    const muchos = ['A', 'B', 'C', 'D', 'E'].map((n) => linea(`Producto ${n}`));
    montar([pedido(1, { lines: muchos })]);
    const t = tarjeta('Cliente 1');
    expect(within(t).getByText('Producto C')).toBeInTheDocument();
    expect(within(t).queryByText('Producto D')).not.toBeInTheDocument();
    await userEvent.click(within(t).getByRole('button', { name: '+2 más' }));
    expect(within(t).getByText('Producto E')).toBeInTheDocument();
  });

  // ORIGEN SIN PLAZO (el pedido web): hace cuánto llegó, y sus datos de contacto.
  it('sin plazo dice hace cuánto llegó y muestra teléfono y dirección', () => {
    montar([pedido(1, { sourceName: 'Web', decideBefore: null, placedAt: new Date(Date.now() - 3 * 60000).toISOString(),
      customerPhone: '55 1234 5678', address: 'Av. Juárez 120' })]);
    const t = tarjeta('Cliente 1');
    expect(within(t).getByText(/hace 3 min/)).toBeInTheDocument();
    expect(within(t).getByText('55 1234 5678')).toBeInTheDocument();
    expect(within(t).getByText('Av. Juárez 120')).toBeInTheDocument();
    expect(within(t).getByText(/Web · F1/)).toBeInTheDocument();
  });

  it('aceptar manda a cocina e imprime la comanda en esta tableta', async () => {
    vi.mocked(api.aceptarEntrante).mockResolvedValue({ id: 900, number: 14, platformRef: 'F1' });
    montar([pedido(1)]);
    await userEvent.click(screen.getByRole('button', { name: 'Aceptar' }));
    expect(api.aceptarEntrante).toHaveBeenCalledWith(expect.objectContaining({ source: 'platform', id: 1 }));
    expect(await screen.findByText('Pedido #14 aceptado')).toBeInTheDocument();
    await waitFor(() => expect(comanda).toHaveBeenLastCalledWith(expect.objectContaining({ id: 900 })));
  });

  // Aceptar el ÚLTIMO pendiente vacía la columna. Si la comanda se volviera a montar en otro lugar
  // del árbol, olvidaría que ya salió y se imprimiría dos veces en la plancha.
  it('aceptar el último imprime una sola comanda aunque la columna se vacíe', async () => {
    vi.mocked(api.aceptarEntrante).mockResolvedValue({ id: 901, number: 15, platformRef: 'F1' });
    const r = montar([pedido(1)]);
    await userEvent.click(screen.getByRole('button', { name: 'Aceptar' }));
    await waitFor(() => expect(comanda).toHaveBeenLastCalledWith(expect.objectContaining({ id: 901 })));
    r.con([]);
    expect(screen.queryByText('Por aceptar')).not.toBeInTheDocument();
    expect(montajes).toHaveBeenCalledTimes(1);
  });

  // SIN CAJA ABIERTA EL FOLIO TODAVÍA NO EXISTE (lo pone el turno al abrir): «Pedido #0 aceptado» no
  // le dice nada a nadie. Se nombra al cliente.
  it('aceptado sin folio todavía, nombra al cliente', async () => {
    vi.mocked(api.aceptarEntrante).mockResolvedValue({ id: 902, number: 0, platformRef: 'F1' });
    montar([pedido(1)]);
    await userEvent.click(screen.getByRole('button', { name: 'Aceptar' }));
    expect(await screen.findByText('Pedido de Cliente 1 aceptado')).toBeInTheDocument();
  });

  it('la hoja de rechazo nombra al cliente, no el código', async () => {
    montar([pedido(1)]);
    await userEvent.click(screen.getByRole('button', { name: 'Rechazar' }));
    expect(within(await screen.findByRole('dialog')).getByText('Rechazar el pedido de Cliente 1')).toBeInTheDocument();
  });

  // VARIAS TABLETAS (US4): la que pierde no ve un error del sistema ni imprime nada.
  it('si otra tableta ya lo atendió, lo dice sin alarmar y no imprime', async () => {
    vi.mocked(api.aceptarEntrante).mockRejectedValue(new ApiError(409, 'ALREADY_DECIDED', 'ya', 'r'));
    montar([pedido(1)]);
    await userEvent.click(screen.getByRole('button', { name: 'Aceptar' }));
    expect(await screen.findByText('Otra tableta ya lo atendió')).toBeInTheDocument();
    expect(comanda).not.toHaveBeenCalledWith(expect.objectContaining({ id: expect.any(Number) }));
  });

  it('si la plataforma no confirma, dice qué hacer', async () => {
    vi.mocked(api.aceptarEntrante).mockRejectedValue(new ApiError(409, 'CONFLICT', 'no confirmó', 'r'));
    montar([pedido(1)]);
    await userEvent.click(screen.getByRole('button', { name: 'Aceptar' }));
    expect(await screen.findByText('No se pudo aceptar')).toBeInTheDocument();
  });

  // RECHAZAR: motivo obligatorio, de la lista, en palabras de quien opera.
  it('rechazar pide un motivo y manda su código', async () => {
    vi.mocked(api.rechazarEntrante).mockResolvedValue(undefined);
    montar([pedido(1)]);
    await userEvent.click(screen.getByRole('button', { name: 'Rechazar' }));
    const hoja = await screen.findByRole('dialog');
    const confirmar = within(hoja).getByRole('button', { name: 'Rechazar pedido' });
    expect(confirmar).toBeDisabled();
    for (const m of api.MOTIVOS_DE_RECHAZO) expect(within(hoja).getByRole('radio', { name: m.label })).toBeInTheDocument();
    await userEvent.click(within(hoja).getByRole('radio', { name: 'Se acabó un producto' }));
    await userEvent.click(confirmar);
    expect(api.rechazarEntrante).toHaveBeenCalledWith(expect.objectContaining({ id: 1 }), 'out_of_stock');
    expect(await screen.findByText('Pedido rechazado')).toBeInTheDocument();
  });

  // SIN RESPUESTA: el servidor pudo haber aceptado y la red cortó la respuesta. Decir «no se pudo»
  // e invitar a reintentar terminaba en un «otra tableta ya lo atendió» falso y sin comanda.
  it('si se pierde la respuesta, no afirma que falló y dice dónde imprimir la comanda', async () => {
    vi.mocked(api.aceptarEntrante).mockRejectedValue(new TypeError('Failed to fetch'));
    montar([pedido(1)]);
    await userEvent.click(screen.getByRole('button', { name: 'Aceptar' }));
    expect(await screen.findByText('No se supo si quedó aceptado')).toBeInTheDocument();
    expect(screen.getByText(/En preparación/)).toBeInTheDocument();
  });

  // LA HOJA DE RECHAZO NO SOBREVIVE A SU PEDIDO. Otra tableta lo decidió con la hoja abierta: al
  // llegar el siguiente, la hoja reaparecía diciendo «Rechazar» el anterior con el motivo puesto.
  it('si el pedido que se iba a rechazar ya no está, la hoja se cierra y olvida el motivo', async () => {
    const r = montar([pedido(1)]);
    await userEvent.click(screen.getByRole('button', { name: 'Rechazar' }));
    await userEvent.click(within(await screen.findByRole('dialog')).getByRole('radio', { name: 'Ya cerramos' }));
    r.con([]);
    r.con([pedido(2)]);
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    await userEvent.click(screen.getByRole('button', { name: 'Rechazar' }));
    const hoja = await screen.findByRole('dialog');
    expect(within(hoja).getByText(/Rechazar el pedido de Cliente 2/)).toBeInTheDocument();
    expect(within(hoja).getByRole('button', { name: 'Rechazar pedido' })).toBeDisabled();
  });

  // EL MÁS URGENTE SIEMPRE ABIERTO: aceptado el primero, el que sube no se queda plegado.
  it('al decidir el primero, el siguiente se abre solo', () => {
    const r = montar([pedido(1), pedido(2)]);
    expect(within(tarjeta('Cliente 2')).queryByText('Taro grande')).toBeNull();
    r.con([pedido(2)]);
    expect(within(tarjeta('Cliente 2')).getByText('Taro grande')).toBeInTheDocument();
  });

  // Oculta (pestaña «Cerrados») no pinta nada, pero la comanda en vuelo no se pierde.
  it('oculta no pinta la columna y conserva la comanda montada', () => {
    vi.mocked(hook.useEntrantes).mockReturnValue({ pedidos: [pedido(1)], refrescar: vi.fn() });
    const qc = new QueryClient();
    render(<QueryClientProvider client={qc}><Provider><ColumnaPorAceptar oculta /></Provider></QueryClientProvider>);
    expect(screen.queryByText('Por aceptar')).toBeNull();
    expect(montajes).toHaveBeenCalledTimes(1);
    expect(useColumnaPorAceptar.getState().visible).toBe(false);
  });

  it('todo lo que se toca mide al menos 44 px', () => {
    montar([pedido(1), pedido(2)]);
    for (const b of screen.getAllByRole('button')) {
      expect(b.style.minHeight || getComputedStyle(b).minHeight, b.textContent ?? '').toMatch(/^(4[4-9]|[5-9]\d)px$/);
    }
  });
});
