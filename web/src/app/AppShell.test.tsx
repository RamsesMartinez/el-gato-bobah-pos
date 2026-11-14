import { afterEach, describe, expect, test, vi } from 'vitest';
import { act, render, screen, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../components/ui/provider';
import { useSessionStore } from '../stores/session';
import { useAppUpdate } from '../stores/appUpdate';
import { AppShell } from './AppShell';

vi.mock('../api/pos', () => ({ posApi: { logout: vi.fn() } }));
vi.mock('./SystemInfo', () => ({ SystemInfo: () => null }));
const roster = vi.hoisted(() => vi.fn(() => Promise.resolve({ enabled: false, people: [] })));
vi.mock('../api/attendance', () => ({ attendanceApi: { roster: () => roster() } }));
const pendingWaste = vi.hoisted(() => vi.fn(() => Promise.resolve([] as unknown[])));
vi.mock('../api/waste', () => ({ wasteApi: { pending: () => pendingWaste() } }));

// Lo que trae hoy la sesión de un administrador para las pantallas del riel.
const ADMIN = ['cash.operate', 'expenses.capture', 'inventory.view', 'sales.view', 'reports.view', 'menu.configure',
  'platforms.configure', 'employees.manage', 'roles.manage', 'business.configure'];

function renderView(path = '/') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><Provider><MemoryRouter initialEntries={[path]}><AppShell /></MemoryRouter></Provider></QueryClientProvider>);
}

