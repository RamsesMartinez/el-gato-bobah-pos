import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Provider } from '../components/ui/provider';
import { TemaRapido } from './TemaRapido';

beforeEach(() => {
  localStorage.clear();
  document.documentElement.className = '';
});

// Un toque cambia el tema (#290, F), y la preferencia se queda en ESTE aparato: la luz del mostrador
// no es la de la oficina.
test('alterna entre claro y oscuro y lo guarda en el aparato', async () => {
  localStorage.setItem('theme', 'light');
  render(<Provider><TemaRapido /></Provider>);
  await userEvent.click(await screen.findByRole('button', { name: 'Cambiar a tema oscuro' }));
  expect(localStorage.getItem('theme')).toBe('dark');
  await userEvent.click(await screen.findByRole('button', { name: 'Cambiar a tema claro' }));
  expect(localStorage.getItem('theme')).toBe('light');
});
