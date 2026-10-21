import { beforeEach, describe, expect, test, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../../components/ui/provider';
import type { Presentation, Unit } from '../../api/backoffice';
import { PresentationsSheet, type PresentationsIngredient } from './PresentationsSheet';

const back = vi.hoisted(() => ({ createPresentation: vi.fn(), updatePresentation: vi.fn(), setCountPlace: vi.fn(), countPlaces: vi.fn(), createCountPlace: vi.fn(), presentationSuggestions: vi.fn() }));
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

// Las sugerencias las da el servidor por clase de unidad (spec 042, FR-011): aquí, las de la clase del
// insumo, con su cantidad.
const SUGGESTED: Record<string, { name: string; qtyInBase: string }[]> = {
  volumen: [{ name: 'Caja', qtyInBase: '12000' }],
  pieza: [{ name: 'Bolsa', qtyInBase: '10' }],
  masa: [{ name: 'Bulto', qtyInBase: '25000' }],
};

function montar(ingredient: PresentationsIngredient = leche, presentations: Presentation[] = [], suggest = true) {
  back.presentationSuggestions.mockImplementation((k: string) => Promise.resolve(suggest ? SUGGESTED[k] ?? [] : []));
  const onClose = vi.fn();
  render(
    <Provider>
      <QueryClientProvider client={new QueryClient()}>
        <PresentationsSheet isOpen ingredient={ingredient} presentations={presentations}
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
    // «Caja» es base y la empresa ya la usa con 12 L: un solo botón, que trae su cantidad; nada de
    // otra clase de unidad.
    await waitFor(() => expect(back.presentationSuggestions).toHaveBeenCalled());
    expect(screen.getAllByRole('button', { name: /^Caja/ })).toHaveLength(1);
    fireEvent.click(screen.getByRole('button', { name: /^Caja/ }));
    expect(screen.queryByRole('button', { name: /Bulto/ })).toBeNull();
    expect(screen.queryByRole('button', { name: 'cda' })).toBeNull();
    await waitFor(() => expect(screen.getByLabelText('Cuánto trae')).toHaveValue('12'));
    expect(screen.getByText('1 caja = 12 L')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Guardar empaque' }));
    await waitFor(() => expect(back.createPresentation).toHaveBeenCalledWith(10, { name: 'Caja', qty: '12', unitId: 7, approximate: false }));
  });

  // Revisión de código: con un empaque a medio capturar, «Listo» lo tiraba sin avisar y «Guardar
  // empaque» quedaba abajo del contenido que se desplaza.
  test('con un empaque a medio capturar, el pie dice Cancelar y Guardar empaque, no Listo', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: '+ Agregar empaque' }));
    expect(screen.queryByRole('button', { name: 'Listo' })).toBeNull();
    expect(screen.getByRole('button', { name: 'Cancelar' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Guardar empaque' })).toBeInTheDocument();
  });

  test('«Otro» deja escribir un nombre que no está entre los botones', async () => {
    montar(leche, [], false);
    fireEvent.click(await screen.findByRole('button', { name: '+ Agregar empaque' }));
    fireEvent.click(screen.getByRole('button', { name: 'Otro' }));
    expect(screen.getByLabelText('Nombre del empaque')).toHaveAttribute('placeholder', 'Ej. Cubeta, Garrafa');
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
    fireEvent.click(await screen.findByRole('button', { name: /^Bolsa/ }));
    fireEvent.change(screen.getByLabelText('Cuánto trae'), { target: { value: '12.5' } });
    expect(screen.getByRole('button', { name: 'Guardar empaque' })).toBeDisabled();
    expect(screen.getByText(/piezas enteras/)).toBeInTheDocument();
  });

  // Regla 18: lo que se compra por kilo y se usa por pieza lleva un empaque aproximado.
  test('un insumo por pieza puede tener un empaque aproximado por kilo', async () => {
    montar(aguacate, [], false);
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

// Spec 043 (maqueta «Hoja de empaque nuevo»): botones base por clase de unidad, nada elegido al abrir,
// una frase que dice lo que se va a guardar y sin la ubicación.
describe('empaque nuevo sin adivinar (spec 043)', () => {
  const masa = { id: 12, name: 'Azúcar', baseUnitCode: 'g', baseUnitKind: 'masa' };
  const botones = () => screen.getAllByRole('button').filter((b) => b.closest('[data-packs]')).map((b) => b.textContent);

  test.each([
    [masa, ['Bolsa', 'Caja', 'Costal', 'Paquete', 'Otro']],
    [leche, ['Botella', 'Galón', 'Garrafa', 'Caja', 'Otro']],
    [aguacate, ['Caja', 'Paquete', 'Bolsa', 'Otro']],
  ])('una empresa sin empaques ve los base de su clase (%#)', async (ing, esperado) => {
    montar(ing, [], false);
    fireEvent.click(await screen.findByRole('button', { name: '+ Agregar empaque' }));
    expect(botones()).toEqual(esperado);
  });

  test('los que la empresa ya usa se suman sin repetir los base, y nada viene elegido', async () => {
    montar(masa);
    fireEvent.click(await screen.findByRole('button', { name: '+ Agregar empaque' }));
    await waitFor(() => expect(botones()).toEqual(['Bolsa', 'Caja', 'Costal', 'Paquete', 'Bulto · 25 kg', 'Otro']));
    expect(screen.getAllByRole('button').filter((b) => b.closest('[data-packs]') && b.getAttribute('aria-pressed') === 'true')).toHaveLength(0);
    expect(screen.queryByLabelText('Nombre del empaque')).toBeNull();
  });

  test('pregunta «¿En qué empaque lo compras?» y la frase dice lo que se guarda, junto al pie', async () => {
    montar(masa, [], false);
    fireEvent.click(await screen.findByRole('button', { name: '+ Agregar empaque' }));
    expect(screen.getByText('¿En qué empaque lo compras?')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Bolsa' }));
    fireEvent.click(screen.getByRole('button', { name: 'kg' }));
    fireEvent.change(screen.getByLabelText('Cuánto trae'), { target: { value: '5' } });
    const frase = screen.getByText('1 bolsa = 5 kg');
    expect(frase.closest('.chakra-drawer__footer'), 'la frase no está junto al pie').not.toBeNull();
    fireEvent.change(screen.getByLabelText('Cuánto trae'), { target: { value: '15' } });
    expect(screen.getByText('1 bolsa = 15 kg')).toBeInTheDocument();
    expect(screen.queryByText(/Se cuenta en|se convierte a/)).toBeNull();
  });

  test('el selector de unidad de «Trae» es el mismo que el del mínimo', async () => {
    montar(masa, [], false);
    fireEvent.click(await screen.findByRole('button', { name: '+ Agregar empaque' }));
    expect(screen.getByRole('group', { name: 'Unidad' })).toBeInTheDocument();
  });

  test('no ofrece la ubicación, aunque venga del almacén', async () => {
    montar(masa, [], false);
    await screen.findByRole('dialog');
    expect(screen.queryByText(/Ubicación|Lugar/)).toBeNull();
  });
});
