import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, within, act } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { useSessionStore } from '../../../stores/session';
import { Provider } from '../../../components/ui/provider';
import { DuplicatesSheet } from './DuplicatesSheet';
import { ArticlesTab } from './ArticlesTab';

const item = (id: number, name: string, onHand = '1000') => ({ id, name, baseUnitId: 5, baseUnitCode: 'ml', onHand });
const back = vi.hoisted(() => ({
  duplicateGroups: vi.fn(),
  markDistinct: vi.fn(() => Promise.resolve()),
  articles: vi.fn(() => Promise.resolve([])),
  similarArticles: vi.fn(() => Promise.resolve({ items: [] })),
  countPlaces: vi.fn(() => Promise.resolve([])),
  units: vi.fn(() => Promise.resolve({ items: [] })),
}));
vi.mock('../../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));

function montar(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><Provider>{ui}</Provider></QueryClientProvider>);
}

// Maqueta «Unir insumos», opción C: los grupos de posibles duplicados (FR-021 a FR-023).
describe('Posibles duplicados', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useSessionStore.setState({ user: { id: 1, companyId: 2, name: 'Ana', role: 'admin', permissions: ['inventory.view', 'inventory.configure'] } as never });
    back.duplicateGroups.mockResolvedValue({ groups: [
      { items: [item(1, 'Leche entera', '8400'), item(2, 'Leche entera Marca B', '3000')] },
      { items: [item(4, 'Vaso 16 oz'), item(5, 'Vaso 16 oz (caja)'), item(6, 'Vaso 16 oz térmico')] },
    ] });
  });
  afterEach(() => { vi.useRealTimers(); });

  it('en Artículos, el botón dice cuántos grupos hay y no aparece sin ninguno', async () => {
    montar(<ArticlesTab onOpen={vi.fn()} />);
    expect(await screen.findByRole('button', { name: /Posibles duplicados/ })).toHaveTextContent('2');
  });

  it('sin grupos no se pinta el botón', async () => {
    back.duplicateGroups.mockResolvedValue({ groups: [] });
    montar(<ArticlesTab onOpen={vi.fn()} />);
    await waitFor(() => expect(back.duplicateGroups).toHaveBeenCalled());
    expect(screen.queryByRole('button', { name: /Posibles duplicados/ })).toBeNull();
  });

  it('sin permiso de configurar no se pide ni se ve', async () => {
    useSessionStore.setState({ user: { id: 1, companyId: 2, name: 'Ana', role: 'cajero', permissions: ['inventory.view'] } as never });
    montar(<ArticlesTab onOpen={vi.fn()} />);
    await new Promise((r) => setTimeout(r, 50));
    expect(back.duplicateGroups).not.toHaveBeenCalled();
    expect(screen.queryByRole('button', { name: /Posibles duplicados/ })).toBeNull();
  });

  it('tocar un grupo lo abre en su lugar: cuál se queda, y «No son lo mismo» por par', async () => {
    montar(<DuplicatesSheet onClose={vi.fn()} onMerged={vi.fn()} />);
    fireEvent.click(await screen.findByRole('button', { name: /Vaso 16 oz · Vaso 16 oz \(caja\)/ }));
    const group = screen.getByRole('group', { name: 'Vaso 16 oz' });
    expect(within(group).getAllByRole('radio')).toHaveLength(3);
    expect(within(group).getAllByRole('button', { name: /No son lo mismo/ })).toHaveLength(3);
  });

  it('«No son lo mismo» se puede deshacer y solo se guarda pasados unos segundos', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    montar(<DuplicatesSheet onClose={vi.fn()} onMerged={vi.fn()} />);
    fireEvent.click(await screen.findByRole('button', { name: /Leche entera · Leche entera Marca B/ }));
    fireEvent.click(screen.getByRole('button', { name: 'No son lo mismo: Leche entera y Leche entera Marca B' }));
    expect(screen.getByText(/Par descartado/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Deshacer' }));
    await act(async () => { await vi.advanceTimersByTimeAsync(6000); });
    expect(back.markDistinct).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'No son lo mismo: Leche entera y Leche entera Marca B' }));
    await act(async () => { await vi.advanceTimersByTimeAsync(6000); });
    expect(back.markDistinct).toHaveBeenCalledWith(1, 2);
  });

  it('un par descartado se guarda aunque se cierre la hoja antes de tiempo', async () => {
    const { unmount } = montar(<DuplicatesSheet onClose={vi.fn()} onMerged={vi.fn()} />);
    fireEvent.click(await screen.findByRole('button', { name: /Leche entera · Leche entera Marca B/ }));
    fireEvent.click(screen.getByRole('button', { name: 'No son lo mismo: Leche entera y Leche entera Marca B' }));
    unmount();
    expect(back.markDistinct).toHaveBeenCalledWith(1, 2);
  });

  it('«Revisar unión» abre la unión con el elegido como el que se queda y los demás marcados', async () => {
    montar(<DuplicatesSheet onClose={vi.fn()} onMerged={vi.fn()} />);
    fireEvent.click(await screen.findByRole('button', { name: /Leche entera · Leche entera Marca B/ }));
    fireEvent.click(screen.getByRole('radio', { name: /Leche entera Marca B/ }));
    fireEvent.click(screen.getByRole('button', { name: 'Revisar unión' }));
    expect(await screen.findByText('Unir en Leche entera Marca B')).toBeInTheDocument();
  });
});
