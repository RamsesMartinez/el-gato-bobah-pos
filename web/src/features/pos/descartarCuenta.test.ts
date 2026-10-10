import { beforeEach, describe, expect, test, vi } from 'vitest';
import { QueryClient } from '@tanstack/react-query';
import { ApiError } from '../../api/client';
import { descartarCuenta } from './descartarCuenta';

const api = vi.hoisted(() => ({ discardDraft: vi.fn(), getDraft: vi.fn() }));
vi.mock('../../api/pos', () => ({ posApi: api }));
const toasts = vi.hoisted(() => ({ create: vi.fn() }));
vi.mock('../../components/ui/toaster', () => ({ toaster: toasts }));

// DESCARTAR CON LA VERSIÓN QUE VIO LA TABLETA.
//
// Otra tableta agregó a la cuenta justo antes: el servidor responde 409 DRAFT_CHANGED y no descarta
// nada. La pantalla tiene que enseñar la cuenta como quedó y decirlo, no darla por descartada.

let qc: QueryClient;
beforeEach(() => {
  qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  vi.clearAllMocks();
});

describe('descartar una cuenta', () => {
  test('manda la versión que la tableta vio', async () => {
    api.discardDraft.mockResolvedValue(undefined);
    await expect(descartarCuenta(qc, 'd-1', 5)).resolves.toBe(true);
    expect(api.discardDraft).toHaveBeenCalledWith('d-1', 5);
    expect(toasts.create).not.toHaveBeenCalled();
  });

  test('si cambió en otra tableta, no la da por descartada: recarga la cuenta y avisa', async () => {
    api.discardDraft.mockRejectedValue(new ApiError(409, 'DRAFT_CHANGED', 'x', 'r'));
    const recargar = vi.spyOn(qc, 'invalidateQueries');
    await expect(descartarCuenta(qc, 'd-1', 5)).resolves.toBe(false);
    expect(recargar).toHaveBeenCalledWith({ queryKey: ['pos', 'draft', 'd-1'] });
    expect(recargar).toHaveBeenCalledWith({ queryKey: ['pos', 'accounts'] });
    expect(toasts.create).toHaveBeenCalledWith(expect.objectContaining({ title: 'La cuenta cambió en otra tableta' }));
    expect(toasts.create.mock.calls[0][0].description).toMatch(/no se descartó/i);
  });

  test('cualquier otro rechazo lo deja pasar a quien llama', async () => {
    api.discardDraft.mockRejectedValue(new ApiError(409, 'DRAFT_SENT', 'Ya se mandó a cocina', 'r'));
    await expect(descartarCuenta(qc, 'd-1', 5)).rejects.toBeInstanceOf(ApiError);
    expect(toasts.create).not.toHaveBeenCalled();
  });
});
