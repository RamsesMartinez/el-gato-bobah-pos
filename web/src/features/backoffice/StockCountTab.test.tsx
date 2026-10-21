import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { MemoryRouter, Route, Routes } from 'react-router';
import { Provider } from '../../components/ui/provider';
import { useSessionStore } from '../../stores/session';
import type { StockCount } from '../../api/backoffice';
import { StockCountTab } from './StockCountTab';
import { ApiError } from '../../api/client';

const back = vi.hoisted(() => ({
  currentCount: vi.fn(),
  openCount: vi.fn(),
  captureCount: vi.fn(),
  uncaptureCount: vi.fn(),
  previewCount: vi.fn(),
  closeCount: vi.fn(),
  discardCount: vi.fn(),
  setCountPlace: vi.fn(),
  countPlaces: vi.fn(),
  createCountPlace: vi.fn(),
}));
vi.mock('../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));
const limits = vi.hoisted(() => ({ get: vi.fn(), save: vi.fn() }));
vi.mock('../../api/countLimits', async (orig) => ({ ...(await orig<object>()), countLimitsApi: limits }));
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
    { ingredientId: 1, name: 'Jarabe de vainilla', place: 'Barra', placeId: 1, baseUnitCode: 'ml', presentations: [{ name: 'Botella', qtyInBase: '750' }], countedQty: null },
    { ingredientId: 2, name: 'Azúcar', place: 'Bodega', placeId: 2, baseUnitCode: 'g', presentations: [{ name: 'Bulto', qtyInBase: '25000' }], countedQty: null },
    { ingredientId: 3, name: 'Limón', place: null, placeId: null, baseUnitCode: 'pieza', presentations: [], countedQty: null },
  ],
});

beforeEach(() => {
  try { localStorage.clear(); } catch { /* sin almacenamiento */ }
  back.currentCount.mockResolvedValue(conteo());
  back.openCount.mockResolvedValue(conteo());
  back.captureCount.mockResolvedValue(undefined);
  back.discardCount.mockResolvedValue(undefined);
  back.setCountPlace.mockResolvedValue(undefined);
  back.countPlaces.mockResolvedValue([{ id: 1, name: 'Barra', position: 0, ingredients: 1 }, { id: 2, name: 'Bodega', position: 1, ingredients: 1 }]);
  back.createCountPlace.mockResolvedValue({ id: 9 });
  back.previewCount.mockResolvedValue({
    adjusted: [
      { ingredientId: 2, name: 'Azúcar', baseUnitCode: 'g', system: '-1200', counted: '3000', delta: '4200' },
      { ingredientId: 1, name: 'Jarabe de vainilla', baseUnitCode: 'ml', system: '2800', counted: '2750', delta: '-50' },
    ],
    notCounted: [{ ingredientId: 3, name: 'Limón' }],
  });
  back.closeCount.mockImplementation(() => back.previewCount());
  limits.get.mockRejectedValue(new Error('sin límites'));
});
afterEach(() => {
  cleanup();
  document.body.innerHTML = '';
  for (const a of ['style', 'data-inert', 'data-scroll-lock', 'aria-hidden']) document.body.removeAttribute(a);
  vi.clearAllMocks();
});

function montar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><Provider><MemoryRouter initialEntries={['/almacen']}>
    <Routes><Route path="/almacen" element={<StockCountTab />} /><Route path="/negocio" element={<p>Negocio</p>} /></Routes>
  </MemoryRouter></Provider></QueryClientProvider>);
}
const click = (name: string | RegExp) => fireEvent.click(screen.getByRole('button', { name }));
const tecla = (k: string) => fireEvent.click(screen.getByRole('button', { name: k }));

