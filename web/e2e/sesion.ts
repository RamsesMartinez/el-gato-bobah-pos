import { expect, test, type Page, type Response } from '@playwright/test';
import { EMPRESA, PASSWORD, USUARIO } from './ambiente';

// iniciarSesion llena el formulario de entrada si está en pantalla, y ESPERA a que la entrada pase.
//
// Por qué no basta con tocar «Entrar»: `/auth` tiene un limitador de 60 peticiones por minuto y por
// IP, y la suite no corre sola — otras suites prueban contra el mismo ambiente desde la misma
// máquina. Con el limitador lleno el formulario se queda en pantalla, y el test esperaba 30 s a un
// POS que nunca iba a cargar: un fallo intermitente que no dice nada del código. Medido el
// 2026-10-08 con tres suites a la vez: dos tests caídos por corrida, cada vez en otro lado.
//
// Con un 429 se espera a que el limitador se vacíe y se reintenta, hasta tres veces, y se dice en
// la bitácora. Cualquier otra cosa que deje el formulario en pantalla sigue fallando.
export async function iniciarSesion(page: Page) {
  const usuario = page.getByPlaceholder('usuario@empresa');
  for (let intento = 1; intento <= 3; intento++) {
    if (!(await usuario.isVisible().catch(() => false))) return;
    let limitado = false;
    const escucha = (r: Response) => { if (r.status() === 429 && r.url().includes('/auth/')) limitado = true; };
    page.on('response', escucha);
    await usuario.fill(`${USUARIO}@${EMPRESA}`);
    await page.getByPlaceholder('Contraseña').fill(PASSWORD);
    await page.getByRole('button', { name: 'Entrar', exact: true }).click();
    await expect.poll(async () => limitado || !(await usuario.isVisible().catch(() => false)), { timeout: 20_000 })
      .toBe(true);
    page.off('response', escucha);
    if (!limitado) return;
    console.log(`[e2e] /auth respondió 429 (intento ${intento}); se espera a que el limitador se vacíe`);
    test.info().setTimeout(test.info().timeout + 70_000);
    await page.waitForTimeout(60_000);
    await page.reload();
    await page.waitForLoadState('networkidle');
  }
  throw new Error('/auth siguió respondiendo 429 después de tres intentos');
}
