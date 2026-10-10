import { afterEach, expect, test, vi } from 'vitest';
import { posApi } from './pos';

// /me/preferences responde {"value": …}, no el valor suelto. Leer la envoltura como si fuera el id
// dejaba la terminal del usuario siempre vacía, y con dos o más terminales nunca llegaba puesta.
afterEach(() => vi.unstubAllGlobals());

function responde(body: unknown) {
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify(body), {
    status: 200, headers: { 'Content-Type': 'application/json' },
  })));
}

test('la terminal del usuario se lee de la envoltura {value}', async () => {
  responde({ value: 3 });
  expect(await posApi.defaultTerminal()).toBe(3);
});

test('sin preferencia guardada no hay terminal del usuario', async () => {
  responde({ value: null });
  expect(await posApi.defaultTerminal()).toBeNull();
});
