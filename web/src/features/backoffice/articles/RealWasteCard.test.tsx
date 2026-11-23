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
});
