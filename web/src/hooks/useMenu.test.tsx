import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider, focusManager } from '@tanstack/react-query';
import { createElement, type ReactNode } from 'react';

import { posApi } from '../api/pos';
import type { Menu } from '../types/pos';
import { useMenu } from './useMenu';

// El 2026-10-10 la tableta siguió ofreciendo un extra que ya no existía. Estas pruebas fallan si la
// tableta deja de revisar el menú sola, o si una revisión sin red le quita el menú que tenía.
const menu = { version: 1, categories: [], products: [] } as unknown as Menu;

function envoltura(qc: QueryClient) {
  return ({ children }: { children: ReactNode }) => createElement(QueryClientProvider, { client: qc }, children);
}

describe('useMenu', () => {
  beforeEach(() => vi.useFakeTimers({ shouldAdvanceTime: true }));
  afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks(); focusManager.setFocused(undefined); });

  it('revisa el menú cada minuto', async () => {
    const spy = vi.spyOn(posApi, 'menu').mockResolvedValue(menu);
    const qc = new QueryClient();
    renderHook(() => useMenu(), { wrapper: envoltura(qc) });
    await waitFor(() => expect(spy).toHaveBeenCalledTimes(1));
    await act(async () => { await vi.advanceTimersByTimeAsync(60_000); });
    await waitFor(() => expect(spy).toHaveBeenCalledTimes(2));
  });

  it('revisa al volver al frente aunque haya revisado hace poco', async () => {
    const spy = vi.spyOn(posApi, 'menu').mockResolvedValue(menu);
    const qc = new QueryClient();
    renderHook(() => useMenu(), { wrapper: envoltura(qc) });
    await waitFor(() => expect(spy).toHaveBeenCalledTimes(1));
    act(() => { focusManager.setFocused(false); focusManager.setFocused(true); });
    await waitFor(() => expect(spy).toHaveBeenCalledTimes(2));
  });

  it('una revisión sin red conserva el menú que ya tenía', async () => {
    const spy = vi.spyOn(posApi, 'menu').mockResolvedValueOnce(menu).mockRejectedValue(new Error('sin red'));
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useMenu(), { wrapper: envoltura(qc) });
    await waitFor(() => expect(result.current.data).toEqual(menu));
    await act(async () => { await vi.advanceTimersByTimeAsync(60_000); });
    await waitFor(() => expect(spy).toHaveBeenCalledTimes(2));
    expect(result.current.data).toEqual(menu);
  });
});
