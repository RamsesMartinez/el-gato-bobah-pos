import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../../components/ui/provider';
import { ModifierSheet } from './ModifierSheet';
import { useTicketStore } from '../../stores/ticket';
import { useSessionStore } from '../../stores/session';
import type { MenuProduct, TicketModifier } from '../../types/pos';

vi.mock('../../api/pos', () => ({ posApi: { menu: () => Promise.resolve({}) } }));
vi.mock('../../api/admin', () => ({ adminApi: {} }));
vi.mock('../../hooks/useMenu', () => ({
  useMenu: () => ({ data: { platforms: [], platformPrices: {}, platformModPrices: {} } }),
}));

const salsa = (id: number, name: string, maxPerLine: number) =>
  ({ id, name, priceDelta: '0', maxPerLine, favorite: false });

// "Salsas" tal como está en producción: pide 2, y cada salsa admite hasta 2 iguales.
const producto = {
  id: 1, name: 'Boneless', price: '200', categoryId: 1, description: '', imageUrl: null, trackStock: false,
  groups: [{
    id: 7, title: 'Salsas', min: 2, max: 2,
    options: [salsa(10, 'Mango habanero', 2), salsa(11, 'Búfalo', 2), salsa(12, 'Sin salsa', 1)],
  }],
} as unknown as MenuProduct;

function montar(onConfirm: (m: TicketModifier[], n: string, q: number) => void) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  useSessionStore.setState({ user: { id: 1, name: 'Ana', role: 'cajero' } as never });
  useTicketStore.getState().descartarTodo();
  return render(
    <QueryClientProvider client={qc}>
      <Provider>
        <ModifierSheet product={producto} isOpen onClose={() => {}} onConfirm={onConfirm} />
      </Provider>
    </QueryClientProvider>,
  );
}

describe('elegir dos veces la misma salsa', () => {
  beforeEach(() => vi.clearAllMocks());

  // El caso reportado: el grupo pide dos salsas y el cliente las quiere las dos de mango.
  it('el "+" aparece al elegir una salsa repetible y manda cantidad 2', () => {
    const onConfirm = vi.fn();
    montar(onConfirm);

    // Antes de elegir nada no hay nada que repetir.
    expect(screen.queryByLabelText('Otra vez Mango habanero')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /^Mango habanero/ }));
    const mas = screen.getByLabelText('Otra vez Mango habanero');

    fireEvent.click(mas);
    expect(screen.getByRole('button', { name: /Mango habaneros*×2/ })).toBeInTheDocument();
    // Al llenar el grupo el "+" desaparece: ya no cabe otra.
    expect(screen.queryByLabelText('Otra vez Mango habanero')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /agregar|confirmar|listo/i }));
    expect(onConfirm).toHaveBeenCalled();
    const mods = onConfirm.mock.calls[0][0] as TicketModifier[];
    expect(mods).toHaveLength(1);
    expect(mods[0]).toMatchObject({ optionId: 10, qty: 2 });
  });

  // 818 de las opciones tienen maxPerLine 1. Para ellas nada cambia: sin "+", el chip sigue siendo
  // un toggle de un toque.
  it('una opción que no admite repetirse no ofrece el "+"', () => {
    montar(vi.fn());
    fireEvent.click(screen.getByRole('button', { name: /^Sin salsa/ }));
    expect(screen.queryByLabelText('Otra vez Sin salsa')).not.toBeInTheDocument();
  });

  // El "+" solo suma. Con el grupo lleno por dos salsas distintas no aparece, porque para crecer
  // tendría que quitarle una a la otra — y eso no puede esconderse detrás de un "+".
  it('con el grupo lleno por dos salsas distintas no ofrece repetir', () => {
    montar(vi.fn());
    fireEvent.click(screen.getByRole('button', { name: /^Mango habanero/ }));
    fireEvent.click(screen.getByRole('button', { name: /^Búfalo/ }));
    expect(screen.queryByLabelText('Otra vez Mango habanero')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Otra vez Búfalo')).not.toBeInTheDocument();
  });

  // Quitar sigue costando UN toque. Es la razón por la que el "+" existe en vez de hacer que el
  // toque repetido incremente: corregir una salsa mal elegida es más frecuente que pedirla doble.
  it('tocar el chip lo quita de un solo toque, aunque vaya en ×2', () => {
    montar(vi.fn());
    const chip = () => screen.getByRole('button', { name: /^Mango habanero/ });
    fireEvent.click(chip());
    fireEvent.click(screen.getByLabelText('Otra vez Mango habanero'));
    expect(screen.getByRole('button', { name: /Mango habaneros*×2/ })).toBeInTheDocument();

    fireEvent.click(chip());
    expect(screen.queryByRole('button', { name: /Mango habaneros*×2/ })).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Otra vez Mango habanero')).not.toBeInTheDocument();
  });
});
