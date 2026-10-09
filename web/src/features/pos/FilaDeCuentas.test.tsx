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

function montar(ancho: number, cuentas = CINCO, seleccion: string | null = null,
  extra: { recientes?: string[]; nueva?: boolean } = {}) {
  const onElegir = vi.fn();
  const onNueva = vi.fn();
  const onVerTodas = vi.fn();
  render(
    <Provider>
      <FilaDeCuentas cuentas={cuentas} seleccionada={seleccion} ancho={ancho}
        recientes={extra.recientes} nueva={extra.nueva}
        onElegir={onElegir} onNueva={onNueva} onVerTodas={onVerTodas} />
    </Provider>,
  );
  return { onElegir, onNueva, onVerTodas };
}

describe('cuántas caben', () => {
  // Fichas de 112 px, gaps de 6 y los dos botones de 44: lo que no cabe completo no se pinta a
  // medias. ~457 px es la fila a 1024×600 con el panel abierto (después de buscar, precios y
  // editar): ahí caben TRES —la activa, la anterior y una más—. Con 120 px cabían dos, y la cuenta
  // anterior se iba a «+N» en cuanto se abría otra.
  test.each([
    [457, 3], [447, 2], [330, 2], [329, 1], [612, 4], [180, 0], [212, 1],
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

describe('la activa y la anterior siguen a la vista (validación como usuario nuevo)', () => {
  // Con el ticket abierto caben tres. Ordenar por «debe dinero» mandaba la cuenta que se capturaba
  // a «+N» en cuanto se abría otra, y para volver había que buscarla en la lista.
  test('la recién usada va junto a la activa, antes que las que deben', () => {
    const orden = ordenDeLaFila(CINCO, 'o:6', ['o:6', 'd:a']).map((c) => c.key);
    expect(orden.slice(0, 2)).toEqual(['o:6', 'd:a']);
  });

  test('las usadas van por uso reciente; las demás como antes', () => {
    const orden = ordenDeLaFila(CINCO, 'o:6', ['o:6', 'o:5', 'd:a']).map((c) => c.key);
    expect(orden).toEqual(['o:6', 'o:5', 'd:a', 'o:1', 'o:2']);
  });

  // «+» con una cuenta en captura: ésta no sale de la fila y la nueva se ve como ficha vacía, sin
  // existir todavía en el servidor.
  test('con una cuenta nueva sin productos, su ficha va primero y la anterior junto a ella', () => {
    montar(457, CINCO, null, { nueva: true, recientes: ['d:a'] });
    const fichas = screen.getAllByRole('button').filter((b) => b.dataset.ficha === 'true');
    expect(fichas).toHaveLength(3);
    expect(fichas[0]).toHaveAccessibleName('Cuenta nueva, sin productos');
    expect(fichas[0]).toHaveAttribute('aria-pressed', 'true');
    expect(fichas[1]).toHaveAccessibleName(/^Levkoy ·/);
  });

  // Lo que falta es lo que se cobra: con la ficha angosta, la etiqueta larga cede, la cifra no.
  test('la cifra no cede ancho; la etiqueta sí', () => {
    montar(2000);
    const ficha = screen.getByRole('button', { name: /^Mesa afuera ·/ });
    expect(getComputedStyle(within(ficha).getByText('falta $130')).flexShrink).toBe('0');
  });
});

describe('FilaDeCuentas', () => {
  test('pinta solo las fichas que caben y «+N» cuenta exactamente las demás', () => {
    montar(330);
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
    montar(457, CINCO, null, { nueva: true });
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
    const { onElegir, onNueva, onVerTodas } = montar(330);
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
