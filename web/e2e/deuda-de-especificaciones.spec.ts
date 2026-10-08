import { test, expect, type Page } from '@playwright/test';

import { tokenDeApi } from './ambiente';
import { botonCobrar, crearCuentaPorApi, entrar as entrarAlPos, fichas, mandarPorApi, productoPorNombre } from './pos';

// TAREAS QUE QUEDARON ABIERTAS EN SPECS YA ENTREGADOS.
//
// Las dos exigen el ambiente DESPLEGADO y una ventana de 1024×600, y por eso se quedaron sin
// cerrar: no se pueden medir en vitest ni contra un build local. Viven aquí y no en el spec para
// que dejen de depender de que alguien se acuerde de medirlas a mano.
//
//   - spec 001, T037 — paridad local ↔ desplegado del ticket (SC-006 / FR-009).
//   - spec 005, T047 — renglones de producto con seis pedidos en curso (SC-005).

async function entrar(page: Page) {
  await entrarAlPos(page);
  const ahoraNo = page.getByRole('button', { name: /Ahora no/i });
  if (await ahoraNo.isVisible().catch(() => false)) await ahoraNo.click();
  await page.waitForLoadState('networkidle');
}

// Mismo método que el documento de presupuesto: el mosaico es el único grid de más de una columna
// con más de cuatro hijos, y el piso real es min(contenedor.bottom, 600).
async function renglonesVisibles(page: Page): Promise<number> {
  return page.evaluate(() => {
    const grid = [...document.querySelectorAll('div')].find((d) => {
      const s = getComputedStyle(d);
      return s.display === 'grid' && s.gridTemplateColumns.split(' ').length > 1 && d.children.length > 4;
    });
    if (!grid) return 0;
    let cont: HTMLElement | null = grid.parentElement;
    while (cont && getComputedStyle(cont).overflowY !== 'auto') cont = cont.parentElement;
    const caja = (cont ?? grid).getBoundingClientRect();
    const gap = parseFloat(getComputedStyle(grid).rowGap || '0');
    const paso = grid.children[0].getBoundingClientRect().height + gap;
    return Math.floor((Math.min(caja.bottom, 600) - caja.top + gap) / paso);
  });
}

// spec 005 · SC-005, con la fila de cuentas de la 030 — las cuentas no se comen el alto del catálogo.
//
// La fila existe para que el operador vea todas las cuentas sin salir de la pantalla de venta. Lo
// que no puede hacer es cobrarle el espacio al mosaico: vive en el MISMO renglón que el buscador y
// pinta solo las fichas que caben, así que el mosaico tiene los mismos renglones con cero cuentas que
// con seis.
test('005/T047 · con cuentas vivas el mosaico conserva sus renglones', async ({ page }) => {
  await entrar(page);
  const antes = await renglonesVisibles(page);
  expect(antes, 'no se encontró el mosaico').toBeGreaterThan(0);

  const jwt = await tokenDeApi();
  const producto = await productoPorNombre(jwt, 'Dedos de Queso Pza');
  for (let i = 0; i < 6; i++) await crearCuentaPorApi(jwt, producto);
  await page.reload();
  await entrar(page);
  await expect(fichas(page).first()).toBeVisible({ timeout: 30_000 });
  const despues = await renglonesVisibles(page);

  console.log(`[medido 1024×600] 005/SC-005 · mosaico: ${antes} renglones sin cuentas nuevas, ${despues} con seis más`);
  expect(despues, 'la fila de cuentas le cobró alto al mosaico').toBe(antes);
  expect(despues, 'el mosaico se quedó sin renglones').toBeGreaterThanOrEqual(2);
});

