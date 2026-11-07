import { beforeEach, describe, expect, test, vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../../components/ui/provider';
import { ApiError } from '../../api/client';
import { RolesPage } from './RolesPage';

const api = vi.hoisted(() => ({
  permissionCatalog: vi.fn(),
  roles: vi.fn(),
  createRole: vi.fn(),
  updateRole: vi.fn(),
  resetRole: vi.fn(),
  deactivateRole: vi.fn(),
  reactivateRole: vi.fn(),
  deleteRole: vi.fn(),
}));
vi.mock('../../api/admin', () => ({ adminApi: api }));

const CATALOG = [
  { code: 'orders.cancel', area: 'orders', label: 'Cancelar pedidos' },
  { code: 'orders.move_lines', area: 'orders', label: 'Pasar productos' },
  { code: 'cash.operate', area: 'cash', label: 'Abrir, cerrar y operar caja' },
  { code: 'cash.configure', area: 'cash', label: 'Configurar cajas' },
  { code: 'employees.manage', area: 'team', label: 'Administrar empleados' },
  { code: 'roles.manage', area: 'team', label: 'Administrar roles' },
];
const ALL = CATALOG.map((p) => p.code);

const ADMIN = { id: 1, systemCode: 'admin', name: 'Admin', active: true, version: 3, activeUsers: 2, allUsers: 2, permissions: ALL, added: [], removed: [] };
// Sugerido ajustado: le agregaron «Cancelar pedidos» y le quitaron «Configurar cajas».
const CAJERO = {
  id: 3, systemCode: 'cajero', name: 'Cajero', active: true, version: 5, activeUsers: 3, allUsers: 3,
  permissions: ['orders.cancel', 'orders.move_lines', 'cash.operate'], added: ['orders.cancel'], removed: ['cash.configure'],
};
const BARISTA = { id: 9, systemCode: '', name: 'Barista', active: false, version: 2, activeUsers: 0, allUsers: 0, permissions: ['cash.operate'], added: [], removed: [] };

function montar(props: Partial<Parameters<typeof RolesPage>[0]> = {}) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const onCreatingChange = vi.fn();
  const onShowPeople = vi.fn();
  render(
    <QueryClientProvider client={qc}><Provider>
      <RolesPage creating={false} onCreatingChange={onCreatingChange} onShowPeople={onShowPeople} {...props} />
    </Provider></QueryClientProvider>,
  );
  return { onCreatingChange, onShowPeople };
}

const alto = (el: Element) => parseInt(getComputedStyle(el).minHeight || '0', 10);

async function abrir(u: ReturnType<typeof userEvent.setup>, name: string) {
  await u.click(await screen.findByRole('button', { name: new RegExp(`^${name}`) }));
  return screen.findByRole('dialog');
}

beforeEach(() => {
  vi.clearAllMocks();
  api.permissionCatalog.mockResolvedValue({ items: CATALOG });
  api.roles.mockResolvedValue({ items: [ADMIN, CAJERO, BARISTA] });
  api.updateRole.mockResolvedValue({ id: 3 });
  api.createRole.mockResolvedValue({ id: 20 });
  api.resetRole.mockResolvedValue({ id: 3 });
  api.deactivateRole.mockResolvedValue({ id: 3 });
  api.reactivateRole.mockResolvedValue({ id: 9 });
  api.deleteRole.mockResolvedValue(undefined);
});

describe('lista de roles', () => {
  test('cada fila dice personas y permisos, con a lo más dos chips: tipo y estado', async () => {
    montar();
    const cajero = await screen.findByRole('button', { name: /^Cajero/ });
    expect(within(cajero).getByText('3 personas · 3 permisos')).toBeInTheDocument();
    expect(within(cajero).getByText('sugerido')).toBeInTheDocument();
    expect(within(cajero).getByText('ajustado')).toBeInTheDocument();
    const barista = screen.getByRole('button', { name: /^Barista/ });
    expect(within(barista).getByText('0 personas · 1 permiso')).toBeInTheDocument();
    expect(within(barista).getByText('propio')).toBeInTheDocument();
    // Inactivo gana sobre ajustado: el estado es uno solo.
    expect(within(barista).getByText('inactivo')).toBeInTheDocument();
    expect(alto(cajero)).toBeGreaterThanOrEqual(56);
  });
});

