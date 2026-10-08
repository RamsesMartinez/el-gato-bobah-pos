import { act, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { ApiError } from '../../api/client';
import type { DraftView } from '../../types/pos';
import { usePosStore } from '../../stores/pos';
import { reportarResultado } from './useSinConexion';
import { reiniciarCaptura, useCuenta } from './useCuenta';

const api = vi.hoisted(() => ({
  createDraft: vi.fn(),
  getDraft: vi.fn(),
  addDraftLine: vi.fn(),
  changeDraftLine: vi.fn(),
  removeDraftLine: vi.fn(),
  patchDraft: vi.fn(),
  liveAccounts: vi.fn(),
  folioNames: vi.fn(),
  order: vi.fn(),
}));
vi.mock('../../api/pos', () => ({ posApi: api }));

const toasts = vi.hoisted(() => ({ create: vi.fn() }));
vi.mock('../../components/ui/toaster', () => ({ toaster: toasts }));

function draft(over: Partial<DraftView> = {}): DraftView {
  return {
    id: 'd-1', orderId: null, folioName: 'Levkoy', status: 'capturando', headerVersion: 1,
    updatedAt: '2026-10-08T18:00:00Z', createdAt: '2026-10-08T18:00:00Z', openedBy: 'Ana',
    serviceType: 'mostrador', customerName: null, platformId: null, platformOrderRef: null,
    deliveryFee: '0.00', discount: null, lines: [], subtotal: '0.00', discountTotal: '0.00',
    total: '0.00', unavailable: [], ...over,
  };
}
function renglon(over: Partial<NonNullable<DraftView['lines']>[number]> = {}) {
  return {
    id: 'l-1', version: 1, productId: 41, productName: 'Taro', qty: '2', unitPrice: '55.00',
    modifiers: [], notes: '', lineTotal: '110.00', available: true, ...over,
  };
}

const TARO = { productId: 41, name: 'Taro', unitPrice: 55, qty: 1, modifiers: [] };

let qc: QueryClient;
function wrapper({ children }: { children: ReactNode }) {
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

beforeEach(() => {
  qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  vi.resetAllMocks();
  reiniciarCaptura();
  reportarResultado(null);
  Object.defineProperty(window.navigator, 'onLine', { configurable: true, get: () => true });
  localStorage.clear();
  usePosStore.setState({ selected: null });
  usePosStore.getState().reiniciarNueva();
  api.liveAccounts.mockResolvedValue({ items: [], outstanding: '0.00', serverTime: '' });
  api.folioNames.mockResolvedValue({ items: ['Levkoy', 'Persa'] });
});
afterEach(() => vi.useRealTimers());

describe('la cuenta nace con el primer producto (US2)', () => {
  test('tocar un producto sin cuenta la crea en el servidor con ids nuevos y la selecciona', async () => {
    api.createDraft.mockImplementation(async (b) => draft({ id: b.id, lines: [renglon({ id: b.lines[0].opId, qty: '1' })] }));
    const { result } = renderHook(() => useCuenta(), { wrapper });

    act(() => result.current.agregar(TARO));

    await waitFor(() => expect(api.createDraft).toHaveBeenCalledTimes(1));
    const body = api.createDraft.mock.calls[0][0];
    expect(body.id).toMatch(/[0-9a-f-]{36}/);
    expect(body.orderId).toBeNull();
    expect(body.lines).toEqual([{ opId: expect.stringMatching(/[0-9a-f-]{36}/), productId: 41, qty: '1', modifiers: [], notes: '' }]);
    expect(body.header).toMatchObject({ serviceType: 'mostrador', platformId: null });
    expect(usePosStore.getState().selected).toEqual({ kind: 'draft', id: body.id });
  });

  // Dos toques rápidos: el segundo no puede llegar al servidor antes de que la cuenta exista, o
  // respondería «no existe» y el producto se perdería.
  test('el segundo toque espera a que la cuenta exista y va como agregado', async () => {
    let soltar: (v: DraftView) => void = () => {};
    api.createDraft.mockImplementation((b) => new Promise<DraftView>((r) => { soltar = () => r(draft({ id: b.id })); }));
    api.addDraftLine.mockImplementation(async (id) => draft({ id }));
    const { result } = renderHook(() => useCuenta(), { wrapper });

    act(() => result.current.agregar(TARO));
    act(() => result.current.agregar(TARO));
    await waitFor(() => expect(api.createDraft).toHaveBeenCalledTimes(1));
    expect(api.addDraftLine).not.toHaveBeenCalled();
    await act(async () => soltar(draft()));
    await waitFor(() => expect(api.addDraftLine).toHaveBeenCalledTimes(1));
    expect(api.addDraftLine.mock.calls[0][0]).toBe(api.createDraft.mock.calls[0][0].id);
  });
});

describe('lo que el servidor no confirmó no se ve como guardado (D-4)', () => {
  test('el renglón se ve «guardando» al instante y los botones de enviar y cobrar se apagan', async () => {
    api.createDraft.mockImplementation(() => new Promise(() => {}));
    const { result } = renderHook(() => useCuenta(), { wrapper });
    act(() => result.current.agregar(TARO));
    expect(result.current.vista.nuevos).toEqual([expect.objectContaining({ name: 'Taro', guardando: true })]);
    expect(result.current.vista.guardando).toBe(true);
  });

  test('si falla sale de la cuenta y el aviso reintenta con el MISMO opId', async () => {
    api.createDraft.mockRejectedValueOnce(new ApiError(0, 'NETWORK', 'Sin conexión con el servidor', 'r'));
    api.createDraft.mockImplementationOnce(async (b) => draft({ id: b.id, lines: [renglon({ id: b.lines[0].opId })] }));
    const { result } = renderHook(() => useCuenta(), { wrapper });

    act(() => result.current.agregar(TARO));
    await waitFor(() => expect(toasts.create).toHaveBeenCalled());
    expect(result.current.vista.nuevos).toEqual([]);
    const aviso = toasts.create.mock.calls[0][0];
    expect(aviso.title).toMatch(/No se guardó/);
    expect(aviso.action.label).toBe('Reintentar');

    act(() => aviso.action.onClick());
    await waitFor(() => expect(api.createDraft).toHaveBeenCalledTimes(2));
    expect(api.createDraft.mock.calls[1][0].lines[0].opId).toBe(api.createDraft.mock.calls[0][0].lines[0].opId);
    expect(api.createDraft.mock.calls[1][0].id).toBe(api.createDraft.mock.calls[0][0].id);
  });

  test('sin conexión agregar no se aplica en silencio: no llama al servidor y lo dice', () => {
    Object.defineProperty(window.navigator, 'onLine', { configurable: true, get: () => false });
    const { result } = renderHook(() => useCuenta(), { wrapper });
    act(() => result.current.agregar(TARO));
    expect(api.createDraft).not.toHaveBeenCalled();
    expect(result.current.vista.nuevos).toEqual([]);
    expect(result.current.sinConexion).toBe(true);
    expect(toasts.create).toHaveBeenCalledWith(expect.objectContaining({ title: expect.stringMatching(/conexión/i) }));
  });
});

describe('otra tableta cambió el renglón (D-5)', () => {
  async function conCuenta(l = renglon()) {
    usePosStore.getState().seleccionar({ kind: 'draft', id: 'd-1' });
    api.getDraft.mockResolvedValueOnce(draft({ lines: [l] }));
    const hook = renderHook(() => useCuenta(), { wrapper });
    await waitFor(() => expect(hook.result.current.vista.nuevos).toHaveLength(1));
    return hook;
  }

  test('«−» con la cuenta cambiada recarga y avisa', async () => {
    const { result } = await conCuenta();
    api.changeDraftLine.mockRejectedValueOnce(new ApiError(409, 'DRAFT_CHANGED', 'x', 'r'));
    api.getDraft.mockResolvedValueOnce(draft({ lines: [renglon({ qty: '5', version: 3 })] }));
    act(() => result.current.menos(result.current.vista.nuevos[0]));
    await waitFor(() => expect(api.getDraft).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(toasts.create).toHaveBeenCalledWith(
      expect.objectContaining({ title: expect.stringMatching(/otra tableta/) })));
    expect(api.changeDraftLine.mock.calls[0][2]).toEqual({ expectedVersion: 1, qty: '1' });
  });

  // El reintento de un «−» que SÍ entró regresa como conflicto (la versión ya avanzó). Avisar ahí
  // le diría al operador que otra tableta tocó su cuenta cuando fue él.
  test('si el renglón ya quedó como se pidió, no avisa', async () => {
    const { result } = await conCuenta();
    api.changeDraftLine.mockRejectedValueOnce(new ApiError(409, 'DRAFT_CHANGED', 'x', 'r'));
    api.getDraft.mockResolvedValueOnce(draft({ lines: [renglon({ qty: '1', version: 2 })] }));
    act(() => result.current.menos(result.current.vista.nuevos[0]));
    await waitFor(() => expect(api.getDraft).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(result.current.vista.nuevos[0].qty).toBe(1));
    expect(toasts.create).not.toHaveBeenCalled();
  });

  test('«−» sobre el último quita el renglón con su versión', async () => {
    const { result } = await conCuenta(renglon({ qty: '1', version: 4 }));
    api.removeDraftLine.mockResolvedValueOnce(draft({ lines: [] }));
    act(() => result.current.menos(result.current.vista.nuevos[0]));
    await waitFor(() => expect(api.removeDraftLine).toHaveBeenCalledWith('d-1', 'l-1', 4));
  });

  // El «+» es un agregado, no un cambio de cantidad: dos tabletas que tocan «+» suman 2.
  test('«+» va como agregado sobre el renglón, con un opId nuevo', async () => {
    const { result } = await conCuenta();
    api.addDraftLine.mockResolvedValueOnce(draft({ lines: [renglon({ qty: '3' })] }));
    act(() => result.current.mas(result.current.vista.nuevos[0]));
    await waitFor(() => expect(api.addDraftLine).toHaveBeenCalled());
    expect(api.addDraftLine.mock.calls[0][1]).toEqual({ opId: expect.any(String), intoLineId: 'l-1', qty: '1' });
  });

  // Una cuenta que el servidor ya no tiene —otra empresa en la tableta, descartada— se olvida sin
  // aviso: abrir la tableta no puede empezar con un error.
  test('una cuenta seleccionada que el servidor no encuentra se olvida sin aviso', async () => {
    usePosStore.getState().seleccionar({ kind: 'draft', id: 'd-x' });
    api.getDraft.mockRejectedValueOnce(new ApiError(404, 'NOT_FOUND', 'x', 'r'));
    renderHook(() => useCuenta(), { wrapper });
    await waitFor(() => expect(usePosStore.getState().selected).toBeNull());
    expect(toasts.create).not.toHaveBeenCalled();
  });
});

describe('lo «Nuevo» de un pedido ya enviado', () => {
  // Otra tableta ya había empezado lo «Nuevo» de ese pedido: el servidor devuelve ESA cuenta con el
  // renglón sumado, y esta tableta la adopta. Seguir con el id propio mandaría los siguientes
  // toques a una cuenta que no existe.
  test('el id devuelto por el servidor reemplaza al propio', async () => {
    usePosStore.getState().seleccionar({ kind: 'order', id: 12 });
    api.order.mockResolvedValue({ id: 12, number: 1, folioName: 'Khao', status: 'abierta', lines: [], payments: [],
      total: '0', outstanding: '0', paid: false, deliveryPlatformId: null });
    api.createDraft.mockImplementation(async (b) => draft({ id: 'de-otra-tableta', orderId: b.orderId, folioName: null }));
    api.addDraftLine.mockImplementation(async (id) => draft({ id, orderId: 12, folioName: null }));
    const { result } = renderHook(() => useCuenta(), { wrapper });
    await waitFor(() => expect(api.order).toHaveBeenCalled());

    act(() => result.current.agregar(TARO));
    await waitFor(() => expect(api.createDraft).toHaveBeenCalledTimes(1));
    expect(api.createDraft.mock.calls[0][0].orderId).toBe(12);
    await waitFor(() => expect(result.current.vista.borradorId).toBe('de-otra-tableta'));

    act(() => result.current.agregar(TARO));
    await waitFor(() => expect(api.addDraftLine).toHaveBeenCalledTimes(1));
    expect(api.addDraftLine.mock.calls[0][0]).toBe('de-otra-tableta');
  });
});

describe('lo que pasa en otra tableta (US7)', () => {
  // US7 AS1: la otra tableta la mandó a cocina; ésta lo dice y la sigue mostrando, ya como pedido.
  test('la cuenta enviada en otra tableta avisa y pasa a ser el pedido', async () => {
    usePosStore.getState().seleccionar({ kind: 'draft', id: 'd-1' });
    api.getDraft.mockResolvedValueOnce(draft({ folioName: 'Siamés', lines: [renglon()] }));
    api.order.mockResolvedValue({ id: 30, number: 4, folioName: 'Siamés', status: 'abierta', lines: [], payments: [], total: '110', outstanding: '110', paid: false, deliveryPlatformId: null });
    const { result } = renderHook(() => useCuenta(), { wrapper });
    await waitFor(() => expect(result.current.vista.nombre).toBe('Siamés'));
    api.getDraft.mockResolvedValueOnce(draft({ folioName: 'Siamés', status: 'enviada', orderId: 30, lines: [renglon()] }));
    await act(async () => { await qc.invalidateQueries({ queryKey: ['pos', 'draft', 'd-1'] }); });
    await waitFor(() => expect(toasts.create).toHaveBeenCalledWith(expect.objectContaining({ title: 'Siamés se mandó a cocina en otra tableta' })));
    expect(usePosStore.getState().selected).toEqual({ kind: 'order', id: 30 });
  });

  test('un cobro mientras esta tableta cobra no avisa', async () => {
    usePosStore.getState().seleccionar({ kind: 'order', id: 30 });
    api.order.mockResolvedValueOnce({ id: 30, number: 4, folioName: 'Siamés', status: 'abierta', lines: [], payments: [], total: '110', outstanding: '110', paid: false, deliveryPlatformId: null });
    const { result } = renderHook(() => useCuenta({ cobrando: true }), { wrapper });
    await waitFor(() => expect(result.current.vista.nombre).toBe('Siamés'));
    api.order.mockResolvedValueOnce({ id: 30, number: 4, folioName: 'Siamés', status: 'abierta', lines: [], payments: [], total: '110', outstanding: '0', paid: true, deliveryPlatformId: null });
    await act(async () => { await qc.invalidateQueries({ queryKey: ['orders', 30] }); });
    await waitFor(() => expect(api.order).toHaveBeenCalledTimes(2));
    expect(toasts.create).not.toHaveBeenCalled();
  });
});

// EL FOLIO QUE SE TECLEA EN LA HOJA TIENE QUE LLEGAR ANTES DE MANDAR.
//
// Era el defecto más caro del folio de plataforma: la hoja escribe el folio y manda en el MISMO
// manejador, y el pedido nacía sin él —el dato es irrecuperable (Rappi conserva 3 meses, Uber 31
// días)—. Ahora el folio es un cambio de cabecera en la fila de la cuenta, y «Enviar» espera a que
// la fila se vacíe.
test('esperar() no resuelve hasta que el folio quedó guardado', async () => {
  usePosStore.getState().seleccionar({ kind: 'draft', id: 'd-1' });
  api.getDraft.mockResolvedValue(draft({ platformId: 3, lines: [renglon()] }));
  let soltar: () => void = () => {};
  api.patchDraft.mockImplementation((_id, b) => new Promise((r) => { soltar = () => r(draft({ platformId: 3, platformOrderRef: b.platformOrderRef })); }));
  const { result } = renderHook(() => useCuenta(), { wrapper });
  await waitFor(() => expect(result.current.vista.nuevos).toHaveLength(1));

  act(() => result.current.cabecera({ platformOrderRef: 'UBER-77' }));
  let listo = false;
  const espera = result.current.esperar().then(() => { listo = true; });
  await waitFor(() => expect(api.patchDraft).toHaveBeenCalledWith('d-1', { expectedHeaderVersion: 1, platformOrderRef: 'UBER-77' }));
  expect(listo, 'el envío saldría antes de guardar el folio').toBe(false);
  await act(async () => { soltar(); await espera; });
  expect(listo).toBe(true);
});
