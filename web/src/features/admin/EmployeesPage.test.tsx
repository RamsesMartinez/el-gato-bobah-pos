import { beforeEach, expect, test, vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../../components/ui/provider';
import { useSessionStore } from '../../stores/session';
import { EmployeesPage } from './EmployeesPage';

const api = vi.hoisted(() => ({
  users: vi.fn(),
  roles: vi.fn(),
  permissionCatalog: vi.fn(),
  createUser: vi.fn(),
  updateUser: vi.fn(),
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
