import { afterEach, describe, expect, test, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../../components/ui/provider';
import { CountVsSalesReport } from './CountVsSalesReport';
import { useSessionStore } from '../../stores/session';
import type { CountVsSalesItem } from '../../api/waste';

const waste = vi.hoisted(() => ({ countVsSales: vi.fn() }));
const back = vi.hoisted(() => ({ setArticleExpectedWaste: vi.fn(() => Promise.resolve({ pct: '3' })) }));
vi.mock('../../api/waste', async (orig) => ({ ...(await orig<object>()), wasteApi: waste }));
vi.mock('../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));
afterEach(() => vi.clearAllMocks());

const item = (p: Partial<CountVsSalesItem>): CountVsSalesItem => ({
  ingredientId: 1, name: 'Leche', unitCode: 'ml', hasPrevious: true, initial: '0', entries: '0', sold: '10000',
  expectedWaste: '0', registered: '0', transfersOut: '0', system: '0', counted: '0', diff: '-300', pct: '-3',
  group: 'small', suggestedPct: null, ...p,
});

function montar(perms: string[]) {
  useSessionStore.setState({ user: { id: 1, companyId: 2, name: 'Ana', role: 'gerente', permissions: perms } as never });
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><Provider><CountVsSalesReport countId={9} /></Provider></QueryClientProvider>);
}

// Conteo contra ventas (spec 096, maqueta C): tres grupos de qué hacer, no solo qué pasó.
describe('Conteo contra ventas', () => {
  test('agrupa por qué hacer y dice cuántos hay en cada grupo', async () => {
    waste.countVsSales.mockResolvedValue({ firstCount: false, items: [
      item({ ingredientId: 1, name: 'Leche', group: 'suggest', suggestedPct: '3' }),
      item({ ingredientId: 2, name: 'Tortilla', group: 'review', pct: '-12' }),
      item({ ingredientId: 3, name: 'Azúcar', group: 'small', pct: '-0.5' }),
    ] });
    montar(['inventory.health_view', 'menu.configure']);
    expect(await screen.findByText('Revisar receta o captura · 1')).toBeInTheDocument();
    expect(screen.getByText('Merma estable: usar el % sugerido · 1')).toBeInTheDocument();
    expect(screen.getByText('Diferencia chica · 1')).toBeInTheDocument();
    expect(within(screen.getByRole('row', { name: /Tortilla/ })).getByText('-12 %')).toBeInTheDocument();
  });

  test('«Usar 3 %» captura la merma del artículo', async () => {
    waste.countVsSales.mockResolvedValue({ firstCount: false, items: [item({ group: 'suggest', suggestedPct: '3' })] });
    montar(['inventory.health_view', 'menu.configure']);
    fireEvent.click(await screen.findByRole('button', { name: 'Usar 3 %' }));
    await waitFor(() => expect(back.setArticleExpectedWaste).toHaveBeenCalledWith(1, 3));
    expect(await screen.findByText('Merma de 3 % capturada')).toBeInTheDocument();
  });

  test('sin permiso de configurar el menú se ve la sugerencia pero no el botón', async () => {
    waste.countVsSales.mockResolvedValue({ firstCount: false, items: [item({ group: 'suggest', suggestedPct: '3' })] });
    montar(['inventory.health_view']);
    expect(await screen.findByText('Sugerido: 3 %')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Usar 3 %' })).toBeNull();
  });

  // Revisión de código: un error se veía como espera eterna y «Usar» fallaba sin decir nada.
  test('si no se pudo leer lo dice, y si «Usar» falla lo dice en el renglón', async () => {
    waste.countVsSales.mockRejectedValueOnce(new Error('500'));
    montar(['inventory.health_view']);
    expect(await screen.findByText(/No se pudo leer lo vendido contra lo contado/)).toBeInTheDocument();
  });

  test('«Usar» que falla deja el botón y dice que no se guardó', async () => {
    waste.countVsSales.mockResolvedValue({ firstCount: false, items: [item({ group: 'suggest', suggestedPct: '3' })] });
    back.setArticleExpectedWaste.mockRejectedValueOnce(new Error('red'));
    montar(['inventory.health_view', 'menu.configure']);
    fireEvent.click(await screen.findByRole('button', { name: 'Usar 3 %' }));
    expect(await screen.findByText('No se guardó; vuelve a tocarlo')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Usar 3 %' })).toBeInTheDocument();
  });

  test('el primer conteo dice que no hay con qué comparar', async () => {
    waste.countVsSales.mockResolvedValue({ firstCount: true, items: [] });
    montar(['inventory.health_view']);
    expect(await screen.findByText(/Es el primer conteo de este almacén/)).toBeInTheDocument();
  });
});
