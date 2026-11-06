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

const sinEspera = vi.hoisted(() => ({ waiting: {} as unknown }));
Object.defineProperty(navigator, 'serviceWorker', {
  configurable: true,
  value: { addEventListener: () => {}, getRegistration: () => Promise.resolve({ waiting: sinEspera.waiting }) },
});
const reload = vi.fn();
Object.defineProperty(window, 'location', { configurable: true, value: { ...window.location, reload } });

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

  test('aplicar activa la versión que espera', async () => {
    sinEspera.waiting = {};
    initPwa();
    sw.onNeedRefresh!();
    useAppUpdate.getState().apply();
    await vi.waitFor(() => expect(sw.updateSW).toHaveBeenCalledWith(false));
  });

  // Otra pestaña ya aplicó la versión: aquí no queda ninguna esperando, el guard de una recarga por
  // pestaña pudo no recargar, y «Actualizar» mandaba skipWaiting a nadie. El botón se quedaba verde
  // sin hacer nada. Sin versión en espera, recargar ya carga la nueva.
  test('si otra pestaña ya la aplicó, actualizar recarga en vez de no hacer nada', async () => {
    sinEspera.waiting = null;
    reload.mockClear();
    initPwa();
    sw.onNeedRefresh!();
    useAppUpdate.getState().apply();
    await vi.waitFor(() => expect(reload).toHaveBeenCalledTimes(1));
    expect(sw.updateSW).not.toHaveBeenCalled();
  });
});
