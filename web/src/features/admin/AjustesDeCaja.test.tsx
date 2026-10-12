import { vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../../components/ui/provider';

const api = vi.hoisted(() => ({
  cashConcepts: vi.fn(), updateCashConcept: vi.fn(), mergeCashConcept: vi.fn(),
  cardTerminals: vi.fn(), createCardTerminal: vi.fn(), updateCardTerminal: vi.fn(),
  cardCountModes: vi.fn(), setCardCountMode: vi.fn(),
  summaryEmails: vi.fn(), setSummaryEmails: vi.fn(),
  allCashRegisters: vi.fn(), setFixedFloat: vi.fn(),
}));
vi.mock('../../api/backoffice', () => ({ backofficeApi: api }));
vi.mock('../../components/ui/toaster', () => ({ toaster: { create: vi.fn() } }));

import { AjustesDeCaja } from './AjustesDeCaja';
import { useSessionStore } from '../../stores/session';

beforeEach(() => {
  api.cashConcepts.mockResolvedValue({ items: [
    { id: 1, name: 'Hielo', categoryId: null, categoryName: null, supplierId: null, supplierName: null },
    { id: 2, name: 'hielo bolsa', categoryId: null, categoryName: null, supplierId: null, supplierName: null },
  ] });
  api.cardTerminals.mockResolvedValue({ items: [{ id: 5, branchId: 1, branchName: 'Matriz', name: 'Getnet', archived: false }] });
  api.cardCountModes.mockResolvedValue({ items: [{ branchId: 1, name: 'Matriz', mode: 'auto' }] });
  api.summaryEmails.mockResolvedValue({ emails: ['dueno@ejemplo.com'] });
  for (const f of [api.updateCashConcept, api.mergeCashConcept, api.createCardTerminal, api.updateCardTerminal, api.setCardCountMode]) f.mockResolvedValue(null);
  api.setSummaryEmails.mockResolvedValue({ emails: [] });
  api.allCashRegisters.mockResolvedValue({ items: [
    { id: 1, name: 'Caja principal', isPrimary: true, isActive: true, fixedFloat: '1000' },
    { id: 2, name: 'Barra', isPrimary: false, isActive: true, fixedFloat: null },
  ] });
  api.setFixedFloat.mockResolvedValue(null);
  useSessionStore.setState({ user: { id: 1, companyId: 1, name: 'Gerente', role: 'gerente', permissions: ['cash.configure'] } });
});

function pinta() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<Provider><QueryClientProvider client={qc}><AjustesDeCaja /></QueryClientProvider></Provider>);
}

// Punto 9: el arqueo de tarjeta es un ajuste por sucursal, con dos botones, sin desplegable.
test('el modo de arqueo se cambia con un toque por sucursal', async () => {
  pinta();
  await userEvent.click(await screen.findByRole('button', { name: 'Por terminal' }));
  expect(api.setCardCountMode).toHaveBeenCalledWith(1, 'per_terminal');
});

// Punto 8: terminales por sucursal, con nombre propio.
test('agrega una terminal a la sucursal', async () => {
  pinta();
  await userEvent.type(await screen.findByLabelText('Nueva terminal en Matriz'), 'Hey');
  await userEvent.click(screen.getByRole('button', { name: 'Agregar terminal' }));
  expect(api.createCardTerminal).toHaveBeenCalledWith(1, 'Hey');
});

// Punto 3: juntar un concepto duplicado en otro pide destino y confirmación, y no está pegado a
// «Archivar».
test('juntar conceptos pide el destino y confirmar', async () => {
  pinta();
  const fila = (await screen.findByText('hielo bolsa')).closest('[data-concepto]') as HTMLElement;
  await userEvent.click(within(fila).getByRole('button', { name: 'Juntar con…' }));
  await userEvent.click(screen.getByRole('button', { name: /Elegir concepto/ }));
  await userEvent.click(await screen.findByRole('button', { name: /^Hielo$/ }));
  await userEvent.click(screen.getByRole('button', { name: 'Juntar' }));
  await waitFor(() => expect(api.mergeCashConcept).toHaveBeenCalledWith(2, 1));
});

