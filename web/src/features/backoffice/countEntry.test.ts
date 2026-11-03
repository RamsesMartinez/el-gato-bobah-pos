import { describe, expect, test } from 'vitest';
import { diffWords, entryTotal, grossNet, isBigDiff, looseUnitOf, looseUnitsOf, packsTotal, restoreWeighed, withGross, closedPacksOf } from './countEntry';

describe('entryTotal', () => {
  test('empaques más suelto, en la unidad base', () => {
    // 3 botellas de 750 ml + 0.5 L abiertos = 2,750 ml.
    expect(entryTotal('3', '750', '0.5', 1000)).toBe(2750);
    expect(entryTotal('', null, '12', 1)).toBe(12);
  });
  test('las dos vacías es «no contado», no cero', () => {
    expect(entryTotal('', '750', '', 1000)).toBeNull();
  });
  test('un cero tecleado es «no hay»', () => {
    expect(entryTotal('0', '750', '', 1000)).toBe(0);
  });
  test('lo mal tecleado no se adivina', () => {
    expect(entryTotal('3,5', '750', '', 1000)).toBeNull();
  });
});

describe('looseUnitOf', () => {
  test('lo suelto se cuenta en la unidad grande; las piezas sin decimales', () => {
    expect(looseUnitOf('g')).toMatchObject({ label: 'kg', factor: 1000, decimals: true });
    expect(looseUnitOf('pieza')).toMatchObject({ label: 'piezas', decimals: false });
  });
});

describe('diffWords e isBigDiff', () => {
  test('sobraban o faltaban, nunca un negativo a secas', () => {
    expect(diffWords(4200, 'g')).toBe('sobraban 4.2 kg');
    expect(diffWords(-300, 'g')).toBe('faltaban 300 g');
  });
  test('una diferencia grande se marca; una chica no', () => {
    expect(isBigDiff(-300, 500, 800)).toBe(true);
    expect(isBigDiff(-20, 980, 1000)).toBe(false);
    expect(isBigDiff(4200, 3000, -1200)).toBe(true); // el sistema estaba en negativo
  });
});

// Spec 042, FR-012: el conteo ofrece todos los empaques, un selector de unidad que abre en la base
// y, en piezas con equivalencia de peso, g y kg para pesar lo suelto.
describe('looseUnitsOf', () => {
  it('abre en la unidad base y ofrece la grande', () => {
    expect(looseUnitsOf('g', false).map((u) => u.label)).toEqual(['g', 'kg']);
    expect(looseUnitsOf('ml', false).map((u) => u.label)).toEqual(['ml', 'L']);
    expect(looseUnitsOf('cm', false).map((u) => u.label)).toEqual(['cm', 'm']);
  });
  it('en piezas no hay selector, salvo que se pueda pesar', () => {
    expect(looseUnitsOf('pieza', false).map((u) => u.label)).toEqual(['piezas']);
    const w = looseUnitsOf('pieza', true);
    expect(w.map((u) => u.label)).toEqual(['piezas', 'g', 'kg']);
    expect(w[1].weightCode).toBe('g');
  });
});

describe('packsTotal', () => {
  it('suma cada empaque por lo que trae; vacío es «no contado»', () => {
    expect(packsTotal(['2', '', '1'], ['5000', '15000', '1000'])).toBe(11000);
    expect(packsTotal(['', ''], ['5000', '15000'])).toBeNull();
    expect(packsTotal(['0'], ['5000'])).toBe(0);
  });
});

