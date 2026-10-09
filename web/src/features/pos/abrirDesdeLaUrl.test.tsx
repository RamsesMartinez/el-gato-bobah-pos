import { renderHook, waitFor } from '@testing-library/react';
import { MemoryRouter, useLocation } from 'react-router';
import { useEffect, type ReactNode } from 'react';
import { beforeEach, describe, expect, test, vi } from 'vitest';
import { ApiError } from '../../api/client';
import { usePosStore } from '../../stores/pos';
import { leerCuentaDeLaUrl, useAbrirDesdeLaUrl } from './abrirDesdeLaUrl';

const api = vi.hoisted(() => ({ order: vi.fn(), getDraft: vi.fn() }));
vi.mock('../../api/pos', () => ({ posApi: api }));
const toasts = vi.hoisted(() => ({ create: vi.fn() }));
vi.mock('../../components/ui/toaster', () => ({ toaster: toasts }));

const UUID = '5f0c1a2b-3c4d-4e5f-8a9b-0c1d2e3f4a5b';

describe('leer la cuenta pedida en la URL', () => {
  test.each([
    ['', null],
    ['?pedido=12', { kind: 'order', id: 12 }],
    [`?cuenta=${UUID}`, { kind: 'draft', id: UUID }],
    ['?pedido=0', { kind: 'malformada' }],
    ['?pedido=12abc', { kind: 'malformada' }],
    ['?pedido=-3', { kind: 'malformada' }],
    ['?cuenta=no-es-uuid', { kind: 'malformada' }],
    ['?otra=1', null],
  ])('%s', (q, esperado) => {
    expect(leerCuentaDeLaUrl(new URLSearchParams(q))).toEqual(esperado);
  });
});

const ubicacion = { search: '' };
function Ruta() { const { search } = useLocation(); useEffect(() => { ubicacion.search = search; }, [search]); return null; }
function envolver(inicial: string) {
  return ({ children }: { children: ReactNode }) => (
    <MemoryRouter initialEntries={[`/pos${inicial}`]}><Ruta />{children}</MemoryRouter>
  );
}

beforeEach(() => {
  vi.resetAllMocks();
  usePosStore.setState({ selected: { kind: 'draft', id: 'la-de-antes' } });
});

describe('«Abrir cuenta» del tablero y del cierre (FR-019)', () => {
  test('?pedido=12 abre ese pedido y limpia el parámetro', async () => {
    api.order.mockResolvedValue({ id: 12 });
    renderHook(() => useAbrirDesdeLaUrl(), { wrapper: envolver('?pedido=12') });
    await waitFor(() => expect(usePosStore.getState().selected).toEqual({ kind: 'order', id: 12 }));
    expect(ubicacion.search).toBe('');
  });

  test('?cuenta=<uuid> abre esa cuenta en captura', async () => {
    api.getDraft.mockResolvedValue({ id: UUID, status: 'capturando', orderId: null });
    renderHook(() => useAbrirDesdeLaUrl(), { wrapper: envolver(`?cuenta=${UUID}`) });
    await waitFor(() => expect(usePosStore.getState().selected).toEqual({ kind: 'draft', id: UUID }));
  });

  // Una cuenta que ya se mandó abre su pedido, que es lo que sigue vivo.
  test('?cuenta= de una ya enviada abre su pedido', async () => {
    api.getDraft.mockResolvedValue({ id: UUID, status: 'enviada', orderId: 40 });
    renderHook(() => useAbrirDesdeLaUrl(), { wrapper: envolver(`?cuenta=${UUID}`) });
    await waitFor(() => expect(usePosStore.getState().selected).toEqual({ kind: 'order', id: 40 }));
  });

  test('un pedido que no existe lo dice y no cambia la cuenta abierta', async () => {
    api.order.mockRejectedValue(new ApiError(404, 'NOT_FOUND', 'x', 'r'));
    renderHook(() => useAbrirDesdeLaUrl(), { wrapper: envolver('?pedido=999') });
    await waitFor(() => expect(toasts.create).toHaveBeenCalledWith(expect.objectContaining({ title: 'Esa cuenta ya no existe' })));
    expect(usePosStore.getState().selected).toEqual({ kind: 'draft', id: 'la-de-antes' });
  });

  // Un parámetro presente y malformado se rechaza; nunca cae a «otra cuenta» en silencio.
  test('un valor malformado se rechaza igual, sin preguntarle al servidor', async () => {
    renderHook(() => useAbrirDesdeLaUrl(), { wrapper: envolver('?pedido=abc') });
    await waitFor(() => expect(toasts.create).toHaveBeenCalledWith(expect.objectContaining({ title: 'Esa cuenta ya no existe' })));
    expect(api.order).not.toHaveBeenCalled();
    expect(usePosStore.getState().selected).toEqual({ kind: 'draft', id: 'la-de-antes' });
    expect(ubicacion.search).toBe('');
  });

  test('sin parámetro no hace nada', () => {
    renderHook(() => useAbrirDesdeLaUrl(), { wrapper: envolver('') });
    expect(api.order).not.toHaveBeenCalled();
    expect(toasts.create).not.toHaveBeenCalled();
  });
});
