import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Provider } from '../components/ui/provider';
import { SearchSheet } from './SearchSheet';

// Spec 063: la hoja de búsqueda de la receta encuentra por otro nombre y lo señala con la etiqueta
// común (maqueta «Coincidió con», B); por el nombre principal no lleva etiqueta.
const options = [
  { value: '1', label: 'Leche entera', hint: 'ml', aliases: ['Leche de vaca', 'Leche Lala'] },
  { value: '2', label: 'Leche Lala deslactosada', hint: 'ml' },
  { value: '3', label: 'Azúcar', hint: 'g' },
];

test('encuentra por otro nombre, dice cuál y por nombre no lleva etiqueta', async () => {
  const u = userEvent.setup();
  const onPick = vi.fn();
  render(<Provider><SearchSheet open title="Agregar insumo" options={options} onPick={onPick} onClose={vi.fn()} /></Provider>);
  await u.type(await screen.findByRole('textbox', { name: 'Buscar' }), 'lala');
  const leche = await screen.findByRole('button', { name: /Leche entera/ });
  expect(within(leche).getByText(/también: Leche Lala/)).toBeInTheDocument();
  const deslac = screen.getByRole('button', { name: /Leche Lala deslactosada/ });
  expect(within(deslac).queryByText(/también:/)).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /Azúcar/ })).not.toBeInTheDocument();
  await u.click(leche);
  expect(onPick).toHaveBeenCalledWith('1');
});

test('lo que coincide con otro nombre no ofrece crearlo como nuevo', async () => {
  const u = userEvent.setup();
  render(<Provider><SearchSheet open title="Agregar insumo" options={options} onPick={vi.fn()} onClose={vi.fn()}
    renderCreate={(q) => <button type="button">Crear «{q}»</button>} /></Provider>);
  await u.type(await screen.findByRole('textbox', { name: 'Buscar' }), 'leche lala');
  expect(await screen.findByRole('button', { name: /Leche entera/ })).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /Crear/ })).not.toBeInTheDocument();
});