// «Salir» medía 46×32 (validación como usuario nuevo): bajo el mínimo de 44 px de la constitución.
describe('el menú lateral', () => {
  afterEach(() => useAppUpdate.setState({ needRefresh: false, apply: () => {} }));

  // Roles por empresa (spec 068): el riel sale de los PERMISOS de la sesión, no del nombre del rol.
  test('el riel ofrece lo que los permisos abren, se llame como se llame el rol', async () => {
    useSessionStore.setState({ user: { id: 1, name: 'Ana', role: 'admin', permissions: [] } as never });
    const { unmount } = renderView();
    await screen.findByText('Vender');
    for (const l of ['Caja', 'Inventario', 'Menú', 'Personal', 'Reportes']) {
      expect(screen.queryByText(l), `${l} sin permiso`).toBeNull();
    }
    expect(screen.getByText('Pedidos')).toBeInTheDocument();
    expect(screen.getByText('Ajustes')).toBeInTheDocument();
    unmount();
    // Con solo «ver ventas», Caja lleva directo a Ventas: nunca a la pantalla de caja, que no abre.
    useSessionStore.setState({ user: { id: 2, name: 'Bety', role: '', permissions: ['sales.view', 'roles.manage'] } as never });
    renderView();
    expect(await screen.findByRole('link', { name: 'Caja' })).toHaveAttribute('href', '/ventas');
    expect(screen.getByRole('link', { name: 'Personal' })).toHaveAttribute('href', '/empleados');
  });
  test('«Salir» mide al menos 44 px de alto', () => {
    useSessionStore.setState({ user: { id: 1, name: 'Ana', role: 'admin', permissions: ADMIN } as never });
    renderView();
    const salir = screen.getByRole('button', { name: 'Salir' });
    expect(parseInt(getComputedStyle(salir).minHeight || '0', 10)).toBeGreaterThanOrEqual(44);
  });

  // La asistencia es activable por empresa: apagada, Personal no ocupa lugar si no le queda nada.
  test('«Asistencia» aparece solo con la asistencia activa', async () => {
    useSessionStore.setState({ user: { id: 1, name: 'Ana', role: 'cajero', permissions: [] } as never });
    roster.mockResolvedValueOnce({ enabled: false, people: [] });
    const { unmount } = renderView();
    await screen.findByText('Vender');
    expect(screen.queryByText('Personal')).toBeNull();
    unmount();
    roster.mockResolvedValueOnce({ enabled: true, people: [] });
    renderView();
    expect(await screen.findByRole('link', { name: 'Personal' })).toHaveAttribute('href', '/asistencia');
  });

  // La campana de notificaciones (#291) va junto al tema rápido, en el mismo renglón, para no quitarle
  // alto a los módulos. El hueco existe antes que la campana: el siguiente cambio del pie no se lo come.
  test('el pie deja el hueco de la campana junto al tema', async () => {
    useSessionStore.setState({ user: { id: 1, name: 'Ana', role: 'admin', permissions: ADMIN } as never });
    const { container } = renderView();
    await screen.findByText('Vender');
    const hueco = container.querySelector('[data-bell-slot]') as HTMLElement;
    expect(hueco).not.toBeNull();
    expect(parseInt(getComputedStyle(hueco).minHeight, 10)).toBeGreaterThanOrEqual(44);
    const tema = screen.getByRole('button', { name: /Cambiar a tema/ });
    expect(hueco.parentElement!.contains(tema)).toBe(true);
  });

  // A 600 px la flecha de «hay más abajo» iba encima del último ítem y su rótulo se encimaba con el
  // nombre de quien entró. Va en su propio renglón, fuera de la lista.
  test('la flecha de «hay más» no se encima sobre la lista', async () => {
    useSessionStore.setState({ user: { id: 1, name: 'Administrador', role: 'admin', permissions: ADMIN } as never });
    const { container } = renderView();
    await screen.findByText('Vender');
    const flecha = container.querySelector('[data-rail-more]') as HTMLElement;
    expect(flecha).not.toBeNull();
    expect(getComputedStyle(flecha).position).not.toBe('absolute');
    const lista = container.querySelector('[data-rail-list]')!;
    expect(lista.contains(flecha)).toBe(false);
  });

  // Con «Reportes» abierto en una ventana baja, su módulo quedaba partido bajo el pliegue. Cómo se
  // calcula el desplazamiento lo prueba railScroll.test.ts; aquí, que la pantalla abierta lo pide.
  test('el módulo de la pantalla abierta se lleva a la vista', async () => {
    const spy = vi.spyOn(await import('./railScroll'), 'railScrollFor');
    useSessionStore.setState({ user: { id: 1, name: 'Administrador', role: 'admin', permissions: ADMIN } as never });
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<QueryClientProvider client={qc}><Provider><MemoryRouter initialEntries={['/reportes']}><AppShell /></MemoryRouter></Provider></QueryClientProvider>);
    await screen.findByText('Reportes');
    await vi.waitFor(() => expect(spy).toHaveBeenCalled());
    expect((spy.mock.calls.at(-1)![1] as HTMLElement).textContent).toContain('Reportes');
  });

  // Lo que se echó a perder (spec 039) sale por PERMISO y no por rol: lo usan cajero y cocina, que no
  // entran a Almacén. Vive en Inventario; con solo ese destino, Inventario abre directo ahí.
  test('«Se echó a perder» sale con su permiso, dentro de Inventario', async () => {
    useSessionStore.setState({ user: { id: 1, name: 'Ana', role: 'cajero', permissions: [] } as never });
    const { unmount } = renderView();
    await screen.findByText('Vender');
    expect(screen.queryByText('Inventario')).toBeNull();
    unmount();
    useSessionStore.setState({ user: { id: 1, name: 'Ana', role: 'cajero', permissions: ['inventory.write_off'] } as never });
    renderView();
    expect(await screen.findByRole('link', { name: 'Inventario' })).toHaveAttribute('href', '/merma');
  });

  // Quien aprueba ve en el riel cuántas esperan: en Vender y Pedidos es el único aviso.
  test('Inventario lleva el número de mermas por aprobar para quien aprueba', async () => {
    pendingWaste.mockResolvedValue([{ id: 1 }, { id: 2 }]);
    useSessionStore.setState({ user: { id: 1, name: 'Gerente', role: 'gerente', permissions: ['inventory.write_off', 'inventory.write_off_approve'] } as never });
    renderView();
    const link = await screen.findByRole('link', { name: /Inventario/ });
    expect(await within(link).findByLabelText('2 por aprobar en Se echó a perder')).toHaveTextContent('2');
  });

  // El aviso de versión nueva vive en el lugar del logo (spec 064): ni encima de la pantalla ni con
  // un renglón propio que le quite alto a la lista del riel.
  test('sin versión nueva está el logo y no el botón de actualizar', async () => {
    useAppUpdate.setState({ needRefresh: false, apply: () => {} });
    useSessionStore.setState({ user: { id: 1, name: 'Ana', role: 'admin', permissions: ADMIN } as never });
    renderView();
    await screen.findByText('Vender');
    expect(screen.getByAltText('El Gato Bobah')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Actualizar a la versión nueva' })).toBeNull();
  });

  test('con versión nueva el botón ocupa el lugar del logo', async () => {
    useAppUpdate.setState({ needRefresh: true, apply: () => {} });
    useSessionStore.setState({ user: { id: 1, name: 'Ana', role: 'admin', permissions: ADMIN } as never });
    renderView();
    await screen.findByText('Vender');
    const boton = screen.getByRole('button', { name: 'Actualizar a la versión nueva' });
    expect(screen.queryByAltText('El Gato Bobah')).toBeNull();
    const lista = document.querySelector('[data-rail-list]')!;
    // Va arriba de la lista, donde estaba el logo.
    expect(boton.compareDocumentPosition(lista) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  // Si el alto del riel cambiara con el aviso, la opción abierta podría quedar partida: se vuelve a
  // llevar a la vista cuando llega la versión nueva.
  test('al llegar una versión nueva se vuelve a llevar a la vista la opción abierta', async () => {
    useAppUpdate.setState({ needRefresh: false, apply: () => {} });
    const spy = vi.spyOn(await import('./railScroll'), 'railScrollFor');
    useSessionStore.setState({ user: { id: 1, name: 'Administrador', role: 'admin', permissions: ADMIN } as never });
    renderView('/reportes');
    await screen.findByText('Reportes');
    await vi.waitFor(() => expect(spy).toHaveBeenCalled());
    const antes = spy.mock.calls.length;
    act(() => useAppUpdate.setState({ needRefresh: true }));
    await vi.waitFor(() => expect(spy.mock.calls.length).toBeGreaterThan(antes));
  });
});

