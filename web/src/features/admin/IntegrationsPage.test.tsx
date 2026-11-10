import { beforeEach, describe, expect, test, vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../../components/ui/provider';
import { useSessionStore } from '../../stores/session';
import { connectCommand, mcpUrl } from '../../api/integrations';
import { IntegrationsPage } from './IntegrationsPage';

const api = vi.hoisted(() => ({ list: vi.fn(), create: vi.fn(), revoke: vi.fn() }));
vi.mock('../../api/integrations', async (orig) => ({ ...(await orig<object>()), integrationsApi: api }));

const tok = (over: object) => ({
  id: 1, name: 'Claude de la laptop', prefix: 'egbpat_2_1_abcd', ownerId: 1, ownerName: 'Ana', scopes: ['read'],
  createdAt: '2026-11-01T00:00:00Z', expiresAt: '2027-02-01T00:00:00Z', lastUsedAt: null, revokedAt: null, status: 'active', ...over,
});

function montar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><Provider><IntegrationsPage /></Provider></QueryClientProvider>);
}

beforeEach(() => {
  vi.clearAllMocks();
  useSessionStore.setState({ user: { id: 1, companyId: 2, companySlug: 'gatobobah', name: 'Ana', role: 'admin', permissions: ['integrations.manage'] } });
  api.list.mockResolvedValue({ items: [
    tok({ id: 3, name: 'Viejo', status: 'revoked', revokedAt: '2026-11-02T00:00:00Z' }),
    tok({ id: 2, name: 'Laptop', scopes: ['read', 'write:expenses'] }),
  ] });
});

describe('IntegrationsPage', () => {
  test('activos primero; revocados y vencidos plegados', async () => {
    montar();
    expect(await screen.findByText('Laptop')).toBeVisible();
    expect(screen.queryByText('Viejo')).toBeNull();
    await userEvent.click(screen.getByRole('button', { name: /Ver revocados y vencidos \(1\)/ }));
    expect(await screen.findByText('Viejo')).toBeVisible();
  });

  test('la tarjeta no muestra el prefijo del token', async () => {
    montar();
    await screen.findByText('Laptop');
    expect(screen.queryByText(/egbpat_/)).toBeNull();
  });

  test('crear: nombre por omisión, consultar fijo, contraseña obligatoria, y el token no se cierra al tocar el fondo', async () => {
    api.create.mockResolvedValue({ ...tok({ id: 9, name: 'Claude de Ana' }), token: 'egbpat_2_9_' + 'a'.repeat(64) });
    montar();
    await userEvent.click(await screen.findByRole('button', { name: 'Crear token' }));
    const hoja = await screen.findByRole('dialog');
    expect(within(hoja).getByLabelText('Nombre')).toHaveValue('Claude de Ana');
    expect(within(hoja).getByText(/Siempre puede consultar/)).toBeVisible();
    expect(within(hoja).queryByRole('checkbox', { name: /Consultar/ })).toBeNull();
    const crear = within(hoja).getByRole('button', { name: 'Crear token' });
    expect(crear).toBeDisabled();
    await userEvent.click(within(hoja).getByRole('checkbox', { name: /Gastos/ }));
    await userEvent.type(within(hoja).getByLabelText('Tu contraseña'), 'secreta');
    await userEvent.click(crear);
    await waitFor(() => expect(api.create).toHaveBeenCalledWith({
      name: 'Claude de Ana', scopes: ['write:expenses'], expiresInDays: 90, password: 'secreta',
    }));
    expect(await screen.findByText('egbpat_2_9_' + 'a'.repeat(64))).toBeVisible();
    await userEvent.keyboard('{Escape}');
    expect(screen.getByText('egbpat_2_9_' + 'a'.repeat(64))).toBeVisible();
    await userEvent.click(screen.getByRole('button', { name: 'Ya lo copié' }));
    await waitFor(() => expect(screen.queryByText('egbpat_2_9_' + 'a'.repeat(64))).toBeNull());
  });

  test('revocar pide confirmación', async () => {
    api.revoke.mockResolvedValue(undefined);
    montar();
    await userEvent.click(await screen.findByRole('button', { name: 'Revocar Laptop' }));
    expect(api.revoke).not.toHaveBeenCalled();
    await userEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Revocar' }));
    await waitFor(() => expect(api.revoke).toHaveBeenCalledWith(2));
  });
});

describe('comando de conexión', () => {
  test('la URL sale de la API del build, o de la página en desarrollo', () => {
    expect(mcpUrl('https://api.elgatobobah.com/api/v1', 'https://app.elgatobobah.com')).toBe('https://api.elgatobobah.com/mcp');
    expect(mcpUrl('/api/v1', 'http://localhost:3072')).toBe('http://localhost:3072/mcp');
  });
  test('el token va por variable y con comillas simples', () => {
    const c = connectCommand('https://api.elgatobobah.com/mcp', 'gatobobah');
    expect(c).toContain('pos-gatobobah https://api.elgatobobah.com/mcp');
    expect(c).toContain("'Authorization: Bearer ${POS_MCP_TOKEN}'");
    expect(c).not.toMatch(/egbpat_/);
  });
});
