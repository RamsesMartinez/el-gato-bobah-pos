import { beforeEach, describe, expect, test, vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../../components/ui/provider';
import { ApiError } from '../../api/client';
import { toaster } from '../../components/ui/toaster';
import { useSessionStore } from '../../stores/session';
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
  const onShowPeople = vi.fn();
  const r = render(
    <QueryClientProvider client={qc}><Provider>
      <RolesPage onShowPeople={onShowPeople} {...props} />
    </Provider></QueryClientProvider>,
  );
  return { onShowPeople, qc, ...r };
}

function sesion(permissions: string[]) {
  useSessionStore.setState({ user: { id: 1, companyId: 2, name: 'Dueña', role: '', permissions } });
}

const alto = (el: Element) => parseInt(getComputedStyle(el).minHeight || '0', 10);

const editor = () => screen.findByRole('region', { name: 'Permisos del rol' });
const fila = (name: string) => screen.findByRole('button', { name: new RegExp(`^${name}`) });

async function abrir(u: ReturnType<typeof userEvent.setup>, name: string) {
  const row = await fila(name);
  await u.click(row);
  await waitFor(() => expect(row).toHaveAttribute('aria-current', 'true'));
  return editor();
}

async function nuevo(u: ReturnType<typeof userEvent.setup>) {
  await u.click(await screen.findByRole('button', { name: /Rol propio/ }));
  return editor();
}

// columnas cuenta las pistas de grid-template-columns, con o sin repeat().
function columnas(v: string): number {
  const rep = v.match(/^repeat\((\d+),/);
  if (rep) return Number(rep[1]);
  let depth = 0, n = v.trim() ? 1 : 0;
  for (const ch of v.trim()) {
    if (ch === '(') depth++;
    else if (ch === ')') depth--;
    else if (ch === ' ' && depth === 0) n++;
  }
  return n;
}

const area = (ed: HTMLElement, name: string) => within(ed).getByRole('group', { name });

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
  sesion(ALL);
});

