import { render, screen } from '@testing-library/react';
import { Provider } from '../components/ui/provider';
import { AvisosDeCaja } from './AvisosDeCaja';

// Punto 7: propina sin entregar y salidas sin concepto se avisan en Ventas del día y en el cierre.
test('avisa propina sin entregar y salidas sin concepto', () => {
  render(<Provider><AvisosDeCaja tipsPending="186.50" cashOutsWithoutConcept={2} /></Provider>);
  expect(screen.getByText(/Propinas sin entregar: \$186\.50/)).toBeInTheDocument();
  expect(screen.getByText(/2 salidas sin concepto/)).toBeInTheDocument();
});

test('sin nada que avisar no pinta nada', () => {
  render(<Provider><AvisosDeCaja tipsPending="0.40" cashOutsWithoutConcept={0} /></Provider>);
  expect(screen.queryByRole('status')).not.toBeInTheDocument();
});

// Dentro de Caja el aviso no manda «a Caja» (#290, hallazgo 10): dice qué tocar.
test('dentro de Caja dice qué hacer y no se refiere a sí mismo', () => {
  render(<Provider><AvisosDeCaja tipsPending="0" cashOutsWithoutConcept={1} enCaja /></Provider>);
  expect(screen.getByText(/1 salida sin concepto: ponles concepto/)).toBeInTheDocument();
  expect(screen.queryByText(/corrígelas en Caja/)).not.toBeInTheDocument();
});
