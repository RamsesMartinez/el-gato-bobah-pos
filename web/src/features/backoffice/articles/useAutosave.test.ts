import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { ApiError } from '../../../api/client';
import { useAutosave, type AutosaveOptions } from './useAutosave';

// La cola del autoguardado de la ficha (spec 043, T007). Cada caso nombra el borde del plan que cubre.

type Campos = { name: string; min: string };

function montar(over: Partial<AutosaveOptions<Campos>> = {}) {
  const save = vi.fn<AutosaveOptions<Campos>['save']>(async () => ({ updatedAt: 'v-nueva' }));
  const opts: AutosaveOptions<Campos> = {
    initial: { name: 'Hielo', min: '10' },
    version: 'v1',
    save,
    validate: (k, v) => (k === 'name' && v.trim() === '' ? 'El nombre no puede quedar vacío.' : k === 'min' && Number(v) < 0 ? 'El mínimo no puede ser negativo.' : null),
    complete: (k, v) => k !== 'min' || /^\d+(\.\d+)?$/.test(v),
    onReject: vi.fn(),
    onStale: vi.fn(),
    ...over,
  };
  const hook = renderHook(() => useAutosave(opts));
  return { hook, save: (over.save as typeof save) ?? save, opts };
}

const espera = async (ms: number) => { await act(async () => { await vi.advanceTimersByTimeAsync(ms); }); };

beforeEach(() => { vi.useFakeTimers(); });
afterEach(() => { vi.useRealTimers(); });

