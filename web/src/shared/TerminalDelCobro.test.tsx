import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';
import { Provider } from '../components/ui/provider';
import { TerminalDelCobro } from './TerminalDelCobro';
import { terminalEfectiva } from './terminalEfectiva';

const t = (id: number, name: string, archived = false) => ({ id, branchId: 1, branchName: 'Matriz', name, archived });
const terminales = [t(1, 'Terminal B'), t(2, 'Terminal A'), t(3, 'Vieja', true)];

// Con varias terminales se elige en cada cobro (spec 036): la recordada del usuario dejó el débito
// real en Terminal A cuando se pasó en Terminal B. Con una sola no hay nada que elegir.
describe('terminalEfectiva', () => {
  it('la elegida manda', () => expect(terminalEfectiva(terminales, 2)).toBe(2));
  it('una archivada elegida no cuenta', () => expect(terminalEfectiva(terminales, 3)).toBeNull());
  it('una sola activa se usa sola', () => expect(terminalEfectiva([terminales[0], terminales[2]], null)).toBe(1));
  it('varias y ninguna elegida: hay que elegir', () => expect(terminalEfectiva(terminales, null)).toBeNull());
});

function pinta(lista: ReturnType<typeof t>[], elegida: number | null, onChange = vi.fn()) {
  render(<Provider><TerminalDelCobro terminales={lista} elegida={elegida} onChange={onChange} /></Provider>);
  return onChange;
}

test('con dos a cuatro activas: botones de 52 px a la vista, ninguno marcado al abrir', async () => {
  const onChange = pinta(terminales, null);
  const termB = screen.getByRole('button', { name: 'Terminal B' });
  expect(termB).toHaveStyle({ minHeight: '52px' });
  expect(termB).toHaveAttribute('aria-pressed', 'false');
  expect(screen.getByRole('button', { name: 'Terminal A' })).toHaveAttribute('aria-pressed', 'false');
  expect(screen.queryByRole('button', { name: 'Vieja' })).toBeNull();
  await userEvent.click(screen.getByRole('button', { name: 'Terminal A' }));
  expect(onChange).toHaveBeenCalledWith(2);
});

test('la elegida se ve marcada', () => {
  pinta(terminales, 2);
  expect(screen.getByRole('button', { name: 'Terminal A' })).toHaveAttribute('aria-pressed', 'true');
});

test('con más de cuatro, la lista va en el Picker', () => {
  pinta([t(1, 'A'), t(2, 'B'), t(3, 'C'), t(4, 'D'), t(5, 'E')], null);
  expect(screen.getByRole('button', { name: /Elige la terminal/ })).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'A' })).toBeNull();
});

test('con una sola, llega elegida y no hay nada que tocar', () => {
  pinta([t(1, 'Terminal B')], null);
  expect(screen.getByText('Terminal B')).toBeInTheDocument();
  expect(screen.queryByRole('button')).toBeNull();
});
