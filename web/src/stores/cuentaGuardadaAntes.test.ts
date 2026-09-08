import { describe, it, expect, beforeEach, vi } from 'vitest';

// UNA CUENTA GUARDADA POR UNA VERSIÓN ANTERIOR NO PUEDE TUMBAR LA PANTALLA.
//
// El campo `platformOrderRef` nació con el folio de plataforma. Las cuentas que ya estaban abiertas
// en la tableta cuando llegó esa versión NO lo traen, y `hayQuePedirElFolio` hace `.trim()` sobre
// él: `Cannot read properties of undefined (reading 'trim')`, y el POS entero deja de renderizar.
//
// Medido en dev sobre Chrome: pantalla en blanco tras entrar. No lo atrapó nada porque Playwright
// arranca siempre con un perfil limpio —ahí el store nace de `emptyTab()`, que sí trae el campo— y
// porque "vaciar caché y recargar" de Chrome NO borra localStorage: el usuario limpió la caché y
// siguió roto.
//
// El `merge` del store ya resolvía exactamente esto para `folioName` y `envio`, con el porqué
// escrito. El campo nuevo no se agregó ahí.
const PERSISTIDO = {
  state: {
    tabs: [{
      id: 'abc', num: 1, folioName: 'Tigre', lines: [], envio: '',
      serviceType: 'mostrador', customerName: '', platformId: null,
      // sin platformOrderRef: así se guardó antes de la feature
    }],
    activeId: 'abc',
    seq: 2,
  },
  version: 0,
};

describe('una cuenta guardada antes del folio de plataforma', () => {
  beforeEach(() => {
    vi.resetModules();
    localStorage.clear();
    localStorage.setItem('egb:ticket:v2', JSON.stringify(PERSISTIDO));
  });

  it('se rehidrata con el campo nuevo en vacío, no en undefined', async () => {
    const { useTicketStore } = await import('./ticket');
    const cuenta = useTicketStore.getState().tabs[0];
    expect(cuenta.id, 'la cuenta guardada se perdió: un deploy no puede tirar el pedido de un cliente')
      .toBe('abc');
    expect(cuenta.platformOrderRef,
      'quedó en undefined: el primer .trim() sobre él deja el POS en blanco').toBe('');
  });

  it('la regla que decide si pedir el folio no truena con esa cuenta', async () => {
    const { useTicketStore } = await import('./ticket');
    const { hayQuePedirElFolio } = await import('../domain/folioPlataforma');
    const cuenta = useTicketStore.getState().tabs[0];
    expect(() => hayQuePedirElFolio(cuenta.platformId, cuenta.platformOrderRef)).not.toThrow();
  });
});