describe('useAutosave', () => {
  it('dos cambios seguidos salen uno después del otro, con la versión que devolvió el anterior (E1)', async () => {
    let resolver: (v: { updatedAt: string }) => void = () => {};
    const save = vi.fn()
      .mockImplementationOnce(() => new Promise((r) => { resolver = r; }))
      .mockResolvedValueOnce({ updatedAt: 'v3' });
    const { hook } = montar({ save });
    act(() => { hook.result.current.commit('name', 'Hielo grande'); hook.result.current.commit('min', '20'); });
    expect(save).toHaveBeenCalledTimes(1);
    expect(save).toHaveBeenLastCalledWith({ name: 'Hielo grande' }, 'v1');
    expect(hook.result.current.status).toBe('saving');
    await act(async () => { resolver({ updatedAt: 'v2' }); });
    expect(save).toHaveBeenCalledTimes(2);
    expect(save).toHaveBeenLastCalledWith({ min: '20' }, 'v2');
    await espera(0);
    expect(hook.result.current.status).toBe('saved');
  });

  it('al fallar por la red conserva lo escrito y «Reintentar» manda lo mismo (E2)', async () => {
    const save = vi.fn().mockRejectedValueOnce(new TypeError('Failed to fetch')).mockResolvedValueOnce({ updatedAt: 'v2' });
    const { hook, opts } = montar({ save });
    await act(async () => { hook.result.current.commit('min', '25'); });
    expect(hook.result.current.status).toBe('error');
    expect(hook.result.current.unsaved).toEqual(['min']);
    expect(opts.onReject).not.toHaveBeenCalled();
    await act(async () => { hook.result.current.retry(); });
    expect(save).toHaveBeenLastCalledWith({ min: '25' }, 'v1');
    expect(hook.result.current.status).toBe('saved');
    expect(hook.result.current.unsaved).toEqual([]);
  });

  it('si el servidor lo rechaza, avisa para volver a lo guardado y sigue con lo demás (E2)', async () => {
    const rechazo = new ApiError(409, 'NAME_TAKEN', 'ya hay un artículo que se llama así', 'r');
    const save = vi.fn().mockRejectedValueOnce(rechazo).mockResolvedValueOnce({ updatedAt: 'v2' });
    const { hook, opts } = montar({ save });
    await act(async () => { hook.result.current.commit('name', 'Azúcar'); hook.result.current.commit('min', '30'); });
    expect(opts.onReject).toHaveBeenCalledWith('name', 'Hielo', rechazo);
    expect(save).toHaveBeenLastCalledWith({ min: '30' }, 'v1');
    expect(hook.result.current.unsaved).toEqual([]);
  });

  it('409 STALE avisa para recargar la ficha y vacía la cola (E3)', async () => {
    const save = vi.fn().mockRejectedValue(new ApiError(409, 'STALE', 'cambió', 'r'));
    const { hook, opts } = montar({ save });
    await act(async () => { hook.result.current.commit('name', 'Otro'); hook.result.current.commit('min', '5'); });
    expect(opts.onStale).toHaveBeenCalledTimes(1);
    expect(save).toHaveBeenCalledTimes(1);
  });

  it('un segundo toque mientras el mismo campo guarda se ignora', async () => {
    const save = vi.fn(() => new Promise<{ updatedAt: string }>(() => {}));
    const { hook } = montar({ save });
    act(() => { hook.result.current.commit('name', 'Uno'); });
    expect(hook.result.current.busy('name')).toBe(true);
    expect(hook.result.current.busy('min')).toBe(false);
  });

  it('salir de un campo sin cambiarlo no manda nada (E5)', async () => {
    const { hook, save } = montar();
    act(() => { hook.result.current.commit('name', 'Hielo'); });
    expect(save).not.toHaveBeenCalled();
    expect(hook.result.current.status).toBe('idle');
  });

  it('un dato inválido se marca y no se manda (E6)', async () => {
    const { hook, save } = montar();
    act(() => { hook.result.current.commit('name', '  '); });
    expect(save).not.toHaveBeenCalled();
    expect(hook.result.current.invalid).toEqual({ name: 'El nombre no puede quedar vacío.' });
    expect(hook.result.current.unsaved).toEqual(['name']);
  });

  it('esperar la cola resuelve cuando termina el último guardado, y dice si quedó algo sin guardar (E4)', async () => {
    let resolver: (v: { updatedAt: string }) => void = () => {};
    const save = vi.fn(() => new Promise<{ updatedAt: string }>((r) => { resolver = r; }));
    const { hook } = montar({ save });
    act(() => { hook.result.current.commit('min', '40'); });
    let listo: boolean | undefined;
    const p = hook.result.current.whenIdle().then((ok) => { listo = ok; });
    await espera(0);
    expect(listo).toBeUndefined();
    await act(async () => { resolver({ updatedAt: 'v2' }); await p; });
    expect(listo).toBe(true);
  });

  it('cada tecla reinicia la pausa de 2 s; «1», pausa, «5» deja 15 (E12)', async () => {
    let resolver: (v: { updatedAt: string }) => void = () => {};
    const save = vi.fn()
      .mockImplementationOnce(() => new Promise((r) => { resolver = r; }))
      .mockResolvedValue({ updatedAt: 'v3' });
    const { hook } = montar({ save });
    act(() => { hook.result.current.type('min', '1'); });
    expect(hook.result.current.status).toBe('dirty');
    await espera(1900);
    expect(save).not.toHaveBeenCalled();
    await espera(200);
    expect(save).toHaveBeenLastCalledWith({ min: '1' }, 'v1');
    act(() => { hook.result.current.type('min', '15'); });
    await espera(2100);
    act(() => { hook.result.current.type('min', '150'); });
    await espera(2100);
    // El 15 esperaba detrás del 1: el 150 lo reemplaza en vez de mandar los dos.
    await act(async () => { resolver({ updatedAt: 'v2' }); });
    await espera(0);
    expect(save.mock.calls.map((c) => c[0])).toEqual([{ min: '1' }, { min: '150' }]);
  });

  it('«1.», «-» y vacío no se mandan por la pausa; al salir del campo se validan (FR-022)', async () => {
    const { hook, save } = montar();
    for (const v of ['1.', '-', '']) {
      act(() => { hook.result.current.type('min', v); });
      await espera(2100);
    }
    expect(save).not.toHaveBeenCalled();
    act(() => { hook.result.current.commit('min', '-3'); });
    expect(save).not.toHaveBeenCalled();
    expect(hook.result.current.invalid).toEqual({ min: 'El mínimo no puede ser negativo.' });
  });

  it('salir del campo antes de la pausa manda una sola vez', async () => {
    const { hook, save } = montar();
    act(() => { hook.result.current.type('name', 'Hielo frappé'); });
    await act(async () => { hook.result.current.commit('name', 'Hielo frappé'); });
    await espera(2500);
    expect(save).toHaveBeenCalledTimes(1);
  });
});
