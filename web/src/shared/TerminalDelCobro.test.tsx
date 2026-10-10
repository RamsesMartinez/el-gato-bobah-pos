import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';
import { Provider } from '../components/ui/provider';
import { TerminalDelCobro } from './TerminalDelCobro';
import { terminalEfectiva } from './terminalEfectiva';

const terminales = [
  { id: 1, branchId: 1, branchName: 'Matriz', name: 'Getnet', archived: false },
  { id: 2, branchId: 1, branchName: 'Matriz', name: 'Hey', archived: false },
  { id: 3, branchId: 1, branchName: 'Matriz', name: 'Vieja', archived: true },
];

// La terminal llega sola (la del usuario, o la única): el caso común no cuesta un toque más
// (punto 8). Una archivada nunca se usa (EB-29).
describe('terminalEfectiva', () => {
  it('la elegida manda', () => expect(terminalEfectiva(terminales, 1, 2)).toBe(2));
  it('sin elegir, la del usuario', () => expect(terminalEfectiva(terminales, 2, null)).toBe(2));
  it('la del usuario archivada no cuenta', () => expect(terminalEfectiva(terminales, 3, null)).toBeNull());
  it('una sola activa se usa sola', () => expect(terminalEfectiva([terminales[0]], null, null)).toBe(1));
  it('varias y ninguna: hay que elegir', () => expect(terminalEfectiva(terminales, null, null)).toBeNull());
});

test('con terminal resuelta se muestra y se puede cambiar; sin ella se pide', async () => {
  const onChange = vi.fn();
  const { rerender } = render(<Provider><TerminalDelCobro terminales={terminales} porOmision={1} elegida={null} onChange={onChange} /></Provider>);
  expect(screen.getByText('Terminal')).toBeInTheDocument();
  await userEvent.click(screen.getByRole('button', { name: /Getnet/ }));
  await userEvent.click(await screen.findByRole('button', { name: /Hey/ }));
  expect(onChange).toHaveBeenCalledWith(2);
  rerender(<Provider><TerminalDelCobro terminales={terminales} porOmision={null} elegida={null} onChange={onChange} /></Provider>);
  expect(screen.getByRole('button', { name: /Elige la terminal/ })).toBeInTheDocument();
});
