import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';
import { Provider } from '../../components/ui/provider';
import { ConteoDeTerminales } from './ConteoDeTerminales';
import { faltanTerminales } from './terminalesPorContar';

// ARQUEO POR TERMINAL (punto 9): se escribe el total del corte de cada terminal que cobró. Vacío
// no es cero: falta capturarlo.
test('pide el total de cada terminal y avisa cuál falta', async () => {
  const onChange = vi.fn();
  render(<Provider><ConteoDeTerminales terminales={[{ terminalId: 5, name: 'Getnet' }, { terminalId: 6, name: 'Hey' }]}
    valores={{}} onChange={onChange} /></Provider>);
  const campo = screen.getByLabelText('Total del corte de Getnet');
  expect(getComputedStyle(campo).minHeight).toBe('48px');
  await userEvent.type(campo, '1');
  expect(onChange).toHaveBeenCalledWith({ 5: '1' });
});

test('faltanTerminales: vacío falta, cero no', () => {
  const t = [{ terminalId: 5, name: 'Getnet' }, { terminalId: 6, name: 'Hey' }];
  expect(faltanTerminales(t, { 5: '0' })).toEqual(['Hey']);
  expect(faltanTerminales(t, { 5: '0', 6: '120.50' })).toEqual([]);
  expect(faltanTerminales(t, { 5: '', 6: 'x' })).toEqual(['Getnet', 'Hey']);
});
