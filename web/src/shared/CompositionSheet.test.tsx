import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../components/ui/provider';
import { CompositionSheet } from './CompositionSheet';
import type { Composition } from '../api/admin';

const sinCapturar: Composition = { status: '', items: [], editable: true };

const api = vi.hoisted(() => ({
  composition: vi.fn(),
  saveComposition: vi.fn((_k: string, _id: number, _b: unknown) => Promise.resolve({ status: 'confirmed', items: [], editable: true })),
  confirmComposition: vi.fn((_k: string, _id: number) => Promise.resolve({ status: 'confirmed', items: [], editable: true })),
  products: vi.fn(() => Promise.resolve({
    items: [{ id: 7, name: 'Coca-Cola 600 ml' }], total: 1, counts: { act: 1, inact: 0 },
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

function montar(kind: 'product' | 'option' = 'option') {
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
      items: [{ ingredientId: 1, quantity: '250', unitId: 3 }], linkedProductId: null,
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
    await waitFor(() => expect(api.saveComposition).toHaveBeenCalledWith('option', 55, { items: [], linkedProductId: 7 }));
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

  it('un producto con existencias propias no se edita aquí y lo dice', async () => {
    api.composition.mockResolvedValue({ ...sinCapturar, status: 'confirmed', editable: false, reason: 'own_stock' });
    montar('product');
    expect(await screen.findByText(/sus propias existencias/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Guardar' })).not.toBeInTheDocument();
  });
});
