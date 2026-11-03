import { render, screen } from '@testing-library/react';
import { Provider } from '../../components/ui/provider';
import { DiaDelGasto } from './DiaDelGasto';

// El día del gasto es el del turno abierto (spec 032, punto 5): con caja abierta se dice cuál y no
// se edita; sin caja, se elige y arranca en hoy.
test('con caja abierta muestra el día del turno y no deja editarlo', () => {
  render(<Provider><DiaDelGasto turno="2026-10-08" value="2026-10-09" onChange={() => {}} /></Provider>);
  expect(screen.getByText(/Día del gasto: 8 de octubre \(turno abierto\)/)).toBeInTheDocument();
  expect(screen.queryByLabelText('Día del gasto')).not.toBeInTheDocument();
});

test('sin caja abierta se elige el día', () => {
  render(<Provider><DiaDelGasto turno={null} value="2026-10-09" onChange={() => {}} /></Provider>);
  expect(screen.getByRole('button', { name: 'Día del gasto' })).toHaveTextContent('9 oct 2026');
});
