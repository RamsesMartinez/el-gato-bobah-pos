import { describe, expect, test, vi } from 'vitest';
import { fireEvent, render, screen, within } from '@testing-library/react';
import { Provider } from '../../components/ui/provider';
import type { AccountItem } from '../../types/pos';
import { AccountsColumn } from './AccountsColumn';
import { cuenta } from './__fixtures__/cuentas';

const CUENTAS: AccountItem[] = [
  cuenta({ key: 'o:2', orderId: 2, folioName: 'Bosque de Noruega', state: 'delivered_owes', outstanding: '195.00', openedAt: '2026-10-08T15:01:00Z' }),
  cuenta({ key: 'd:a', kind: 'draft', draftId: 'a', orderId: null, number: null, folioName: 'Levkoy', state: 'capturing', total: '74.00', outstanding: '74.00', openedAt: '2026-10-08T15:40:00Z' }),
  cuenta({ key: 'o:6', orderId: 6, folioName: 'Siamés', state: 'paid_in_kitchen', paid: '63.00', total: '63.00', outstanding: '0.00', openedAt: '2026-10-08T15:31:00Z' }),
  cuenta({ key: 'o:1', orderId: 1, folioName: 'Khao Manee', openedAt: '2026-10-08T14:57:00Z' }),
];

function montar(seleccionada: string | null = null, extra: { nueva?: boolean } = {}) {
  const props = { onElegir: vi.fn(), onNueva: vi.fn(), onDeliver: vi.fn(), onCharge: vi.fn() };
  render(<Provider><AccountsColumn cuentas={CUENTAS} seleccionada={seleccionada} nueva={extra.nueva} {...props} /></Provider>);
  return props;
}
const renglones = () => within(screen.getByLabelText('Cuentas', { exact: true })).getAllByRole('button')
  .filter((b) => b.dataset.ficha === 'true');
const alto = (el: Element) => parseInt(getComputedStyle(el).minHeight || '0', 10);

// La columna de cuentas (spec 089, maqueta B): una cuenta por renglón con lo que le falta.
describe('la columna de cuentas', () => {
  // En una lista vertical, reordenar al tocar mueve la cuenta bajo el dedo: el orden es estable, la
  // más vieja arriba (la que lleva más tiempo esperando).
  test('va por antigüedad y no se reordena al elegir', () => {
    montar('o:6');
    expect(renglones().map((b) => b.getAttribute('aria-label')?.split(' · ')[0]))
      .toEqual(['Khao Manee', 'Bosque de Noruega', 'Siamés', 'Levkoy']);
  });

  test('cada renglón dice lo que le falta y la abierta se marca', () => {
    montar('o:6');
    const siames = screen.getByRole('button', { name: 'Siamés · Falta entregar' });
    expect(siames).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByRole('button', { name: 'Bosque de Noruega · Falta cobrar $195' })).toHaveAttribute('aria-pressed', 'false');
  });

  test('una tableta en una cuenta nueva sin productos la ve primero', () => {
    montar(null, { nueva: true });
    expect(renglones()[0]).toHaveAccessibleName('Cuenta nueva, sin productos');
  });

  test('el botón aparece solo cuando le falta UNA cosa', () => {
    montar();
    expect(screen.getByRole('button', { name: 'Entregar Siamés' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Cobrar $195 Bosque de Noruega' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /^(Entregar|Cobrar .*) Khao Manee$/ })).toBeNull();
    expect(screen.queryByRole('button', { name: /^(Entregar|Cobrar .*) Levkoy$/ })).toBeNull();
  });

  test('el botón resuelve la cuenta sin elegirla; tocar el renglón la elige', () => {
    const p = montar();
    fireEvent.click(screen.getByRole('button', { name: 'Entregar Siamés' }));
    expect(p.onDeliver).toHaveBeenCalledWith(expect.objectContaining({ key: 'o:6' }));
    fireEvent.click(screen.getByRole('button', { name: 'Cobrar $195 Bosque de Noruega' }));
    expect(p.onCharge).toHaveBeenCalledWith(expect.objectContaining({ key: 'o:2' }));
    expect(p.onElegir).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: /^Khao Manee ·/ }));
    expect(p.onElegir).toHaveBeenCalledWith(expect.objectContaining({ key: 'o:1' }));
  });

  test('«Nueva» pide una cuenta nueva', () => {
    const p = montar();
    fireEvent.click(screen.getByRole('button', { name: 'Cuenta nueva' }));
    expect(p.onNueva).toHaveBeenCalled();
  });

  test('renglones y botones miden al menos 44 px; el renglón, 56', () => {
    montar();
    for (const b of renglones()) expect(alto(b)).toBeGreaterThanOrEqual(56);
    for (const n of ['Entregar Siamés', 'Cobrar $195 Bosque de Noruega', 'Cuenta nueva']) {
      expect(alto(screen.getByRole('button', { name: n }))).toBeGreaterThanOrEqual(44);
    }
  });

  test('dice cuántas cuentas hay', () => {
    montar();
    expect(screen.getByText('Cuentas (4)')).toBeInTheDocument();
  });

  test('sin cuentas lo dice, y «Nueva» sigue ahí', () => {
    render(<Provider><AccountsColumn cuentas={[]} seleccionada={null} onElegir={vi.fn()} onNueva={vi.fn()} onDeliver={vi.fn()} onCharge={vi.fn()} /></Provider>);
    expect(screen.getByText('No hay cuentas abiertas')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Cuenta nueva' })).toBeInTheDocument();
  });
});
