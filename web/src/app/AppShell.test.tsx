import { describe, expect, test, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { Provider } from '../components/ui/provider';
import { useSessionStore } from '../stores/session';
import { AppShell } from './AppShell';

vi.mock('../api/pos', () => ({ posApi: { logout: vi.fn() } }));
vi.mock('./SystemInfo', () => ({ SystemInfo: () => null }));

// «Salir» medía 46×32 (validación como usuario nuevo): bajo el mínimo de 44 px de la constitución.
describe('el menú lateral', () => {
  test('«Salir» mide al menos 44 px de alto', () => {
    useSessionStore.setState({ user: { id: 1, name: 'Ana', role: 'admin', permissions: [] } as never });
    render(<Provider><MemoryRouter><AppShell /></MemoryRouter></Provider>);
    const salir = screen.getByRole('button', { name: 'Salir' });
    expect(parseInt(getComputedStyle(salir).minHeight || '0', 10)).toBeGreaterThanOrEqual(44);
  });
});
