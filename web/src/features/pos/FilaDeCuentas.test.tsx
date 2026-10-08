import { describe, expect, test, vi } from 'vitest';
import { fireEvent, render, screen, within } from '@testing-library/react';
import { Provider } from '../../components/ui/provider';
import type { AccountItem } from '../../types/pos';
import { FilaDeCuentas } from './FilaDeCuentas';
import { ordenDeLaFila, fichasQueCaben } from './estadosDeCuenta';
import { cuenta } from './__fixtures__/cuentas';


const CINCO: AccountItem[] = [
  cuenta({ key: 'o:1', orderId: 1, folioName: 'Khao Manee', openedAt: '2026-10-08T14:57:00Z' }),
  cuenta({ key: 'd:a', kind: 'draft', draftId: 'a', orderId: null, number: null, folioName: 'Levkoy', state: 'capturing', group: 'capturing', total: '74.00', paid: '0.00', outstanding: '74.00', openedAt: '2026-10-08T15:40:00Z' }),
  cuenta({ key: 'o:6', orderId: 6, folioName: 'Siamés', state: 'paid_in_kitchen', total: '63.00', paid: '63.00', outstanding: '0.00', openedAt: '2026-10-08T15:31:00Z' }),
  cuenta({ key: 'o:5', orderId: 5, folioName: 'Mesa afuera', state: 'partly_paid', total: '240.00', paid: '110.00', outstanding: '130.00', openedAt: '2026-10-08T15:20:00Z' }),
  cuenta({ key: 'o:2', orderId: 2, folioName: 'Bosque de Noruega', state: 'delivered_owes', group: 'delivered_owes', total: '195.00', outstanding: '195.00', openedAt: '2026-10-08T15:01:00Z' }),
];

function montar(ancho: number, cuentas = CINCO, seleccion: string | null = null) {
  const onElegir = vi.fn();
  const onNueva = vi.fn();
  const onVerTodas = vi.fn();
  render(
    <Provider>
      <FilaDeCuentas cuentas={cuentas} seleccionada={seleccion} ancho={ancho}
        onElegir={onElegir} onNueva={onNueva} onVerTodas={onVerTodas} />
    </Provider>,
  );
  return { onElegir, onNueva, onVerTodas };
}

describe('cuántas caben', () => {
  // Fichas de 120 px, gaps de 6 y los dos botones de 44: lo que no cabe completo no se pinta a
  // medias. 612 px es la fila medida a 1024×600 con el panel abierto (antes de «Buscar»).
  test.each([
    [346, 2], [345, 1], [612, 4], [180, 0], [220, 1],
  ])('a %i px caben %i fichas completas además de «+N» y «+»', (ancho, n) => {
    expect(fichasQueCaben(ancho)).toBe(n);
  });
});

describe('el orden de la fila', () => {
  test('primero la seleccionada, luego las que deben, luego por antigüedad', () => {
    const orden = ordenDeLaFila(CINCO, 'o:6').map((c) => c.key);
    expect(orden[0]).toBe('o:6');
    // Deben: Khao (14:57), Bosque (15:01), Mesa (15:20). La cuenta en captura no «debe»: lo que
    // se captura no es deuda todavía.
    expect(orden.slice(1, 4)).toEqual(['o:1', 'o:2', 'o:5']);
    expect(orden[4]).toBe('d:a');
  });
});

describe('FilaDeCuentas', () => {
  test('pinta solo las fichas que caben y «+N» cuenta exactamente las demás', () => {
    montar(346);
    expect(screen.getAllByRole('button', { name: /·/ }).filter((b) => b.dataset.ficha === 'true')).toHaveLength(2);
    expect(screen.getByRole('button', { name: /Ver todas las cuentas \(3 más\)/ })).toHaveTextContent('+3');
    expect(screen.getByRole('button', { name: 'Cuenta nueva' })).toBeInTheDocument();
  });

  test('cuando caben todas, el botón de la lista sigue ahí sin número', () => {
    montar(1200);
    expect(screen.getByRole('button', { name: 'Ver todas las cuentas' })).toBeInTheDocument();
  });

  test.each([
    ['Levkoy', 'Capturando', '$74'],
    ['Khao Manee', 'En cocina', '$194'],
    ['Siamés', 'Pagada · en cocina', 'pagada'],
    ['Mesa afuera', 'Pago parcial', 'falta $130'],
    ['Bosque de Noruega', 'Entregada · debe', 'debe $195'],
  ])('%s dice «%s» y «%s»', (nombre, estado, abajo) => {
    montar(2000);
    const ficha = screen.getByRole('button', { name: new RegExp(`^${nombre} ·`) });
    expect(within(ficha).getByText(estado)).toBeInTheDocument();
    expect(within(ficha).getByText(abajo)).toBeInTheDocument();
  });

  // El descarte vive en el ⋮ del ticket: una ✕ en la ficha, que se toca todo el día, es la acción
  // destructiva junto a la frecuente.
  test('ninguna ficha tiene cerrar', () => {
    montar(2000);
    expect(screen.queryByRole('button', { name: /cerrar|descartar|quitar/i })).toBeNull();
  });

  test('fichas y botones miden al menos 44 px', () => {
    montar(346);
    for (const b of screen.getAllByRole('button')) {
      expect(parseInt(getComputedStyle(b).minHeight || '0', 10), b.getAttribute('aria-label') ?? '').toBeGreaterThanOrEqual(44);
    }
  });

  test('la seleccionada se marca', () => {
    montar(2000, CINCO, 'o:5');
    expect(screen.getByRole('button', { name: /^Mesa afuera ·/ })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByRole('button', { name: /^Levkoy ·/ })).toHaveAttribute('aria-pressed', 'false');
  });

  test('tocar una la elige; «+» pide cuenta nueva; «+N» abre la lista', () => {
    const { onElegir, onNueva, onVerTodas } = montar(346);
    fireEvent.click(screen.getByRole('button', { name: /^Khao Manee ·/ }));
    expect(onElegir).toHaveBeenCalledWith(expect.objectContaining({ key: 'o:1' }));
    fireEvent.click(screen.getByRole('button', { name: 'Cuenta nueva' }));
    expect(onNueva).toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: /Ver todas/ }));
    expect(onVerTodas).toHaveBeenCalled();
  });

  test('una cuenta sin nombre todavía se ve como cuenta nueva', () => {
    montar(2000, [cuenta({ key: 'd:x', kind: 'draft', folioName: null, state: 'capturing', group: 'capturing' })]);
    expect(screen.getByRole('button', { name: /^Cuenta nueva ·/ })).toBeInTheDocument();
  });
});
