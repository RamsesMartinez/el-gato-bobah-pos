import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../components/ui/provider';
import { CompositionSheet } from './CompositionSheet';
import { ApiError } from '../api/client';
import type { Composition, RecipeQuery, RecipeRow } from '../api/admin';

// LA HOJA DE LA RECETA: las palabras del dueño (spec 028, «Vocabulario») y los casos de borde del
// prototipo, uno por prueba.

const vacia: Composition = { status: '', items: [], components: [], editable: true, stamp: '', sameName: [] };

const api = vi.hoisted(() => ({
  composition: vi.fn(),
  saveComposition: vi.fn((_k: string, _id: number, _b: unknown) => Promise.resolve({ status: 'confirmed', items: [], components: [], editable: true })),
  confirmComposition: vi.fn((_k: string, _id: number) => Promise.resolve({ status: 'confirmed', items: [], components: [], editable: true })),
  products: vi.fn(() => Promise.resolve({
    items: [
      { id: 7, name: 'Coca-Cola 600 ml', type: 'simple' },
      { id: 8, name: 'Chai Latte M', type: 'simple' },
      { id: 9, name: 'Combo Fiesta', type: 'combo' },
      { id: 55, name: 'Chai Latte G', type: 'simple' },
    ], total: 4, counts: { act: 4, inact: 0 },
  })),
  recipes: vi.fn((_q: RecipeQuery) => Promise.resolve({ items: [] as RecipeRow[], total: 0, counts: { pending: 0, review: 0, done: 0 }, totals: {} })),
}));
const back = vi.hoisted(() => ({
  ingredients: vi.fn(() => Promise.resolve({
    items: [
      { id: 1, name: 'Leche', baseUnitId: 3, baseUnitCode: 'ml', baseUnitKind: 'volumen', recipeUses: 9, isPrep: false },
      { id: 2, name: 'Azúcar', baseUnitId: 1, baseUnitCode: 'g', baseUnitKind: 'masa', recipeUses: 4, isPrep: false },
      { id: 3, name: 'Vaso 16 oz', baseUnitId: 5, baseUnitCode: 'pieza', baseUnitKind: 'pieza', recipeUses: 12, isPrep: false },
      { id: 4, name: 'Jarabe natural', baseUnitId: 3, baseUnitCode: 'ml', baseUnitKind: 'volumen', recipeUses: 0, isPrep: true },
    ],
  })),
  units: vi.fn(() => Promise.resolve({
    items: [
      { id: 1, code: 'g', name: 'Gramo', kind: 'masa', toBase: '1' },
      { id: 2, code: 'kg', name: 'Kilogramo', kind: 'masa', toBase: '1000' },
      { id: 3, code: 'ml', name: 'Mililitro', kind: 'volumen', toBase: '1' },
      { id: 4, code: 'l', name: 'Litro', kind: 'volumen', toBase: '1000' },
      { id: 5, code: 'pieza', name: 'Pieza', kind: 'pieza', toBase: '1' },
    ],
  })),
  createIngredient: vi.fn((b: { name: string; baseUnitId: number }) => Promise.resolve({ id: 99, name: b.name, baseUnitId: b.baseUnitId, baseUnitCode: 'ml', baseUnitKind: 'volumen', recipeUses: 0 })),
}));
vi.mock('../api/admin', () => ({ adminApi: api }));
vi.mock('../api/backoffice', () => ({ backofficeApi: back }));
vi.mock('../api/pos', () => ({ posApi: { businessSettings: vi.fn(() => Promise.resolve({ timezone: 'America/Mexico_City' })) } }));

function montar(kind: 'product' | 'option' | 'ingredient' = 'product', extra: { onClose?: () => void; onSaved?: () => void } = {}) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <Provider>
        <CompositionSheet kind={kind} id={55} name="Chai Latte G" open onClose={extra.onClose ?? (() => {})} onSaved={extra.onSaved} />
      </Provider>
    </QueryClientProvider>,
  );
}
const guardar = async () => fireEvent.click(await screen.findByRole('button', { name: 'Guardar' }));
async function agregarInsumo(nombre: string) {
  fireEvent.click(await screen.findByRole('button', { name: /Agregar insumo/ }));
  fireEvent.click(await screen.findByRole('button', { name: new RegExp(`^${nombre}`) }));
}

