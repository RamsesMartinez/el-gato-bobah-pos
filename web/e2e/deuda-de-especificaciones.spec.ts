import { test, expect, type Page } from '@playwright/test';

import { USUARIO, EMPRESA, PASSWORD } from './ambiente';

// TAREAS QUE QUEDARON ABIERTAS EN SPECS YA ENTREGADOS.
//
// Las dos exigen el ambiente DESPLEGADO y una ventana de 1024×600, y por eso se quedaron sin
// cerrar: no se pueden medir en vitest ni contra un build local. Viven aquí y no en el spec para
// que dejen de depender de que alguien se acuerde de medirlas a mano.
//
//   - spec 001, T037 — paridad local ↔ desplegado del ticket (SC-006 / FR-009).
//   - spec 005, T047 — renglones de producto con seis pedidos en curso (SC-005).

async function entrar(page: Page) {
  await page.goto('/');
  await page.waitForLoadState('networkidle');
  const usuario = page.getByPlaceholder('usuario@empresa');
  if (await usuario.isVisible().catch(() => false)) {
    await usuario.fill(`${USUARIO}@${EMPRESA}`);
    await page.getByPlaceholder('Contraseña').fill(PASSWORD);
    await page.getByRole('button', { name: 'Entrar', exact: true }).click();
  }
  await expect(page.getByRole('button', { name: 'Cuenta 1' })).toBeVisible({ timeout: 30_000 });
  const ahoraNo = page.getByRole('button', { name: /Ahora no/i });
  if (await ahoraNo.isVisible().catch(() => false)) await ahoraNo.click();
  // La píldora de deuda aparece cuando responde /orders/open, DESPUÉS de que la pantalla ya se
  // pintó. Sin esperar a la red, el test la buscaba antes de tiempo y se saltaba en silencio — que
  // es peor que fallar: un test que se salta no prueba nada y no lo dice.
  await page.waitForLoadState('networkidle');
}

// La píldora de deuda muestra el IMPORTE pendiente y su conteo ("$554\n(5)"), no la palabra "por
// cobrar". Y aparece cuando responde /orders/open, después de que la pantalla ya se pintó: sin
// esperarla, el test se saltaba en silencio — que es peor que fallar, porque un test que se salta
// no prueba nada y no lo dice.
function pildoraDeuda(page: Page) {
  return page.locator('button').filter({ hasText: /^\$[\d,.]+\(\d+\)$/ }).first();
}

async function hayDeuda(page: Page): Promise<boolean> {
  return pildoraDeuda(page).waitFor({ state: 'visible', timeout: 15_000 })
    .then(() => true).catch(() => false);
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

// spec 005 · SC-005 — la barra de pedidos en curso no se come el alto del catálogo.
//
// La barra existe para que el operador vea lo que debe dinero sin salir de la pantalla de venta. Lo
// que no puede hacer es cobrarle el espacio al mosaico: el catálogo es lo que el operador vino a
// usar, y con seis pedidos —el máximo observado en un día— es cuando más grande está la barra.
test('005/T047 · con pedidos en curso el mosaico conserva sus renglones', async ({ page }) => {
  await entrar(page);

  const enCurso = await hayDeuda(page);

  const conBarra = await renglonesVisibles(page);
  expect(conBarra, 'no se encontró el mosaico').toBeGreaterThan(0);

  // El estado SIN barra no se puede fabricar en un ambiente compartido —habría que cobrar los
  // pedidos de alguien más—, así que se mide la propiedad que SC-005 pide de verdad: que la barra
  // SE SOLAPE con el catálogo en vez de empujarlo. Si empujara, el mosaico perdería renglones cada
  // vez que hay deuda, que es justo cuando el operador más los necesita.
  const geometria = await page.evaluate(() => {
    const grid = [...document.querySelectorAll('div')].find((d) => {
      const s = getComputedStyle(d);
      return s.display === 'grid' && s.gridTemplateColumns.split(' ').length > 1 && d.children.length > 4;
    });
    let cont: HTMLElement | null = grid?.parentElement ?? null;
    while (cont && getComputedStyle(cont).overflowY !== 'auto') cont = cont.parentElement;
    const btn = [...document.querySelectorAll('button')].find(
      (b) => /^\$[\d,.]+\(\d+\)$/.test((b.textContent ?? '').trim()));
    if (!cont || !btn) return null;
    const c = cont.getBoundingClientRect();
    const p = btn.getBoundingClientRect();
    return {
      altoCatalogo: Math.round(c.height),
      catalogoLlegaHasta: Math.round(c.bottom),
      barraEmpiezaEn: Math.round(p.top),
    };
  });

  console.log(`[medido 1024×600] 005/SC-005 · pedidos en curso: ${enCurso ? 'sí' : 'no'} · ` +
    `mosaico: ${conBarra} renglones · catálogo: ${geometria?.altoCatalogo}px · ` +
    `la barra empieza en y=${geometria?.barraEmpiezaEn} y el catálogo llega a y=${geometria?.catalogoLlegaHasta}`);

  if (geometria) {
    expect(geometria.barraEmpiezaEn,
      'la barra de pedidos en curso quedó DEBAJO del catálogo en vez de encima: le está cobrando ' +
      'alto al mosaico cada vez que hay deuda, que es cuando el operador más lo necesita')
      .toBeLessThan(geometria.catalogoLlegaHasta);
  }
  expect(conBarra, 'el mosaico se quedó sin renglones con la barra puesta').toBeGreaterThanOrEqual(2);
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

  await entrar(page);

  test.skip(!(await hayDeuda(page)),
    'el ambiente no tiene pedidos por cobrar: no hay ticket que abrir sin crear uno');
  await pildoraDeuda(page).click();

  await page.getByRole('button', { name: 'Ticket', exact: true }).first().click();
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
