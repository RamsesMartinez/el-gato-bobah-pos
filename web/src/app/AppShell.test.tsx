import { describe, expect, test, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../components/ui/provider';
import { useSessionStore } from '../stores/session';
import { AppShell } from './AppShell';

vi.mock('../api/pos', () => ({ posApi: { logout: vi.fn() } }));
vi.mock('./SystemInfo', () => ({ SystemInfo: () => null }));
const roster = vi.hoisted(() => vi.fn(() => Promise.resolve({ enabled: false, people: [] })));
vi.mock('../api/attendance', () => ({ attendanceApi: { roster: () => roster() } }));

function renderView() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><Provider><MemoryRouter><AppShell /></MemoryRouter></Provider></QueryClientProvider>);
}

// «Salir» medía 46×32 (validación como usuario nuevo): bajo el mínimo de 44 px de la constitución.
describe('el menú lateral', () => {
  test('«Salir» mide al menos 44 px de alto', () => {
    useSessionStore.setState({ user: { id: 1, name: 'Ana', role: 'admin', permissions: [] } as never });
    renderView();
    const salir = screen.getByRole('button', { name: 'Salir' });
    expect(parseInt(getComputedStyle(salir).minHeight || '0', 10)).toBeGreaterThanOrEqual(44);
  });

  // La asistencia es activable por empresa: apagada, su ítem no ocupa lugar en un riel que ya no
  // tiene alto libre.
  test('«Asistencia» aparece solo con la asistencia activa', async () => {
    useSessionStore.setState({ user: { id: 1, name: 'Ana', role: 'cajero', permissions: [] } as never });
    roster.mockResolvedValueOnce({ enabled: false, people: [] });
    const { unmount } = renderView();
    await screen.findByText('Vender');
    expect(screen.queryByText('Asistencia')).toBeNull();
    unmount();
    roster.mockResolvedValueOnce({ enabled: true, people: [] });
    renderView();
    expect(await screen.findByText('Asistencia')).toBeInTheDocument();
  });
});
