import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import userEvent from '@testing-library/user-event';

import { Provider } from '../../components/ui/provider';
import { ApiError } from '../../api/client';
import type { LowStockConfig } from '../../api/notifications';
import { SeccionAlertas } from './SeccionAlertas';

const api = vi.hoisted(() => ({ get: vi.fn(), preview: vi.fn(), save: vi.fn(), asked: vi.fn(), list: vi.fn(), batch: vi.fn() }));
vi.mock('../../api/notifications', async (orig) => ({ ...(await orig<object>()), lowStockApi: api }));
const back = vi.hoisted(() => ({ warehouses: vi.fn() }));
vi.mock('../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));
import { useSessionStore } from '../../stores/session';

const cfg = (o: Partial<LowStockConfig> = {}): LowStockConfig => ({
  enabled: false, onMin: true, onOut: true, banner: false, minStock: null, branchMinStock: null, coverageDays: null,
  asked: true, onHand: '1200', unitCode: 'g', branches: 1, updatedAt: 'v1', ...o,
});

function montar(canConfigure = true, onSaved?: (v: string) => void) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><Provider>
    <SeccionAlertas itemType="ingrediente" id={1} name="Queso crema" kind="masa" canConfigure={canConfigure} onSaved={onSaved} />
  </Provider></QueryClientProvider>);
}

beforeEach(() => {
  back.warehouses.mockResolvedValue({ items: [{ id: 1, name: 'Local', isSales: true, isActive: true, itemsWithStock: 2 }] });
  useSessionStore.setState({ user: { id: 1, name: 'Ana', role: 'gerente', permissions: ['inventory.view', 'inventory.configure'] } as never });
  api.get.mockResolvedValue(cfg());
  api.asked.mockResolvedValue(undefined);
  api.preview.mockResolvedValue({ effect: { opens: ['stock_low'], closes: [] }, onHand: '1200', min: '2000', needsCount: false });
  api.save.mockResolvedValue({ effect: { opens: ['stock_low'], closes: [] }, onHand: '1200', min: '2000', needsCount: false });
});
afterEach(() => { cleanup(); vi.clearAllMocks(); });

