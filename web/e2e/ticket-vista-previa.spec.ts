import { randomUUID } from 'node:crypto';
import { expect, test, type Page } from '@playwright/test';
import { API, tokenDeApi } from './ambiente';
import { entregarPorApi, pagarPorApi } from './pos';
import { iniciarSesion } from './sesion';

// #233: la vista previa del ticket se cortaba a la derecha. El documento del ticket mide 80 mm de
// papel MÁS 4 mm de relleno por lado (~333 px) y el marco medía solo los 80 mm (302 px): la columna
// de importes quedaba fuera, «TOTAL» salía sin cifra y cada renglón sin su importe.
//
// El pedido junta lo que ensancha un ticket: el nombre más largo del catálogo, cantidades que dan
// importes de cinco cifras, opciones con precio, envío y descuento.

const cab = (jwt: string) => ({ Authorization: `Bearer ${jwt}`, 'Content-Type': 'application/json' });

interface Opcion { id: number; priceDelta: string; maxPerLine: number }
interface Producto { id: number; name: string; price: string; type: string; groups?: Array<{ options: Opcion[] }> }

// El número de pedido se repite entre turnos de caja; el nombre de cliente distingue el renglón de
// este pedido en la lista de entregadas. El pedido queda cobrado y entregado: no deja nada abierto.
async function pedidoAnchoEntregadoYCobrado(cliente: string) {
  const jwt = await tokenDeApi();
  const menu = await (await fetch(`${API}/pos/menu`, { headers: cab(jwt) })).json() as { products: Producto[] };
  const simples = menu.products.filter((p) => p.type === 'simple' && Number(p.price) > 0);
  const largo = [...simples].sort((a, b) => b.name.length - a.name.length)[0];
  const conOpciones = simples.find((p) => p.groups?.some((g) => g.options.some((o) => Number(o.priceDelta) > 0)));
  if (!largo || !conOpciones) throw new Error('el catálogo no tiene un producto con opciones con precio');
  const opcion = conOpciones.groups!.flatMap((g) => g.options).find((o) => Number(o.priceDelta) > 0)!;

  const r = await fetch(`${API}/orders`, {
    method: 'POST', headers: cab(jwt),
    body: JSON.stringify({
      clientUuid: randomUUID(),
      serviceType: 'domicilio',
      customerName: cliente,
      deliveryFee: 35,
      discountAmount: 10,
      lines: [
        { productId: largo.id, qty: 99, modifiers: [] },
        { productId: conOpciones.id, qty: 1, modifiers: [{ optionId: opcion.id, qty: 1 }] },
      ],
    }),
  });
  if (!r.ok) throw new Error(`crear pedido: ${r.status} ${await r.text()}`);
  const { id } = await r.json() as { id: number };
  await entregarPorApi(jwt, id);
  await pagarPorApi(jwt, id);
  return id;
}

async function abrirSuTicket(page: Page, cliente: string) {
  await page.goto('/pedidos');
  await page.waitForLoadState('networkidle');
  await iniciarSesion(page);
  // Al entrar, la app lleva a Vender: se vuelve a Pedidos ya con sesión.
  await page.goto('/pedidos');
  const renglon = page.locator('[data-delivered-row]').filter({ hasText: cliente });
  await renglon.getByRole('button', { name: 'Ticket', exact: true }).click({ timeout: 30_000 });
  const marco = page.getByTitle('Vista previa del ticket');
  await expect(marco).toBeVisible();
  const doc = (await marco.elementHandle())!;
  const frame = (await doc.contentFrame())!;
  await frame.waitForSelector('td.total');
  return { marco, frame };
}

for (const ancho of [{ w: 1024, h: 600 }, { w: 1280, h: 800 }]) {
  test.describe(`a ${ancho.w}×${ancho.h}`, () => {
    test.use({ viewport: { width: ancho.w, height: ancho.h }, hasTouch: true });

    test('la vista previa deja ver los importes y la cifra del TOTAL (#233)', async ({ page }, info) => {
      const cliente = `Ticket ancho ${randomUUID().slice(0, 6)}`;
      await pedidoAnchoEntregadoYCobrado(cliente);
      const { marco, frame } = await abrirSuTicket(page, cliente);

      // Dentro del documento: nada se sale del ancho que el marco deja ver.
      const dentro = await frame.evaluate(() => {
        const vista = document.documentElement.clientWidth;
        const importes = [...document.querySelectorAll('td.r')].map((td) => ({
          texto: td.textContent?.trim() ?? '', derecha: td.getBoundingClientRect().right,
        }));
        return { vista, ancho: document.documentElement.scrollWidth, importes };
      });
      expect(dentro.importes.length, 'el ticket no trae columna de importes').toBeGreaterThan(3);
      expect(dentro.ancho, `el ticket mide ${dentro.ancho} px y el marco deja ver ${dentro.vista} px`)
        .toBeLessThanOrEqual(dentro.vista);
      for (const i of dentro.importes) {
        expect(i.derecha, `«${i.texto}» termina en ${i.derecha} px; el marco deja ver ${dentro.vista} px`)
          .toBeLessThanOrEqual(dentro.vista);
      }

      // En pantalla: la cifra del TOTAL cae dentro del diálogo, no detrás de su borde.
      const total = await frame.locator('td.r.total').boundingBox();
      const caja = await marco.boundingBox();
      const dialogo = await page.getByRole('dialog').boundingBox();
      expect(total && caja && dialogo, 'no se pudo medir el ticket').toBeTruthy();
      expect(total!.x + total!.width, 'la cifra del TOTAL queda fuera del marco').toBeLessThanOrEqual(caja!.x + caja!.width + 0.5);
      expect(caja!.x + caja!.width, 'el marco se sale del diálogo').toBeLessThanOrEqual(dialogo!.x + dialogo!.width + 0.5);
      await expect(frame.locator('td.r.total')).toHaveText(/\$[\d,]+/);

      // Capturas del arriba y del abajo del ticket, para revisarlo a ojo. Van a test-results/.
      await page.screenshot({ path: info.outputPath(`ticket-${ancho.w}-arriba.png`), animations: 'disabled' });
      await frame.locator('td.r.total').scrollIntoViewIfNeeded();
      await page.screenshot({ path: info.outputPath(`ticket-${ancho.w}-total.png`), animations: 'disabled' });
    });
  });
}
