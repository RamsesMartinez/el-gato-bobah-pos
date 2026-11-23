import { describe, it, expect, vi } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { useSoldOut } from './useSoldOut';

const soldOut = vi.hoisted(() => ({ fn: vi.fn() }));
vi.mock('../api/pos', () => ({ posApi: { soldOut: soldOut.fn } }));

function wrap() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

describe('useSoldOut', () => {
  it('devuelve los agotados como conjunto', async () => {
    soldOut.fn.mockResolvedValue({ productIds: [3, 9] });
    const { result } = renderHook(() => useSoldOut(), { wrapper: wrap() });
    await waitFor(() => expect(result.current.has(9)).toBe(true));
    expect(result.current.has(4)).toBe(false);
  });

  it('una respuesta sin el arreglo (API anterior) es «nada agotado», no un error', async () => {
    soldOut.fn.mockResolvedValue({});
    const { result } = renderHook(() => useSoldOut(), { wrapper: wrap() });
    await waitFor(() => expect(soldOut.fn).toHaveBeenCalled());
    expect(result.current.size).toBe(0);
  });
});
