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

  it('se titula como receta y dice qué sale al vender', async () => {
    montar();
    expect(await screen.findByText('Receta de Chai Latte G')).toBeInTheDocument();
    expect(screen.getByText('Al vender 1, del almacén sale:')).toBeInTheDocument();
    for (const t of ['Lleva insumos', 'Es un combo', 'No gasta insumos']) expect(screen.getByText(t)).toBeInTheDocument();
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

  it('un insumo que no existe se crea ahí mismo, diciendo en qué se mide', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: /Agregar insumo/ }));
    fireEvent.change(await screen.findByLabelText('Buscar'), { target: { value: 'Leche de coco' } });
    fireEvent.click(await screen.findByRole('button', { name: 'Mililitros (ml)' }));
    await waitFor(() => expect(back.createIngredient).toHaveBeenCalledWith({ name: 'Leche de coco', baseUnitId: 3 }));
    expect(await screen.findByLabelText('Cantidad de Leche de coco')).toBeInTheDocument();
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

  it('«No gasta insumos» se guarda vacía y se confirma', async () => {
    montar('option');
    fireEvent.click(await screen.findByText('No gasta insumos'));
    await guardar();
    await waitFor(() => expect(api.confirmComposition).toHaveBeenCalledWith('option', 55));
    expect(api.saveComposition).toHaveBeenCalledWith('option', 55, expect.objectContaining({ items: [], components: [], linkedProductId: null }));
  });

  it('lo que se cargó del sistema anterior se confirma con «Está bien» sin tocar nada', async () => {
    api.composition.mockResolvedValue({ ...vacia, status: 'estimated', items: [{ ingredientId: 2, ingredientName: 'Azúcar', quantity: '5', unitId: 1, unitCode: 'g' }] });
    montar();
    expect(await screen.findByText(/se cargó de tu sistema anterior/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Está bien' }));
    await waitFor(() => expect(api.confirmComposition).toHaveBeenCalledWith('product', 55));
    expect(api.saveComposition).not.toHaveBeenCalled();
  });

  it('un extra que se llama igual en otro grupo recibe la misma receta, si se deja marcado', async () => {
    api.composition.mockResolvedValue({ ...vacia, sameName: [{ id: 77, group: 'Toppings de frappé', stamp: '2026-10-01T00:00:00Z' }] });
    montar('option');
    fireEvent.click(await screen.findByRole('button', { name: '+ Vaso 16 oz' }));
    expect(await screen.findByRole('button', { name: /También en «Toppings de frappé»/ })).toHaveAttribute('aria-pressed', 'true');
    await guardar();
    await waitFor(() => expect(api.saveComposition).toHaveBeenCalledWith('option', 55, expect.objectContaining({ alsoOptionIds: [77], alsoBasedOn: { 77: '2026-10-01T00:00:00Z' } })));
  });

  // Lo encontró una persona nueva: «También en…» quedaba marcado y al escoger «No gasta insumos»
  // desaparecía; los otros extras seguían pendientes.
  it('«No gasta insumos» también se guarda en el extra que se llama igual', async () => {
    api.composition.mockResolvedValue({ ...vacia, sameName: [{ id: 77, group: 'Toppings de frappé', stamp: '' }] });
    montar('option');
    fireEvent.click(await screen.findByText('No gasta insumos'));
    expect(await screen.findByRole('button', { name: /También en «Toppings de frappé»/ })).toHaveAttribute('aria-pressed', 'true');
    await guardar();
    await waitFor(() => expect(api.confirmComposition).toHaveBeenCalledWith('option', 77));
    expect(api.saveComposition).toHaveBeenCalledWith('option', 55, expect.objectContaining({ items: [], alsoOptionIds: [77] }));
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

  it('lo que se descuenta solo no tiene nada que capturar', async () => {
    api.composition.mockResolvedValue({ ...vacia, status: 'confirmed', editable: false, reason: 'own_stock' });
    montar();
    expect(await screen.findByText(/Se descuenta solo/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Guardar' })).not.toBeInTheDocument();
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
