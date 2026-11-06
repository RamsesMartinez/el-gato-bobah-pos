import { beforeEach, describe, expect, test, vi } from 'vitest';

// El aviso de versión nueva vive en el riel (spec 064). El toast arriba al centro tapaba botones de
// la pantalla —«Agregar lugar» en Negocio a 1024×600— y era un segundo aviso del mismo hecho.
const sw = vi.hoisted(() => ({ onNeedRefresh: undefined as undefined | (() => void), updateSW: vi.fn() }));
vi.mock('virtual:pwa-register', () => ({
  registerSW: (opts: { onNeedRefresh: () => void }) => {
    sw.onNeedRefresh = opts.onNeedRefresh;
    return sw.updateSW;
  },
}));
const create = vi.hoisted(() => vi.fn());
vi.mock('../../components/ui/toaster', () => ({ toaster: { create } }));

import { initPwa } from './registerPwa';
import { useAppUpdate } from '../../stores/appUpdate';

describe('versión nueva', () => {
  beforeEach(() => {
    create.mockClear();
    sw.updateSW.mockClear();
    useAppUpdate.setState({ needRefresh: false, apply: () => {} });
  });

  test('prende el aviso del riel y no crea un toast que tape la pantalla', () => {
    initPwa();
    sw.onNeedRefresh!();
    expect(useAppUpdate.getState().needRefresh).toBe(true);
    expect(create.mock.calls.map(([t]) => t.title)).not.toContain('Nueva versión disponible');
    expect(create).not.toHaveBeenCalled();
  });

  test('aplicar activa la versión nueva sin recarga del plugin', () => {
    initPwa();
    sw.onNeedRefresh!();
    useAppUpdate.getState().apply();
    expect(sw.updateSW).toHaveBeenCalledWith(false);
  });
});