describe('Alertas en la ficha', () => {
  test('el renglón dice el estado en palabras', async () => {
    api.get.mockResolvedValue(cfg({ enabled: true, minStock: '2000' }));
    montar();
    expect(await screen.findByText('Avisa al llegar a 2 kg y al acabarse')).toBeTruthy();
  });

  test('una ficha que nunca preguntó pregunta, y «No, por ahora» no vuelve', async () => {
    api.get.mockResolvedValue(cfg({ asked: false }));
    montar();
    expect(await screen.findByText('¿Quieres recibir alertas de stock bajo de Queso crema?')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'No, por ahora' }));
    await waitFor(() => expect(api.asked).toHaveBeenCalledWith(1));
  });

  // Lo vio la prueba con alguien nuevo: «Sí, configurar» cerraba la pregunta y no abría nada (la
  // hoja nacía abierta mientras la pregunta se cerraba; ver CobrarSheet en AGENTS.md), y la pregunta
  // volvía cada vez que se abría la ficha.
  test('«Sí, configurar» abre las alertas ya encendidas y la pregunta no vuelve', async () => {
    api.get.mockResolvedValue(cfg({ asked: false }));
    montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Sí, configurar' }));
    const enviar = await screen.findByRole('checkbox', { name: /Enviar alerta de stock bajo/ });
    expect((enviar as HTMLInputElement).checked).toBe(true);
    await waitFor(() => expect(api.asked).toHaveBeenCalledWith(1));
  });

  // Cada renglón con casilla se toca en 44 px, no solo la casilla de 20.
  test('las casillas de la hoja se tocan en 44 px', async () => {
    api.get.mockResolvedValue(cfg({ asked: false }));
    montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Sí, configurar' }));
    await screen.findByRole('checkbox', { name: /Enviar alerta de stock bajo/ });
    const raices = document.querySelectorAll('[data-scope="checkbox"][data-part="root"]');
    expect(raices.length).toBeGreaterThan(0);
    for (const r of raices) expect(getComputedStyle(r).minHeight).toBe('44px');
  });

  test('sin poder configurar no pregunta ni ofrece «Configurar»', async () => {
    api.get.mockResolvedValue(cfg({ asked: false }));
    montar(false);
    await screen.findByText(/Alertas:/);
    expect(screen.queryByText(/¿Quieres recibir alertas/)).toBeNull();
    expect(screen.queryByRole('button', { name: 'Configurar ›' })).toBeNull();
  });

  test('guardar muestra primero lo que pasará y manda el mínimo en la unidad base', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Configurar ›' }));
    await userEvent.click(screen.getByRole('checkbox', { name: /Enviar alerta de stock bajo/ }));
    const min = screen.getByLabelText(/Mínimo/);
    fireEvent.change(min, { target: { value: '2' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(api.preview).toHaveBeenCalledWith('ingrediente', 1, expect.objectContaining({ enabled: true, minStock: '2000' })));
    expect(api.save).not.toHaveBeenCalled();
    expect(await screen.findByText(/se enviará un aviso al llegar al mínimo al guardar/)).toBeTruthy();
    fireEvent.click(screen.getAllByRole('button', { name: 'Guardar', hidden: true }).at(-1)!);
    await waitFor(() => expect(api.save).toHaveBeenCalled());
  });

  test('un mínimo vacío viaja como «sin mínimo», nunca como 0', async () => {
    api.get.mockResolvedValue(cfg({ enabled: true, minStock: '2000' }));
    montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Configurar ›' }));
    const min = screen.getByLabelText(/Mínimo/);
    fireEvent.change(min, { target: { value: '' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(api.preview).toHaveBeenCalledWith('ingrediente', 1, expect.objectContaining({ minStock: null })));
  });

  // Sin mínimo, «llegue al mínimo» se ve deshabilitada pero seguía viajando encendida.
  test('sin mínimo, «llegue al mínimo» viaja apagada', async () => {
    api.get.mockResolvedValue(cfg({ enabled: true, minStock: '2000', onMin: true }));
    montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Configurar ›' }));
    fireEvent.change(screen.getByLabelText(/Mínimo/), { target: { value: '' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(api.preview).toHaveBeenCalledWith('ingrediente', 1, expect.objectContaining({ minStock: null, onMin: false })));
  });

  // Sin la versión, otra tableta guardaba mínimo 8 y ésta, al cambiar solo la franja, lo regresaba a 5.
  test('la vista previa y el guardado llevan la versión que se vio; si cambió, se vuelve a leer y se dice', async () => {
    api.get.mockResolvedValue(cfg({ enabled: true, minStock: '2000' }));
    montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Configurar ›' }));
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(api.preview).toHaveBeenCalledWith('ingrediente', 1, expect.objectContaining({ expectedUpdatedAt: 'v1' })));
    api.save.mockRejectedValueOnce(new ApiError(409, 'STALE', 'cambió', 'r'));
    const llamadas = api.get.mock.calls.length;
    await screen.findByText(/Se actualizará/);
    fireEvent.click(screen.getAllByRole('button', { name: 'Guardar', hidden: true }).at(-1)!);
    await waitFor(() => expect(api.save).toHaveBeenCalledWith('ingrediente', 1, expect.objectContaining({ expectedUpdatedAt: 'v1' })));
    expect(await screen.findByText(/Alguien más cambió estas alertas/)).toBeTruthy();
    await waitFor(() => expect(api.get.mock.calls.length).toBeGreaterThan(llamadas));
  });

  // La ficha decía «avisa al llegar a 3» y el aviso salía en 10: el mínimo de la sucursal gana.
  test('con mínimo de sucursal, el renglón y la confirmación dicen el mínimo que de verdad se usa', async () => {
    api.get.mockResolvedValue(cfg({ enabled: true, minStock: '3000', branchMinStock: '10000' }));
    api.preview.mockResolvedValue({ effect: { opens: ['stock_low'], closes: [] }, onHand: '1200', min: '10000', needsCount: false });
    montar();
    expect(await screen.findByText(/En esta sucursal avisa al llegar a 10 kg/)).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Configurar ›' }));
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }));
    expect(await screen.findByText(/avisa al llegar a 10 kg/)).toBeTruthy();
  });

  test('los días de cobertura van de 1 a 90 y se dice antes de la vista previa', async () => {
    api.get.mockResolvedValue(cfg({ enabled: true, minStock: '2000' }));
    montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Configurar ›' }));
    fireEvent.change(screen.getByLabelText('Días de cobertura'), { target: { value: '95' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }));
    expect(await screen.findByText(/de 1 a 90 días/)).toBeTruthy();
    fireEvent.change(screen.getByLabelText('Días de cobertura'), { target: { value: '0' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }));
    expect(await screen.findByText(/de 1 a 90 días/)).toBeTruthy();
    expect(api.preview).not.toHaveBeenCalled();
  });

  // Guardar las alertas cambia la versión de la ficha: la ficha la necesita para que su autoguardado
  // no choque con un «cambió mientras la veías» falso.
  test('al guardar avisa a la ficha la versión nueva', async () => {
    api.get.mockResolvedValue(cfg({ enabled: true, minStock: '2000' }));
    api.save.mockResolvedValue({ effect: { opens: [], closes: [] }, onHand: '1200', min: '2000', needsCount: false, updatedAt: 'v2' });
    const onSaved = vi.fn();
    montar(true, onSaved);
    fireEvent.click(await screen.findByRole('button', { name: 'Configurar ›' }));
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }));
    await screen.findByText(/Se actualizará/);
    fireEvent.click(screen.getAllByRole('button', { name: 'Guardar', hidden: true }).at(-1)!);
    await waitFor(() => expect(onSaved).toHaveBeenCalledWith('v2'));
  });
});

