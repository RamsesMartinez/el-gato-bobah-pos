import { expect, type Page } from '@playwright/test';

// Dónde vive cada destino en el menú en dos niveles (spec 085). Copia deliberada de
// web/src/app/nav.ts: si el menú cambia de lugar algo, la prueba que lo busca aquí lo dice.
export const MODULO_DE: Record<string, string> = {
  Vender: 'Vender', Pedidos: 'Pedidos',
  Caja: 'Caja', Ventas: 'Caja', Gastos: 'Caja',
  Almacén: 'Inventario', 'Se echó a perder': 'Inventario',
  Menú: 'Menú', Plataformas: 'Menú',
  Asistencia: 'Personal', Empleados: 'Personal',
  Reportes: 'Reportes',
  Negocio: 'Ajustes', Impresión: 'Ajustes', Integraciones: 'Ajustes', Interfaz: 'Ajustes', 'Mi cuenta': 'Ajustes',
};

export const riel = (page: Page) => page.locator('[data-rail-list]');

// abrirDelMenu llega a un destino como lo haría quien opera: si su módulo abre directo, lo toca; si
// abre panel, toca el módulo y luego el destino. Sin `goto`: una recarga borra lo que vive en la
// pestaña (el aviso de versión, la cuenta en captura).
export async function abrirDelMenu(page: Page, destino: string) {
  const modulo = MODULO_DE[destino];
  if (!modulo) throw new Error(`«${destino}» no está en el menú`);
  const boton = riel(page).getByRole('button', { name: modulo, exact: true });
  if (await boton.count()) {
    await boton.scrollIntoViewIfNeeded();
    await boton.click();
    const panel = page.getByRole('navigation', { name: modulo });
    await expect(panel).toBeVisible();
    await panel.getByRole('link', { name: destino, exact: true }).click();
    await expect(panel).toHaveCount(0);
    return;
  }
  const directo = riel(page).getByRole('link', { name: new RegExp(`^${modulo}`) });
  await directo.scrollIntoViewIfNeeded();
  await directo.click();
}

// rotulosDelRiel: los módulos del primer nivel, en orden.
export async function rotulosDelRiel(page: Page): Promise<string[]> {
  return riel(page).locator(':scope > div > a, :scope > div > button').evaluateAll((els) =>
    els.map((e) => (e.querySelector('p')?.textContent ?? e.textContent ?? '').trim()));
}
