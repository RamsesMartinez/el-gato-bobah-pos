import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../../components/ui/provider';
import type { StockCount } from '../../api/backoffice';
import { StockCountTab } from './StockCountTab';

const back = vi.hoisted(() => ({
  currentCount: vi.fn(),
  openCount: vi.fn(),
  captureCount: vi.fn(),
  uncaptureCount: vi.fn(),
  previewCount: vi.fn(),
  closeCount: vi.fn(),
  discardCount: vi.fn(),
  setCountPlace: vi.fn(),
}));
vi.mock('../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));
vi.mock('../../api/pos', () => ({ posApi: { businessSettings: vi.fn(() => Promise.resolve({ timezone: 'America/Mexico_City' })) } }));

// Doble del Picker sin hoja animada (ver ExpenseDialog.test.tsx).
vi.mock('../../components/Picker', async () => {
  const { useState } = await import('react');
  type Opt = { value: string; label: string };
  function Picker({ value, options, onChange, placeholder, onCreate }: {
    value: string; options: Opt[]; onChange: (v: string) => void; placeholder?: string;
    onCreate?: (name: string) => Promise<Opt | null>;
  }) {
    const [open, setOpen] = useState(false);
    const [q, setQ] = useState('');
    const selected = options.find((o) => o.value === value);
    return (
      <div>
        <button type="button" onClick={() => setOpen(true)}>{selected ? selected.label : placeholder}</button>
        {open && (
          <div>
            {onCreate && <input placeholder="Buscar…" value={q} onChange={(e) => setQ(e.target.value)} />}
            {options.map((o) => <button key={o.value} type="button" onClick={() => { onChange(o.value); setOpen(false); }}>{o.label}</button>)}
            {onCreate && q && <button type="button" onClick={async () => { const o = await onCreate(q); if (o) onChange(o.value); setOpen(false); }}>Crear «{q}»</button>}
          </div>
        )}
      </div>
    );
  }
  return { Picker };
});

const conteo = (): StockCount => ({
  id: 5, openedAt: '2026-10-14T13:40:00Z',
  items: [
    { ingredientId: 1, name: 'Jarabe de vainilla', place: 'Barra', baseUnitCode: 'ml', presentations: [{ name: 'Botella', qtyInBase: '750' }], countedQty: null },
    { ingredientId: 2, name: 'Azúcar', place: 'Bodega', baseUnitCode: 'g', presentations: [{ name: 'Bulto', qtyInBase: '25000' }], countedQty: null },
    { ingredientId: 3, name: 'Limón', place: null, baseUnitCode: 'pieza', presentations: [], countedQty: null },
  ],
});

beforeEach(() => {
  try { localStorage.clear(); } catch { /* sin almacenamiento */ }
  back.currentCount.mockResolvedValue(conteo());
  back.openCount.mockResolvedValue(conteo());
  back.captureCount.mockResolvedValue(undefined);
  back.discardCount.mockResolvedValue(undefined);
  back.setCountPlace.mockResolvedValue(undefined);
  back.previewCount.mockResolvedValue({
    adjusted: [
      { ingredientId: 2, name: 'Azúcar', baseUnitCode: 'g', system: '-1200', counted: '3000', delta: '4200' },
      { ingredientId: 1, name: 'Jarabe de vainilla', baseUnitCode: 'ml', system: '2800', counted: '2750', delta: '-50' },
    ],
    notCounted: [{ ingredientId: 3, name: 'Limón' }],
  });
  back.closeCount.mockImplementation(() => back.previewCount());
});
afterEach(() => {
  cleanup();
  document.body.innerHTML = '';
  for (const a of ['style', 'data-inert', 'data-scroll-lock', 'aria-hidden']) document.body.removeAttribute(a);
  vi.clearAllMocks();
});

function montar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><Provider><StockCountTab /></Provider></QueryClientProvider>);
}
const click = (name: string | RegExp) => fireEvent.click(screen.getByRole('button', { name }));
const tecla = (k: string) => fireEvent.click(screen.getByRole('button', { name: k }));

