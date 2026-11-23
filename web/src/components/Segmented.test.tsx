import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Provider } from './ui/provider';
import { Segmented } from './Segmented';

function wrap(ui: React.ReactElement) {
  return render(<Provider>{ui}</Provider>);
}

const opts = [
  { value: '1', label: 'Local · de venta' },
  { value: '2', label: 'Bodega' },
  { value: 'all', label: 'Todos' },
];

test('dice cuál está elegido y cambia con un toque', async () => {
  const onChange = vi.fn();
  wrap(<Segmented label="Almacén" value="1" options={opts} onChange={onChange} />);
  expect(screen.getByRole('button', { name: 'Local · de venta' })).toHaveAttribute('aria-pressed', 'true');
  expect(screen.getByRole('button', { name: 'Bodega' })).toHaveAttribute('aria-pressed', 'false');
  await userEvent.click(screen.getByRole('button', { name: 'Bodega' }));
  expect(onChange).toHaveBeenCalledWith('2');
});

// Un dedo no acierta por debajo de 44 px (constitución, restricciones del producto).
test('cada opción mide al menos 44 px', () => {
  wrap(<Segmented label="Almacén" value="1" options={opts} onChange={vi.fn()} />);
  for (const b of screen.getAllByRole('button')) {
    expect(parseInt(getComputedStyle(b).minHeight || '0', 10)).toBeGreaterThanOrEqual(44);
  }
});

// El grupo se anuncia con su nombre: con varios segmentados en una pantalla, el lector de pantalla
// tiene que saber de qué es cada uno.
test('el grupo lleva su nombre', () => {
  wrap(<Segmented label="Almacén" value="1" options={opts} onChange={vi.fn()} />);
  expect(screen.getByRole('group', { name: 'Almacén' })).toBeInTheDocument();
});
