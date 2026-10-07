import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../components/ui/provider';
import { CompositionSheet } from './CompositionSheet';
import type { Composition } from '../api/admin';

const sinCapturar: Composition = { status: '', items: [], components: [], editable: true };

const api = vi.hoisted(() => ({
  composition: vi.fn(),
  saveComposition: vi.fn((_k: string, _id: number, _b: unknown) => Promise.resolve({ status: 'confirmed', items: [], editable: true })),
  confirmComposition: vi.fn((_k: string, _id: number) => Promise.resolve({ status: 'confirmed', items: [], editable: true })),
  products: vi.fn(() => Promise.resolve({
    items: [
      { id: 7, name: 'Coca-Cola 600 ml', type: 'simple' },
      { id: 8, name: 'Crepa de Nutella', type: 'simple' },
      { id: 9, name: 'Paquete Fiesta', type: 'combo' },
      { id: 55, name: 'Leche deslactosada', type: 'simple' },
    ], total: 4, counts: { act: 4, inact: 0 },
  })),
}));
const back = vi.hoisted(() => ({
  ingredients: vi.fn(() => Promise.resolve({
    items: [
      { id: 1, name: 'Leche deslactosada', baseUnitId: 3, baseUnitCode: 'ml', baseUnitKind: 'volumen' },
      { id: 2, name: 'Azúcar', baseUnitId: 1, baseUnitCode: 'g', baseUnitKind: 'masa' },
    ],
  })),
  units: vi.fn(() => Promise.resolve({
    items: [
      { id: 1, code: 'g', name: 'Gramo', kind: 'masa', toBase: '1' },
      { id: 2, code: 'kg', name: 'Kilogramo', kind: 'masa', toBase: '1000' },
      { id: 3, code: 'ml', name: 'Mililitro', kind: 'volumen', toBase: '1' },
    ],
  })),
}));
vi.mock('../api/admin', () => ({ adminApi: api }));
vi.mock('../api/backoffice', () => ({ backofficeApi: back }));

function montar(kind: 'product' | 'option' | 'ingredient' = 'option') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <Provider>
        <CompositionSheet kind={kind} id={55} name="Leche deslactosada" open onClose={() => {}} />
      </Provider>
    </QueryClientProvider>,
  );
}