describe('Conteo (spec 038, maqueta «Almacén › Conteo»)', () => {
  test('sin conteo abierto ofrece empezar uno', async () => {
    back.currentCount.mockResolvedValue(null);
    montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Empezar conteo' }));
    await waitFor(() => expect(back.openCount).toHaveBeenCalled());
    expect(await screen.findByTestId('avance')).toHaveTextContent('0 de 3 contados');
  });

  test('la lista va por lugar, con empaques, «Sin contar» y no «0», y sin controles nativos', async () => {
    montar();
    const rows = await screen.findAllByTestId('renglon-conteo');
    expect(rows.map((r) => r.textContent)).toEqual([
      expect.stringContaining('Jarabe de vainilla'), expect.stringContaining('Azúcar'), expect.stringContaining('Limón'),
    ]);
    expect(screen.getByText('Sin lugar')).toBeInTheDocument();
    expect(rows[1]).toHaveTextContent('Bulto (25 kg)');
    expect(rows[1]).toHaveTextContent('Sin contar');
    expect(document.querySelector('select')).toBeNull();
  });

  test('contar con el teclado en pantalla: empaques más lo abierto, en la unidad base', async () => {
    montar();
    fireEvent.click(within((await screen.findAllByTestId('renglon-conteo'))[0]).getByRole('button', { name: 'Contar' }));
    const botellas = await screen.findByLabelText('Botella de Jarabe de vainilla');
    // El teclado del sistema no se abre; las casillas empiezan en «—», no en 0.
    expect(botellas).toHaveAttribute('inputmode', 'none');
    expect(botellas).toHaveAttribute('placeholder', '—');
    expect(screen.getByRole('button', { name: 'Guardar' })).toBeDisabled();
    // En empaques no hay punto: se cuentan enteros.
    expect(screen.queryByRole('button', { name: 'punto' })).toBeNull();
    tecla('3');
    click(/Siguiente casilla/);
    tecla('punto'); tecla('5');
    expect(screen.getByText('= 2.75 L')).toBeInTheDocument();
    click('Guardar');
    await waitFor(() => expect(back.captureCount).toHaveBeenCalledWith(5, 1, '2750'));
    expect(await screen.findByText('✓ 2.75 L')).toBeInTheDocument();
    expect(screen.getByTestId('avance')).toHaveTextContent('1 de 3 contados');
  });

  test('uno por uno: saltar no guarda; guardar y seguir pasa al siguiente; la vista se recuerda', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Uno por uno' }));
    expect(screen.getByText('Jarabe de vainilla')).toBeInTheDocument();
    click('Saltar (no contar hoy)');
    expect(screen.getByText('Azúcar')).toBeInTheDocument();
    expect(back.captureCount).not.toHaveBeenCalled();
    tecla('1');
    click('Guardar y seguir ›');
    await waitFor(() => expect(back.captureCount).toHaveBeenCalledWith(5, 2, '25000'));
    expect(await screen.findByText('Limón')).toBeInTheDocument();
    expect(localStorage.getItem('egb:conteo:vista')).toBe('uno');
  });

  test('el cierre nombra lo no contado, dice sobraban/faltaban y marca lo grande, sin negativos a secas', async () => {
    back.currentCount.mockResolvedValue({ ...conteo(), items: conteo().items.map((i) => (i.ingredientId === 3 ? i : { ...i, countedQty: '1' })) });
    montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Cerrar conteo' }));
    const cierre = await screen.findByTestId('cierre');
    expect(cierre).toHaveTextContent('Sin contar (1): se quedan como están');
    expect(cierre).toHaveTextContent('Limón');
    expect(cierre).toHaveTextContent('Azúcar: contaste 3 kg');
    expect(cierre).toHaveTextContent('sobraban 4.2 kg');
    expect(cierre).toHaveTextContent('faltaban 50 ml');
    expect(cierre).toHaveTextContent('Revisa esta diferencia grande');
    expect(cierre.textContent).not.toMatch(/-\s?1[.,]?2/);
    click('Sí, cerrar y corregir lo guardado');
    await waitFor(() => expect(back.closeCount).toHaveBeenCalledWith(5));
    expect(await screen.findByTestId('conteo-cerrado')).toHaveTextContent('Conteo cerrado: 2 insumos corregidos');
  });

  test('no se puede cerrar sin nada contado, y descartar pide confirmación', async () => {
    montar();
    expect(await screen.findByRole('button', { name: 'Cerrar conteo' })).toBeDisabled();
    click('Descartar conteo');
    expect(back.discardCount).not.toHaveBeenCalled();
    click('Sí, descartar');
    await waitFor(() => expect(back.discardCount).toHaveBeenCalledWith(5));
  });
});

