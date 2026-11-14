import { render, screen } from '@testing-library/react';
import { Provider } from '../../components/ui/provider';
import { Diferencia, ResumenDeDiferencia } from './diferencia';

const pinta = (ui: React.ReactElement) => render(<Provider>{ui}</Provider>);

// La diferencia se lee con palabra, signo e ícono, no solo con color (#290, FR-004; WCAG 1.4.1):
// en la Caja B un faltante pintado en rojo se confundió con un sobrante y nadie recontó.
test('un faltante dice Faltante con el signo menos tipográfico y dos decimales', () => {
  pinta(<Diferencia valor="-120" currency="MXN" />);
  expect(screen.getByText('Faltante')).toBeInTheDocument();
  expect(screen.getByText('−$120.00')).toBeInTheDocument();
});

test('un sobrante dice Sobrante con +', () => {
  pinta(<Diferencia valor="35.5" currency="MXN" />);
  expect(screen.getByText('Sobrante')).toBeInTheDocument();
  expect(screen.getByText('+$35.50')).toBeInTheDocument();
});

test('cero dice Cuadra, y un centavo no se redondea a Cuadra', () => {
  const { unmount } = pinta(<Diferencia valor="0" currency="MXN" />);
  expect(screen.getByText('Cuadra')).toBeInTheDocument();
  unmount();
  pinta(<Diferencia valor="-0.01" currency="MXN" />);
  expect(screen.getByText('−$0.01')).toBeInTheDocument();
});

test('sin dato es una raya, nunca Cuadra', () => {
  pinta(<Diferencia valor={null} currency="MXN" />);
  expect(screen.getByText('—')).toBeInTheDocument();
  expect(screen.queryByText('Cuadra')).not.toBeInTheDocument();
});

// Las tres partes de un corte (#290): el cajón, los otros métodos y el total, en ese orden.
test('el resumen del corte nombra cajón, otros métodos y total', () => {
  pinta(<ResumenDeDiferencia currency="MXN" diferencia={{ drawer: '-120', otherMethods: '0', total: '-120', drawerCovered: true }} />);
  expect(screen.getByText('Cajón (arqueo)')).toBeInTheDocument();
  expect(screen.getByText('Otros métodos (conciliación)')).toBeInTheDocument();
  expect(screen.getByText('Total')).toBeInTheDocument();
  expect(screen.getAllByText('−$120.00')).toHaveLength(2);
  expect(screen.getByText('Cuadra')).toBeInTheDocument();
});

// Un corte sin arqueo (caja que no lleva arqueo, o anterior a la spec 015) no inventa un cajón en $0.
test('sin arqueo no pinta la parte del cajón', () => {
  pinta(<ResumenDeDiferencia currency="MXN" diferencia={{ drawer: null, otherMethods: '10', total: '10', drawerCovered: false }} />);
  expect(screen.queryByText('Cajón (arqueo)')).not.toBeInTheDocument();
  expect(screen.getAllByText('+$10.00')).toHaveLength(2);
});