// spec 001 · SC-006 / FR-009 — el ticket funciona contra el ambiente DESPLEGADO sin instalar nada.
//
// El riesgo que cierra: que la vista previa dependa de algo que solo existe en la máquina de quien
// programa (una extensión, un agente, un archivo local). Si algo solo funciona en localhost, la
// feature no cumple. Lo que NO se puede automatizar es el papel: eso se verifica a mano.
test('001/T037 · el ticket se ve contra el desplegado, sin instalar nada', async ({ page }) => {
  // Lo que se vigila es que la vista previa no dependa de algo que solo existe en la máquina de
  // quien programa: una petición que NO SALE (red caída, host inalcanzable, esquema `file:`) o un
  // recurso que la CSP del desplegado bloquea.
  //
  // NO se vigila cualquier error de consola, y eso es a propósito: el iframe del ticket está
  // sandboxeado sin `allow-scripts` —el control de seguridad haciendo su trabajo— y el navegador lo
  // reporta como error; y el 401 del sondeo de sesión antes del login es el flujo normal. Un assert
  // de "cero errores de consola" convierte los dos en fallos y enseña a ignorar el test.
  const fallos: string[] = [];
  page.on('requestfailed', (r) => fallos.push(`no salió: ${r.method()} ${r.url()} — ${r.failure()?.errorText}`));
  page.on('console', (m) => {
    if (m.type() === 'error' && /Content Security Policy|Refused to load/i.test(m.text())) {
      fallos.push(`CSP: ${m.text()}`);
    }
  });

  // Un pedido propio, creado y mandado como lo haría otra tableta: la prueba ya no depende de que el
  // ambiente tenga deuda. La limpieza de la suite lo cobra y lo entrega al final.
  const jwt = await tokenDeApi();
  const producto = await productoPorNombre(jwt, 'Dedos de Queso Pza');
  const { order } = await mandarPorApi(jwt, (await crearCuentaPorApi(jwt, producto)).id);
  await entrar(page);
  await page.goto(`/pos?pedido=${order.id}`);
  await botonCobrar(page).click({ timeout: 30_000 });
  await page.getByRole('dialog').last().getByRole('button', { name: /^Cuenta$/ }).click();
  const ticket = page.locator('[role="dialog"]').last();
  await expect(ticket).toBeVisible({ timeout: 15_000 });
  // MEDIR DESPUÉS DE QUE LA ANIMACIÓN ASIENTE, no en cuanto el diálogo es "visible".
  //
  // Chakra entra el diálogo con un `scale`, y `boundingBox()` devuelve la caja TRANSFORMADA: el
  // mismo botón mide 42 px a media animación y 44 px asentado. Medido: un assert de piso táctil
  // hecho al instante reporta una violación que no existe, y mandó a "arreglar" un botón que ya
  // cumplía. Vale para cualquier medida de píxeles sobre un diálogo.
  await page.waitForTimeout(600);

  // El logo viaja como data URI y NO como <img src> a la API: la CSP de producción
  // (`img-src 'self' data:`) bloquearía una imagen servida desde el otro dominio, así que un
  // ticket que funcionara en local con un src remoto saldría en blanco en el desplegado.
  const imgs = await ticket.locator('img').evaluateAll((els) =>
    els.map((e) => (e as HTMLImageElement).src.slice(0, 12)));
  for (const src of imgs) {
    expect(src.startsWith('data:'),
      `el ticket carga una imagen desde ${src}…: la CSP del desplegado la bloquea y el papel sale sin logo`)
      .toBeTruthy();
  }

  // El botón de imprimir se alcanza SIN desplazarse, en 1024×600 (SC-007).
  //
  // Se busca DENTRO del diálogo del ticket y no en toda la página: `Imprimir` como nombre accesible
  // hace match por substring, y el shell trae "Impresión" en la navegación. Medir el de la
  // navegación daba 42 px y hacía fallar un test que hablaba de otro botón.
  const imprimir = ticket.getByRole('button', { name: /Imprimir/i }).first();
  await expect(imprimir).toBeVisible();
  const caja = await imprimir.boundingBox();
  expect(caja!.y + caja!.height,
    'hay que desplazarse para llegar a imprimir').toBeLessThanOrEqual(600);
  expect(Math.round(caja!.height), 'el botón de imprimir no llega al piso táctil').toBeGreaterThanOrEqual(44);

  // Nada falló en la red ni en la consola: si la vista previa dependiera de algo instalado, aquí
  // aparecería el intento fallido.
  expect(fallos, `la vista previa falló contra el desplegado:\n${fallos.join('\n')}`).toHaveLength(0);

  console.log('[medido 1024×600] 001/SC-006 · ticket desplegado: sin peticiones fallidas, ' +
    `${imgs.length} imagen(es) en data URI, imprimir visible sin desplazarse. ` +
    'El PAPEL se verifica a mano: no se puede automatizar.');
});