// Prueba con usuarios sin contexto (T065): «Cerrar conteo» apagado sin decir por qué.
test('dice por qué no se puede cerrar todavía', async () => {
  montar();
  expect(await screen.findByRole('button', { name: 'Cerrar conteo' })).toBeDisabled();
  expect(screen.getByText('Cuenta al menos uno para cerrar')).toBeInTheDocument();
});

// Decisión del dueño tras la prueba sin contexto (FR-015f): el lugar se pone desde el conteo, con los
// lugares que la empresa ya usa y «Otro…»; nadie lo encontró en la hoja «Cómo llega».
describe('lugar desde el conteo (FR-015f)', () => {
  test('ofrece los lugares de la empresa y guarda el elegido', async () => {
    montar();
    const fila = (await screen.findAllByTestId('renglon-conteo'))[2]; // Limón, sin lugar
    fireEvent.click(within(fila).getByRole('button', { name: 'Lugar' }));
    expect(within(fila).getByRole('button', { name: 'Barra' })).toBeInTheDocument();
    expect(within(fila).getByRole('button', { name: 'Bodega' })).toBeInTheDocument();
    fireEvent.click(within(fila).getByRole('button', { name: 'Barra' }));
    await waitFor(() => expect(back.setCountPlace).toHaveBeenCalledWith(3, 'Barra'));
    await waitFor(() => expect(back.currentCount.mock.calls.length).toBeGreaterThan(1));
  });

  test('un lugar nuevo se crea escribiéndolo', async () => {
    montar();
    const fila = (await screen.findAllByTestId('renglon-conteo'))[2];
    fireEvent.click(within(fila).getByRole('button', { name: 'Lugar' }));
    fireEvent.change(within(fila).getByPlaceholderText('Buscar…'), { target: { value: 'Refri' } });
    fireEvent.click(within(fila).getByRole('button', { name: /Crear «Refri»/ }));
    await waitFor(() => expect(back.setCountPlace).toHaveBeenCalledWith(3, 'Refri'));
  });
});

// Poner un lugar reordena el conteo; en «Uno por uno» la tarjeta sigue en el mismo insumo y no salta
// al que quedó en su posición.
test('uno por uno sigue en el mismo insumo cuando el lugar lo reordena', async () => {
  montar();
  fireEvent.click(await screen.findByRole('button', { name: 'Uno por uno' }));
  expect(screen.getByText('Jarabe de vainilla')).toBeInTheDocument();
  const reordered = conteo();
  reordered.items = [reordered.items[1], reordered.items[2], { ...reordered.items[0], place: 'Zótano' }];
  back.currentCount.mockResolvedValue(reordered);
  fireEvent.click(screen.getByRole('button', { name: 'Barra' }));
  fireEvent.change(screen.getByPlaceholderText('Buscar…'), { target: { value: 'Zótano' } });
  fireEvent.click(screen.getByRole('button', { name: /Crear «Zótano»/ }));
  await waitFor(() => expect(back.setCountPlace).toHaveBeenCalledWith(1, 'Zótano'));
  await waitFor(() => expect(back.currentCount.mock.calls.length).toBeGreaterThan(1));
  expect(await screen.findByText('Jarabe de vainilla')).toBeInTheDocument();
  expect(screen.queryByText('Azúcar')).toBeNull();
});

// SC-005: un conteo a medias sobrevive a recargar y a cambiar de tableta. Lo contado vive en el
// servidor, no en la memoria de la pantalla: al volver a montar sin nada local, las cifras vuelven.
test('al recargar o abrir en otra tableta, lo ya contado sigue ahí', async () => {
  montar();
  fireEvent.click(within((await screen.findAllByTestId('renglon-conteo'))[0]).getByRole('button', { name: 'Contar' }));
  await screen.findByLabelText('Botella de Jarabe de vainilla');
  tecla('3');
  click(/Siguiente casilla/);
  tecla('punto'); tecla('5');
  click('Guardar');
  await waitFor(() => expect(back.captureCount).toHaveBeenCalledWith(5, 1, '2750'));

  // Otra tableta: sin caché de consultas ni almacenamiento local; solo lo que el servidor guardó.
  const guardado = conteo();
  guardado.items[0].countedQty = '2750';
  back.currentCount.mockResolvedValue(guardado);
  cleanup();
  try { localStorage.clear(); } catch { /* sin almacenamiento */ }
  montar();
  expect(await screen.findByText('✓ 2.75 L')).toBeInTheDocument();
  expect(screen.getByTestId('avance')).toHaveTextContent('1 de 3 contados');
});
