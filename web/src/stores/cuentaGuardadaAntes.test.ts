import { describe, it, expect, beforeEach, vi } from 'vitest';

// UNA CUENTA GUARDADA POR UNA VERSIÓN ANTERIOR NO PUEDE TUMBAR LA PANTALLA.
//
// El defecto concreto: `platformOrderRef` nació con el folio de plataforma y no se agregó al `merge`
// del almacén. `FolioPlataformaSheet` vive SIEMPRE montada y arranca con
// `useState(cuenta.platformOrderRef).trim()`, así que una cuenta ya guardada en la tableta dejaba el
// POS entero sin renderizar al ENTRAR — no al mandar el pedido. Medido en dev sobre Chrome.
//
// Por qué no lo atrapó nada: en vitest y en Playwright la cuenta nace de `emptyTab()`, que sí trae
// el campo, y "vaciar caché y recargar" de Chrome NO borra localStorage, así que limpiar la caché
// tampoco lo curaba.
//
// ESTE ARCHIVO VIGILA LA CLASE, NO EL CAMPO. El primer caso es el defecto que se vio; el segundo
// quita CADA campo de una cuenta, uno a la vez, y falla nombrando el que quedó en `undefined`. Ese
// es el que atrapa al campo número once, que es el que nadie va a recordar.

// La forma en la que se guardaba una cuenta ANTES del folio de plataforma.
const CUENTA_VIEJA = {
  id: 'abc', num: 1, folioName: 'Tigre', lines: [], envio: '',
  serviceType: 'mostrador', customerName: '', platformId: null,
  // sin platformOrderRef
};

function sembrar(tab: Record<string, unknown>) {
  localStorage.setItem('egb:ticket:v2', JSON.stringify({
    state: { tabs: [tab], activeId: tab.id, seq: 2 },
    version: 0,
  }));
}

async function rehidratar() {
  vi.resetModules();
  const { useTicketStore } = await import('./ticket');
  return useTicketStore.getState();
}

beforeEach(() => {
  localStorage.clear();
});

describe('una cuenta guardada antes del folio de plataforma', () => {
  it('se rehidrata con el campo nuevo en vacío, no en undefined', async () => {
    sembrar(CUENTA_VIEJA);
    const cuenta = (await rehidratar()).tabs[0];
    expect(cuenta.id, 'la cuenta guardada se perdió: un deploy no puede tirar el pedido de un cliente')
      .toBe('abc');
    expect(cuenta.platformOrderRef,
      'quedó en undefined: el primer .trim() sobre él deja el POS en blanco').toBe('');
  });

  it('la hoja del folio, que es la que tronaba, puede tomarlo como valor inicial', async () => {
    sembrar(CUENTA_VIEJA);
    const cuenta = (await rehidratar()).tabs[0];
    // Es literalmente lo que hace FolioPlataformaSheet en su primer render, y vive montada siempre.
    expect(() => cuenta.platformOrderRef.trim()).not.toThrow();
  });
});

// LA GUARDIA DE LA CLASE: ningún campo de una cuenta sobrevive como `undefined`.
//
// Se arma quitando un campo a la vez de una cuenta COMPLETA de hoy, que es exactamente la forma que
// tiene una cuenta guardada por la versión en la que ese campo todavía no existía. No enumera los
// campos a mano: los saca de la cuenta que el propio almacén crea, así que un campo nuevo entra a
// este test el día que se agrega, sin que nadie lo escriba aquí.
describe('cualquier campo que una versión anterior no guardara', () => {
  it('se rellena al rehidratar, sea cual sea', async () => {
    localStorage.clear();
    const nueva = (await rehidratar()).tabs[0] as unknown as Record<string, unknown>;
    const campos = Object.keys(nueva);
    expect(campos.length, 'no se pudo leer la forma de una cuenta nueva').toBeGreaterThan(5);

    const huecos: string[] = [];
    for (const falta of campos) {
      const recortada: Record<string, unknown> = { ...nueva, id: 'abc', num: 1 };
      delete recortada[falta];
      localStorage.clear();
      sembrar(recortada);
      const cuenta = (await rehidratar()).tabs[0] as unknown as Record<string, unknown>;
      if (cuenta[falta] === undefined) huecos.push(falta);
    }

    expect(huecos,
      `estos campos quedan en undefined cuando la cuenta guardada no los trae: ${huecos.join(', ')}. ` +
      'El primer código que los toque tumba el render y el operador ve una pantalla en blanco al ' +
      'entrar. Se rellenan en el `merge` de ticket.ts, que lee los defaults de `emptyTab()`.')
      .toHaveLength(0);
  });
});
