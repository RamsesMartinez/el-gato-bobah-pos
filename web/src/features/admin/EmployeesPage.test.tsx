import { beforeEach, describe, expect, test, vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../../components/ui/provider';
import { ApiError } from '../../api/client';
import { useSessionStore } from '../../stores/session';
import { EmployeesPage } from './EmployeesPage';

const api = vi.hoisted(() => ({
  users: vi.fn(),
  roles: vi.fn(),
  permissionCatalog: vi.fn(),
  createUser: vi.fn(),
  updateUser: vi.fn(),
  deactivateRole: vi.fn(),
}));
vi.mock('../../api/admin', () => ({ adminApi: api }));

const ROLES = [
  { id: 1, systemCode: 'admin', name: 'Admin', active: true, version: 1, activeUsers: 1, allUsers: 1, permissions: [], added: [], removed: [] },
  { id: 3, systemCode: 'cajero', name: 'Cajero', active: true, version: 1, activeUsers: 1, allUsers: 1, permissions: [], added: [], removed: [] },
  { id: 9, systemCode: '', name: 'Barista', active: false, version: 1, activeUsers: 1, allUsers: 1, permissions: [], added: [], removed: [] },
];

function sesion(permissions: string[]) {
  useSessionStore.setState({ user: { id: 1, companyId: 2, name: 'Dueño', role: '', permissions } });
}

function montar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><Provider><EmployeesPage /></Provider></QueryClientProvider>);
}

beforeEach(() => {
  vi.clearAllMocks();
  api.users.mockResolvedValue({ items: [
    { id: 1, name: 'Ana', username: 'ana', role: 'cajero', roleId: 3, isActive: true },
    { id: 2, name: 'Beto', username: 'beto', role: '', roleId: 9, isActive: true },
  ] });
  api.roles.mockResolvedValue({ items: ROLES });
  api.permissionCatalog.mockResolvedValue({ items: [] });
  api.createUser.mockResolvedValue({ id: 5 });
  api.updateUser.mockResolvedValue({ id: 2 });
  sesion(['employees.manage', 'roles.manage']);
});

const px = (el: Element) => parseInt(getComputedStyle(el).minHeight || '0', 10);

// En Empleados los campos del alta medían 40 px y «Gestionar» 36: bajo el mínimo de 44 px con el
// que un dedo acierta a la primera (validación con Playwright a 1024×600).
test('todo campo y botón de Empleados mide al menos 44 px', async () => {
  const { container } = montar();
  await screen.findByText('Ana');
  const chicos = [...container.querySelectorAll('input:not([type=checkbox])'), ...screen.getAllByRole('button')]
    .filter((el) => px(el) < 44)
    .map((el) => el.getAttribute('aria-label') || el.getAttribute('placeholder') || el.textContent);
  expect(chicos).toEqual([]);
  expect(px(screen.getAllByRole('button', { name: 'Gestionar' })[0])).toBeGreaterThanOrEqual(44);
});

test('la tabla muestra el nombre del rol, no su código', async () => {
  montar();
  const fila = (await screen.findByText('Ana')).closest('tr')!;
  expect(within(fila).getByText('Cajero')).toBeInTheDocument();
  expect(within(screen.getByText('Beto').closest('tr')!).getByText('Barista')).toBeInTheDocument();
});

test('con los dos permisos hay pestañas Personal y Roles', async () => {
  const u = userEvent.setup();
  montar();
  expect(await screen.findByRole('tab', { name: 'Personal' })).toBeInTheDocument();
  await u.click(screen.getByRole('tab', { name: 'Roles' }));
  expect(await screen.findByRole('button', { name: /^Barista/ })).toBeInTheDocument();
});

test('con una sola pestaña visible no se pinta la barra', async () => {
  sesion(['employees.manage']);
  montar();
  await screen.findByText('Ana');
  expect(screen.queryByRole('tablist')).toBeNull();
});

test('solo con roles.manage se ve la lista de roles y no se piden empleados', async () => {
  sesion(['roles.manage']);
  montar();
  expect(await screen.findByRole('button', { name: /^Cajero/ })).toBeInTheDocument();
  expect(screen.queryByRole('tablist')).toBeNull();
  expect(api.users).not.toHaveBeenCalled();
});

