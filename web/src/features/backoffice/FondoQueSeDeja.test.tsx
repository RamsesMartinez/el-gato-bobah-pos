import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';
import { Provider } from '../../components/ui/provider';
import { FondoQueSeDeja } from './FondoQueSeDeja';
import { fondoValido, fondoExcedeLoContado } from './fondoQueSeDeja';

// Al cerrar se dice cuánto se queda de fondo; el resto se retira (decisión del 2026-10-10). La
// apertura siguiente se compara contra esto.
test('pide el fondo que se queda en un campo de 48 px', async () => {
  const onChange = vi.fn();
  render(<Provider><FondoQueSeDeja value="" onChange={onChange} /></Provider>);
  const campo = screen.getByLabelText('Se queda de fondo');
  expect(getComputedStyle(campo).minHeight).toBe('48px');
  await userEvent.type(campo, '5');
  expect(onChange).toHaveBeenCalledWith('5');
});

test('vacío no es cero: falta capturarlo', () => {
  expect(fondoValido('')).toBe(false);
  expect(fondoValido('0')).toBe(true);
  expect(fondoValido('500.50')).toBe(true);
  expect(fondoValido('-1')).toBe(false);
  expect(fondoValido('x')).toBe(false);
});

// EB-48: el servidor rechaza un fondo mayor a lo contado, pero el aviso llegaba DESPUÉS de tocar
// «Cerrar caja». La pantalla lo dice antes y no deja confirmar.
test('un fondo mayor a lo contado se avisa antes de cerrar', () => {
  expect(fondoExcedeLoContado('600', 500)).toBe(true);
  expect(fondoExcedeLoContado('500', 500)).toBe(false);
  expect(fondoExcedeLoContado('', 500)).toBe(false);
  // Sin conteo todavía no hay contra qué comparar: eso lo pide el aviso de «falta capturar».
  expect(fondoExcedeLoContado('600', null)).toBe(false);
  render(<Provider><FondoQueSeDeja value="600" onChange={vi.fn()} contado={500} /></Provider>);
  expect(screen.getByText(/más de lo contado/)).toBeInTheDocument();
});