// VARIOS ALMACENES (spec 094): por omisión el de venta, que va sin almacén; otro, con su id, y su
// mínimo es el del almacén (branchMinStock), sin tocar el del artículo.
describe('mínimos por almacén (spec 094)', () => {
  test('con un solo almacén no pregunta de cuál y manda sin almacén', async () => {
    api.get.mockResolvedValue(cfg({ enabled: true, minStock: '2000' }));
    montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Configurar ›' }));
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(api.preview).toHaveBeenCalled());
    expect(api.preview.mock.calls[0][2]).not.toHaveProperty('warehouseId');
    expect(api.preview.mock.calls[0][2]).not.toHaveProperty('branchMinStock');
    expect(screen.queryByRole('group', { name: '¿De qué almacén?' })).toBeNull();
  });
  test('con dos se ve y se cambia el mínimo de la Bodega sin tocar el del artículo', async () => {
    back.warehouses.mockResolvedValue({ items: [
      { id: 1, name: 'Local', isSales: true, isActive: true, itemsWithStock: 2 },
      { id: 2, name: 'Bodega', isSales: false, isActive: true, itemsWithStock: 1 },
    ] });
    api.get.mockImplementation((_t: string, _id: number, w?: number) => Promise.resolve(w === 2
      ? cfg({ enabled: true, minStock: '3000', branchMinStock: '5000', warehouseId: 2, warehouseName: 'Bodega' } as Partial<LowStockConfig>)
      : cfg({ enabled: true, minStock: '3000', warehouseId: 1, warehouseName: 'Local' } as Partial<LowStockConfig>)));
    montar();
    const grupo = await screen.findByRole('group', { name: '¿De qué almacén?' });
    expect(screen.getByRole('button', { name: 'Local · de venta' })).toHaveAttribute('aria-pressed', 'true');
    fireEvent.click(screen.getByRole('button', { name: 'Bodega' }));
    expect(await screen.findByText('Mínimo en Bodega: 5 kg · avisa al llegar')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Configurar ›' }));
    const min = screen.getByLabelText(/Mínimo/) as HTMLInputElement;
    expect(min.value).toBe('5');
    fireEvent.change(min, { target: { value: '8' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(api.preview).toHaveBeenCalledWith('ingrediente', 1,
      expect.objectContaining({ warehouseId: 2, branchMinStock: '8000', minStock: '3000' })));
    expect(grupo).toBeInTheDocument();
  });

});

// Revisión y prueba con alguien nuevo (spec 094): las alertas son del artículo y el mínimo de cada
// almacén. Guardar en Bodega mandaba onMin apagado (calculado con el mínimo de Bodega) y el Local
// dejaba de avisar; el renglón de Bodega prometía el aviso del mínimo del artículo; la confirmación
// decía «10 l».
describe('Alertas con otro almacén', () => {
  const dos = () => back.warehouses.mockResolvedValue({ items: [
    { id: 1, name: 'Local', isSales: true, isActive: true, itemsWithStock: 2 },
    { id: 2, name: 'Bodega', isSales: false, isActive: true, itemsWithStock: 1 },
  ] });
  const articulo = { enabled: true, onMin: true, onOut: true, banner: true, minStock: '3000' };

  test('guardar el mínimo de Bodega sin mínimo no apaga las alertas del artículo', async () => {
    dos();
    api.get.mockImplementation((_t: string, _id: number, w?: number) => Promise.resolve(w === 2
      ? cfg({ ...articulo, branchMinStock: null, warehouseId: 2, warehouseName: 'Bodega' } as Partial<LowStockConfig>)
      : cfg({ ...articulo, warehouseId: 1, warehouseName: 'Local' } as Partial<LowStockConfig>)));
    montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Bodega' }));
    await screen.findByText(/Bodega sin mínimo/);
    fireEvent.click(await screen.findByRole('button', { name: 'Configurar ›' }));
    // La hoja de Bodega solo trae su mínimo: los interruptores son del artículo.
    expect(screen.queryByText('Enviar alerta de stock bajo')).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(api.preview).toHaveBeenCalled());
    expect(api.preview.mock.calls[0][2]).toEqual(expect.objectContaining({
      enabled: true, onMin: true, onOut: true, banner: true, minStock: '3000', branchMinStock: null, warehouseId: 2 }));
  });

  test('el renglón de Bodega habla del mínimo de Bodega, no del del artículo', async () => {
    dos();
    api.get.mockImplementation((_t: string, _id: number, w?: number) => Promise.resolve(w === 2
      ? cfg({ ...articulo, branchMinStock: null, onHand: '20000', warehouseId: 2, warehouseName: 'Bodega' } as Partial<LowStockConfig>)
      : cfg({ ...articulo, warehouseId: 1, warehouseName: 'Local' } as Partial<LowStockConfig>)));
    montar();
    expect(await screen.findByText('¿De qué almacén?')).toBeTruthy();
    fireEvent.click(await screen.findByRole('button', { name: 'Bodega' }));
    expect(await screen.findByText(/Bodega sin mínimo/)).toBeTruthy();
    expect(screen.queryByText(/Avisa al llegar a 3 kg/)).toBeNull();
    expect(screen.getByText(/Hay 20 kg en Bodega/)).toBeTruthy();
  });

  test('la confirmación nombra el almacén y escribe la unidad como en el resto de la pantalla', async () => {
    dos();
    api.get.mockImplementation((_t: string, _id: number, w?: number) => Promise.resolve(w === 2
      ? cfg({ ...articulo, minStock: '3000', branchMinStock: null, unitCode: 'ml', warehouseId: 2, warehouseName: 'Bodega' } as Partial<LowStockConfig>)
      : cfg({ ...articulo, unitCode: 'ml', warehouseId: 1, warehouseName: 'Local' } as Partial<LowStockConfig>)));
    api.preview.mockResolvedValue({ effect: { opens: [], closes: [] }, onHand: '20000', min: '10000', needsCount: false });
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<QueryClientProvider client={qc}><Provider>
      <SeccionAlertas itemType="ingrediente" id={1} name="Aceite" kind="volumen" canConfigure />
    </Provider></QueryClientProvider>);
    fireEvent.click(await screen.findByRole('button', { name: 'Bodega' }));
    await screen.findByText(/Bodega sin mínimo/);
    fireEvent.click(await screen.findByRole('button', { name: 'Configurar ›' }));
    fireEvent.change(screen.getByLabelText(/Mínimo/), { target: { value: '10' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }));
    expect(await screen.findByText((_, el) => el?.tagName === 'P' && /^Mínimo de Aceite en Bodega: 10 L\.$/.test(el.textContent ?? ''))).toBeTruthy();
  });

  test('en el de venta la confirmación tampoco baja la unidad a minúscula', async () => {
    api.get.mockResolvedValue(cfg({ enabled: true, onMin: true, minStock: null, unitCode: 'ml' }));
    api.preview.mockResolvedValue({ effect: { opens: [], closes: [] }, onHand: '20000', min: '10000', needsCount: false });
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<QueryClientProvider client={qc}><Provider>
      <SeccionAlertas itemType="ingrediente" id={1} name="Aceite" kind="volumen" canConfigure />
    </Provider></QueryClientProvider>);
    fireEvent.click(await screen.findByRole('button', { name: 'Configurar ›' }));
    fireEvent.change(screen.getByLabelText(/Mínimo/), { target: { value: '10' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }));
    expect(await screen.findByText((_, el) => el?.tagName === 'P' && /avisa al llegar a 10 L/.test(el.textContent ?? ''))).toBeTruthy();
  });

  // Lo vio la prueba con alguien nuevo (cierre del #65): con las alertas del artículo apagadas, la
  // ficha decía «Alertas en Bodega: Apagadas» con un mínimo puesto, y la hoja mandaba a «Local» sin
  // que se entendiera por qué. Las alertas son del artículo; el mínimo, de cada almacén.
  test('con las alertas del artículo apagadas, Bodega dice su mínimo y que no avisa', async () => {
    dos();
    api.get.mockImplementation((_t: string, _id: number, w?: number) => Promise.resolve(w === 2
      ? cfg({ enabled: false, minStock: null, branchMinStock: '5', unitCode: 'pieza', warehouseId: 2, warehouseName: 'Bodega' } as Partial<LowStockConfig>)
      : cfg({ enabled: false, unitCode: 'pieza', warehouseId: 1, warehouseName: 'Local' } as Partial<LowStockConfig>)));
    montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Bodega' }));
    expect(await screen.findByText('Mínimo en Bodega: 5 piezas · no avisa: las alertas del artículo están apagadas')).toBeTruthy();
    expect(screen.queryByText(/Apagadas/)).toBeNull();
  });

  test('desde Bodega se encienden las alertas del artículo sin almacén y sin tocar el mínimo de Local', async () => {
    dos();
    api.get.mockImplementation((_t: string, _id: number, w?: number) => Promise.resolve(w === 2
      ? cfg({ enabled: false, onMin: false, minStock: '3000', branchMinStock: '5000', warehouseId: 2, warehouseName: 'Bodega' } as Partial<LowStockConfig>)
      : cfg({ enabled: false, onMin: false, minStock: '3000', warehouseId: 1, warehouseName: 'Local' } as Partial<LowStockConfig>)));
    api.preview.mockResolvedValue({ effect: { opens: [], closes: [] }, onHand: '1200', min: '3000', needsCount: false });
    api.save.mockResolvedValue({ effect: { opens: [], closes: [] }, onHand: '1200', min: '3000', needsCount: false, updatedAt: 'v2' });
    montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Bodega' }));
    await screen.findByText(/Mínimo en Bodega: 5 kg · no avisa/);
    fireEvent.click(await screen.findByRole('button', { name: 'Configurar ›' }));
    await userEvent.click(await screen.findByRole('checkbox', { name: /Alertas de este artículo, en todos los almacenes/ }));
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(api.preview).toHaveBeenCalledTimes(2));
    // Dos vistas previas: la del artículo (el de venta, sin almacén) y la de Bodega con lo propuesto.
    const vista = api.preview.mock.calls.map((c) => c[2]).find((b) => !('warehouseId' in b));
    expect(vista).toEqual(expect.objectContaining({ enabled: true, onMin: true, minStock: '3000' }));
    expect(vista).not.toHaveProperty('branchMinStock');
    expect(await screen.findByText(/Se encenderán las alertas de/)).toBeTruthy();
    fireEvent.click(screen.getAllByRole('button', { name: 'Guardar', hidden: true }).at(-1)!);
    await waitFor(() => expect(api.save).toHaveBeenCalled());
    const guardado = api.save.mock.calls[0][2];
    expect(guardado).toEqual(expect.objectContaining({ enabled: true, minStock: '3000', expectedUpdatedAt: 'v1' }));
    expect(guardado).not.toHaveProperty('warehouseId');
    expect(guardado).not.toHaveProperty('branchMinStock');
    // El mínimo de Bodega no cambió: no hay segundo guardado.
    await waitFor(() => expect(screen.queryByText(/Se encenderán las alertas de/)).toBeNull());
    expect(api.save).toHaveBeenCalledTimes(1);
  });

  test('encender las alertas y cambiar el mínimo de Bodega guarda los dos, el segundo con la versión nueva', async () => {
    dos();
    api.get.mockImplementation((_t: string, _id: number, w?: number) => Promise.resolve(w === 2
      ? cfg({ enabled: false, minStock: '3000', branchMinStock: '5000', warehouseId: 2, warehouseName: 'Bodega' } as Partial<LowStockConfig>)
      : cfg({ enabled: false, minStock: '3000', warehouseId: 1, warehouseName: 'Local' } as Partial<LowStockConfig>)));
    api.preview.mockResolvedValue({ effect: { opens: [], closes: [] }, onHand: '1200', min: '3000', needsCount: false });
    api.save.mockResolvedValue({ effect: { opens: [], closes: [] }, onHand: '1200', min: '3000', needsCount: false, updatedAt: 'v2' });
    montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Bodega' }));
    await screen.findByText(/Mínimo en Bodega: 5 kg · no avisa/);
    fireEvent.click(await screen.findByRole('button', { name: 'Configurar ›' }));
    await userEvent.click(await screen.findByRole('checkbox', { name: /Alertas de este artículo, en todos los almacenes/ }));
    fireEvent.change(screen.getByLabelText(/Mínimo en Bodega/), { target: { value: '8' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }));
    await screen.findByText(/Se encenderán las alertas de/);
    fireEvent.click(screen.getAllByRole('button', { name: 'Guardar', hidden: true }).at(-1)!);
    await waitFor(() => expect(api.save).toHaveBeenCalledTimes(2));
    expect(api.save.mock.calls[1][2]).toEqual(expect.objectContaining({ warehouseId: 2, branchMinStock: '8000', expectedUpdatedAt: 'v2' }));
  });

  // Lo vio la prueba en pantalla (cierre del #65): la confirmación decía «no se envía nada» (la vista
  // previa era la de Local) y al guardar se abría el aviso de Bodega, con 800 ml contra su mínimo de 2 L.
  test('encender desde Bodega anuncia el aviso que se abre en Bodega', async () => {
    dos();
    api.get.mockImplementation((_t: string, _id: number, w?: number) => Promise.resolve(w === 2
      ? cfg({ enabled: false, minStock: '3000', branchMinStock: '2000', onHand: '800', unitCode: 'ml', warehouseId: 2, warehouseName: 'Bodega' } as Partial<LowStockConfig>)
      : cfg({ enabled: false, minStock: '3000', onHand: '4600', unitCode: 'ml', warehouseId: 1, warehouseName: 'Local' } as Partial<LowStockConfig>)));
    api.preview.mockImplementation((_t: string, _id: number, b: { warehouseId?: number }) => Promise.resolve(b.warehouseId === 2
      ? { effect: { opens: ['stock_low'], closes: [] }, onHand: '800', min: '2000', needsCount: false }
      : { effect: { opens: [], closes: [] }, onHand: '4600', min: '3000', needsCount: false }));
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<QueryClientProvider client={qc}><Provider>
      <SeccionAlertas itemType="ingrediente" id={1} name="Aceite" kind="volumen" canConfigure />
    </Provider></QueryClientProvider>);
    fireEvent.click(await screen.findByRole('button', { name: 'Bodega' }));
    await screen.findByText(/Mínimo en Bodega: 2 L · no avisa/);
    fireEvent.click(await screen.findByRole('button', { name: 'Configurar ›' }));
    await userEvent.click(await screen.findByRole('checkbox', { name: /Alertas de este artículo, en todos los almacenes/ }));
    fireEvent.click(screen.getByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(api.preview).toHaveBeenCalledTimes(2));
    expect(api.preview.mock.calls.map((c) => c[2]).find((b) => b.warehouseId === 2))
      .toEqual(expect.objectContaining({ enabled: true, onMin: true, branchMinStock: '2000' }));
    expect(await screen.findByText('Hoy quedan 800 ml en Bodega, debajo de su mínimo de 2 L: al guardar se abre un aviso.')).toBeTruthy();
    expect(screen.queryByText(/no se envía nada/)).toBeNull();
  });
});
