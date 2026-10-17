import { describe, expect, test, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../components/ui/provider';
import { useSessionStore } from '../stores/session';
import { AppShell } from './AppShell';

vi.mock('../api/pos', () => ({ posApi: { logout: vi.fn() } }));
vi.mock('./SystemInfo', () => ({ SystemInfo: () => null }));
const roster = vi.hoisted(() => vi.fn(() => Promise.resolve({ enabled: false, people: [] })));
vi.mock('../api/attendance', () => ({ attendanceApi: { roster: () => roster() } }));
const pendingWaste = vi.hoisted(() => vi.fn(() => Promise.resolve([] as unknown[])));
vi.mock('../api/waste', () => ({ wasteApi: { pending: () => pendingWaste() } }));

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

  // A 600 px la flecha de «hay más abajo» iba encima del último ítem y su rótulo se encimaba con el
  // nombre de quien entró. Va en su propio renglón, fuera de la lista.
  test('la flecha de «hay más» no se encima sobre la lista', async () => {
    useSessionStore.setState({ user: { id: 1, name: 'Administrador', role: 'admin', permissions: [] } as never });
    const { container } = renderView();
    await screen.findByText('Vender');
    const flecha = container.querySelector('[data-rail-more]') as HTMLElement;
    expect(flecha).not.toBeNull();
    expect(getComputedStyle(flecha).position).not.toBe('absolute');
    const lista = screen.getByText('Vender').closest('a')!.parentElement!;
    expect(lista.contains(flecha)).toBe(false);
  });

  // Con «Reportes» abierto a 600 px, su ítem quedaba partido bajo el pliegue del menú.
  test('el ítem de la pantalla abierta se lleva a la vista', async () => {
    const spy = vi.fn();
    Element.prototype.scrollIntoView = spy;
    useSessionStore.setState({ user: { id: 1, name: 'Administrador', role: 'admin', permissions: [] } as never });
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<QueryClientProvider client={qc}><Provider><MemoryRouter initialEntries={['/reportes']}><AppShell /></MemoryRouter></Provider></QueryClientProvider>);
    await screen.findByText('Reportes');
    await vi.waitFor(() => expect(spy).toHaveBeenCalled());
    expect((spy.mock.contexts.at(-1) as HTMLElement).textContent).toContain('Reportes');
  });

  // Lo que se echó a perder (spec 039): el renglón sale por PERMISO y no por rol, porque lo usan
  // cajero y cocina, que no entran a Almacén. Para quien administra va después de «Reportes», como
  // Asistencia, para no sacar a «Reportes» de los ocho visibles a 600 px.
  test('«Se echó a perder» sale con su permiso, después de «Reportes»', async () => {
    useSessionStore.setState({ user: { id: 1, name: 'Ana', role: 'cajero', permissions: [] } as never });
    const { unmount } = renderView();
    await screen.findByText('Vender');
    expect(screen.queryByText('Se echó a perder')).toBeNull();
    unmount();
    useSessionStore.setState({ user: { id: 1, name: 'Ana', role: 'cajero', permissions: ['inventory.write_off'] } as never });
    const second = renderView();
    expect(await screen.findByText('Se echó a perder')).toBeInTheDocument();
    second.unmount();
    useSessionStore.setState({ user: { id: 1, name: 'Admin', role: 'admin', permissions: ['inventory.write_off'] } as never });
    renderView();
    const labels = Array.from(document.querySelectorAll('a')).map((a) => a.textContent);
    expect(labels.indexOf('Se echó a perder')).toBeGreaterThan(labels.indexOf('Reportes'));
  });

  // La prueba con alguien nuevo no encontró el renglón: con la asistencia activa quedaba más abajo
  // del pliegue. Lo de todos los días va antes que checar, que además está en la pantalla de bloqueo.
  test('«Se echó a perder» va antes de «Asistencia»', async () => {
    roster.mockResolvedValueOnce({ enabled: true, people: [] });
    useSessionStore.setState({ user: { id: 1, name: 'Admin', role: 'admin', permissions: ['inventory.write_off'] } as never });
    renderView();
    await screen.findByText('Asistencia');
    const labels = Array.from(document.querySelectorAll('a')).map((a) => a.textContent);
    expect(labels.indexOf('Se echó a perder')).toBeLessThan(labels.indexOf('Asistencia'));
  });

  // Quien aprueba ve en el riel cuántas esperan: en Vender y Pedidos es el único aviso.
  test('el renglón lleva el número de mermas por aprobar para quien aprueba', async () => {
    pendingWaste.mockResolvedValue([{ id: 1 }, { id: 2 }]);
    useSessionStore.setState({ user: { id: 1, name: 'Gerente', role: 'gerente', permissions: ['inventory.write_off', 'inventory.write_off_approve'] } as never });
    renderView();
    const link = (await screen.findByText('Se echó a perder')).closest('a')!;
    expect(await within(link).findByText('2')).toBeInTheDocument();
  });
});
