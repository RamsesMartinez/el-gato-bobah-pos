import { act, renderHook } from '@testing-library/react';
import { afterEach, expect, test } from 'vitest';
import { useLastWarehouse } from './useLastWarehouse';

const ws = [
  { id: 1, name: 'Local', isSales: true, isActive: true, itemsWithStock: 0 },
  { id: 2, name: 'Bodega', isSales: false, isActive: true, itemsWithStock: 0 },
];
afterEach(() => { try { localStorage.clear(); } catch { /* sin almacenamiento */ } });

test('sin recuerdo, el de venta', () => {
  const { result } = renderHook(() => useLastWarehouse(ws));
  expect(result.current[0]).toBe(1);
});

test('recuerda el último elegido en el equipo', () => {
  const { result } = renderHook(() => useLastWarehouse(ws));
  act(() => result.current[1](2));
  expect(result.current[0]).toBe(2);
  const { result: otra } = renderHook(() => useLastWarehouse(ws));
  expect(otra.current[0]).toBe(2);
});

// Un almacén archivado o de otro negocio que quedó guardado no se propone: cae al de venta.
test('un recuerdo que ya no está en la lista cae al de venta', () => {
  localStorage.setItem('egb:compra:almacen', '99');
  const { result } = renderHook(() => useLastWarehouse(ws));
  expect(result.current[0]).toBe(1);
});

test('sin almacenes todavía no propone ninguno', () => {
  const { result } = renderHook(() => useLastWarehouse([]));
  expect(result.current[0]).toBeNull();
});
