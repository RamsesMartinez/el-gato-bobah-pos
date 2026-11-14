import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';
import { Provider } from '../../components/ui/provider';
import { RepartirPropinas, PropinasDelCierre } from './RepartirPropinas';
import type { TipsPending } from '../../api/backoffice';

const pendiente: TipsPending = {
  total: '186.50', cash: '96.50', nonCash: '90', inherited: '0',
  people: [{ id: 1, name: 'Ana' }, { id: 2, name: 'Beto' }, { id: 3, name: 'Carla' }],
};

function abrir(onEntregar = vi.fn(), onClose = vi.fn()) {
  render(<Provider><RepartirPropinas isOpen pendiente={pendiente} currency="MXN"
    onEntregar={onEntregar} onClose={onClose} guardando={false} /></Provider>);
  return { onEntregar, onClose };
}

// Opción C (decidida 2026-10-09): a una persona cuesta tocarla y entregar.
test('a una persona: tocarla y entregar manda el reparto parejo', async () => {
  const { onEntregar } = abrir();
  await userEvent.click(screen.getByRole('button', { name: 'Ana' }));
  await userEvent.click(screen.getByRole('button', { name: /Entregar \$186 a Ana/ }));
  expect(onEntregar).toHaveBeenCalledWith({ mode: 'parejo', recipients: [{ userId: 1 }] });
});

test('parejo entre tres dice el monto entero por persona y el sobrante', async () => {
  abrir();
  for (const n of ['Ana', 'Beto', 'Carla']) await userEvent.click(screen.getByRole('button', { name: n }));
  expect(screen.getByRole('button', { name: /Entregar \$62 a cada uno \(3\)/ })).toBeEnabled();
  expect(screen.getByText(/Quedan \$0\.50 para el siguiente reparto/)).toBeInTheDocument();
});

test('ajustado con centavos no deja entregar', async () => {
  const { onEntregar } = abrir();
  await userEvent.click(screen.getByRole('button', { name: 'Ana' }));
  await userEvent.click(screen.getByRole('button', { name: 'Ajustar montos' }));
  await userEvent.type(screen.getByLabelText('Monto para Ana'), '40.50');
  expect(screen.getByText('La propina se entrega en pesos enteros')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: /Entregar a 1 persona/ })).toBeDisabled();
  expect(onEntregar).not.toHaveBeenCalled();
});

test('Cancelar no entrega nada', async () => {
  const { onEntregar, onClose } = abrir();
  await userEvent.click(screen.getByRole('button', { name: 'Ana' }));
  await userEvent.click(screen.getByRole('button', { name: 'Cancelar' }));
  expect(onClose).toHaveBeenCalled();
  expect(onEntregar).not.toHaveBeenCalled();
});

// Al cerrar con un peso o más pendiente se pregunta, sin opción preseleccionada (EB-11).
test('el cierre pregunta qué hacer con la propina pendiente, sin elegir por el cajero', async () => {
  const onEntregarAhora = vi.fn();
  const onDecidir = vi.fn();
  const { rerender } = render(<Provider><PropinasDelCierre pendiente={pendiente} currency="MXN" decision={null}
    onEntregarAhora={onEntregarAhora} onDecidir={onDecidir} /></Provider>);
  expect(screen.getByText(/Quedan \$186\.50 de propina por entregar/)).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Se queda en caja' })).toHaveAttribute('aria-pressed', 'false');
  await userEvent.click(screen.getByRole('button', { name: 'Entregar ahora' }));
  expect(onEntregarAhora).toHaveBeenCalled();
  await userEvent.click(screen.getByRole('button', { name: 'Se queda en caja' }));
  expect(onDecidir).toHaveBeenCalledWith('quedan_en_caja');
  // Centavos no se pueden entregar: no hay nada que preguntar.
  rerender(<Provider><PropinasDelCierre pendiente={{ ...pendiente, total: '0.50' }} currency="MXN" decision={null}
    onEntregarAhora={onEntregarAhora} onDecidir={onDecidir} /></Provider>);
  expect(screen.queryByRole('button', { name: 'Se queda en caja' })).not.toBeInTheDocument();
});

test('«Se queda en caja» se puede desmarcar', async () => {
  const onDecidir = vi.fn();
  render(<Provider><PropinasDelCierre pendiente={pendiente} currency="MXN" decision="quedan_en_caja"
    onEntregarAhora={vi.fn()} onDecidir={onDecidir} /></Provider>);
  await userEvent.click(screen.getByRole('button', { name: 'Se queda en caja' }));
  expect(onDecidir).toHaveBeenCalledWith(null);
});

// «Ajustar montos» arranca con el reparto parejo: se corrige un monto, no se teclean todos.
test('ajustar montos arranca con el reparto parejo precargado', async () => {
  const { onEntregar } = abrir();
  for (const n of ['Ana', 'Beto']) await userEvent.click(screen.getByRole('button', { name: n }));
  await userEvent.click(screen.getByRole('button', { name: 'Ajustar montos' }));
  expect(screen.getByLabelText('Monto para Ana')).toHaveValue('93');
  expect(screen.getByLabelText('Monto para Beto')).toHaveValue('93');
  await userEvent.click(screen.getByRole('button', { name: /Entregar a 2 personas/ }));
  expect(onEntregar).toHaveBeenCalledWith({ mode: 'ajustado', recipients: [{ userId: 1, amount: 93 }, { userId: 2, amount: 93 }] });
});

// La hoja dice de qué medio viene lo pendiente: la de tarjeta se paga con efectivo del cajón y
// quien reparte tiene que saber cuánto de eso sale de la caja sin haber entrado en efectivo.
test('la hoja dice cuánto de lo pendiente es efectivo y cuánto tarjeta u otro medio', () => {
  abrir();
  expect(screen.getByText(/Efectivo \$96\.50/)).toBeInTheDocument();
  expect(screen.getByText(/Tarjeta y otros \$90/)).toBeInTheDocument();
});