describe('la receta', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.composition.mockResolvedValue(vacia);
  });

  it('se titula como receta, pregunta qué se resta y ofrece las cuatro tarjetas', async () => {
    montar();
    expect(await screen.findByText('Receta de Chai Latte G')).toBeInTheDocument();
    expect(screen.getByText('Cuando vendes Chai Latte G, ¿qué se resta de lo que tienes?')).toBeInTheDocument();
    for (const t of ['Se prepara aquí', 'Se compra hecho', 'Es un combo', 'No se cuenta']) expect(screen.getByText(t)).toBeInTheDocument();
    // FR-019: «restar», nunca «descontar».
    expect(document.body.textContent).not.toMatch(/descuenta|descontar/i);
  });

  it('la cantidad se escribe; cambiar a litros convierte el número', async () => {
    montar();
    await agregarInsumo('Leche');
    fireEvent.change(await screen.findByLabelText('Cantidad de Leche'), { target: { value: '200' } });
    fireEvent.click(screen.getByRole('button', { name: 'L' }));
    expect(screen.getByLabelText('Cantidad de Leche')).toHaveValue('0.2');
    await guardar();
    await waitFor(() => expect(api.saveComposition).toHaveBeenCalledWith('product', 55, expect.objectContaining({
      items: [{ ingredientId: 1, quantity: '0.2', unitId: 4 }], basedOn: '',
    })));
  });

  it('acepta coma decimal', async () => {
    montar();
    await agregarInsumo('Azúcar');
    fireEvent.change(await screen.findByLabelText('Cantidad de Azúcar'), { target: { value: '7,5' } });
    await guardar();
    await waitFor(() => expect(api.saveComposition).toHaveBeenCalledWith('product', 55, expect.objectContaining({
      items: [{ ingredientId: 2, quantity: '7.5', unitId: 1 }],
    })));
  });

  it('sin cantidad no deja guardar y dice qué falta', async () => {
    montar();
    await agregarInsumo('Leche');
    fireEvent.change(await screen.findByLabelText('Cantidad de Leche'), { target: { value: '' } });
    expect(await screen.findByText('Escribe la cantidad de cada insumo.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Guardar' })).toBeDisabled();
  });

  it('los más usados se agregan de un toque', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: '+ Vaso 16 oz' }));
    expect(await screen.findByLabelText('Cantidad de Vaso 16 oz')).toHaveValue('1');
  });

  it('un insumo repetido se avisa', async () => {
    montar();
    await agregarInsumo('Leche');
    await agregarInsumo('Leche');
    expect(await screen.findByText('Leche ya está en la receta.')).toBeInTheDocument();
  });

  // Spec 042, FR-003: el insumo nuevo nace al guardar la receta, no al tocar su unidad.
  it('un insumo que no existe se agrega diciendo en qué se mide y viaja en el guardado', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: /Agregar insumo/ }));
    fireEvent.change(await screen.findByLabelText('Buscar'), { target: { value: 'Leche de coco' } });
    fireEvent.click(await screen.findByRole('button', { name: 'Mililitros (ml)' }));
    const cantidad = await screen.findByLabelText('Cantidad de Leche de coco (nuevo)');
    expect(back.createIngredient).not.toHaveBeenCalled();
    fireEvent.change(cantidad, { target: { value: '30' } });
    await guardar();
    await waitFor(() => expect(api.saveComposition).toHaveBeenCalledWith('product', 55, expect.objectContaining({
      items: [{ newArticle: { name: 'Leche de coco', baseUnitId: 3 }, quantity: '30', unitId: 3 }],
    })));
    expect(back.createIngredient).not.toHaveBeenCalled();
  });

  it('copiar la receta de otro parecido', async () => {
    api.recipes.mockImplementation((q: RecipeQuery) => Promise.resolve({
      items: q.status === 'done' ? [{ id: 8, name: 'Chai Latte M', group: '', status: 'done', mode: 'items', summary: '', lines: 2, soldPerMonth: '0' }] : [],
      total: 1, counts: { pending: 0, review: 0, done: 1 }, totals: {},
    }));
    api.composition.mockImplementation((_k: string, id: number) => Promise.resolve(id === 8
      ? { ...vacia, status: 'confirmed', items: [{ ingredientId: 1, ingredientName: 'Leche', quantity: '200.0000', unitId: 3, unitCode: 'ml' }] }
      : vacia));
    montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Chai Latte M' }));
    expect(await screen.findByLabelText('Cantidad de Leche')).toHaveValue('200');
  });

  it('un combo: productos con piezas; no ofrece combos ni a sí mismo', async () => {
    montar();
    fireEvent.click(await screen.findByText('Es un combo'));
    fireEvent.click(await screen.findByRole('button', { name: /Agregar producto al combo/ }));
    await screen.findByRole('button', { name: /^Chai Latte M/ });
    expect(screen.queryByRole('button', { name: /^Combo Fiesta/ })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /^Chai Latte G/ })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /^Chai Latte M/ }));
    fireEvent.click(await screen.findByRole('button', { name: 'Una pieza más de Chai Latte M' }));
    await guardar();
    await waitFor(() => expect(api.saveComposition).toHaveBeenCalledWith('product', 55, expect.objectContaining({
      items: [], components: [{ productId: 8, quantity: 2 }],
    })));
  });

  it('un extra puede ser otro producto, combo incluido', async () => {
    montar('option');
    fireEvent.click(await screen.findByText('Es otro producto'));
    fireEvent.click(await screen.findByRole('button', { name: /Elegir producto/ }));
    fireEvent.click(await screen.findByRole('button', { name: /^Combo Fiesta/ }));
    await guardar();
    await waitFor(() => expect(api.saveComposition).toHaveBeenCalledWith('option', 55, expect.objectContaining({ linkedProductId: 9 })));
  });

  it('«No gasta insumos» se guarda vacía y confirmada en un solo paso', async () => {
    montar('option');
    fireEvent.click(await screen.findByText('No gasta insumos'));
    await guardar();
    await waitFor(() => expect(api.saveComposition).toHaveBeenCalledWith('option', 55,
      expect.objectContaining({ mode: 'nothing', items: [], components: [], linkedProductId: null })));
    expect(api.confirmComposition).not.toHaveBeenCalledWith('option', 55);
  });

  it('lo que se cargó del sistema anterior se confirma con «Está bien» sin tocar nada', async () => {
    api.composition.mockResolvedValue({ ...vacia, status: 'estimated', items: [{ ingredientId: 2, ingredientName: 'Azúcar', quantity: '5', unitId: 1, unitCode: 'g' }] });
    montar();
    expect(await screen.findByText(/Las cargó el sistema y nadie las ha confirmado/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Está bien' }));
    await waitFor(() => expect(api.confirmComposition).toHaveBeenCalledWith('product', 55));
    expect(api.saveComposition).not.toHaveBeenCalled();
  });

  // Lo encontró producción (2026-10-11): la casilla venía encendida y copió la receta de la salsa
  // «Lemon Pepper» al sazonador en polvo del mismo nombre. Ahora viene apagada, va una por grupo y
  // cada una dice lo que ese extra lleva hoy.
  describe('«También en…»', () => {
    const gemelos = [
      { id: 77, group: 'Salsa extra', stamp: '2026-10-01T00:00:00Z', today: '36 ml Lemon Pepper Salsa' },
      { id: 78, group: 'Sazonador de polvo', stamp: '2026-10-02T00:00:00Z', today: '5 g Sazonador Lemon Pepper' },
    ];

    it('viene apagada y no copia nada si no se marca', async () => {
      api.composition.mockResolvedValue({ ...vacia, sameName: gemelos });
      montar('option');
      fireEvent.click(await screen.findByRole('button', { name: '+ Vaso 16 oz' }));
      for (const g of ['Salsa extra', 'Sazonador de polvo']) {
        expect(screen.getByRole('button', { name: new RegExp(`También en «${g}»`) })).toHaveAttribute('aria-pressed', 'false');
      }
      await guardar();
      await waitFor(() => expect(api.saveComposition).toHaveBeenCalled());
      const body = api.saveComposition.mock.calls[0][2] as { alsoOptionIds?: number[] };
      expect(body.alsoOptionIds ?? []).toEqual([]);
    });

    // La revisión de tableta: un aria-label con solo el grupo callaba justo el «Hoy» al lector de
    // pantalla, y con varios gemelos el último quedaba bajo el corte sin que nada dijera cuántos son.
    it('dice cuántos son y lo que lleva hoy cada uno, también al lector de pantalla', async () => {
      api.composition.mockResolvedValue({ ...vacia, sameName: gemelos });
      montar('option');
      fireEvent.click(await screen.findByRole('button', { name: '+ Vaso 16 oz' }));
      expect(screen.getByText('Se llama igual en 2 grupos más. Marca dónde guardar esta misma receta:')).toBeInTheDocument();
      expect(screen.getByRole('button', { name: /También en «Sazonador de polvo».*Hoy: 5 g Sazonador Lemon Pepper/ })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: /También en «Salsa extra».*Hoy: 36 ml Lemon Pepper Salsa/ })).toBeInTheDocument();
    });

    // Con varios insumos la lista de casillas quedaba bajo el corte de la tableta: va antes de
    // los insumos, justo debajo del tipo de receta, para que siempre se vea sin desplazar.
    it('va antes de los insumos', async () => {
      api.composition.mockResolvedValue({ ...vacia, sameName: gemelos });
      montar('option');
      const agregar = await screen.findByRole('button', { name: /Agregar insumo/ });
      const casilla = screen.getByRole('button', { name: /También en «Salsa extra»/ });
      expect(casilla.compareDocumentPosition(agregar) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    });

    it('copia solo a los que se marcan, con la marca de cada uno', async () => {
      api.composition.mockResolvedValue({ ...vacia, sameName: gemelos });
      montar('option');
      fireEvent.click(await screen.findByRole('button', { name: '+ Vaso 16 oz' }));
      fireEvent.click(screen.getByRole('button', { name: /También en «Salsa extra»/ }));
      expect(screen.getByRole('button', { name: /También en «Salsa extra»/ })).toHaveAttribute('aria-pressed', 'true');
      await guardar();
      await waitFor(() => expect(api.saveComposition).toHaveBeenCalledWith('option', 55,
        expect.objectContaining({ alsoOptionIds: [77], alsoBasedOn: { 77: '2026-10-01T00:00:00Z' } })));
    });

    it('«No gasta insumos» también se guarda en el que se marca', async () => {
      api.composition.mockResolvedValue({ ...vacia, sameName: gemelos });
      montar('option');
      fireEvent.click(await screen.findByText('No gasta insumos'));
      fireEvent.click(await screen.findByRole('button', { name: /También en «Salsa extra»/ }));
      await guardar();
      await waitFor(() => expect(api.confirmComposition).toHaveBeenCalledWith('option', 77));
      expect(api.confirmComposition).not.toHaveBeenCalledWith('option', 78);
      expect(api.saveComposition).toHaveBeenCalledWith('option', 55, expect.objectContaining({ items: [], alsoOptionIds: [77] }));
    });
  });

  it('una receta nueva no empieza con un aviso de error', async () => {
    montar();
    await screen.findByRole('button', { name: '+ Vaso 16 oz' });
    expect(screen.queryByText('Agrega al menos un insumo.')).not.toBeInTheDocument();
  });

  it('al agregar un insumo el cursor queda en su cantidad', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: '+ Vaso 16 oz' }));
    await waitFor(() => expect(screen.getByLabelText('Cantidad de Vaso 16 oz')).toHaveFocus());
  });

  it('si otra persona la cambió, lo dice y ofrece abrirla de nuevo', async () => {
    api.saveComposition.mockRejectedValueOnce(new ApiError(409, 'CONFLICT', 'otra persona cambió esta receta mientras la editabas', 'r1'));
    montar();
    fireEvent.click(await screen.findByRole('button', { name: '+ Vaso 16 oz' }));
    await guardar();
    expect(await screen.findByText(/otra persona cambió esta receta/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Abrir de nuevo' })).toBeInTheDocument();
  });

  it('sin conexión, lo escrito se queda y el botón dice «Reintentar»', async () => {
    api.saveComposition.mockRejectedValueOnce(new TypeError('Failed to fetch'));
    montar();
    fireEvent.click(await screen.findByRole('button', { name: '+ Vaso 16 oz' }));
    await guardar();
    expect(await screen.findByText(/no hay conexión/)).toBeInTheDocument();
    expect(screen.getByLabelText('Cantidad de Vaso 16 oz')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Reintentar' }));
    await waitFor(() => expect(api.saveComposition).toHaveBeenCalledTimes(2));
  });

  it('cerrar con cambios pregunta antes de descartarlos', async () => {
    const onClose = vi.fn();
    montar('product', { onClose });
    fireEvent.click(await screen.findByRole('button', { name: '+ Vaso 16 oz' }));
    fireEvent.click(screen.getByRole('button', { name: 'Cancelar' }));
    expect(await screen.findByText('¿Descartar los cambios?')).toBeInTheDocument();
    expect(onClose).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'Descartar' }));
    expect(onClose).toHaveBeenCalled();
  });

  it('al guardar avisa a la lista para abrir la siguiente', async () => {
    const onSaved = vi.fn();
    montar('product', { onSaved });
    fireEvent.click(await screen.findByRole('button', { name: '+ Vaso 16 oz' }));
    await guardar();
    await waitFor(() => expect(onSaved).toHaveBeenCalled());
  });

  it('un preparado necesita cuánto rinde', async () => {
    api.composition.mockResolvedValue({ ...vacia, yieldUnitCode: 'ml' });
    montar('ingredient');
    fireEvent.click(await screen.findByText('Es un preparado'));
    await agregarInsumo('Azúcar');
    fireEvent.change(await screen.findByLabelText('Cantidad de Azúcar'), { target: { value: '500' } });
    expect(await screen.findByText('Falta cuánto rinde.')).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText('Cuánto rinde'), { target: { value: '1000' } });
    await guardar();
    await waitFor(() => expect(api.saveComposition).toHaveBeenCalledWith('ingredient', 55, expect.objectContaining({ yield: '1000' })));
  });


  it('un combo que deja elegir no se reescribe', async () => {
    api.composition.mockResolvedValue({ ...vacia, status: 'confirmed', editable: false, reason: 'package_choices' });
    montar();
    expect(await screen.findByText(/deja elegir entre productos/)).toBeInTheDocument();
  });

  // Lo pidió el dueño: al abrir una receta guardada no se distinguía lo que ya lleva de lo que se
  // acaba de agregar o cambiar.
  describe('lo guardado contra lo que se está cambiando', () => {
    const guardada: Composition = {
      ...vacia, status: 'confirmed', items: [
        { ingredientId: 1, ingredientName: 'Leche', quantity: '200', unitId: 3, unitCode: 'ml' },
        { ingredientId: 2, ingredientName: 'Azúcar', quantity: '5', unitId: 1, unitCode: 'g' },
      ],
    };
    const fila = (nombre: string) => screen.getByLabelText(`Cantidad de ${nombre}`).closest('[data-row]') as HTMLElement;

    it('dice qué significa el número y que eso es lo guardado', async () => {
      api.composition.mockResolvedValue(guardada);
      montar();
      expect(await screen.findByText('Por cada venta')).toBeInTheDocument();
      expect(screen.getByText('Guardado · 2 insumos')).toBeInTheDocument();
      expect(screen.queryByText(/Sin guardar/)).not.toBeInTheDocument();
    });

    it('lo nuevo y lo cambiado se marcan, con lo que tenía antes', async () => {
      api.composition.mockResolvedValue(guardada);
      montar();
      fireEvent.change(await screen.findByLabelText('Cantidad de Leche'), { target: { value: '250' } });
      expect(fila('Leche')).toHaveTextContent('Antes 200 ml');
      expect(fila('Azúcar')).not.toHaveTextContent('Antes');
      fireEvent.click(screen.getByRole('button', { name: '+ Vaso 16 oz' }));
      expect(fila('Vaso 16 oz')).toHaveTextContent('Nuevo');
      expect(screen.getByText('Sin guardar: 1 nuevo · 1 cambiado')).toBeInTheDocument();
    });

    it('cambiar de unidad sin cambiar la cantidad no es un cambio', async () => {
      api.composition.mockResolvedValue(guardada);
      montar();
      await screen.findByLabelText('Cantidad de Leche');
      fireEvent.click(within(fila('Leche')).getByRole('button', { name: 'L' }));
      expect(screen.getByLabelText('Cantidad de Leche')).toHaveValue('0.2');
      expect(fila('Leche')).not.toHaveTextContent('Antes');
    });

    it('quitar algo guardado lo deja tachado hasta guardar, y se puede regresar', async () => {
      api.composition.mockResolvedValue(guardada);
      montar();
      fireEvent.click(await screen.findByRole('button', { name: 'Quitar Azúcar' }));
      expect(screen.getByText('Sin guardar: 1 quitado')).toBeInTheDocument();
      fireEvent.click(screen.getByRole('button', { name: 'Regresar Azúcar' }));
      expect(screen.getByLabelText('Cantidad de Azúcar')).toHaveValue('5');
      fireEvent.click(screen.getByRole('button', { name: 'Quitar Azúcar' }));
      await guardar();
      await waitFor(() => expect(api.saveComposition).toHaveBeenCalled());
      const body = api.saveComposition.mock.calls[0][2] as { items: { ingredientId: number }[] };
      expect(body.items.map((i) => i.ingredientId)).toEqual([1]);
    });
  });

  // Al salir de la cantidad la franja reaparecía, la hoja crecía hacia arriba y el botón que se iba
  // a tocar (quitar, guardar) se movía bajo el dedo: el toque caía en otra cosa.
  it('la franja «Al vender 1» no aparece ni desaparece al escribir', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: '+ Vaso 16 oz' }));
    const qty = await screen.findByLabelText('Cantidad de Vaso 16 oz');
    fireEvent.focus(qty);
    expect(screen.getByText('Al vender 1:')).toBeInTheDocument();
    fireEvent.blur(qty);
    expect(screen.getByText('Al vender 1:')).toBeInTheDocument();
  });
});

