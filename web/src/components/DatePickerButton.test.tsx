import { render, screen, fireEvent } from '@testing-library/react';
import { vi, expect, test } from 'vitest';
import { Provider } from './ui/provider';
import { DatePickerButton } from './DatePickerButton';
import { formatDay, monthGrid } from '../utils/calendar';

test('el mes empieza en lunes: octubre de 2026 empieza en jueves', () => {
  const g = monthGrid(2026, 9);
  expect(g.slice(0, 4)).toEqual([null, null, null, '2026-10-01']);
  expect(g.filter(Boolean)).toHaveLength(31);
});

test('elegir un día lo devuelve en AAAA-MM-DD y los días después del máximo no se tocan', async () => {
  const onChange = vi.fn();
  render(<Provider><DatePickerButton label="Día" value="2026-10-10" max="2026-10-11" onChange={onChange} /></Provider>);
  fireEvent.click(screen.getByRole('button', { name: 'Día' }));
  expect(await screen.findByRole('button', { name: '12' })).toBeDisabled();
  fireEvent.click(screen.getByRole('button', { name: '9' }));
  expect(onChange).toHaveBeenCalledWith('2026-10-09');
  expect(formatDay('2026-10-09')).toBe('9 oct 2026');
});

test('un campo opcional se puede quitar', async () => {
  const onChange = vi.fn();
  render(<Provider><DatePickerButton label="Hasta" value="2026-10-10" clearable onChange={onChange} /></Provider>);
  fireEvent.click(screen.getByRole('button', { name: 'Hasta' }));
  fireEvent.click(await screen.findByRole('button', { name: 'Quitar fecha' }));
  expect(onChange).toHaveBeenCalledWith('');
});
