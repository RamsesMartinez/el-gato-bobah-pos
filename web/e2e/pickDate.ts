import type { Page } from '@playwright/test';

const MONTHS = ['enero', 'febrero', 'marzo', 'abril', 'mayo', 'junio', 'julio', 'agosto', 'septiembre', 'octubre', 'noviembre', 'diciembre'];

// pickDate elige `iso` (AAAA-MM-DD) en el selector de fecha táctil rotulado `label`: abre la hoja,
// se mueve de mes y toca el día. Reemplaza a `fill()` sobre un `input type="date"`, que el POS ya no
// usa (la constitución prohíbe el calendario del sistema en la tableta).
export async function pickDate(page: Page, label: string, iso: string): Promise<void> {
  await page.getByRole('button', { name: label, exact: true }).click();
  const sheet = page.getByRole('dialog', { name: label });
  const [y, m, d] = iso.split('-').map(Number);
  const target = y * 12 + (m - 1);
  for (let i = 0; i < 36; i++) {
    const shown = (await sheet.getByText(/^[a-z]+ \d{4}$/).textContent()) ?? '';
    const [sm, sy] = shown.split(' ');
    const current = Number(sy) * 12 + MONTHS.indexOf(sm);
    if (current === target) break;
    await sheet.getByRole('button', { name: current > target ? 'Mes anterior' : 'Mes siguiente' }).click();
  }
  await sheet.getByRole('button', { name: String(d), exact: true }).click();
}
