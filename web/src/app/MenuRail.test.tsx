import { describe, expect, test } from 'vitest';
import { render, screen, within, fireEvent, act } from '@testing-library/react';
import { Link, MemoryRouter, Route, Routes, useLocation } from 'react-router';
import { Provider } from '../components/ui/provider';
import { MenuRail } from './MenuRail';
import { visibleModules } from './nav';
import type { Permission } from './permissions';
import type { SessionUser } from '../stores/session';

const ADMIN: Permission[] = ['cash.operate', 'expenses.capture', 'inventory.view', 'inventory.write_off', 'sales.view',
  'reports.view', 'menu.configure', 'platforms.configure', 'employees.manage', 'business.configure', 'integrations.manage'];
const CAJERO: Permission[] = ['cash.operate', 'inventory.write_off'];

function Donde() {
  return <span data-testid="donde">{useLocation().pathname}</span>;
}
const donde = () => screen.getByTestId('donde').textContent;

function renderRail(perms: Permission[], path = '/pos', opts: { wide?: boolean; pending?: number } = {}) {
  const user = { id: 1, name: 'X', role: '', permissions: perms } as unknown as SessionUser;
  const modules = visibleModules(user, { attendanceEnabled: true });
  return render(
    <Provider>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="*" element={<><MenuRail modules={modules} wide={opts.wide ?? false} pendingWaste={opts.pending ?? 0} /><Donde /><Link to="/reportes">fuera del riel</Link></>} />
        </Routes>
      </MemoryRouter>
    </Provider>,
  );
}

const panel = () => screen.queryByRole('navigation', { name: /./ });

describe('el riel en dos niveles (maqueta C)', () => {
  test('Vender y Pedidos abren directo y van separados del resto', () => {
    renderRail(ADMIN);
    const vender = screen.getByRole('link', { name: 'Vender' });
    const pedidos = screen.getByRole('link', { name: 'Pedidos' });
    expect(vender).toHaveAttribute('href', '/pos');
    expect(pedidos).toHaveAttribute('href', '/pedidos');
    const divisoria = document.querySelector('[data-rail-divider]')!;
    expect(divisoria).not.toBeNull();
    expect(pedidos.compareDocumentPosition(divisoria) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(divisoria.compareDocumentPosition(screen.getByRole('button', { name: 'Caja' })) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  test('cada botón del primer nivel mide al menos 72 px', () => {
    renderRail(ADMIN);
    const botones = [...screen.getAllByRole('link'), ...screen.getAllByRole('button')].filter((b) => b.textContent !== 'fuera del riel');
    expect(botones.length).toBe(8);
    for (const b of botones) expect(parseInt(getComputedStyle(b).minHeight, 10), b.textContent ?? '').toBeGreaterThanOrEqual(72);
  });

  // El cajero solo tiene Caja dentro de Caja: tocarlo no le pide un segundo toque.
  test('un módulo con un solo destino abre directo', () => {
    renderRail(CAJERO);
    expect(screen.getByRole('link', { name: 'Caja' })).toHaveAttribute('href', '/caja');
    expect(screen.queryByRole('button', { name: 'Caja' })).toBeNull();
  });

  test('lo que espera aprobación de «Se echó a perder» va sobre Inventario y se lee', () => {
    renderRail(ADMIN, '/pos', { pending: 3 });
    const inv = screen.getByRole('button', { name: /Inventario/ });
    expect(within(inv).getByLabelText('3 por aprobar en Se echó a perder')).toHaveTextContent('3');
  });

  test('tocar un módulo abre su panel con sus destinos y el foco en el primero', () => {
    renderRail(ADMIN);
    const caja = screen.getByRole('button', { name: 'Caja' });
    expect(caja).toHaveAttribute('aria-expanded', 'false');
    fireEvent.click(caja);
    expect(caja).toHaveAttribute('aria-expanded', 'true');
    const nav = panel()!;
    expect(caja.getAttribute('aria-controls')).toBe(nav.id);
    const enlaces = within(nav).getAllByRole('link');
    expect(enlaces.map((a) => a.textContent)).toEqual(['Caja', 'Ventas', 'Gastos']);
    expect(document.activeElement).toBe(enlaces[0]);
    for (const a of enlaces) expect(parseInt(getComputedStyle(a).minHeight, 10)).toBeGreaterThanOrEqual(52);
  });

  test('elegir un destino navega y cierra el panel', () => {
    renderRail(ADMIN);
    fireEvent.click(screen.getByRole('button', { name: 'Caja' }));
    fireEvent.click(within(panel()!).getByRole('link', { name: 'Gastos' }));
    expect(donde()).toBe('/gastos');
    expect(panel()).toBeNull();
  });

  test('Escape cierra sin navegar y devuelve el foco al módulo', () => {
    renderRail(ADMIN, '/pos');
    const ajustes = screen.getByRole('button', { name: 'Ajustes' });
    fireEvent.click(ajustes);
    fireEvent.keyDown(document.activeElement!, { key: 'Escape' });
    expect(panel()).toBeNull();
    expect(donde()).toBe('/pos');
    expect(document.activeElement).toBe(ajustes);
  });

  test('tocar el velo cierra sin navegar', () => {
    renderRail(ADMIN, '/pos');
    fireEvent.click(screen.getByRole('button', { name: 'Menú' }));
    fireEvent.click(document.querySelector('[data-rail-veil]')!);
    expect(panel()).toBeNull();
    expect(donde()).toBe('/pos');
  });

  test('tocar otro módulo cambia de panel', () => {
    renderRail(ADMIN);
    fireEvent.click(screen.getByRole('button', { name: 'Caja' }));
    fireEvent.click(screen.getByRole('button', { name: 'Personal' }));
    expect(within(panel()!).getAllByRole('link').map((a) => a.textContent)).toEqual(['Asistencia', 'Empleados']);
    expect(screen.getByRole('button', { name: 'Caja' })).toHaveAttribute('aria-expanded', 'false');
  });

  test('cambiar de ruta por otro lado cierra el panel', () => {
    renderRail(ADMIN);
    fireEvent.click(screen.getByRole('button', { name: 'Caja' }));
    act(() => { fireEvent.click(screen.getByRole('link', { name: 'fuera del riel' })); });
    expect(panel()).toBeNull();
  });

  // Emparejar es una pantalla de Plataformas: el módulo Menú se ve abierto y, en su panel, Plataformas.
  test('la pantalla abierta marca su módulo y su destino', () => {
    renderRail(ADMIN, '/plataformas/3/emparejar');
    const menu = screen.getByRole('button', { name: 'Menú' });
    expect(menu).toHaveAttribute('data-active', 'true');
    expect(screen.getByRole('button', { name: 'Caja' })).toHaveAttribute('data-active', 'false');
    fireEvent.click(menu);
    expect(within(panel()!).getByRole('link', { name: 'Plataformas' })).toHaveAttribute('aria-current', 'page');
  });

  test('en pantalla ancha se ven todos los destinos sin panel', () => {
    renderRail(ADMIN, '/pos', { wide: true, pending: 2 });
    for (const l of ['Ventas', 'Gastos', 'Almacén', 'Plataformas', 'Asistencia', 'Empleados', 'Negocio', 'Integraciones', 'Mi cuenta']) {
      expect(screen.getByRole('link', { name: l }), l).toBeInTheDocument();
    }
    expect(screen.queryByRole('button', { expanded: false })).toBeNull();
    expect(screen.getByLabelText('2 por aprobar en Se echó a perder')).toBeInTheDocument();
  });
});