// «SE COMPRA HECHO» Y LA SALIDA DE LA EXISTENCIA PROPIA (spec 037, maqueta B simplificada v7).
describe('se compra hecho', () => {
  const conLata = (extra: Record<string, unknown> = {}) => ({
    items: [
      { id: 3, name: 'Vaso 16 oz', baseUnitId: 5, baseUnitCode: 'pieza', baseUnitKind: 'pieza', recipeUses: 12, isPrep: false, onHand: '40' },
      { id: 10, name: 'chai latte g', baseUnitId: 5, baseUnitCode: 'pieza', baseUnitKind: 'pieza', recipeUses: 0, isPrep: false, onHand: '0', ...extra },
    ],
  });
  const propia = { ...vacia, status: 'confirmed', ownStock: true, productOnHand: '12', ownStockBranches: 1, stamp: 's1' };

  beforeEach(() => {
    vi.clearAllMocks();
    api.composition.mockResolvedValue(vacia);
  });

  it('un producto con existencia propia abre en «Se compra hecho» con el insumo de nombre parecido', async () => {
    api.composition.mockResolvedValue(propia);
    back.ingredients.mockResolvedValueOnce(conLata());
    montar();
    expect(await screen.findByRole('button', { name: /Se compra hecho/ })).toHaveAttribute('aria-pressed', 'true');
    expect(await screen.findByText(/Cada venta resta 1 de/)).toHaveTextContent('chai latte g');
    // El insumo no tiene existencia: las 12 pasan a él y lo dice.
    expect(screen.getByText(/El sistema tenía contadas 12/)).toBeInTheDocument();
    await guardar();
    await waitFor(() => expect(api.saveComposition).toHaveBeenCalledWith('product', 55,
      expect.objectContaining({ mode: 'items', items: [{ ingredientId: 10, quantity: '1', unitId: 5 }] })));
  });

  it('sin uno parecido, se crea con el nombre del producto y por pieza', async () => {
    back.ingredients.mockResolvedValueOnce({ items: [conLata().items[0]] });
    montar();
    fireEvent.click(await screen.findByText('Se compra hecho'));
    expect(await screen.findByText(/se crea uno nuevo con este nombre/)).toBeInTheDocument();
    await guardar();
    await waitFor(() => expect(api.saveComposition).toHaveBeenCalledWith('product', 55,
      expect.objectContaining({ mode: 'items', items: [{ newArticle: { name: 'Chai Latte G', baseUnitId: 5 }, quantity: '1', unitId: 5 }] })));
    expect(back.createIngredient).not.toHaveBeenCalled();
  });

  it('el buscador marca el elegido y deja «Crear una nueva» al final', async () => {
    back.ingredients.mockResolvedValueOnce(conLata());
    montar();
    fireEvent.click(await screen.findByText('Se compra hecho'));
    fireEvent.click(await screen.findByRole('button', { name: /Cambiar lo que se resta/ }));
    const opciones = await screen.findAllByRole('button', { name: /chai latte g|Vaso 16 oz|Crear una nueva/ });
    expect(opciones[opciones.length - 1]).toHaveTextContent('Crear una nueva');
    expect(screen.getAllByRole('button', { name: /chai latte g/ }).some((b) => b.textContent?.includes('✓'))).toBe(true);
  });

  it('un six: el número se cambia con − y +, escondidos hasta pedirlos', async () => {
    back.ingredients.mockResolvedValueOnce(conLata());
    montar();
    fireEvent.click(await screen.findByText('Se compra hecho'));
    expect(screen.queryByRole('button', { name: 'Una más por venta' })).not.toBeInTheDocument();
    fireEvent.click(await screen.findByRole('button', { name: /¿Una venta lleva más de una\?/ }));
    for (let i = 0; i < 5; i++) fireEvent.click(screen.getByRole('button', { name: 'Una más por venta' }));
    expect(screen.getByText(/Cada venta resta 6 de/)).toBeInTheDocument();
    await guardar();
    await waitFor(() => expect(api.saveComposition).toHaveBeenCalledWith('product', 55,
      expect.objectContaining({ items: [{ ingredientId: 10, quantity: '6', unitId: 5 }] })));
  });

  it('si las dos cuentas tienen existencia, pregunta cuántas hay de verdad y no guarda hasta elegir', async () => {
    api.composition.mockResolvedValue(propia);
    back.ingredients.mockResolvedValueOnce(conLata({ onHand: '20' }));
    montar();
    expect(await screen.findByText('¿Cuántas tienes de verdad?')).toBeInTheDocument();
    for (const o of [/^12/, /^20/, /^32 \(las dos\)/, /^Otra/]) expect(screen.getByRole('button', { name: o })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Guardar' })).toBeDisabled();
    fireEvent.click(screen.getByRole('button', { name: /^32 \(las dos\)/ }));
    await guardar();
    await waitFor(() => expect(api.saveComposition).toHaveBeenCalledWith('product', 55,
      expect.objectContaining({ actualStock: '32', seenProductOnHand: '12', seenIngredientOnHand: '20' })));
  });

  it('«Otra» se escribe y se acomoda sobre el teclado', async () => {
    api.composition.mockResolvedValue(propia);
    back.ingredients.mockResolvedValueOnce(conLata({ onHand: '20' }));
    montar();
    fireEvent.click(await screen.findByRole('button', { name: /^Otra/ }));
    const input = await screen.findByLabelText('Cuántas tienes de verdad');
    const scroll = vi.fn();
    (input as HTMLInputElement).scrollIntoView = scroll;
    fireEvent.focus(input);
    expect(scroll).toHaveBeenCalled();
    expect(screen.getByRole('button', { name: 'Guardar' })).toBeDisabled();
    fireEvent.change(input, { target: { value: '25' } });
    await guardar();
    await waitFor(() => expect(api.saveComposition).toHaveBeenCalledWith('product', 55, expect.objectContaining({ actualStock: '25' })));
  });

  it('salir de la existencia propia a otra tarjeta avisa y confirma antes de dejarla en cero', async () => {
    api.composition.mockResolvedValue(propia);
    back.ingredients.mockResolvedValueOnce(conLata());
    montar();
    fireEvent.click(await screen.findByText('No se cuenta'));
    expect(await screen.findByText(/siguen donde están; el sistema solo deja de contarlas/)).toBeInTheDocument();
    await guardar();
    expect(await screen.findByRole('alertdialog')).toHaveTextContent(/no vuelve sola/);
    expect(api.saveComposition).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'Sí, dejar de contarla' }));
    await waitFor(() => expect(api.saveComposition).toHaveBeenCalledWith('product', 55, expect.objectContaining({ mode: 'nothing' })));
  });

  it('sin existencia propia no hay aviso de latas (FR-020)', async () => {
    montar();
    fireEvent.click(await screen.findByText('No se cuenta'));
    expect(screen.queryByText(/siguen donde están; el sistema solo deja de contarlas/)).not.toBeInTheDocument();
  });

  it('«Se prepara aquí» o «Es un combo» sin contenido no se guardan', async () => {
    montar();
    fireEvent.click(await screen.findByText('Se prepara aquí'));
    expect(await screen.findByRole('button', { name: 'Guardar' })).toBeDisabled();
    fireEvent.click(screen.getByText('Es un combo'));
    expect(screen.getByRole('button', { name: 'Guardar' })).toBeDisabled();
  });

  it('una venta entre mostrar y guardar: lo dice y vuelve a preguntar con las cifras nuevas', async () => {
    api.composition.mockResolvedValue(propia);
    back.ingredients.mockResolvedValue(conLata({ onHand: '20' }));
    api.saveComposition.mockRejectedValueOnce(new ApiError(409, 'CONFLICT', 'la existencia cambió mientras decidías; vuelve a revisar las cifras', 'r1'));
    montar();
    fireEvent.click(await screen.findByRole('button', { name: /^32 \(las dos\)/ }));
    await guardar();
    expect(await screen.findByRole('alert')).toHaveTextContent(/la existencia cambió/);
    const antes = api.composition.mock.calls.length;
    const antesIng = back.ingredients.mock.calls.length;
    fireEvent.click(screen.getByRole('button', { name: 'Abrir de nuevo' }));
    await waitFor(() => expect(api.composition.mock.calls.length).toBeGreaterThan(antes));
    await waitFor(() => expect(back.ingredients.mock.calls.length).toBeGreaterThan(antesIng));
  });

  // Visto en las capturas (13-oct): con existencia propia negativa decía «tenía contadas -3», que no
  // se entiende. Hay 30 productos así en producción: se vendieron más de los que se registraron.
  it('con existencia propia negativa dice que se vendieron más de las registradas', async () => {
    api.composition.mockResolvedValue({ ...propia, productOnHand: '-3' });
    back.ingredients.mockResolvedValueOnce(conLata());
    montar();
    expect(await screen.findByText(/se vendieron 3 más de las que había registradas/)).toBeInTheDocument();
    expect(screen.queryByText(/contadas -3/)).not.toBeInTheDocument();
  });

  // Visto en la captura de «No se cuenta» (13-oct): «Tus -3 de Jarritos siguen donde están», justo
  // antes de una acción que no se deshace.
  it('salir de una existencia negativa no habla de «-3 latas» ni en el aviso ni en la confirmación', async () => {
    api.composition.mockResolvedValue({ ...propia, productOnHand: '-3' });
    back.ingredients.mockResolvedValueOnce(conLata());
    montar();
    fireEvent.click(await screen.findByText('No se cuenta'));
    expect(await screen.findByText(/se vendieron 3 más de las que había registradas/)).toBeInTheDocument();
    await guardar();
    expect(await screen.findByRole('alertdialog')).not.toHaveTextContent('-3');
    expect(document.body.textContent).not.toMatch(/Tus -3/);
  });

  it('crear uno nuevo lo dice claro y el botón dice qué hace', async () => {
    back.ingredients.mockResolvedValueOnce({ items: [conLata().items[0]] });
    montar();
    fireEvent.click(await screen.findByText('Se compra hecho'));
    expect(await screen.findByText(/no hay uno parecido: se crea uno nuevo con este nombre/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Cambiar lo que se resta' })).toHaveTextContent('Elegir otro');
  });
});
