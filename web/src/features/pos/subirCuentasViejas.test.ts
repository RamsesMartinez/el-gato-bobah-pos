import { beforeEach, describe, expect, test, vi } from 'vitest';
import { usePosStore } from '../../stores/pos';
import { uuidv5 } from '../../utils/uuidv5';
import { reiniciarSubida, subirCuentasViejas } from './subirCuentasViejas';

const api = vi.hoisted(() => ({ importDrafts: vi.fn() }));
vi.mock('../../api/pos', () => ({ posApi: api }));
const toasts = vi.hoisted(() => ({ create: vi.fn() }));
vi.mock('../../components/ui/toaster', () => ({ toaster: toasts }));

const LLAVE = 'egb:ticket:v2';
const T1 = '11111111-1111-4111-8111-111111111111';
const T2 = '22222222-2222-4222-8222-222222222222';

function linea(lineId: string, over: Record<string, unknown> = {}) {
  return { lineId, productId: 41, name: 'Taro', unitPrice: 55, qty: 2, modifiers: [{ optionId: 7, groupId: 1, name: 'Perlas', priceDelta: 10, qty: 1 }], notes: 'sin hielo', ...over };
}
function pestaña(id: string, over: Record<string, unknown> = {}) {
  return {
    id, num: 1, folioName: 'Persa', lines: [linea('l-1')], envio: '', descuento: '', descuentoModo: 'monto',
    serviceType: 'mostrador', customerName: '', platformId: null, platformOrderRef: '', ...over,
  };
}
function sembrar(tabs: unknown[], activeId = T1) {
  localStorage.setItem(LLAVE, JSON.stringify({ state: { tabs, activeId, seq: 3 }, version: 0 }));
}

beforeEach(() => {
  localStorage.clear();
  vi.clearAllMocks();
  reiniciarSubida();
  usePosStore.setState({ selected: null });
});