// Spec 058 (US4): contar pesando con el empaque. La pantalla repite la regla del servidor para
// mostrar la cuenta; la cifra que se guarda es la que responde el servidor.
describe('grossNet', () => {
  it('en g resta lo que pesan vacíos y no divide', () => {
    const r = grossNet({ weight: '1000', weightFactor: 1, tareG: '50', containers: 3, baseCode: 'g' });
    expect(r).toEqual({ ok: true, net: 850, words: '1,000 g − 3 × 50 g = 850 g' });
  });
  it('en ml divide el neto entre la densidad, a 4 decimales', () => {
    const r = grossNet({ weight: '850', weightFactor: 1, tareG: '120', containers: 2, baseCode: 'ml', density: '1.1' });
    expect(r).toEqual({ ok: true, net: 554.5455, words: '850 g − 2 × 120 g = 610 g ÷ 1.10 = 554.5 ml' });
  });
  it('lo pesado en kg se cuenta en g', () => {
    const r = grossNet({ weight: '0.85', weightFactor: 1000, tareG: '120', containers: 2, baseCode: 'ml', density: '1.1' });
    expect(r).toMatchObject({ ok: true, net: 554.5455 });
    expect(r && r.ok && r.words.startsWith('850 g − 2 × 120 g')).toBe(true);
  });
  it('un bruto menor que los empaques vacíos es un error, no un negativo', () => {
    const r = grossNet({ weight: '200', weightFactor: 1, tareG: '120', containers: 2, baseCode: 'g' });
    expect(r).toEqual({ ok: false, error: 'Pesa menos que 2 empaques vacíos' });
    expect(grossNet({ weight: '100', weightFactor: 1, tareG: '120', containers: 1, baseCode: 'g' }))
      .toEqual({ ok: false, error: 'Pesa menos que 1 empaque vacío' });
  });
  it('un neto cero es válido: «no hay» dentro de los empaques', () => {
    expect(grossNet({ weight: '240', weightFactor: 1, tareG: '120', containers: 2, baseCode: 'g' })).toMatchObject({ ok: true, net: 0 });
  });
  it('sin peso no hay cuenta; mal tecleado o sin empaques tampoco se adivina', () => {
    expect(grossNet({ weight: '', weightFactor: 1, tareG: '120', containers: 1, baseCode: 'g' })).toBeNull();
    expect(grossNet({ weight: '1,000', weightFactor: 1, tareG: '120', containers: 1, baseCode: 'g' })).toBeNull();
    expect(grossNet({ weight: '500', weightFactor: 1, tareG: '120', containers: 0, baseCode: 'g' })).toBeNull();
    expect(grossNet({ weight: '500', weightFactor: 1, tareG: '120', containers: 1, baseCode: 'ml', density: null })).toBeNull();
  });
});

describe('withGross', () => {
  it('suma lo cerrado y lo pesado; sin cerrado cuenta como cero', () => {
    expect(withGross(3000, 554.5455)).toBe(3554.5455);
    expect(withGross(null, 610)).toBe(610);
  });
});

// #204: al volver a abrir lo contado pesando, las casillas vuelven como se llenaron.
describe('restoreWeighed', () => {
  const packs = [{ id: 7, qtyInBase: '5000', tareG: '20' }, { id: 9, qtyInBase: '15000' }];
  const w = { closedQty: '20000', grossG: '3000', containers: 1, tareG: '20', closedPacks: [{ presentationId: 9, qty: 1 }, { presentationId: 7, qty: 1 }] };
  test('lo cerrado vuelve como se repartió, con la báscula y los empaques pesados', () => {
    expect(restoreWeighed(w, packs)).toEqual({ counts: ['1', '1'], grossWeight: '3000', containers: '1', packId: 7 });
  });
  test('sin cerrados, las casillas de empaques quedan vacías (no en cero)', () => {
    expect(restoreWeighed({ ...w, closedQty: '0', closedPacks: [] }, packs)?.counts).toEqual(['', '']);
  });
  // La prueba con usuario nuevo: «1 de 15 kg y 1 de 5 kg» se reabría como «4 de 5 kg».
  test('sin el reparto guardado y con dos tamaños, no se adivina', () => {
    expect(restoreWeighed({ ...w, closedPacks: null }, packs)).toBeNull();
    expect(restoreWeighed({ ...w, closedPacks: undefined }, packs)).toBeNull();
  });
  test('sin el reparto guardado y un solo empaque, se sabe cuántos', () => {
    expect(restoreWeighed({ ...w, closedQty: '10000', closedPacks: null }, [packs[0]])?.counts).toEqual(['2']);
    expect(restoreWeighed({ ...w, closedQty: '7000', closedPacks: null }, [packs[0]])).toBeNull();
  });
  test('un reparto con un empaque que ya no está no se reabre', () => {
    expect(restoreWeighed({ ...w, closedPacks: [{ presentationId: 99, qty: 1 }] }, packs)).toBeNull();
  });
  test('si ningún empaque pesa vacío lo mismo que al capturar, no se reabre la pesada', () => {
    expect(restoreWeighed({ ...w, tareG: '25' }, packs)).toBeNull();
  });
  test('sin pesada no hay nada que reabrir', () => {
    expect(restoreWeighed(null, packs)).toBeNull();
    expect(restoreWeighed(undefined, packs)).toBeNull();
  });
});

// closedPacksOf es lo que se manda de lo cerrado por empaque: solo los que llevan algo.
describe('closedPacksOf', () => {
  test('cada empaque con su cantidad, sin los vacíos', () => {
    expect(closedPacksOf(['1', '', '0'], [{ id: 7 }, { id: 9 }, { id: 3 }])).toEqual([{ presentationId: 7, qty: 1 }]);
  });
  test('con un empaque sin id (API anterior) no se manda reparto', () => {
    expect(closedPacksOf(['1'], [{}])).toBeUndefined();
  });
});
