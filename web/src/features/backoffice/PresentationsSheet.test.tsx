import { beforeEach, describe, expect, test, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../../components/ui/provider';
import type { Presentation, Unit } from '../../api/backoffice';
import { PresentationsSheet, type PresentationsIngredient } from './PresentationsSheet';

const back = vi.hoisted(() => ({ createPresentation: vi.fn(), updatePresentation: vi.fn(), setCountPlace: vi.fn(), countPlaces: vi.fn(), createCountPlace: vi.fn() }));
vi.mock('../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));

const units: Unit[] = [
  { id: 1, code: 'g', name: 'Gramo', kind: 'masa', toBase: '1' },
  { id: 2, code: 'kg', name: 'Kilogramo', kind: 'masa', toBase: '1000' },
  { id: 3, code: 'ml', name: 'Mililitro', kind: 'volumen', toBase: '1' },
  { id: 5, code: 'cda', name: 'Cucharada', kind: 'volumen', toBase: '15' },
  { id: 7, code: 'l', name: 'Litro', kind: 'volumen', toBase: '1000' },
  { id: 8, code: 'pieza', name: 'Pieza', kind: 'pieza', toBase: '1' },
];
const leche = { id: 10, name: 'Leche entera', baseUnitCode: 'ml', baseUnitKind: 'volumen' };
const aguacate = { id: 11, name: 'Aguacate', baseUnitCode: 'pieza', baseUnitKind: 'pieza' };
const pres = (over: Partial<Presentation>): Presentation => ({
  id: 1, ingredientId: 10, name: 'Caja', qtyInBase: '12000', baseUnitCode: 'ml', active: true, approximate: false, purchaseUnitId: null, ...over,
});

function montar(ingredient: PresentationsIngredient = leche, presentations: Presentation[] = [], usedNames = ['Caja', 'Bolsa', 'Bulto']) {
  const onClose = vi.fn();
  render(
    <Provider>
      <QueryClientProvider client={new QueryClient()}>
        <PresentationsSheet isOpen ingredient={ingredient} presentations={presentations} usedNames={usedNames}
          units={units} onClose={onClose} />
      </QueryClientProvider>
    </Provider>,
  );
  return { onClose };
}

beforeEach(() => {
  back.createPresentation.mockReset().mockResolvedValue(pres({ id: 99 }));
  back.updatePresentation.mockReset().mockResolvedValue(pres({}));
});

describe('Cómo llega un insumo (spec 038, maqueta B)', () => {
  test('lista los empaques con su contenido y los que ya no llegan aparte', async () => {
    montar(leche, [pres({}), pres({ id: 2, name: 'Galón', qtyInBase: '3785.4' }), pres({ id: 3, name: 'Tira', active: false })]);
    const dialog = await screen.findByRole('dialog');
    expect(dialog).toHaveTextContent('Caja · 12 L');
    expect(dialog).toHaveTextContent('Galón · 3.79 L');
    expect(dialog).toHaveTextContent('Ya no llega en: Tira');
    expect(dialog.querySelector('select')).toBeNull();
  });

  // Caso borde 1: «25» a secas en un insumo en gramos sería un error de 1,000 veces.
  test('un empaque nuevo pide nombre, número y unidad, y muestra la equivalencia antes de guardar', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: '+ Agregar empaque' }));
    // Los nombres que la empresa ya usa, no una lista fija; las cucharadas no se ofrecen.
    fireEvent.click(screen.getByRole('button', { name: 'Caja' }));
    expect(screen.queryByRole('button', { name: 'cda' })).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'L' }));
    fireEvent.change(screen.getByLabelText('Cuánto trae'), { target: { value: '12' } });
    expect(screen.getByText('= 12 L')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Guardar empaque' }));
    await waitFor(() => expect(back.createPresentation).toHaveBeenCalledWith(10, { name: 'Caja', qty: '12', unitId: 7, approximate: false }));
  });

  test('«Otro…» deja escribir un nombre que la empresa no ha usado', async () => {
    montar(leche, [], []);
    fireEvent.click(await screen.findByRole('button', { name: '+ Agregar empaque' }));
    fireEvent.click(screen.getByRole('button', { name: 'Otro…' }));
    fireEvent.change(screen.getByLabelText('Nombre del empaque'), { target: { value: 'Garrafa' } });
    fireEvent.click(screen.getByRole('button', { name: 'L' }));
    fireEvent.change(screen.getByLabelText('Cuánto trae'), { target: { value: '5' } });
    fireEvent.click(screen.getByRole('button', { name: 'Guardar empaque' }));
    await waitFor(() => expect(back.createPresentation).toHaveBeenCalledWith(10, expect.objectContaining({ name: 'Garrafa' })));
  });

  // Regla 16: un empaque de un insumo por pieza trae piezas enteras.
  test('en un insumo por pieza, el contenido con decimales no se guarda', async () => {
    montar(aguacate);
    fireEvent.click(await screen.findByRole('button', { name: '+ Agregar empaque' }));
    fireEvent.click(screen.getByRole('button', { name: 'Bolsa' }));
    fireEvent.change(screen.getByLabelText('Cuánto trae'), { target: { value: '12.5' } });
    expect(screen.getByRole('button', { name: 'Guardar empaque' })).toBeDisabled();
    expect(screen.getByText(/piezas enteras/)).toBeInTheDocument();
  });

  // Regla 18: lo que se compra por kilo y se usa por pieza lleva un empaque aproximado.
  test('un insumo por pieza puede tener un empaque aproximado por kilo', async () => {
    montar(aguacate, [], []);
    fireEvent.click(await screen.findByRole('button', { name: '+ Agregar empaque' }));
    fireEvent.click(screen.getByRole('button', { name: 'Se compra por peso o líquido' }));
    fireEvent.click(screen.getByRole('button', { name: 'kg' }));
    fireEvent.change(screen.getByLabelText('Cuánto trae'), { target: { value: '4' } });
    expect(screen.getByText(/≈ 4 piezas/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Guardar empaque' }));
    await waitFor(() => expect(back.createPresentation).toHaveBeenCalledWith(11, { name: 'Kilo', qty: '4', unitId: 2, approximate: true }));
  });

  test('«Ya no llega así» la desactiva sin borrarla', async () => {
    montar(leche, [pres({})]);
    const row = (await screen.findByText('Caja · 12 L')).closest('[data-row]') as HTMLElement;
    fireEvent.click(within(row).getByRole('button', { name: 'Ya no llega así' }));
    await waitFor(() => expect(back.updatePresentation).toHaveBeenCalledWith(1, { name: 'Caja', qty: '12', unitId: 7, approximate: false, active: false }));
  });
});

describe('lugar para contar (spec 040)', () => {
  test('desde Almacén se ofrece el lugar de la lista', async () => {
    back.countPlaces.mockResolvedValue([{ id: 3, name: 'Barra', position: 0, ingredients: 1 }]);
    montar({ ...leche, countPlaceId: 3 });
    expect(await screen.findByRole('button', { name: /Barra/ })).toBeInTheDocument();
  });
  test('desde la captura de una compra no se ofrece', async () => {
    montar();
    await screen.findByRole('dialog');
    expect(screen.queryByText('Lugar')).toBeNull();
  });
});