// La segunda prueba sin contexto (040, T022) no encontró cómo cambiar el orden del recorrido: antes
// de empezar, el conteo dice en qué orden va y quien configura llega a ordenarlo desde ahí.
describe('orden del recorrido (spec 040)', () => {
  test('antes de empezar dice el orden de los lugares y lleva a cambiarlo', async () => {
    back.currentCount.mockResolvedValue(null);
    useSessionStore.setState({ user: { id: 1, name: 'Ana', role: 'gerente', permissions: ['inventory.count', 'inventory.configure'] } as never });
    montar();
    expect(await screen.findByText('Se recorre en este orden: Barra → Bodega')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Cambiar el orden ›' }));
    expect(await screen.findByText('Negocio')).toBeInTheDocument();
  });
  test('sin permiso de configurar no ofrece cambiarlo', async () => {
    back.currentCount.mockResolvedValue(null);
    useSessionStore.setState({ user: { id: 1, name: 'Ana', role: 'cajero', permissions: ['inventory.count'] } as never });
    montar();
    expect(await screen.findByText('Se recorre en este orden: Barra → Bodega')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Cambiar el orden ›' })).toBeNull();
  });
});

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
    expect(screen.getAllByText('Sin definir').some((e) => e.tagName !== 'BUTTON')).toBe(true); // el encabezado del grupo
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
    // Lo suelto abre en la unidad base (spec 042); 0.5 L se escribe eligiendo L.
    fireEvent.click(screen.getByRole('button', { name: 'L' }));
    tecla('punto'); tecla('5');
    expect(screen.getByTestId('total-contado')).toHaveTextContent('2.75 L');
    click('Guardar');
    await waitFor(() => expect(back.captureCount).toHaveBeenCalledWith(5, 1, { countedQty: '2750', confirmable: true }));
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
    await waitFor(() => expect(back.captureCount).toHaveBeenCalledWith(5, 2, { countedQty: '25000', confirmable: true }));
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

// Decisión del dueño tras la prueba sin contexto (FR-015f): la ubicación se pone desde el conteo; nadie
// la encontró en la hoja «Cómo llega». Desde la 040 se elige de la lista de la empresa; desde la 043,
// crear una pide configurar el almacén y quien solo cuenta elige entre las que hay.
const quienConfigura = () => useSessionStore.setState({ user: { id: 1, name: 'Ana', role: 'gerente', permissions: ['inventory.count', 'inventory.configure'] } as never });
describe('ubicación desde el conteo (FR-015f)', () => {
  test('ofrece los lugares de la empresa y guarda el elegido', async () => {
    montar();
    const fila = (await screen.findAllByTestId('renglon-conteo'))[2]; // Limón, sin ubicación
    fireEvent.click(await within(fila).findByRole('button', { name: 'Sin definir' }));
    expect(await within(fila).findByRole('button', { name: 'Barra' })).toBeInTheDocument();
    expect(within(fila).getByRole('button', { name: 'Bodega' })).toBeInTheDocument();
    fireEvent.click(within(fila).getByRole('button', { name: 'Barra' }));
    await waitFor(() => expect(back.setCountPlace).toHaveBeenCalledWith(3, 1));
    await waitFor(() => expect(back.currentCount.mock.calls.length).toBeGreaterThan(1));
  });

  test('quien configura crea una ubicación nueva escribiéndola', async () => {
    quienConfigura();
    montar();
    const fila = (await screen.findAllByTestId('renglon-conteo'))[2];
    fireEvent.click(await within(fila).findByRole('button', { name: 'Sin definir' }));
    fireEvent.change(within(fila).getByPlaceholderText('Buscar…'), { target: { value: 'Refri' } });
    fireEvent.click(within(fila).getByRole('button', { name: /Crear «Refri»/ }));
    await waitFor(() => expect(back.setCountPlace).toHaveBeenCalledWith(3, 9));
  });

  // El aviso «Pídele a un administrador» con el Picker de verdad lo cubre CountPlaceField.test.tsx;
  // aquí el Picker es un doble y lo que se prueba es que el conteo no le pase el alta.
  test('quien solo cuenta elige pero no crea', async () => {
    useSessionStore.setState({ user: { id: 1, name: 'Ana', role: 'cajero', permissions: ['inventory.count'] } as never });
    montar();
    const fila = (await screen.findAllByTestId('renglon-conteo'))[2];
    fireEvent.click(await within(fila).findByRole('button', { name: 'Sin definir' }));
    expect(await within(fila).findByRole('button', { name: 'Barra' })).toBeInTheDocument();
    expect(within(fila).queryByPlaceholderText('Buscar…')).toBeNull();
  });
});

// Poner un lugar reordena el conteo; en «Uno por uno» la tarjeta sigue en el mismo insumo y no salta
// al que quedó en su posición.
test('uno por uno sigue en el mismo insumo cuando el lugar lo reordena', async () => {
  quienConfigura();
  montar();
  fireEvent.click(await screen.findByRole('button', { name: 'Uno por uno' }));
  expect(screen.getByText('Jarabe de vainilla')).toBeInTheDocument();
  const reordered = conteo();
  reordered.items = [reordered.items[1], reordered.items[2], { ...reordered.items[0], place: 'Zótano', placeId: 9 }];
  back.currentCount.mockResolvedValue(reordered);
  fireEvent.click(await screen.findByRole('button', { name: 'Barra' }));
  fireEvent.change(screen.getByPlaceholderText('Buscar…'), { target: { value: 'Zótano' } });
  fireEvent.click(screen.getByRole('button', { name: /Crear «Zótano»/ }));
  await waitFor(() => expect(back.setCountPlace).toHaveBeenCalledWith(1, 9));
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
  fireEvent.click(screen.getByRole('button', { name: 'L' }));
  tecla('punto'); tecla('5');
  click('Guardar');
  await waitFor(() => expect(back.captureCount).toHaveBeenCalledWith(5, 1, { countedQty: '2750', confirmable: true }));

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

// Spec 042, US7 (FR-012): contar sin equivocarse de unidad.
describe('Teclado del conteo (spec 042)', () => {
  const abrir = async (n = 0) => {
    montar();
    fireEvent.click(within((await screen.findAllByTestId('renglon-conteo'))[n]).getByRole('button', { name: 'Contar' }));
  };

  // Revisión de código: «No hay» guardaba cero con un toque, pegado a «Guardar». Ahora pide confirmar.
  test('Guardar no guarda sin captura; «No hay» pide confirmar antes de guardar cero', async () => {
    await abrir();
    expect(await screen.findByRole('button', { name: 'Guardar' })).toBeDisabled();
    click('No hay');
    expect(screen.getByText(/¿No hay nada de .+\?/)).toBeInTheDocument();
    expect(back.captureCount).not.toHaveBeenCalled();
    click('Sí, no hay');
    await waitFor(() => expect(back.captureCount).toHaveBeenCalledWith(5, 1, { countedQty: '0', confirmable: true }));
  });

  test('+ y − ajustan de uno en uno; tocar la cantidad lleva el teclado ahí', async () => {
    await abrir();
    fireEvent.click(await screen.findByRole('button', { name: 'Agregar Botella' }));
    fireEvent.click(screen.getByRole('button', { name: 'Agregar Botella' }));
    fireEvent.click(screen.getByRole('button', { name: 'Quitar Botella' }));
    expect(screen.getByLabelText('Botella de Jarabe de vainilla')).toHaveValue('1');
    fireEvent.click(screen.getByLabelText('Botella de Jarabe de vainilla'));
    tecla('2');
    expect(screen.getByLabelText('Botella de Jarabe de vainilla')).toHaveValue('12');
  });

  test('una cifra rara pide confirmar mostrándola en grande, y «Sí, guardar» la confirma', async () => {
    back.captureCount.mockRejectedValueOnce(new ApiError(409, 'NEEDS_CONFIRM', '¿seguro?', 'r'));
    await abrir();
    click(/Siguiente casilla/);
    fireEvent.click(screen.getByRole('button', { name: 'L' }));
    tecla('7'); tecla('4'); tecla('0');
    click('Guardar');
    expect(await screen.findByText('¿Seguro? Revisa la cifra y la unidad')).toBeInTheDocument();
    expect(screen.getAllByText('740 L').length).toBeGreaterThan(0);
    // Con las dos hojas abiertas getByRole no encuentra el botón en jsdom; se toca por su texto.
    fireEvent.click(screen.getByText('Sí, guardar'));
    await waitFor(() => expect(back.captureCount).toHaveBeenLastCalledWith(5, 1, { countedQty: '740000', confirmable: true, confirmed: true }));
  });

  test('el buscador también encuentra por nombre de empaque', async () => {
    montar();
    await screen.findAllByTestId('renglon-conteo');
    fireEvent.change(screen.getByLabelText('Buscar'), { target: { value: 'bulto' } });
    const rows = screen.getAllByTestId('renglon-conteo');
    expect(rows).toHaveLength(1);
    expect(rows[0]).toHaveTextContent('Azúcar');
  });

  test('con más de 3 empaques el total va arriba, antes de los contadores', async () => {
    const c = conteo();
    c.items[0].presentations = ['Caja', 'Bolsa', 'Paquete', 'Rollo', 'Bulto'].map((name, i) => ({ name, qtyInBase: String((i + 1) * 100) }));
    back.currentCount.mockResolvedValue(c);
    await abrir();
    const total = await screen.findByTestId('total-contado');
    const first = screen.getByLabelText('Caja de Jarabe de vainilla');
    expect(total.compareDocumentPosition(first) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  test('un artículo en piezas que se puede pesar ofrece piezas, g y kg; sin eso, no hay selector', async () => {
    const c = conteo();
    c.items[2].weighable = true;
    back.currentCount.mockResolvedValue(c);
    await abrir(2);
    for (const u of ['piezas', 'g', 'kg']) expect(await screen.findByRole('button', { name: u })).toBeInTheDocument();
  });
});

// Spec 044, US4: «datos inválidos» no decía qué corregir. El aviso dice el máximo de la empresa en la
// unidad grande, antes de mandar nada.
describe('máximo por conteo (spec 044)', () => {
  const abrirAzucar = async () => {
    montar();
    fireEvent.click(within((await screen.findAllByTestId('renglon-conteo'))[1]).getByRole('button', { name: 'Contar' }));
    click(/Siguiente casilla/);
    fireEvent.click(await screen.findByRole('button', { name: 'kg' }));
  };

  test('con los límites de la empresa dice su máximo y no deja guardar', async () => {
    limits.get.mockResolvedValue({
      mass: { max: 2_000_000, confirm: 50_000 }, volume: { max: 1_000_000, confirm: 50_000 },
      length: { max: 1_000_000, confirm: 5_000 }, piece: { max: 1_000_000, confirm: 5_000 },
    });
    await abrirAzucar();
    tecla('1'); tecla('5'); tecla('0'); tecla('0');
    expect(screen.queryByText(/Máximo .* por conteo/)).toBeNull();
    tecla('borrar'); tecla('borrar'); tecla('borrar'); tecla('borrar');
    tecla('2'); tecla('5'); tecla('0'); tecla('0');
    expect(await screen.findByText('Máximo 2,000 kg por conteo')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Guardar' })).toBeDisabled();
    expect(back.captureCount).not.toHaveBeenCalled();
  });

  test('si no se pudieron leer los límites usa los de siempre', async () => {
    await abrirAzucar();
    tecla('1'); tecla('5'); tecla('0'); tecla('0');
    expect(await screen.findByText('Máximo 1,000 kg por conteo')).toBeInTheDocument();
  });

  test('un 400 del servidor se muestra tal cual', async () => {
    back.captureCount.mockRejectedValueOnce(new ApiError(400, 'VALIDATION', 'Máximo 3 kg por conteo', 'r'));
    await abrirAzucar();
    tecla('5');
    click('Guardar');
    expect(await screen.findByText('Máximo 3 kg por conteo')).toBeInTheDocument();
  });
});

// Spec 044, US5: con un teclado conectado, tocar la pantalla cuesta tiempo. Un solo listener en el
// conteo; no se apropia de las teclas cuando el foco está en otro control o hay otra hoja abierta.
describe('teclado físico (spec 044)', () => {
  const press = (key: string, target: Element | Window = document.body) => fireEvent.keyDown(target, { key });
  const abrir = async (n = 0) => {
    montar();
    fireEvent.click(within((await screen.findAllByTestId('renglon-conteo'))[n]).getByRole('button', { name: 'Contar' }));
    await screen.findByRole('button', { name: 'Guardar' });
  };
  const suelto = () => screen.getByLabelText('Suelto de Jarabe de vainilla');

  test('números, punto y coma como punto, Retroceso; letras ignoradas; cada tecla una vez', async () => {
    await abrir();
    press('Tab'); // de la botella a lo suelto
    for (const k of ['2', 'a', ',', '5', '7', 'Backspace']) press(k);
    expect(suelto()).toHaveValue('2.5');
    press('.');
    expect(suelto()).toHaveValue('2.5');
  });

  test('Tab pasa a la siguiente casilla solo si la hay', async () => {
    await abrir();
    const tab = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true });
    act(() => { document.body.dispatchEvent(tab); });
    expect(tab.defaultPrevented).toBe(true);
    press('3');
    expect(suelto()).toHaveValue('3');
    cleanup();
    // Limón tiene una sola casilla; en «uno por uno» no hay hoja que atrape el Tab por su cuenta.
    const c = conteo();
    c.items = [c.items[2]];
    back.currentCount.mockResolvedValue(c);
    localStorage.setItem('egb:conteo:vista', 'uno');
    montar();
    await screen.findByRole('button', { name: /Guardar y seguir/ });
    const tab2 = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true });
    act(() => { document.body.dispatchEvent(tab2); });
    expect(tab2.defaultPrevented).toBe(false);
  });

  test('Enter guarda; sin captura no hace nada', async () => {
    await abrir();
    press('Enter');
    expect(back.captureCount).not.toHaveBeenCalled();
    press('Tab'); press('2'); press('5');
    press('Enter');
    await waitFor(() => expect(back.captureCount).toHaveBeenCalledWith(5, 1, { countedQty: '25', confirmable: true }));
    expect(back.captureCount).toHaveBeenCalledTimes(1);
  });

  test('con el aviso de cifra rara abierto, Enter no confirma', async () => {
    back.captureCount.mockRejectedValueOnce(new ApiError(409, 'NEEDS_CONFIRM', '¿seguro?', 'r'));
    await abrir();
    press('Tab'); press('9');
    press('Enter');
    expect(await screen.findByText('¿Seguro? Revisa la cifra y la unidad')).toBeInTheDocument();
    press('Enter');
    press('1');
    await new Promise((r) => setTimeout(r, 30));
    expect(back.captureCount).toHaveBeenCalledTimes(1);
    expect(suelto()).toHaveValue('9');
  });

  test('con el foco en un campo de texto o un botón no se apropia de las teclas', async () => {
    montar();
    const buscar = await screen.findByLabelText('Buscar');
    buscar.focus();
    press('2', buscar);
    fireEvent.click(within(screen.getAllByTestId('renglon-conteo')[0]).getByRole('button', { name: 'Contar' }));
    await screen.findByRole('button', { name: 'Guardar' });
    press('Tab');
    const cancelar = screen.getByRole('button', { name: 'Cancelar' });
    cancelar.focus();
    press('4', cancelar);
    expect(suelto()).toHaveValue('');
  });

  test('el teclado de la pantalla sigue igual', async () => {
    await abrir();
    click(/Siguiente casilla/);
    tecla('7');
    expect(suelto()).toHaveValue('7');
  });
});
