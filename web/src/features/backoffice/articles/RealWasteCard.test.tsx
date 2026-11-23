import { describe, expect, test } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Provider } from '../../../components/ui/provider';
import { RealWasteRows } from './RealWasteCard';
import type { RealWaste } from '../../../api/admin';

const rw = (p: Partial<RealWaste>): RealWaste => ({ warehouseId: 1, warehouseName: 'Local', isSales: true, wasted: '0', consumed: '0', ratio: null, state: 'no_data', ...p });

function montar(items: RealWaste[], expected: number | null) {
  render(<Provider><RealWasteRows items={items} expected={expected} unit="g" /></Provider>);
}

// La merma real de 90 días (spec 096, maqueta A2): un renglón por almacén, y el aviso solo cuando
// pasa la esperada. Solo informa.
describe('Merma real en la ficha del artículo', () => {
  test('un renglón por almacén con su porcentaje', () => {
    montar([rw({ ratio: 4.5, state: 'ratio' }), rw({ warehouseId: 2, warehouseName: 'Bodega', isSales: false, wasted: '400', state: 'no_sales' })], 5);
    expect(screen.getByText('Local')).toBeInTheDocument();
    expect(screen.getByText('4.5 %')).toBeInTheDocument();
    expect(screen.getByText('Bodega')).toBeInTheDocument();
    expect(screen.getByText('Sin ventas')).toBeInTheDocument();
    expect(screen.getByText('400 g mermados')).toBeInTheDocument();
    expect(screen.queryByText('Pasa lo esperado')).toBeNull();
  });

  // Prueba con alguien nuevo: «1 pieza mermados». Concuerda en número y en género con la unidad.
  test('lo mermado concuerda con la unidad', () => {
    const pinta = (wasted: string, unit: string) => {
      const { unmount } = render(<Provider><RealWasteRows items={[rw({ wasted, state: 'no_sales' })]} expected={null} unit={unit} /></Provider>);
      const texto = screen.getByText(/mermad/).textContent;
      unmount();
      return texto;
    };
    expect(pinta('1', 'pieza')).toBe('1 pieza mermada');
    expect(pinta('2', 'pieza')).toBe('2 piezas mermadas');
    expect(pinta('400', 'g')).toBe('400 g mermados');
    expect(pinta('1', 'ml')).toBe('1 ml mermado');
  });

  test('avisa cuando la real pasa la esperada', () => {
    montar([rw({ ratio: 18, state: 'ratio' })], 15);
    expect(screen.getByText('Pasa lo esperado')).toBeInTheDocument();
  });

  test('sin datos lo dice en vez de un 0 %', () => {
    montar([rw({})], null);
    expect(screen.getByText('Sin datos')).toBeInTheDocument();
    expect(screen.queryByText('0 %')).toBeNull();
  });

  test('sin almacenes con datos no queda vacío', () => {
    montar([], 5);
    expect(screen.getByText('Sin datos en 90 días')).toBeInTheDocument();
  });

  test('un 0 % con ventas dice que no hay merma registrada', () => {
    montar([rw({ ratio: 0, state: 'ratio', consumed: '50' })], 5);
    expect(screen.getByText(/sin merma registrada/)).toBeInTheDocument();
  });

  // A 1024×600 la tarjeta va junto a otras y «400 g mermados» + «Sin ventas» se comían el renglón:
  // el nombre del almacén, que encogía hasta lo que quedara, quedaba con ancho 0 (cierre del #65).
  test('el nombre del almacén no encoge: lo demás baja de renglón', () => {
    montar([rw({ warehouseId: 2, warehouseName: 'Bodega', isSales: false, wasted: '400', state: 'no_sales' })], 5);
    const name = screen.getByText('Bodega');
    expect(getComputedStyle(name).flexShrink).toBe('0');
    expect(getComputedStyle(name.parentElement!).flexWrap).toBe('wrap');
  });

  // A 1024×600 el «0 %» quedaba solo en un renglón y «· sin merma registrada» en otro: la cifra y su
  // explicación bajan juntas o no bajan.
  test('la cifra y su explicación no se parten en dos renglones', () => {
    montar([rw({ ratio: 0, state: 'ratio', consumed: '50' })], 5);
    const value = screen.getByText(/sin merma registrada/).parentElement!;
    expect(value.textContent).toContain('0 %');
    expect(getComputedStyle(value).whiteSpace).toBe('nowrap');
  });
});
