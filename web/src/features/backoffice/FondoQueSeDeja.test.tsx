import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';
import { Provider } from '../../components/ui/provider';
import { money } from '../../utils/format';
import { FondoQueSeDeja } from './FondoQueSeDeja';
import { fondoValido, fondoExcedeLoContado, retiroDelCierre, personasDelRetiro, resumenDelRetiro, avisoDeQuienSeLoLleva } from './fondoQueSeDeja';

// Hoja de cierre, opción A de las maquetas (plan.md, «Maquetas», fila «Hoja de cierre»): la pregunta
// «¿Se retira dinero del cajón?» abre en Sí, el fondo llega con el fijo y el retiro se ve en el mismo
// renglón del campo para que el teclado del sistema no lo tape.

const personas = [{ id: 1, name: 'Ana' }, { id: 2, name: 'Luis' }, { id: 3, name: 'Marta' }];

function pinta(over: Partial<React.ComponentProps<typeof FondoQueSeDeja>> = {}) {
  const props: React.ComponentProps<typeof FondoQueSeDeja> = {
    value: '1000', onChange: vi.fn(), contado: 5300, fijo: 1000, retirar: true, onRetirar: vi.fn(),
    personas, quien: 2, onQuien: vi.fn(), currency: 'MXN', ...over,
  };
  render(<Provider><FondoQueSeDeja {...props} /></Provider>);
  return props;
}

test('el campo y los botones miden al menos 48 px', () => {
  pinta();
  expect(getComputedStyle(screen.getByLabelText('Se queda de fondo')).minHeight).toBe('48px');
  for (const nombre of [/Sí, retirar/, /No, todo se queda/, 'Luis']) {
    expect(getComputedStyle(screen.getByRole('button', { name: nombre })).minHeight).toBe('48px');
  }
});

test('con Sí: el retiro va en el renglón del fondo y «Solo hoy» aparece al cambiar el fijo', async () => {
  const p = pinta({ value: '800' });
  expect(screen.getByText(/Retiro \$4,500/)).toBeInTheDocument();
  expect(screen.getByText('Solo hoy')).toBeInTheDocument();
  await userEvent.click(screen.getByRole('button', { name: 'Marta' }));
  expect(p.onQuien).toHaveBeenCalledWith(3);
  await userEvent.click(screen.getByRole('button', { name: /No, todo se queda/ }));
  expect(p.onRetirar).toHaveBeenCalledWith(false);
});

test('con el fijo intacto no dice «Solo hoy»', () => {
  pinta();
  expect(screen.queryByText('Solo hoy')).toBeNull();
});

test('con No: sin campo ni personas, se queda lo contado y se ve contra qué fijo', () => {
  pinta({ retirar: false, value: '5300' });
  expect(screen.queryByLabelText('Se queda de fondo')).toBeNull();
  expect(screen.queryByRole('button', { name: 'Luis' })).toBeNull();
  expect(screen.getByText(/el fijo es \$1,000/)).toBeInTheDocument();
});

test('sin fijo configurado el campo llega vacío y no hay pregunta de «Solo hoy»', () => {
  pinta({ fijo: null, value: '' });
  expect(screen.getByLabelText('Se queda de fondo')).toHaveValue('');
  expect(screen.queryByText('Solo hoy')).toBeNull();
});

test('vacío no es cero: falta capturarlo', () => {
  expect(fondoValido('')).toBe(false);
  expect(fondoValido('0')).toBe(true);
  expect(fondoValido('500.50')).toBe(true);
  expect(fondoValido('-1')).toBe(false);
  expect(fondoValido('x')).toBe(false);
});

// EB-48: el aviso va antes de tocar «Cerrar caja».
test('un fondo mayor a lo contado se avisa antes de cerrar', () => {
  expect(fondoExcedeLoContado('600', 500)).toBe(true);
  expect(fondoExcedeLoContado('500', 500)).toBe(false);
  expect(fondoExcedeLoContado('', 500)).toBe(false);
  expect(fondoExcedeLoContado('600', null)).toBe(false);
  pinta({ value: '6000' });
  expect(screen.getByText(/más de lo contado/)).toBeInTheDocument();
});

test('el retiro es lo contado menos el fondo, y nada si falta alguno o sobra el fondo', () => {
  expect(retiroDelCierre(5300, '1000')).toBe(4300);
  expect(retiroDelCierre(5300, '5300')).toBe(0);
  expect(retiroDelCierre(null, '1000')).toBeNull();
  expect(retiroDelCierre(5300, '')).toBeNull();
  expect(retiroDelCierre(500, '600')).toBeNull();
  expect(retiroDelCierre(100.1, '0.05')).toBe(100.05);
});

test('quien cierra va primero en la fila de personas', () => {
  expect(personasDelRetiro(personas, 3).map((p) => p.id)).toEqual([3, 1, 2]);
  expect(personasDelRetiro(personas, 99).map((p) => p.id)).toEqual([1, 2, 3]);
});

// La persona viene marcada por omisión y el retiro no se corrige: la confirmación lo dice.
test('el resumen de la confirmación nombra el retiro y a quién', () => {
  expect(resumenDelRetiro({ retirar: true, retiro: 4300, quien: 'Luis', contado: 5300, currency: 'MXN' }))
    .toBe('Se retiran $4,300 · se los lleva Luis. No podrás modificarla después.');
  expect(resumenDelRetiro({ retirar: false, retiro: 0, quien: null, contado: 5300, currency: 'MXN' }))
    .toBe('Todo se queda en el cajón: $5,300. No podrás modificarla después.');
  expect(resumenDelRetiro({ retirar: true, retiro: 0, quien: null, contado: 1000, currency: 'MXN' }))
    .toBe('Todo se queda en el cajón: $1,000. No podrás modificarla después.');
  expect(resumenDelRetiro({ retirar: true, retiro: null, quien: null, contado: null, currency: 'MXN' }))
    .toBe('No podrás modificarla después.');
});

// Revisión de código: con «No» no hay retiro que rebase lo contado, así que la caja no se pinta de
// rojo sin un mensaje que lo explique (fijo $1,000 en un cajón con $800).
test('con No, un fijo mayor a lo contado no pinta la caja de rojo', () => {
  const { container } = render(<Provider><FondoQueSeDeja value="1000" onChange={vi.fn()} contado={800} fijo={1000}
    retirar={false} onRetirar={vi.fn()} personas={personas} quien={2} onQuien={vi.fn()} currency="MXN" /></Provider>);
  expect(container.querySelector('[data-alerta]')).toBeNull();
  const conSi = render(<Provider><FondoQueSeDeja value="1000" onChange={vi.fn()} contado={800} fijo={1000}
    retirar onRetirar={vi.fn()} personas={personas} quien={2} onQuien={vi.fn()} currency="MXN" /></Provider>);
  expect(conSi.container.querySelector('[data-alerta]')).not.toBeNull();
});

// Revisión de código: elegir a la persona no es «capturar lo contado»; el aviso va aparte y dice qué
// tocar y por cuánto.
test('falta quién se lleva el retiro: aviso propio con el monto', () => {
  expect(avisoDeQuienSeLoLleva(true, 4300, 'MXN')).toBe(`Elige quién se lleva el retiro de ${money(4300, 'MXN')}`);
  expect(avisoDeQuienSeLoLleva(false, 4300, 'MXN')).toBeNull();
  expect(avisoDeQuienSeLoLleva(true, null, 'MXN')).toBeNull();
});