// Decisión del 2026-10-09: el resumen va a los correos que se capturan aquí.
test('agrega un correo del resumen y lo guarda', async () => {
  pinta();
  await userEvent.type(await screen.findByLabelText('Correo nuevo'), 'conta@ejemplo.mx');
  await userEvent.click(screen.getByRole('button', { name: 'Agregar correo' }));
  await userEvent.click(screen.getByRole('button', { name: 'Guardar correos' }));
  expect(api.setSummaryEmails).toHaveBeenCalledWith(['dueno@ejemplo.com', 'conta@ejemplo.mx']);
});

// Una terminal se renombra (el banco cambia el aparato y el nombre deja de ser cierto).
test('renombra una terminal', async () => {
  pinta();
  const fila = (await screen.findByText('Getnet')).closest('[data-terminal]') as HTMLElement;
  await userEvent.click(within(fila).getByRole('button', { name: 'Renombrar' }));
  const campo = screen.getByLabelText('Nuevo nombre de Getnet');
  await userEvent.clear(campo);
  await userEvent.type(campo, 'Getnet barra');
  await userEvent.click(screen.getByRole('button', { name: 'Guardar' }));
  await waitFor(() => expect(api.updateCardTerminal).toHaveBeenCalledWith(5, { name: 'Getnet barra' }));
});

// Archivar una terminal pide confirmación, igual que un concepto: un toque de más dejaba a la
// sucursal sin la terminal con la que cobra.
test('archivar una terminal pide confirmar', async () => {
  pinta();
  const fila = (await screen.findByText('Getnet')).closest('[data-terminal]') as HTMLElement;
  await userEvent.click(within(fila).getByRole('button', { name: 'Archivar' }));
  expect(api.updateCardTerminal).not.toHaveBeenCalled();
  const hoja = await screen.findByRole('dialog');
  await userEvent.click(within(hoja).getByRole('button', { name: 'Archivar' }));
  await waitFor(() => expect(api.updateCardTerminal).toHaveBeenCalledWith(5, { archived: true }));
});

// Spec 035, opción B de las maquetas (plan.md, «Maquetas», fila «Ajustes › Caja: fondo fijo»): un
// renglón por caja que abre una hoja con teclado propio; el campo no abre el teclado del sistema.
describe('fondo fijo', () => {
  test('un renglón de 56 px por caja con su monto o «Sin configurar»', async () => {
    pinta();
    const fila = await screen.findByRole('button', { name: /Caja principal/ });
    expect(getComputedStyle(fila).minHeight).toBe('56px');
    expect(within(fila).getByText('$1,000')).toBeInTheDocument();
    expect(within(screen.getByRole('button', { name: /Barra/ })).getByText(/Sin configurar/)).toBeInTheDocument();
  });

  test('se teclea en la hoja y se guarda', async () => {
    pinta();
    await userEvent.click(await screen.findByRole('button', { name: /Barra/ }));
    const hoja = await screen.findByRole('dialog');
    const campo = within(hoja).getByLabelText('Fondo fijo');
    expect(campo).toHaveAttribute('readonly');
    expect(campo).toHaveAttribute('inputmode', 'none');
    for (const k of ['5', '0', '0']) await userEvent.click(within(hoja).getByRole('button', { name: k }));
    expect(campo).toHaveValue('500');
    await userEvent.click(within(hoja).getByRole('button', { name: 'Guardar' }));
    await waitFor(() => expect(api.setFixedFloat).toHaveBeenCalledWith(2, 500));
  });

  test('«Quitar fondo fijo» manda null', async () => {
    pinta();
    await userEvent.click(await screen.findByRole('button', { name: /Caja principal/ }));
    const hoja = await screen.findByRole('dialog');
    await userEvent.click(within(hoja).getByRole('button', { name: 'Quitar fondo fijo' }));
    await waitFor(() => expect(api.setFixedFloat).toHaveBeenCalledWith(1, null));
  });

  test('sin el permiso se ve el monto pero no se abre nada', async () => {
    useSessionStore.setState({ user: { id: 1, companyId: 1, name: 'Cajero', role: 'cajero', permissions: [] } });
    pinta();
    expect(await screen.findByText('Caja principal')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Caja principal/ })).toBeNull();
  });
});
