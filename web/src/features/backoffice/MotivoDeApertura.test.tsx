import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';
import { Provider } from '../../components/ui/provider';
import { MotivoDeApertura } from './MotivoDeApertura';

// Lo contado no coincide con el cierre anterior: se pide el motivo de la lista, sin decir cuánto
// había (el conteo es a ciegas, punto 6).
test('elige un motivo de la lista y confirma', async () => {
  const onConfirmar = vi.fn();
  render(<Provider><MotivoDeApertura isOpen guardando={false} onConfirmar={onConfirmar} onVolver={vi.fn()} /></Provider>);
  expect(screen.queryByText(/\$/)).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Abrir caja' })).toBeDisabled();
  await userEvent.click(screen.getByRole('button', { name: 'Cambié el fondo' }));
  await userEvent.click(screen.getByRole('button', { name: 'Abrir caja' }));
  expect(onConfirmar).toHaveBeenCalledWith('float_changed', '');
});

test('«Otro» exige escribir el motivo', async () => {
  const onConfirmar = vi.fn();
  render(<Provider><MotivoDeApertura isOpen guardando={false} onConfirmar={onConfirmar} onVolver={vi.fn()} /></Provider>);
  await userEvent.click(screen.getByRole('button', { name: 'Otro' }));
  expect(screen.getByRole('button', { name: 'Abrir caja' })).toBeDisabled();
  await userEvent.type(screen.getByLabelText('Motivo'), 'se llevó cambio');
  await userEvent.click(screen.getByRole('button', { name: 'Abrir caja' }));
  expect(onConfirmar).toHaveBeenCalledWith('other', 'se llevó cambio');
});
