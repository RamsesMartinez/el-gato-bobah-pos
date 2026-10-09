import { beforeEach, describe, expect, test } from 'vitest';
import { usePosStore, seleccionValida } from './pos';

const LLAVE = 'egb:pos:v3';

beforeEach(() => {
  localStorage.clear();
  usePosStore.setState({ selected: null });
  usePosStore.getState().reiniciarNueva();
});

describe('lo que la tableta guarda: solo cuál cuenta está abierta', () => {
  test('persiste la selección bajo egb:pos:v3', () => {
    usePosStore.getState().seleccionar({ kind: 'draft', id: 'd-1' });
    expect(JSON.parse(localStorage.getItem(LLAVE) ?? '{}').state).toEqual({
      selected: { kind: 'draft', id: 'd-1' },
    });
  });

  // La cuenta vive en el servidor. Si la cabecera de una cuenta que todavía no nace se guardara
  // aquí, volvería a haber dos fuentes para lo mismo — y una tableta con una forma vieja guardada
  // es la pantalla en blanco del caso 18.
  test('no guarda renglones ni cabeceras', () => {
    usePosStore.getState().seleccionar({ kind: 'order', id: 12 });
    usePosStore.getState().cambiarNueva({ platformId: 3, platformOrderRef: 'A1' });
    const guardado = JSON.parse(localStorage.getItem(LLAVE) ?? '{}').state;
    expect(Object.keys(guardado)).toEqual(['selected']);
  });

  test.each([
    ['un texto suelto', 'd-1'],
    ['sin tipo', { id: 'd-1' }],
    ['tipo desconocido', { kind: 'mesa', id: 4 }],
    ['pedido con id de texto', { kind: 'order', id: '12' }],
    ['cuenta con id numérico', { kind: 'draft', id: 12 }],
    ['cuenta con id vacío', { kind: 'draft', id: '' }],
    ['pedido con id cero', { kind: 'order', id: 0 }],
  ])('una selección guardada con forma rara (%s) se descarta al cargar sin tumbar nada', async (_n, raro) => {
    localStorage.setItem(LLAVE, JSON.stringify({ state: { selected: raro }, version: 0 }));
    await usePosStore.persist.rehydrate();
    expect(usePosStore.getState().selected).toBeNull();
  });

  test('una selección bien formada sobrevive a la recarga', async () => {
    localStorage.setItem(LLAVE, JSON.stringify({ state: { selected: { kind: 'order', id: 12 } }, version: 0 }));
    await usePosStore.persist.rehydrate();
    expect(usePosStore.getState().selected).toEqual({ kind: 'order', id: 12 });
  });

  // El servidor no la encuentra (otra empresa en la tableta, o ya se descartó): se olvida sin
  // aviso. Pero SOLO si sigue siendo la seleccionada: una respuesta tardía de una cuenta vieja no
  // puede borrar la que el operador acaba de abrir.
  test('olvidar limpia solo si es la que está abierta', () => {
    usePosStore.getState().seleccionar({ kind: 'draft', id: 'nueva' });
    usePosStore.getState().olvidar({ kind: 'draft', id: 'vieja' });
    expect(usePosStore.getState().selected).toEqual({ kind: 'draft', id: 'nueva' });
    usePosStore.getState().olvidar({ kind: 'draft', id: 'nueva' });
    expect(usePosStore.getState().selected).toBeNull();
  });
});

describe('la cuenta que todavía no nace', () => {
  // Cada cuenta nueva arranca en MOSTRADOR aunque la anterior haya sido de plataforma: un toque de
  // más por pedido de plataforma, a cambio de que no se cobre precio de Uber en mostrador por
  // inercia.
  test('empezar una cuenta nueva deja sin selección y vuelve a mostrador', () => {
    usePosStore.getState().seleccionar({ kind: 'draft', id: 'd-1' });
    usePosStore.getState().cambiarNueva({ platformId: 3, platformOrderRef: 'A1', serviceType: 'domicilio' });
    usePosStore.getState().cuentaNueva();
    const s = usePosStore.getState();
    expect(s.selected).toBeNull();
    expect(s.nueva).toEqual({ platformId: null, platformOrderRef: '', serviceType: 'mostrador', customerName: '' });
  });

  // Cambiar de plataforma tira el folio: uno de Uber colgando de Rappi es basura silenciosa.
  test('cambiar de plataforma tira el folio de la anterior', () => {
    usePosStore.getState().cambiarNueva({ platformId: 3, platformOrderRef: 'A1' });
    usePosStore.getState().cambiarNueva({ platformId: 4 });
    expect(usePosStore.getState().nueva.platformOrderRef).toBe('');
  });
});

test('seleccionValida', () => {
  expect(seleccionValida({ kind: 'draft', id: 'x' })).toBe(true);
  expect(seleccionValida(null)).toBe(false);
});
