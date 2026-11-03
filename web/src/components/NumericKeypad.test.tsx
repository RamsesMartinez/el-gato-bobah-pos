import { describe, expect, test, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { Provider } from './ui/provider';
import { NumericKeypad } from './NumericKeypad';
import { pressKey } from './pressKey';

describe('pressKey', () => {
  test.each([
    ['', '3', true, '3'],
    ['3', '.', true, '3.'],
    ['3.', '.', true, '3.'], // un solo punto
    ['3', '.', false, '3'], // sin decimales en piezas o empaques
    ['', '.', true, '0.'],
    ['0', '7', true, '7'],
    ['21', 'back', true, '2'],
    ['', 'back', true, ''],
  ])('%j + %j (decimales %s) = %j', (v, k, dec, want) => {
    expect(pressKey(v, k, dec)).toBe(want);
  });
});

describe('NumericKeypad', () => {
  test('teclas de 56 px, punto solo con decimales, borrar y siguiente casilla', () => {
    const onKey = vi.fn();
    const onNext = vi.fn();
    const { rerender } = render(<Provider><NumericKeypad allowDecimal={false} onKey={onKey} onNext={onNext} /></Provider>);
    expect(screen.queryByRole('button', { name: 'punto' })).toBeNull();
    for (const b of screen.getAllByRole('button')) {
      expect(parseInt(getComputedStyle(b).minHeight || '0', 10)).toBeGreaterThanOrEqual(56);
    }
    fireEvent.click(screen.getByRole('button', { name: '7' }));
    fireEvent.click(screen.getByRole('button', { name: 'Borrar' }));
    fireEvent.click(screen.getByRole('button', { name: /Siguiente casilla/ }));
    expect(onKey.mock.calls).toEqual([['7'], ['back']]);
    expect(onNext).toHaveBeenCalled();
    rerender(<Provider><NumericKeypad allowDecimal onKey={onKey} /></Provider>);
    expect(screen.getByRole('button', { name: 'punto' })).toBeInTheDocument();
  });
});