describe('hoja del rol', () => {
  test('tocar un rol abre la hoja con las áreas plegadas y «n de m»', async () => {
    const u = userEvent.setup();
    montar();
    const hoja = await abrir(u, 'Cajero');
    expect(within(hoja).getByRole('button', { name: /Pedidos.*2 de 2/ })).toBeInTheDocument();
    expect(within(hoja).getByRole('button', { name: /Caja.*1 de 2/ })).toBeInTheDocument();
    expect(within(hoja).queryByRole('checkbox', { name: /Pasar productos/ })).toBeNull();
    // El ajuste de un sugerido se dice con palabras, no con «+1 −1».
    expect(within(hoja).getByText('+1 agregado · −1 quitado')).toBeInTheDocument();
  });

  test('marcar y desmarcar cuenta «N cambios» y Guardar manda solo lo tocado', async () => {
    const u = userEvent.setup();
    montar();
    const hoja = await abrir(u, 'Cajero');
    const guardar = within(hoja).getByRole('button', { name: 'Guardar' });
    expect(guardar).toBeDisabled();

    await u.click(within(hoja).getByRole('button', { name: /Pedidos/ }));
    await u.click(within(hoja).getByRole('checkbox', { name: /Pasar productos/ }));
    expect(within(hoja).getByText('1 cambio')).toBeInTheDocument();
    await u.click(within(hoja).getByRole('button', { name: /^Caja/ }));
    await u.click(within(hoja).getByRole('checkbox', { name: /Configurar cajas/ }));
    expect(within(hoja).getByText('2 cambios')).toBeInTheDocument();
    // Volver a tocar lo mismo no es un cambio.
    await u.click(within(hoja).getByRole('checkbox', { name: /Pasar productos/ }));
    expect(within(hoja).getByText('1 cambio')).toBeInTheDocument();

    await u.click(guardar);
    await waitFor(() => expect(api.updateRole).toHaveBeenCalledTimes(1));
    expect(api.updateRole).toHaveBeenCalledWith(3, { version: 5, add: ['cash.configure'] });
  });

  test('en un sugerido, lo que difiere del producto dice «agregado» o «quitado»', async () => {
    const u = userEvent.setup();
    montar();
    const hoja = await abrir(u, 'Cajero');
    await u.click(within(hoja).getByRole('button', { name: /Pedidos/ }));
    await u.click(within(hoja).getByRole('button', { name: /^Caja/ }));
    expect(within(hoja).getByRole('checkbox', { name: /Cancelar pedidos.*agregado/ })).toBeInTheDocument();
    expect(within(hoja).getByRole('checkbox', { name: /Configurar cajas.*quitado/ })).toBeInTheDocument();
    expect(within(hoja).getByRole('checkbox', { name: /^Pasar productos$/ })).toBeInTheDocument();
  });

  test('«Marcar todas» marca el área entera y mide 44 px', async () => {
    const u = userEvent.setup();
    montar();
    const hoja = await abrir(u, 'Cajero');
    await u.click(within(hoja).getByRole('button', { name: /^Caja/ }));
    const marcar = within(hoja).getByRole('button', { name: 'Marcar todas' });
    expect(alto(marcar)).toBeGreaterThanOrEqual(44);
    await u.click(marcar);
    expect(within(hoja).getByRole('checkbox', { name: /Configurar cajas/ })).toHaveAttribute('aria-checked', 'true');
    expect(within(hoja).getByRole('button', { name: /^Caja.*2 de 2/ })).toBeInTheDocument();
  });

  test('Cerrar con cambios pide confirmar y descartar no guarda', async () => {
    const u = userEvent.setup();
    montar();
    const hoja = await abrir(u, 'Cajero');
    await u.click(within(hoja).getByRole('button', { name: /Pedidos/ }));
    await u.click(within(hoja).getByRole('checkbox', { name: /Pasar productos/ }));
    await u.click(within(hoja).getByRole('button', { name: 'Cerrar' }));
    expect(await screen.findByText('¿Descartar los cambios?')).toBeInTheDocument();
    await u.click(screen.getByRole('button', { name: 'Descartar' }));
    await waitFor(() => expect(screen.queryByText('Cajero', { selector: '[role=dialog] *' })).toBeNull());
    expect(api.updateRole).not.toHaveBeenCalled();
  });

  test('Cerrar sin cambios cierra sin preguntar', async () => {
    const u = userEvent.setup();
    montar();
    const hoja = await abrir(u, 'Cajero');
    await u.click(within(hoja).getByRole('button', { name: 'Cerrar' }));
    expect(screen.queryByText('¿Descartar los cambios?')).toBeNull();
  });

  test('el administrador muestra «Administrar roles» fijo y con su motivo', async () => {
    const u = userEvent.setup();
    montar();
    const hoja = await abrir(u, 'Admin');
    await u.click(within(hoja).getByRole('button', { name: /Personal/ }));
    const fijo = within(hoja).getByRole('checkbox', { name: /Administrar roles/ });
    expect(fijo).toHaveAttribute('aria-checked', 'true');
    expect(fijo).toBeDisabled();
    expect(within(hoja).getByText('El administrador siempre puede administrar roles')).toBeInTheDocument();
    // «Marcar todas» / quitar todas no lo toca.
    await u.click(within(hoja).getByRole('button', { name: /Quitar todas/ }));
    expect(fijo).toHaveAttribute('aria-checked', 'true');
    // El admin no se inactiva ni se borra.
    expect(within(hoja).queryByRole('button', { name: 'Inactivar' })).toBeNull();
    expect(within(hoja).queryByRole('button', { name: 'Borrar' })).toBeNull();
  });
});

