import { render, screen, fireEvent } from '@testing-library/react';
import { vi, expect, test } from 'vitest';
import { Provider } from './ui/provider';
import { ExpiryPickerButton } from './ExpiryPickerButton';

const renderPicker = (props: Partial<Parameters<typeof ExpiryPickerButton>[0]> = {}) => {
  const onChange = vi.fn();
  render(<Provider><ExpiryPickerButton label="Caduca Perlas" value="" today="2026-11-23" onChange={onChange} {...props} /></Provider>);
  return onChange;
};

test('mes y luego día: 3 toques para una fecha a cuatro meses', async () => {
  const onChange = renderPicker();
  fireEvent.click(screen.getByRole('button', { name: 'Caduca Perlas' }));
  fireEvent.click(await screen.findByRole('button', { name: 'mar 2027' }));
  fireEvent.click(await screen.findByRole('button', { name: '15' }));
  expect(onChange).toHaveBeenCalledWith('2027-03-15');
});

test('la página son los 12 meses desde el actual; sin lo ya caducado no se va hacia atrás', async () => {
  renderPicker();
  fireEvent.click(screen.getByRole('button', { name: 'Caduca Perlas' }));
  expect(await screen.findByRole('button', { name: 'nov 2026' })).toBeEnabled();
  expect(screen.getByRole('button', { name: 'oct 2027' })).toBeEnabled();
  expect(screen.getByRole('button', { name: 'Meses anteriores' })).toBeDisabled();
  fireEvent.click(screen.getByRole('button', { name: 'Meses siguientes' }));
  expect(await screen.findByRole('button', { name: 'nov 2027' })).toBeEnabled();
});

test('hoy es el último día que se puede elegir hacia atrás al recibir', async () => {
  const onChange = renderPicker();
  fireEvent.click(screen.getByRole('button', { name: 'Caduca Perlas' }));
  fireEvent.click(await screen.findByRole('button', { name: 'nov 2026' }));
  expect(await screen.findByRole('button', { name: '22' })).toBeDisabled();
  fireEvent.click(screen.getByRole('button', { name: '23' }));
  expect(onChange).toHaveBeenCalledWith('2026-11-23');
});

test('en el conteo se puede elegir lo que ya caducó', async () => {
  const onChange = renderPicker({ allowPast: true });
  fireEvent.click(screen.getByRole('button', { name: 'Caduca Perlas' }));
  fireEvent.click(await screen.findByRole('button', { name: 'Meses anteriores' }));
  fireEvent.click(await screen.findByRole('button', { name: 'oct 2026' }));
  fireEvent.click(await screen.findByRole('button', { name: '20' }));
  expect(onChange).toHaveBeenCalledWith('2026-10-20');
});

test('«No trae fecha» deja el campo vacío', async () => {
  const onChange = renderPicker({ value: '2027-01-10' });
  fireEvent.click(screen.getByRole('button', { name: 'Caduca Perlas' }));
  fireEvent.click(await screen.findByRole('button', { name: 'No trae fecha' }));
  expect(onChange).toHaveBeenCalledWith('');
});