test('el alta nace sin rol y exige escoger uno activo', async () => {
  const u = userEvent.setup();
  montar();
  await screen.findByText('Ana');
  await u.type(screen.getByPlaceholderText('Nombre'), 'Carla');
  await u.type(screen.getByPlaceholderText('Usuario (login)'), 'carla');
  await u.type(screen.getByPlaceholderText(/Contraseña/), 'una-clave-larga');
  const crear = screen.getByRole('button', { name: 'Crear' });
  expect(crear).toBeDisabled();

  await u.click(screen.getByRole('button', { name: /Elige un rol/ }));
  const hoja = await screen.findByRole('dialog');
  // Un rol inactivo no se ofrece para dar de alta.
  expect(within(hoja).queryByRole('button', { name: /Barista/ })).toBeNull();
  await u.click(within(hoja).getByRole('button', { name: 'Cajero' }));
  expect(crear).toBeEnabled();
  await u.click(crear);
  await waitFor(() => expect(api.createUser).toHaveBeenCalledTimes(1));
  expect(api.createUser.mock.calls[0][0]).toMatchObject({ name: 'Carla', username: 'carla', roleId: 3 });
});

// El Picker es una hoja inferior que se abre ENCIMA del diálogo de edición: tocarla no puede
// contar como «tocar fuera» del diálogo, y cerrarla no puede cerrarlo.
test('el Picker de rol sobre el diálogo de edición responde y cerrarlo no cierra el diálogo', async () => {
  const u = userEvent.setup();
  montar();
  const fila = (await screen.findByText('Beto')).closest('tr')!;
  await u.click(within(fila).getByRole('button', { name: 'Gestionar' }));
  const dialogo = await screen.findByRole('dialog');
  // El rol actual sale aunque esté inactivo.
  const disparador = within(dialogo).getByRole('button', { name: /Barista/ });
  await u.click(disparador);
  const hojas = await screen.findAllByRole('dialog');
  const hoja = hojas[hojas.length - 1];
  expect(within(hoja).getByRole('button', { name: /Barista.*inactivo/ })).toBeInTheDocument();
  expect(within(hoja).queryByRole('button', { name: /^Admin/ })).toBeInTheDocument();
  await u.click(within(hoja).getByRole('button', { name: 'Cajero' }));

  await waitFor(() => expect(screen.getAllByRole('dialog')).toHaveLength(1));
  expect(screen.getByRole('button', { name: 'Guardar datos' })).toBeInTheDocument();
  expect(within(screen.getByRole('dialog')).getByRole('button', { name: /Cajero/ })).toBeInTheDocument();
  await u.click(screen.getByRole('button', { name: 'Guardar datos' }));
  await waitFor(() => expect(api.updateUser).toHaveBeenCalledTimes(1));
  expect(api.updateUser.mock.calls[0]).toEqual([2, expect.objectContaining({ roleId: 3 })]);
});