describe('las cuentas de la versión anterior suben al servidor una vez (D-12)', () => {
  test('dos pestañas con productos van en una llamada con ids de pestaña y llaves estables', async () => {
    sembrar([pestaña(T1), pestaña(T2, { folioName: 'Siamés', lines: [linea('l-9', { notes: undefined })] })]);
    api.importDrafts.mockResolvedValue({ results: [
      { id: T1, outcome: 'created', draftId: T1, orderId: null },
      { id: T2, outcome: 'created', draftId: T2, orderId: null },
    ] });
    await subirCuentasViejas();
    expect(api.importDrafts).toHaveBeenCalledTimes(1);
    const cuentas = api.importDrafts.mock.calls[0][0];
    expect(cuentas.map((c: { id: string }) => c.id)).toEqual([T1, T2]);
    expect(cuentas[0]).toMatchObject({
      folioName: 'Persa',
      header: { serviceType: 'mostrador', platformId: null, customerName: null, platformOrderRef: null },
      lines: [{ opId: uuidv5('l-1', T1), productId: 41, qty: '2', modifiers: [{ optionId: 7, qty: 1 }], notes: 'sin hielo' }],
    });
    expect(cuentas[1].lines[0]).toMatchObject({ opId: uuidv5('l-9', T2), notes: '' });
  });

  test('con todas las pestañas resueltas borra la llave y abre la que estaba activa', async () => {
    sembrar([pestaña(T1)]);
    api.importDrafts.mockResolvedValue({ results: [{ id: T1, outcome: 'exists', draftId: T1, orderId: null }] });
    await subirCuentasViejas();
    expect(localStorage.getItem(LLAVE)).toBeNull();
    expect(usePosStore.getState().selected).toEqual({ kind: 'draft', id: T1 });
  });

  // Un deploy no puede tirar lo que alguien estaba capturando: si la red falla, la llave se queda
  // y la siguiente carga lo vuelve a intentar con las MISMAS llaves.
  test('si falla la red la conserva', async () => {
    sembrar([pestaña(T1)]);
    api.importDrafts.mockRejectedValue(new TypeError('Failed to fetch'));
    await subirCuentasViejas();
    expect(localStorage.getItem(LLAVE)).not.toBeNull();
  });

  test('si falta el resultado de una pestaña la conserva', async () => {
    sembrar([pestaña(T1), pestaña(T2)]);
    api.importDrafts.mockResolvedValue({ results: [{ id: T1, outcome: 'created', draftId: T1, orderId: null }] });
    await subirCuentasViejas();
    expect(localStorage.getItem(LLAVE)).not.toBeNull();
  });

  // Caso 18: la forma vieja que dejaba la pantalla en blanco. Ahora se salta con aviso y no
  // detiene a las demás.
  test('una pestaña que no se entiende se salta con aviso y no bloquea a las otras', async () => {
    sembrar([{ id: T1, lines: 'no-es-lista' }, pestaña(T2)]);
    api.importDrafts.mockResolvedValue({ results: [{ id: T2, outcome: 'created', draftId: T2, orderId: null }] });
    await subirCuentasViejas();
    expect(api.importDrafts.mock.calls[0][0].map((c: { id: string }) => c.id)).toEqual([T2]);
    expect(toasts.create).toHaveBeenCalledWith(expect.objectContaining({ title: expect.stringMatching(/no se pudo recuperar/i) }));
    expect(localStorage.getItem(LLAVE)).toBeNull();
  });

  // Absorbe cuentaGuardadaAntes: una cuenta guardada antes de que existieran el folio de
  // plataforma, el envío o el descuento sube igual, con esos campos en vacío.
  test.each(['platformOrderRef', 'envio', 'descuento', 'descuentoModo', 'customerName', 'serviceType', 'platformId', 'folioName'])(
    'una pestaña guardada sin %s sube igual', async (campo) => {
      const t = pestaña(T1) as Record<string, unknown>;
      delete t[campo];
      sembrar([t]);
      api.importDrafts.mockResolvedValue({ results: [{ id: T1, outcome: 'created', draftId: T1, orderId: null }] });
      await subirCuentasViejas();
      expect(api.importDrafts).toHaveBeenCalledTimes(1);
      expect(api.importDrafts.mock.calls[0][0][0].lines).toHaveLength(1);
    });

  // Si la pestaña ya se había mandado (la red se cayó DESPUÉS de que el servidor confirmó), no
  // queda ninguna cuenta viva: crearla mandaría la comida dos veces.
  test('una ya enviada no queda seleccionada como cuenta en captura', async () => {
    sembrar([pestaña(T1)]);
    api.importDrafts.mockResolvedValue({ results: [{ id: T1, outcome: 'already_sent', draftId: null, orderId: 88 }] });
    await subirCuentasViejas();
    expect(usePosStore.getState().selected).toEqual({ kind: 'order', id: 88 });
  });

  test('sin pestañas con productos no llama al servidor y borra la llave', async () => {
    sembrar([pestaña(T1, { lines: [] })]);
    await subirCuentasViejas();
    expect(api.importDrafts).not.toHaveBeenCalled();
    expect(localStorage.getItem(LLAVE)).toBeNull();
  });

  test('envío y descuento capturados viajan en la cabecera', async () => {
    sembrar([pestaña(T1, { serviceType: 'domicilio', envio: '35', descuento: '10', descuentoModo: 'pct' })]);
    api.importDrafts.mockResolvedValue({ results: [{ id: T1, outcome: 'created', draftId: T1, orderId: null }] });
    await subirCuentasViejas();
    expect(api.importDrafts.mock.calls[0][0][0].header).toMatchObject({ deliveryFee: '35.00', discount: { percent: '10' } });
  });

  test('corre una sola vez por carga', async () => {
    sembrar([pestaña(T1)]);
    api.importDrafts.mockRejectedValue(new TypeError('Failed to fetch'));
    await subirCuentasViejas();
    await subirCuentasViejas();
    expect(api.importDrafts).toHaveBeenCalledTimes(1);
  });

  test('más de 20 pestañas van en tandas de 20', async () => {
    const ids = Array.from({ length: 21 }, (_, i) => `00000000-0000-4000-8000-${String(i).padStart(12, '0')}`);
    sembrar(ids.map((id) => pestaña(id)), ids[0]);
    api.importDrafts.mockImplementation(async (cs: Array<{ id: string }>) => ({
      results: cs.map((c) => ({ id: c.id, outcome: 'created', draftId: c.id, orderId: null })),
    }));
    await subirCuentasViejas();
    expect(api.importDrafts.mock.calls.map((c) => c[0].length)).toEqual([20, 1]);
    expect(localStorage.getItem(LLAVE)).toBeNull();
  });
});
