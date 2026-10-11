import { fireEvent, screen, within } from '@testing-library/react';
import { MONTH_NAMES } from '../utils/calendar';

// pickDate elige `iso` (AAAA-MM-DD) en el DatePickerButton rotulado `label`: abre la hoja, se mueve
// de mes hasta el del día y lo toca. Es el reemplazo de teclear en un `input type="date"`, que ya no
// existe en el POS.
export async function pickDate(label: string, iso: string): Promise<void> {
  fireEvent.click(screen.getByRole('button', { name: label }));
  const sheet = await screen.findByRole('dialog', { name: label });
  const [y, m, d] = iso.split('-').map(Number);
  const target = `${MONTH_NAMES[m - 1]} ${y}`;
  for (let i = 0; i < 36; i++) {
    const shown = within(sheet).getByText(/^[a-z]+ \d{4}$/).textContent ?? '';
    if (shown === target) break;
    const [sm, sy] = shown.split(' ');
    const before = Number(sy) * 12 + MONTH_NAMES.indexOf(sm) > y * 12 + (m - 1);
    fireEvent.click(within(sheet).getByRole('button', { name: before ? 'Mes anterior' : 'Mes siguiente' }));
  }
  fireEvent.click(within(sheet).getByRole('button', { name: String(d) }));
}