// El servidor rechaza (403) asignar un rol que puede más que quien actúa y tocar a un empleado que
// puede más que él: la pantalla no lo ofrece.
describe('nadie administra a quien puede más que él', () => {
  const ROLES_CON_PERMISOS = [
    { ...ROLES[0], permissions: ['employees.manage', 'roles.manage', 'cash.configure'] },
    { ...ROLES[1], permissions: ['cash.operate'] },
    { ...ROLES[2], active: true, permissions: ['cash.operate'] },
  ];
  beforeEach(() => {
    api.roles.mockResolvedValue({ items: ROLES_CON_PERMISOS });
    api.users.mockResolvedValue({ items: [
      { id: 1, name: 'Ana', username: 'ana', role: 'cajero', roleId: 3, isActive: true },
      { id: 4, name: 'Dora', username: 'dora', role: 'admin', roleId: 1, isActive: true },
    ] });
    sesion(['employees.manage', 'roles.manage', 'cash.operate']);
  });

  test('el alta solo ofrece roles que caben en los permisos de la sesión', async () => {
    const u = userEvent.setup();
    montar();
    await screen.findByText('Ana');
    await u.click(screen.getByRole('button', { name: /Elige un rol/ }));
    const hoja = await screen.findByRole('dialog');
    expect(within(hoja).getByRole('button', { name: 'Cajero' })).toBeInTheDocument();
    expect(within(hoja).queryByRole('button', { name: /^Admin/ })).toBeNull();
  });

  test('al editar a alguien que cabe, el Picker no ofrece un rol que puede más', async () => {
    const u = userEvent.setup();
    montar();
    const fila = (await screen.findByText('Ana')).closest('tr')!;
    await u.click(within(fila).getByRole('button', { name: 'Gestionar' }));
    const dialogo = await screen.findByRole('dialog');
    await u.click(within(dialogo).getByRole('button', { name: /Cajero/ }));
    const hojas = await screen.findAllByRole('dialog');
    expect(within(hojas[hojas.length - 1]).queryByRole('button', { name: /^Admin/ })).toBeNull();
  });

  test('a quien puede más no se le editan datos, contraseña ni PIN', async () => {
    const u = userEvent.setup();
    montar();
    const fila = (await screen.findByText('Dora')).closest('tr')!;
    await u.click(within(fila).getByRole('button', { name: 'Gestionar' }));
    const dialogo = await screen.findByRole('dialog');
    expect(within(dialogo).getByText('Esta persona tiene permisos que tu usuario no tiene: no puedes cambiar sus datos, contraseña ni PIN.')).toBeInTheDocument();
    expect(within(dialogo).queryByRole('button', { name: 'Guardar datos' })).toBeNull();
    expect(within(dialogo).queryByRole('button', { name: 'Restablecer' })).toBeNull();
    expect(within(dialogo).queryByRole('button', { name: 'Fijar PIN' })).toBeNull();
    // La sesión no configura asistencia: el horario no se ofrece (antes abría una hoja vacía y
    // editable con un 403 detrás, que decía que la persona no tenía horario).
    expect(within(dialogo).queryByRole('button', { name: 'Horario y asistencia' })).toBeNull();
  });

  test('con «configurar asistencia» sí se ofrece el horario', async () => {
    sesion(['employees.manage', 'roles.manage', 'cash.operate', 'attendance.configure']);
    const u = userEvent.setup();
    montar();
    const fila = (await screen.findByText('Ana')).closest('tr')!;
    await u.click(within(fila).getByRole('button', { name: 'Gestionar' }));
    const dialogo = await screen.findByRole('dialog');
    expect(within(dialogo).getByRole('button', { name: 'Horario y asistencia' })).toBeInTheDocument();
  });
});

// Con el filtro «Solo <rol>» y nadie con ese rol, la tabla quedaba vacía sin decir nada.
test('el filtro por rol sin nadie dice «Nadie tiene este rol» y ofrece quitarlo', async () => {
  const u = userEvent.setup();
  api.roles.mockResolvedValue({ items: [...ROLES, { ...ROLES[2], id: 12, name: 'Mesero', active: true, activeUsers: 0, allUsers: 0 }] });
  montar();
  await u.click(await screen.findByRole('tab', { name: 'Roles' }));
  // Se llega a Personal filtrado por «Ver a esas personas».
  api.deactivateRole.mockRejectedValue(new ApiError(409, 'ROLE_IN_USE', 'Hay personas.', 'r'));
  await u.click(await screen.findByRole('button', { name: /^Mesero/ }));
  const hoja = await screen.findByRole('dialog');
  await u.click(within(hoja).getByRole('button', { name: 'Inactivar' }));
  await u.click(await within(hoja).findByRole('button', { name: 'Ver a esas personas' }));
  expect(await screen.findByText('Nadie tiene este rol')).toBeInTheDocument();
  await u.click(screen.getByRole('button', { name: 'Quitar el filtro' }));
  expect(await screen.findByText('Ana')).toBeInTheDocument();
  expect(screen.queryByText('Nadie tiene este rol')).toBeNull();
});

// Tocar el texto «Activo» no movía el interruptor, y «Guardar datos» decía «Guardado» con la persona
// todavía activa: quien administra creía que la había dado de baja.
test('todo el renglón «Activo» es el control y mide al menos 44 px', async () => {
  const u = userEvent.setup();
  montar();
  const fila = (await screen.findByText('Beto')).closest('tr')!;
  await u.click(within(fila).getByRole('button', { name: 'Gestionar' }));
  const dialogo = await screen.findByRole('dialog');
  const renglon = within(dialogo).getByText('Activo').closest('label')!;
  expect(renglon).not.toBeNull();
  expect(px(renglon)).toBeGreaterThanOrEqual(44);
  await u.click(within(dialogo).getByText('Activo'));
  await u.click(within(dialogo).getByRole('button', { name: 'Guardar datos' }));
  await waitFor(() => expect(api.updateUser).toHaveBeenCalledTimes(1));
  expect(api.updateUser.mock.calls[0]).toEqual([2, expect.objectContaining({ isActive: false })]);
});