describe('lista de roles', () => {
  test('cada fila dice personas y permisos, con a lo más dos chips: tipo y estado', async () => {
    montar();
    const cajero = await fila('Cajero');
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

// Opción B «Dos paneles» (#243): la lista y el rol a la vez, sin hoja que tape la lista.
describe('dos paneles', () => {
  test('al abrir queda elegido el primer rol, marcado en la lista, sin hoja encima', async () => {
    montar();
    const ed = await editor();
    expect(within(ed).getByText('Admin')).toBeInTheDocument();
    expect(screen.queryByRole('dialog')).toBeNull();
    const admin = await fila('Admin');
    expect(admin).toHaveAttribute('aria-current', 'true');
    expect(getComputedStyle(admin).borderLeftWidth).toBe('4px');
    expect(screen.getByRole('button', { name: /^Cajero/ })).not.toHaveAttribute('aria-current');
  });

  test('los permisos van en dos columnas y todas las áreas están abiertas', async () => {
    const u = userEvent.setup();
    montar();
    const ed = await abrir(u, 'Cajero');
    for (const g of ['Pedidos', 'Caja', 'Personal']) {
      const cols = getComputedStyle(within(area(ed, g)).getByTestId('permisos-del-area')).gridTemplateColumns;
      expect(columnas(cols)).toBeGreaterThanOrEqual(2);
    }
    expect(within(ed).getAllByRole('checkbox')).toHaveLength(CATALOG.length);
    for (const c of within(ed).getAllByRole('checkbox')) expect(alto(c)).toBeGreaterThanOrEqual(44);
  });

  test('en Pedidos se dice que tomar pedidos y cobrar lo puede cualquier rol, sin casilla', async () => {
    const u = userEvent.setup();
    montar();
    const ed = await abrir(u, 'Cajero');
    expect(within(area(ed, 'Pedidos')).getByText('Tomar pedidos y cobrar lo puede cualquier rol')).toBeInTheDocument();
    expect(within(area(ed, 'Caja')).queryByText('Tomar pedidos y cobrar lo puede cualquier rol')).toBeNull();
    expect(within(ed).queryByRole('checkbox', { name: /Tomar pedidos/ })).toBeNull();
  });

  test('cambiar de rol con cambios pide descartar; seguir editando los conserva', async () => {
    const u = userEvent.setup();
    montar();
    let ed = await abrir(u, 'Cajero');
    await u.click(within(ed).getByRole('checkbox', { name: /Pasar productos/ }));
    await u.click(screen.getByRole('button', { name: /^Barista/ }));
    expect(await screen.findByText('¿Descartar los cambios?')).toBeInTheDocument();
    await u.click(screen.getByRole('button', { name: 'Seguir editando' }));
    ed = await editor();
    expect(within(ed).getByText('Cajero')).toBeInTheDocument();
    expect(within(ed).getByText('1 cambio')).toBeInTheDocument();

    await u.click(screen.getByRole('button', { name: /^Barista/ }));
    await u.click(await screen.findByRole('button', { name: 'Descartar' }));
    await within(await editor()).findByDisplayValue('Barista');
    expect(screen.getByRole('button', { name: /^Barista/ })).toHaveAttribute('aria-current', 'true');
    expect(api.updateRole).not.toHaveBeenCalled();
  });

  test('cambiar de rol sin cambios no pregunta', async () => {
    const u = userEvent.setup();
    montar();
    await abrir(u, 'Cajero');
    await u.click(screen.getByRole('button', { name: /^Barista/ }));
    expect(screen.queryByText('¿Descartar los cambios?')).toBeNull();
    await within(await editor()).findByDisplayValue('Barista');
  });

  test('«+ Rol propio» con cambios también pide descartar', async () => {
    const u = userEvent.setup();
    montar();
    const ed = await abrir(u, 'Cajero');
    await u.click(within(ed).getByRole('checkbox', { name: /Pasar productos/ }));
    await u.click(screen.getByRole('button', { name: /Rol propio/ }));
    expect(await screen.findByText('¿Descartar los cambios?')).toBeInTheDocument();
  });

  test('la pestaña recibe las pestañas de Empleados arriba de la lista', async () => {
    montar({ tabs: <span>pestañas</span> });
    expect(await screen.findByText('pestañas')).toBeInTheDocument();
  });
});

describe('editor del rol', () => {
  test('cada área dice «n de m» y el ajuste de un sugerido con palabras', async () => {
    const u = userEvent.setup();
    montar();
    const ed = await abrir(u, 'Cajero');
    expect(within(area(ed, 'Pedidos')).getByText('2 de 2')).toBeInTheDocument();
    expect(within(area(ed, 'Caja')).getByText('1 de 2')).toBeInTheDocument();
    // El ajuste de un sugerido se dice con palabras, no con «+1 −1».
    expect(within(ed).getByText('+1 agregado · −1 quitado')).toBeInTheDocument();
  });

  test('marcar y desmarcar cuenta «N cambios» y Guardar manda solo lo tocado', async () => {
    const u = userEvent.setup();
    montar();
    const ed = await abrir(u, 'Cajero');
    const guardar = within(ed).getByRole('button', { name: 'Guardar' });
    expect(guardar).toBeDisabled();
    expect(alto(guardar)).toBeGreaterThanOrEqual(44);

    await u.click(within(ed).getByRole('checkbox', { name: /Pasar productos/ }));
    expect(within(ed).getByText('1 cambio')).toBeInTheDocument();
    await u.click(within(ed).getByRole('checkbox', { name: /Configurar cajas/ }));
    expect(within(ed).getByText('2 cambios')).toBeInTheDocument();
    // Volver a tocar lo mismo no es un cambio.
    await u.click(within(ed).getByRole('checkbox', { name: /Pasar productos/ }));
    expect(within(ed).getByText('1 cambio')).toBeInTheDocument();

    await u.click(guardar);
    await waitFor(() => expect(api.updateRole).toHaveBeenCalledTimes(1));
    expect(api.updateRole).toHaveBeenCalledWith(3, { version: 5, add: ['cash.configure'] });
  });

  test('guardar deja el mismo rol elegido, ya con lo guardado', async () => {
    const u = userEvent.setup();
    montar();
    const ed = await abrir(u, 'Cajero');
    await u.click(within(ed).getByRole('checkbox', { name: /Configurar cajas/ }));
    api.roles.mockResolvedValue({ items: [ADMIN, { ...CAJERO, version: 6, permissions: [...CAJERO.permissions, 'cash.configure'], removed: [] }, BARISTA] });
    await u.click(within(ed).getByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(api.roles).toHaveBeenCalledTimes(2));
    const otra = await editor();
    await waitFor(() => expect(within(otra).getByText('0 cambios')).toBeInTheDocument());
    expect(within(otra).getByText('Cajero')).toBeInTheDocument();
    expect(within(otra).getByRole('checkbox', { name: /Configurar cajas/ })).toHaveAttribute('aria-checked', 'true');
  });

  test('en un sugerido, lo que difiere del producto dice «agregado» o «quitado»', async () => {
    const u = userEvent.setup();
    montar();
    const ed = await abrir(u, 'Cajero');
    expect(within(ed).getByRole('checkbox', { name: /Cancelar pedidos.*agregado/ })).toBeInTheDocument();
    expect(within(ed).getByRole('checkbox', { name: /Configurar cajas.*quitado/ })).toBeInTheDocument();
    expect(within(ed).getByRole('checkbox', { name: /^Pasar productos$/ })).toBeInTheDocument();
  });

  test('«Marcar todas» marca el área entera y mide 44 px', async () => {
    const u = userEvent.setup();
    montar();
    const ed = await abrir(u, 'Cajero');
    const caja = area(ed, 'Caja');
    const marcar = within(caja).getByRole('button', { name: 'Marcar todas' });
    expect(alto(marcar)).toBeGreaterThanOrEqual(44);
    await u.click(marcar);
    expect(within(ed).getByRole('checkbox', { name: /Configurar cajas/ })).toHaveAttribute('aria-checked', 'true');
    expect(within(caja).getByText('2 de 2')).toBeInTheDocument();
  });

  test('el administrador muestra «Administrar roles» fijo y con su motivo', async () => {
    const u = userEvent.setup();
    montar();
    const ed = await abrir(u, 'Admin');
    const fijo = within(ed).getByRole('checkbox', { name: /Administrar roles/ });
    expect(fijo).toHaveAttribute('aria-checked', 'true');
    expect(fijo).toBeDisabled();
    expect(within(ed).getByText('El administrador siempre puede administrar roles')).toBeInTheDocument();
    // «Marcar todas» / quitar todas no lo toca.
    await u.click(within(area(ed, 'Personal')).getByRole('button', { name: 'Quitar todas' }));
    expect(fijo).toHaveAttribute('aria-checked', 'true');
    // El admin no se inactiva ni se borra.
    expect(within(ed).queryByRole('button', { name: 'Inactivar' })).toBeNull();
    expect(within(ed).queryByRole('button', { name: 'Borrar' })).toBeNull();
  });
});

describe('acciones del rol', () => {
  test('«Volver a lo sugerido» en un sugerido ajustado', async () => {
    const u = userEvent.setup();
    montar();
    const ed = await abrir(u, 'Cajero');
    await u.click(within(ed).getByRole('button', { name: 'Volver a lo sugerido' }));
    // Deshace los ajustes de todas las personas con ese rol: se confirma antes.
    expect(await screen.findByText('¿Regresar Cajero a lo sugerido?')).toBeInTheDocument();
    expect(api.resetRole).not.toHaveBeenCalled();
    await u.click(screen.getByRole('button', { name: 'Cancelar' }));
    expect(api.resetRole).not.toHaveBeenCalled();
    await u.click(within(ed).getByRole('button', { name: 'Volver a lo sugerido' }));
    await u.click(await screen.findByRole('button', { name: 'Regresar a lo sugerido' }));
    await waitFor(() => expect(api.resetRole).toHaveBeenCalledWith(3, 5));
  });

  test('un propio que ya tuvo alguien dice por qué no se borra', async () => {
    const u = userEvent.setup();
    api.roles.mockResolvedValue({ items: [ADMIN, CAJERO, { ...BARISTA, allUsers: 2 }] });
    montar();
    const ed = await abrir(u, 'Barista');
    expect(within(ed).queryByRole('button', { name: 'Borrar' })).toBeNull();
    expect(within(ed).getByText('Alguien lo tiene asignado (aunque esté dado de baja): solo se puede inactivar')).toBeInTheDocument();
  });

  test('inactivar con personas activas muestra cuántas y lleva a Personal filtrado', async () => {
    const u = userEvent.setup();
    api.deactivateRole.mockRejectedValue(new ApiError(409, 'ROLE_IN_USE', 'Hay 3 personas activas con este rol.', 'r1'));
    const { onShowPeople } = montar();
    const ed = await abrir(u, 'Cajero');
    await u.click(within(ed).getByRole('button', { name: 'Inactivar' }));
    expect(await within(ed).findByText('Hay 3 personas activas con este rol.')).toBeInTheDocument();
    await u.click(within(ed).getByRole('button', { name: 'Ver a esas personas' }));
    expect(onShowPeople).toHaveBeenCalledWith(3);
  });

  test('un propio sin usuarios se reactiva y se borra con confirmación', async () => {
    const u = userEvent.setup();
    montar();
    const ed = await abrir(u, 'Barista');
    expect(within(ed).queryByRole('button', { name: 'Volver a lo sugerido' })).toBeNull();
    await u.click(within(ed).getByRole('button', { name: 'Reactivar' }));
    await waitFor(() => expect(api.reactivateRole).toHaveBeenCalledWith(9, 2));
  });

  test('borrar pide confirmar con ConfirmSheet y deja elegido el primer rol', async () => {
    const u = userEvent.setup();
    montar();
    const ed = await abrir(u, 'Barista');
    await u.click(within(ed).getByRole('button', { name: 'Borrar' }));
    expect(await screen.findByText('¿Borrar el rol Barista?')).toBeInTheDocument();
    expect(api.deleteRole).not.toHaveBeenCalled();
    api.roles.mockResolvedValue({ items: [ADMIN, CAJERO] });
    const confirmar = screen.getAllByRole('button', { name: 'Borrar' }).find((b) => b.getAttribute('data-destructive') === 'true')!;
    await u.click(confirmar);
    await waitFor(() => expect(api.deleteRole).toHaveBeenCalledWith(9));
    await waitFor(() => expect(screen.queryByRole('button', { name: /^Barista/ })).toBeNull());
    expect(within(await editor()).getByText('Admin')).toBeInTheDocument();
  });

  test('un sugerido no ofrece borrar', async () => {
    const u = userEvent.setup();
    montar();
    const ed = await abrir(u, 'Cajero');
    expect(within(ed).queryByRole('button', { name: 'Borrar' })).toBeNull();
  });
});

describe('rol propio nuevo', () => {
  test('nombre y «Copiar permisos de…» en un renglón, sin abrir el teclado solo', async () => {
    const u = userEvent.setup();
    montar();
    const ed = await nuevo(u);
    expect(screen.queryByRole('dialog')).toBeNull();
    const nombre = within(ed).getByPlaceholderText('Nombre del rol');
    expect(document.activeElement).not.toBe(nombre);
    const copiar = within(ed).getByRole('button', { name: /Copiar permisos de/ });
    expect(nombre.closest('[data-row="name"]')).toBe(copiar.closest('[data-row="name"]'));

    const crear = within(ed).getByRole('button', { name: 'Crear' });
    expect(crear).toBeDisabled();
    await u.click(copiar);
    await u.click(await screen.findByRole('button', { name: 'Cajero' }));
    await u.type(nombre, 'Encargado');
    api.roles.mockResolvedValue({ items: [ADMIN, CAJERO, BARISTA, { ...BARISTA, id: 20, name: 'Encargado', active: true }] });
    await u.click(crear);
    await waitFor(() => expect(api.createRole).toHaveBeenCalledTimes(1));
    const arg = api.createRole.mock.calls[0][0];
    expect(arg.name).toBe('Encargado');
    expect([...arg.permissions].sort()).toEqual(['cash.operate', 'orders.cancel', 'orders.move_lines']);
    // El rol recién creado queda elegido.
    await waitFor(() => expect(screen.getByRole('button', { name: /^Encargado/ })).toHaveAttribute('aria-current', 'true'));
  });

  test('un nombre vacío no se crea', async () => {
    const u = userEvent.setup();
    montar();
    const ed = await nuevo(u);
    await u.click(within(ed).getByRole('checkbox', { name: /Pasar productos/ }));
    await u.type(within(ed).getByPlaceholderText('Nombre del rol'), '   ');
    expect(within(ed).getByRole('button', { name: 'Crear' })).toBeDisabled();
  });

  test('un nombre repetido muestra el mensaje del servidor', async () => {
    const u = userEvent.setup();
    api.createRole.mockRejectedValue(new ApiError(409, 'ROLE_NAME_TAKEN', 'Ya hay un rol que se llama así.', 'r2'));
    montar();
    const ed = await nuevo(u);
    await u.type(within(ed).getByPlaceholderText('Nombre del rol'), 'cajero');
    await u.click(within(ed).getByRole('button', { name: 'Crear' }));
    expect(await screen.findAllByText('Ya hay un rol que se llama así.')).not.toHaveLength(0);
  });
});

// El editor calculaba la diferencia contra el rol RECARGADO: la lista ['roles'] se vuelve a pedir sola
// (al volver al frente, o al guardar otra cosa) y lo que otra persona agregó mientras el rol estaba
// abierto viajaba como «quitado», con la versión nueva, y el servidor lo aceptaba.
describe('el editor compara contra lo que mostró al abrir', () => {
  test('un permiso que otra persona agregó con el rol abierto no viaja como quitado', async () => {
    const u = userEvent.setup();
    const { qc } = montar();
    const ed = await abrir(u, 'Cajero');
    // Otra persona le agrega «Configurar cajas» al Cajero.
    api.roles.mockResolvedValue({ items: [ADMIN, { ...CAJERO, version: 6, permissions: [...CAJERO.permissions, 'cash.configure'], removed: [] }, BARISTA] });
    await qc.invalidateQueries({ queryKey: ['roles'] });
    await waitFor(() => expect(api.roles).toHaveBeenCalledTimes(2));
    // Sin tocar nada, el editor no dice que hay cambios.
    expect(within(ed).getByText('0 cambios')).toBeInTheDocument();

    await u.click(within(ed).getByRole('checkbox', { name: /Administrar empleados/ }));
    await u.click(within(ed).getByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(api.updateRole).toHaveBeenCalledTimes(1));
    expect(api.updateRole).toHaveBeenCalledWith(3, { version: 5, add: ['employees.manage'] });
  });

  test('ROLE_CHANGED deja de editar, refresca el rol y avisa', async () => {
    const u = userEvent.setup();
    const aviso = vi.spyOn(toaster, 'create');
    api.updateRole.mockRejectedValue(new ApiError(409, 'ROLE_CHANGED', 'El rol cambió.', 'r9'));
    montar();
    const ed = await abrir(u, 'Cajero');
    await u.click(within(ed).getByRole('checkbox', { name: /Pasar productos/ }));
    api.roles.mockResolvedValue({ items: [ADMIN, { ...CAJERO, version: 6 }, BARISTA] });
    await u.click(within(ed).getByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(aviso).toHaveBeenCalledWith(expect.objectContaining({ title: 'Alguien cambió este rol; ya se ve como quedó' })));
    await waitFor(() => expect(api.roles).toHaveBeenCalledTimes(2));
    // Lo marcado sobre la versión vieja se descarta: el editor vuelve a 0 cambios.
    await waitFor(async () => expect(within(await editor()).getByText('0 cambios')).toBeInTheDocument());
    expect(api.updateRole).toHaveBeenCalledTimes(1);
  });
});

describe('acciones con cambios sin guardar', () => {
  test('«Volver a lo sugerido» e «Inactivar» no se ofrecen mientras hay cambios', async () => {
    const u = userEvent.setup();
    montar();
    const ed = await abrir(u, 'Cajero');
    await u.click(within(ed).getByRole('checkbox', { name: /Pasar productos/ }));
    expect(within(ed).getByRole('button', { name: 'Volver a lo sugerido' })).toBeDisabled();
    expect(within(ed).getByRole('button', { name: 'Inactivar' })).toBeDisabled();
    expect(within(ed).getByText('Guarda o descarta los cambios primero')).toBeInTheDocument();
  });
});

test('sin administrar empleados, el rol en uso no ofrece «Ver a esas personas»', async () => {
  const u = userEvent.setup();
  sesion(ALL.filter((p) => p !== 'employees.manage'));
  api.deactivateRole.mockRejectedValue(new ApiError(409, 'ROLE_IN_USE', 'Hay 3 personas activas con este rol.', 'r1'));
  montar();
  const ed = await abrir(u, 'Cajero');
  await u.click(within(ed).getByRole('button', { name: 'Inactivar' }));
  expect(await within(ed).findByText('Hay 3 personas activas con este rol.')).toBeInTheDocument();
  expect(within(ed).queryByRole('button', { name: 'Ver a esas personas' })).toBeNull();
});

test('un rol propio con el nombre en blanco no se puede guardar', async () => {
  const u = userEvent.setup();
  montar();
  const ed = await abrir(u, 'Barista');
  const nombre = within(ed).getByPlaceholderText('Nombre del rol');
  await u.clear(nombre);
  await u.type(nombre, '   ');
  expect(within(ed).getByRole('button', { name: 'Guardar' })).toBeDisabled();
});

// El servidor rechaza dar lo que uno no tiene y tocar un rol que puede más (403): el editor no lo ofrece.
describe('nadie administra lo que puede más que él', () => {
  const MIA = ['orders.cancel', 'orders.move_lines', 'cash.operate', 'employees.manage', 'roles.manage'];

  test('lo que la sesión no tiene no se puede marcar, ni con «Marcar todas»', async () => {
    const u = userEvent.setup();
    sesion(MIA);
    montar();
    const ed = await abrir(u, 'Cajero');
    const ajeno = within(ed).getByRole('checkbox', { name: /Configurar cajas/ });
    expect(ajeno).toBeDisabled();
    expect(within(ed).getAllByText('Tu usuario no lo tiene').length).toBeGreaterThan(0);
    await u.click(within(area(ed, 'Caja')).getByRole('button', { name: 'Marcar todas' }));
    expect(ajeno).toHaveAttribute('aria-checked', 'false');
    // Volver a lo sugerido le daría «Configurar cajas»: no se ofrece.
    expect(within(ed).queryByRole('button', { name: 'Volver a lo sugerido' })).toBeNull();
  });

  test('un rol que puede más que la sesión se abre en solo lectura', async () => {
    const u = userEvent.setup();
    sesion(MIA);
    montar();
    const ed = await abrir(u, 'Admin');
    expect(within(ed).getByText('Este rol tiene permisos que tu usuario no tiene. Solo puedes verlo.')).toBeInTheDocument();
    expect(within(ed).queryByRole('button', { name: 'Guardar' })).toBeNull();
    expect(within(ed).queryByRole('button', { name: 'Inactivar' })).toBeNull();
    expect(within(ed).getByRole('checkbox', { name: /Pasar productos/ })).toBeDisabled();
    expect(within(ed).queryByRole('button', { name: /Quitar todas|Marcar todas/ })).toBeNull();
  });

  test('copiar permisos de un rol que puede más trae solo lo que la sesión tiene', async () => {
    const u = userEvent.setup();
    sesion(MIA);
    montar();
    const ed = await nuevo(u);
    await u.click(within(ed).getByRole('button', { name: /Copiar permisos de/ }));
    await u.click(await screen.findByRole('button', { name: 'Admin' }));
    await u.type(within(ed).getByPlaceholderText('Nombre del rol'), 'Encargado');
    await u.click(within(ed).getByRole('button', { name: 'Crear' }));
    await waitFor(() => expect(api.createRole).toHaveBeenCalledTimes(1));
    expect([...api.createRole.mock.calls[0][0].permissions].sort()).toEqual([...MIA].sort());
  });
});