describe('acciones del rol', () => {
  test('«Volver a lo sugerido» en un sugerido ajustado', async () => {
    const u = userEvent.setup();
    montar();
    const hoja = await abrir(u, 'Cajero');
    await u.click(within(hoja).getByRole('button', { name: 'Volver a lo sugerido' }));
    await waitFor(() => expect(api.resetRole).toHaveBeenCalledWith(3, 5));
  });

  test('inactivar con personas activas muestra cuántas y lleva a Personal filtrado', async () => {
    const u = userEvent.setup();
    api.deactivateRole.mockRejectedValue(new ApiError(409, 'ROLE_IN_USE', 'Hay 3 personas activas con este rol.', 'r1'));
    const { onShowPeople } = montar();
    const hoja = await abrir(u, 'Cajero');
    await u.click(within(hoja).getByRole('button', { name: 'Inactivar' }));
    expect(await within(hoja).findByText('Hay 3 personas activas con este rol.')).toBeInTheDocument();
    await u.click(within(hoja).getByRole('button', { name: 'Ver a esas personas' }));
    expect(onShowPeople).toHaveBeenCalledWith(3);
  });

  test('un propio sin usuarios se reactiva y se borra con confirmación', async () => {
    const u = userEvent.setup();
    montar();
    const hoja = await abrir(u, 'Barista');
    expect(within(hoja).queryByRole('button', { name: 'Volver a lo sugerido' })).toBeNull();
    await u.click(within(hoja).getByRole('button', { name: 'Reactivar' }));
    await waitFor(() => expect(api.reactivateRole).toHaveBeenCalledWith(9, 2));
  });

  test('borrar pide confirmar con ConfirmSheet', async () => {
    const u = userEvent.setup();
    montar();
    const hoja = await abrir(u, 'Barista');
    await u.click(within(hoja).getByRole('button', { name: 'Borrar' }));
    expect(await screen.findByText('¿Borrar el rol Barista?')).toBeInTheDocument();
    expect(api.deleteRole).not.toHaveBeenCalled();
    const confirmar = screen.getAllByRole('button', { name: 'Borrar' }).find((b) => b.getAttribute('data-destructive') === 'true')!;
    await u.click(confirmar);
    await waitFor(() => expect(api.deleteRole).toHaveBeenCalledWith(9));
  });

  test('un sugerido no ofrece borrar', async () => {
    const u = userEvent.setup();
    montar();
    const hoja = await abrir(u, 'Cajero');
    expect(within(hoja).queryByRole('button', { name: 'Borrar' })).toBeNull();
  });
});

describe('rol propio nuevo', () => {
  test('nombre y «Copiar permisos de…» en un renglón, sin abrir el teclado solo', async () => {
    const u = userEvent.setup();
    montar({ creating: true });
    const hoja = await screen.findByRole('dialog');
    const nombre = within(hoja).getByPlaceholderText('Nombre del rol');
    expect(document.activeElement).not.toBe(nombre);
    const copiar = within(hoja).getByRole('button', { name: /Copiar permisos de/ });
    expect(nombre.closest('[data-row="name"]')).toBe(copiar.closest('[data-row="name"]'));

    const crear = within(hoja).getByRole('button', { name: 'Crear' });
    expect(crear).toBeDisabled();
    await u.click(copiar);
    await u.click(await screen.findByRole('button', { name: 'Cajero' }));
    await u.type(nombre, 'Encargado');
    await u.click(crear);
    await waitFor(() => expect(api.createRole).toHaveBeenCalledTimes(1));
    const arg = api.createRole.mock.calls[0][0];
    expect(arg.name).toBe('Encargado');
    expect([...arg.permissions].sort()).toEqual(['cash.operate', 'orders.cancel', 'orders.move_lines']);
  });

  test('un nombre repetido muestra el mensaje del servidor', async () => {
    const u = userEvent.setup();
    api.createRole.mockRejectedValue(new ApiError(409, 'ROLE_NAME_TAKEN', 'Ya hay un rol que se llama así.', 'r2'));
    montar({ creating: true });
    const hoja = await screen.findByRole('dialog');
    await u.type(within(hoja).getByPlaceholderText('Nombre del rol'), 'cajero');
    await u.click(within(hoja).getByRole('button', { name: 'Crear' }));
    expect(await screen.findAllByText('Ya hay un rol que se llama así.')).not.toHaveLength(0);
  });
});
