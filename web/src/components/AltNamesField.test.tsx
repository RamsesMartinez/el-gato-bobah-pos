import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { Provider } from './ui/provider';
import { AltNamesField } from './AltNamesField';
import type { SimilarAlt } from './altNames';

// Spec 063, maqueta «Editor «Otros nombres»» opción A, con las decisiones de la revisión de tableta
// (R14): «×» de 44 px, aviso de parecidos ANTES de agregar y sin bloquear, tope de 10.
function Harness({ initial = [], similar = async () => [], error }: {
  initial?: string[]; similar?: (n: string) => Promise<SimilarAlt[]>; error?: string;
}) {
  const [value, setValue] = useState(initial);
  const [draft, setDraft] = useState('');
  return (
    <Provider>
      <AltNamesField ownName="Ice Coffee" value={value} onChange={setValue} draft={draft} onDraftChange={setDraft}
        similar={similar} error={error} />
      <output data-testid="value">{value.join('|')}</output>
    </Provider>
  );
}
const value = () => screen.getByTestId('value').textContent;

test('agrega con el botón y con Enter, limpio, y quita con una «×» de 44 px', async () => {
  const u = userEvent.setup();
  render(<Harness />);
  await u.type(screen.getByRole('textbox', { name: /otro nombre/i }), '  Café   frío ');
  await u.click(screen.getByRole('button', { name: 'Agregar' }));
  await waitFor(() => expect(value()).toBe('Café frío'));
  await u.type(screen.getByRole('textbox', { name: /otro nombre/i }), 'Café helado{Enter}');
  await waitFor(() => expect(value()).toBe('Café frío|Café helado'));
  const quitar = screen.getByRole('button', { name: 'Quitar «Café frío»' });
  expect(parseInt(getComputedStyle(quitar).minWidth, 10)).toBeGreaterThanOrEqual(44);
  expect(parseInt(getComputedStyle(quitar).minHeight, 10)).toBeGreaterThanOrEqual(44);
  await u.click(quitar);
  expect(value()).toBe('Café helado');
});

test('con 10 ya no ofrece el campo y lo dice', () => {
  render(<Harness initial={Array.from({ length: 10 }, (_, i) => `Nombre ${i}`)} />);
  expect(screen.queryByRole('textbox', { name: /otro nombre/i })).not.toBeInTheDocument();
  expect(screen.getByText(/ya tiene 10/i)).toBeInTheDocument();
  expect(screen.getByText(/10 de 10/)).toBeInTheDocument();
});

test('repetido en la lista o igual al nombre: no lo agrega y dice por qué', async () => {
  const u = userEvent.setup();
  render(<Harness initial={['Café frío']} />);
  await u.type(screen.getByRole('textbox', { name: /otro nombre/i }), 'cafe FRIO{Enter}');
  expect(await screen.findByText(/ya está en la lista/i)).toBeInTheDocument();
  await u.clear(screen.getByRole('textbox', { name: /otro nombre/i }));
  await u.type(screen.getByRole('textbox', { name: /otro nombre/i }), 'ice coffee{Enter}');
  expect(await screen.findByText(/ya es el nombre/i)).toBeInTheDocument();
  expect(value()).toBe('Café frío');
});

test('con parecidos, el primer toque avisa y el segundo agrega: no bloquea', async () => {
  const u = userEvent.setup();
  const similar = vi.fn(async () => [{ id: 7, name: 'Café frío de olla' }, { id: 8, name: 'Frappé', viaAlt: 'Café frío batido' }]);
  render(<Harness similar={similar} />);
  await u.type(screen.getByRole('textbox', { name: /otro nombre/i }), 'Café frío');
  // Toca Agregar sin esperar la pausa: el aviso igual llega antes de agregar.
  await u.click(screen.getByRole('button', { name: 'Agregar' }));
  expect(await screen.findByText(/se parece a/i)).toBeInTheDocument();
  expect(screen.getByText(/Café frío de olla/)).toBeInTheDocument();
  expect(value()).toBe('');
  await u.click(screen.getByRole('button', { name: 'Agregar de todos modos' }));
  await waitFor(() => expect(value()).toBe('Café frío'));
  expect(similar).toHaveBeenCalledWith('Café frío');
});

test('el choque que respondió el servidor se ve bajo el campo', () => {
  render(<Harness error="«café frío» ya es otro nombre de «Latte»" />);
  expect(screen.getByRole('alert')).toHaveTextContent('ya es otro nombre de «Latte»');
});