describe('la hoja «Qué lleva»', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.composition.mockResolvedValue(sinCapturar);
  });

  it('agregar un insumo con su cantidad y guardar manda el renglón en la unidad del insumo', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: /Agregar insumo/ }));
    fireEvent.click(await screen.findByRole('button', { name: /Elegir insumo/ }));
    fireEvent.click(await screen.findByText('Leche deslactosada'));
    fireEvent.change(screen.getByLabelText('Cantidad'), { target: { value: '250' } });
    fireEvent.click(await screen.findByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(api.saveComposition).toHaveBeenCalledWith('option', 55, {
      items: [{ ingredientId: 1, quantity: '250', unitId: 3 }], linkedProductId: null, components: [],
    }));
  });

  it('quitar es un botón de 44 px y se deshace con un toque', async () => {
    api.composition.mockResolvedValue({
      ...sinCapturar, status: 'confirmed',
      items: [{ ingredientId: 2, ingredientName: 'Azúcar', quantity: '5', unitId: 1, unitCode: 'g' }],
    });
    montar();
    const quitar = await screen.findByRole('button', { name: /Quitar Azúcar/ });
    expect(getComputedStyle(quitar).minHeight).toBe('44px');
    fireEvent.click(quitar);
    expect(screen.queryByRole('button', { name: /Quitar Azúcar/ })).not.toBeInTheDocument();
    // Un toque para deshacer: recapturar el renglón serían varios.
    fireEvent.click(screen.getByRole('button', { name: 'Deshacer' }));
    expect(screen.getByRole('button', { name: /Quitar Azúcar/ })).toBeInTheDocument();
  });

  it('un extra que es un producto del catálogo se liga a él', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Es un producto' }));
    fireEvent.click(await screen.findByRole('button', { name: /Elegir producto/ }));
    fireEvent.click(await screen.findByText('Coca-Cola 600 ml'));
    // La hoja del selector se cierra con animación: hasta entonces lo de abajo queda oculto.
    fireEvent.click(await screen.findByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(api.saveComposition).toHaveBeenCalledWith('option', 55, { items: [], linkedProductId: 7, components: [] }));
  });

  it('un extra puede ser un paquete del catálogo', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Es un producto' }));
    fireEvent.click(await screen.findByRole('button', { name: /Elegir producto/ }));
    expect(await screen.findByText('Paquete Fiesta')).toBeInTheDocument();
  });

  // ARMAR UN PAQUETE: productos con cuántas piezas lleva cada uno, con el mismo número de toques que
  // un renglón de insumo.
  it('un producto se arma como paquete con productos y piezas', async () => {
    montar('product');
    fireEvent.click(await screen.findByRole('button', { name: 'Es un paquete' }));
    fireEvent.click(await screen.findByRole('button', { name: /Agregar producto/ }));
    fireEvent.click(await screen.findByRole('button', { name: /Elegir producto/ }));
    // Ni él mismo ni otro paquete se ofrecen: un paquete no lleva paquetes.
    expect(screen.queryByText('Paquete Fiesta')).not.toBeInTheDocument();
    fireEvent.click(await screen.findByText('Crepa de Nutella'));
    fireEvent.click(await screen.findByRole('button', { name: 'Una pieza más de Crepa de Nutella' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(api.saveComposition).toHaveBeenCalledWith('product', 55, {
      items: [], linkedProductId: null, components: [{ productId: 8, quantity: 2 }],
    }));
  });

  it('un paquete guardado se ve con sus productos', async () => {
    api.composition.mockResolvedValue({
      ...sinCapturar, status: 'confirmed', components: [{ productId: 8, productName: 'Crepa de Nutella', quantity: 2 }],
    });
    montar('product');
    expect(await screen.findByRole('button', { name: 'Es un paquete' })).toHaveAttribute('data-active');
    expect(screen.getByText('2')).toBeInTheDocument();
  });

  it('lo estimado se ve como estimado y se confirma sin editar', async () => {
    api.composition.mockResolvedValue({
      ...sinCapturar, status: 'estimated',
      items: [{ ingredientId: 2, ingredientName: 'Azúcar', quantity: '5', unitId: 1, unitCode: 'g' }],
    });
    montar();
    expect(await screen.findByText(/Estimado/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Confirmar' }));
    await waitFor(() => expect(api.confirmComposition).toHaveBeenCalledWith('option', 55));
  });

  it('lo que no lleva nada se confirma así', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: 'No lleva nada' }));
    await waitFor(() => expect(api.confirmComposition).toHaveBeenCalledWith('option', 55));
  });

  // UN INSUMO QUE SE PREPARA AQUÍ: lo que lleva y cuánto rinde, en la unidad del insumo.
  it('un insumo se captura como preparado con su rendimiento', async () => {
    api.composition.mockResolvedValue({ ...sinCapturar, yieldUnitCode: 'ml' });
    montar('ingredient');
    fireEvent.click(await screen.findByRole('button', { name: 'Lo preparo aquí' }));
    fireEvent.click(await screen.findByRole('button', { name: /Agregar insumo/ }));
    fireEvent.click(await screen.findByRole('button', { name: /Elegir insumo/ }));
    fireEvent.click(await screen.findByText('Azúcar'));
    fireEvent.change(screen.getByLabelText('Cantidad'), { target: { value: '500' } });
    fireEvent.change(screen.getByLabelText('Rinde'), { target: { value: '1000' } });
    expect(screen.getByText('ml')).toBeInTheDocument();
    fireEvent.click(await screen.findByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(api.saveComposition).toHaveBeenCalledWith('ingredient', 55, {
      items: [{ ingredientId: 2, quantity: '500', unitId: 1 }], linkedProductId: null, components: [], yield: '1000',
    }));
  });

  it('un insumo preparado sin rendimiento no se puede guardar', async () => {
    api.composition.mockResolvedValue({ ...sinCapturar, yieldUnitCode: 'ml' });
    montar('ingredient');
    fireEvent.click(await screen.findByRole('button', { name: 'Lo preparo aquí' }));
    fireEvent.click(await screen.findByRole('button', { name: /Agregar insumo/ }));
    fireEvent.click(await screen.findByRole('button', { name: /Elegir insumo/ }));
    fireEvent.click(await screen.findByText('Azúcar'));
    fireEvent.change(screen.getByLabelText('Cantidad'), { target: { value: '500' } });
    expect(await screen.findByRole('button', { name: 'Guardar' })).toBeDisabled();
  });

  it('un insumo que se compra hecho no ofrece «No lleva nada»', async () => {
    montar('ingredient');
    expect(await screen.findByRole('button', { name: 'Lo compro hecho' })).toHaveAttribute('data-active');
    expect(screen.queryByRole('button', { name: 'No lleva nada' })).not.toBeInTheDocument();
  });

  it('un paquete que deja elegir no se edita aquí y lo dice al abrir', async () => {
    api.composition.mockResolvedValue({ ...sinCapturar, status: 'confirmed', editable: false, reason: 'package_choices' });
    montar('product');
    expect(await screen.findByText(/deja elegir entre productos/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Guardar' })).not.toBeInTheDocument();
  });

  it('un producto con existencias propias no se edita aquí y lo dice', async () => {
    api.composition.mockResolvedValue({ ...sinCapturar, status: 'confirmed', editable: false, reason: 'own_stock' });
    montar('product');
    expect(await screen.findByText(/sus propias existencias/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Guardar' })).not.toBeInTheDocument();
  });
});
