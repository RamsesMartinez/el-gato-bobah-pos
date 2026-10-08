import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Provider } from './ui/provider';
import { ListRow } from './ListRow';

function wrap(ui: React.ReactElement) {
  return render(<Provider>{ui}</Provider>);
}

const alto = (el: HTMLElement) => parseInt(getComputedStyle(el).minHeight || '0', 10);

// El alto por omisión es el que el Picker ya tenía: sacar la fila a su propio archivo no puede
// cambiar cómo se ven las hojas que ya existen.
test('por omisión la fila mide lo mismo que la del Picker (52 px)', () => {
  wrap(<ListRow label="Walmart" onClick={vi.fn()} />);
  expect(alto(screen.getByRole('button', { name: /Walmart/ }))).toBe(52);
});

test('acepta un alto mayor: 56 px para la lista de pedidos', () => {
  wrap(<ListRow label="Mesa 3 #14" minH={56} onClick={vi.fn()} />);
  expect(alto(screen.getByRole('button', { name: /Mesa 3 #14/ }))).toBe(56);
});

// Un dedo no acierta por debajo de 44 px: ni el alto por omisión ni uno pedido más chico bajan de ahí.
test('nunca queda por debajo de 44 px, aunque se pida menos', () => {
  wrap(<ListRow label="Chica" minH={30} onClick={vi.fn()} />);
  expect(alto(screen.getByRole('button', { name: /Chica/ }))).toBeGreaterThanOrEqual(44);
});

test('se toca, y marca la seleccionada con su pista', async () => {
  const u = userEvent.setup();
  const onClick = vi.fn();
  wrap(<ListRow label="Sams" hint="$120" selected onClick={onClick} />);
  expect(screen.getByText('$120')).toBeInTheDocument();
  await u.click(screen.getByRole('button', { name: /Sams/ }));
  expect(onClick).toHaveBeenCalledOnce();
});
