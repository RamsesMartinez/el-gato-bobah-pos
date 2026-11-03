import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { cleanup, render, screen, waitFor } from '@testing-library/react';

import { Provider } from '../../components/ui/provider';
import { AppearancePage } from './AppearancePage';

// Spec 045 (US5): pedir modo oscuro al sistema no cambiaba nada, porque el tema por omisión era
// «claro». Sin elección guardada manda el sistema; una elección guardada se respeta (B8).

const realMatchMedia = window.matchMedia;
function sistemaOscuro(dark: boolean) {
  window.matchMedia = ((query: string) => ({
    matches: dark && query.includes('dark'), media: query, onchange: null,
    addEventListener: () => {}, removeEventListener: () => {}, addListener: () => {}, removeListener: () => {}, dispatchEvent: () => false,
  })) as typeof window.matchMedia;
}
const montar = () => render(<Provider><AppearancePage /></Provider>);
const marcado = (name: string) => screen.getByRole('button', { name }).getAttribute('aria-pressed');

beforeEach(() => { try { localStorage.clear(); } catch { /* sin almacenamiento */ } document.documentElement.className = ''; });
afterEach(() => { cleanup(); window.matchMedia = realMatchMedia; vi.restoreAllMocks(); });

test('«Como el sistema» va primero y está elegida si nunca se eligió tema', async () => {
  sistemaOscuro(false);
  montar();
  const botones = screen.getAllByRole('button').map((b) => b.textContent?.trim()).filter((t) => ['Como el sistema', 'Claro', 'Oscuro'].includes(t ?? ''));
  expect(botones).toEqual(['Como el sistema', 'Claro', 'Oscuro']);
  await waitFor(() => expect(marcado('Como el sistema')).toBe('true'));
});

test('con el sistema en oscuro y sin elección, la app se pinta oscura', async () => {
  sistemaOscuro(true);
  montar();
  await waitFor(() => expect(document.documentElement).toHaveClass('dark'));
});

test('una elección guardada antes se respeta aunque el sistema diga otra cosa', async () => {
  sistemaOscuro(true);
  localStorage.setItem('theme', 'light');
  montar();
  await waitFor(() => expect(marcado('Claro')).toBe('true'));
  expect(document.documentElement).not.toHaveClass('dark');
});
