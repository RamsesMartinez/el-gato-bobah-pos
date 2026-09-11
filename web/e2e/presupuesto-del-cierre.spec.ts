import { test, expect, type Page } from '@playwright/test';
import { USUARIO, EMPRESA, PASSWORD } from './ambiente';

// CUÁNTO ALTO Y CUÁNTOS TOQUES CUESTAN LAS DOS PANTALLAS QUE TOCA LA SPEC 015, medido en un
// navegador real a 1024×600.
//
// Existe porque el plan de esa feature afirmó dos cosas falsas y las dos se pudieron haber ido al
// código: que la tabla del cierre "se acorta" —contando campos quitados en vez de renglones— y que
// Ajustes tiene 1024 px de ancho, cuando su contenedor son 560. Aquí no se supone nada.
//
// LO QUE SE MIDE ES EL CONTENEDOR QUE SE DESPLAZA, NO EL DOCUMENTO. La primera versión de este
// archivo leía `document.documentElement.scrollHeight` y reportaba 600 px —exactamente el
// viewport— en las dos pantallas: el AppShell es `h="100dvh" overflow="hidden"` y quien hace
// scroll es el `<Box flex="1" overflowY="auto">` que envuelve al `<Outlet>`. Medir el documento
// da siempre el alto de la ventana, y el número se lee como si todo cupiera.
//
// Se corre contra el ambiente que se le diga (`E2E_BASE_URL`), así que el MISMO archivo da el
// "antes" contra el desplegado y el "después" contra el candidato.

// EL "ANTES", medido el 2026-09-10 contra app-dev con la spec 003 desplegada y los diez métodos
// del negocio: tres «en línea» en automático, cuatro cuyo dinero cae en el cajón.
//
// Solo se COMPARA lo que no depende del turno. El alto de la tabla del cierre y el número de
// capturas quedan determinados por cómo están configurados los métodos, así que se pueden medir
// en dos ambientes y comparar; el alto total de una pantalla depende de cuántos movimientos y
// cuántas cajas tenga el turno, así que se imprime y no se usa como vara.
const ANTES = {
  tablaDelCierre: 575, // px, 10 renglones
  capturasDelCierre: 7, // 6 campos de declarado + el botón de contar el efectivo
  metodosVisiblesEnNegocio: 0, // de 10: la sección arranca en y≈1,210, muy por debajo del pliegue
};

// Lo que pide la constitución para cualquier control que se toque con el dedo.
const MINIMO_TAPPABLE = 44;

// Y cuánto hueco tiene que quedar entre dos de ellos en el mismo renglón. La constitución no pone
// número; éste sale de que un dedo que falla lo hace por milímetros, así que medio objetivo de
// separación es el piso razonable. Medido: 46 px y 63 px entre los tres interruptores de un
// método. Se afirma porque el hueco NO es padding fijo —lo dan los encabezados «Va al cajón» y
// «Automático», que son más anchos que el interruptor—, así que acortar un encabezado lo cierra
// sin que nadie lo note.
const MINIMO_ENTRE_CONTROLES = 22;

async function entrar(page: Page) {
  await page.goto('/');
  await page.waitForLoadState('networkidle');
  const usuario = page.getByPlaceholder('usuario@empresa');
  if (await usuario.isVisible().catch(() => false)) {
    await usuario.fill(`${USUARIO}@${EMPRESA}`);
    await page.getByPlaceholder('Contraseña').fill(PASSWORD);
    await page.getByRole('button', { name: 'Entrar', exact: true }).click();
  }
  await expect(page.getByRole('button', { name: 'Salir' })).toBeVisible({ timeout: 30_000 });
}

// elScroller: el contenedor que de verdad se desplaza.
//
// No basta con "el más alto de los que tienen overflow-y": una caja con `overflowX="auto"` —la que
// envuelve la tabla de métodos— computa su `overflow-y` a `auto` sola, y si queda seleccionada la
// medición reporta el alto de esa tabla en vez del de la pantalla. Se toman solo los contenedores
// que no viven dentro de otro desplazable (quedan la barra lateral y el área de contenido) y de
// esos el más alto. Corre dentro del navegador, así que se define en cada `evaluate`.
function elScroller(): Element {
  const seDesplaza = (e: Element) => ['auto', 'scroll'].includes(getComputedStyle(e).overflowY);
  const todos = [...document.querySelectorAll('*')].filter(seDesplaza);
  const externos = todos.filter((e) => !todos.some((otro) => otro !== e && otro.contains(e)));
  return externos.sort((a, b) => b.clientHeight - a.clientHeight)[0] ?? document.scrollingElement!;
}

test('P1 · la tabla del cierre no creció y no pide más capturas (SC-006)', async ({ page }) => {
  await entrar(page);
  await page.goto('/caja');
  await expect(page.getByRole('tab', { name: 'Cajas' })).toBeVisible({ timeout: 30_000 });
  await expect(page.getByText('Cierre — declarado por método')).toBeVisible({ timeout: 30_000 });

  const medida = await page.evaluate(({ fuenteDelScroller }) => {
    const buscar = new Function(`return (${fuenteDelScroller})()`) as () => Element;
    const t = document.querySelector('table')!;
    const c = buscar();
    return {
      alto: Math.round(t.getBoundingClientRect().height),
      renglones: t.querySelectorAll('tbody tr').length,
      // Cada punto de captura es un toque del operador: un campo que teclear o el botón que abre
      // la hoja de denominaciones. Cerrar y confirmar cuestan lo mismo antes y después, así que no
      // entran en la cuenta — lo que SC-006 protege es que el arqueo único no se pague con más
      // capturas.
      capturas: t.querySelectorAll('tbody input').length + t.querySelectorAll('tbody button').length,
      contenido: Math.round(c.scrollHeight),
      visible: Math.round(c.clientHeight),
    };
  }, { fuenteDelScroller: elScroller.toString() });

  console.log(`[medición] cierre: ${medida.alto}px en ${medida.renglones} renglones · ` +
    `${medida.capturas} capturas · /caja ${medida.contenido}px de contenido en ${medida.visible} visibles`);

  expect(medida.capturas,
    `el cierre pide ${medida.capturas} capturas y antes pedía ${ANTES.capturasDelCierre}: el arqueo único se pagó con más toques (SC-006)`)
    .toBeLessThanOrEqual(ANTES.capturasDelCierre);
  expect(medida.alto,
    `la tabla del cierre mide ${medida.alto}px y antes medía ${ANTES.tablaDelCierre}px`)
    .toBeLessThanOrEqual(ANTES.tablaDelCierre);
});

test('P2 · los métodos en Ajustes: cuántos renglones, cuántos se ven y qué tan grandes son', async ({ page }) => {
  await entrar(page);
  await page.goto('/negocio');
  await expect(page.getByText(/^(Corte de caja|Métodos de cobro)$/)).toBeVisible({ timeout: 30_000 });

  const medida = await page.evaluate(({ fuenteDelScroller }) => {
    const buscar = new Function(`return (${fuenteDelScroller})()`) as () => Element;
    const enc = [...document.querySelectorAll('p,div,span,h1,h2,h3')]
      .find((e) => /^(Corte de caja|Métodos de cobro)$/.test((e.textContent ?? '').trim()));
    if (!enc) return null;
    // El renglón de un método es su fila de interruptores: uno en la lista vieja, tres en la tabla
    // nueva. Se agrupan por posición vertical para contar renglones y no controles, y se toman
    // solo los que van DESPUÉS del encabezado — el interruptor del arqueo ciego vive en la misma
    // caja, arriba, y sin excluirlo cuenta como un método más.
    const interruptores = [...enc.parentElement!.querySelectorAll('[data-scope="switch"][data-part="root"]')]
      .filter((s) => enc.compareDocumentPosition(s) & Node.DOCUMENT_POSITION_FOLLOWING);
    const porRenglon = new Map<number, DOMRect>();
    for (const s of interruptores) {
      const r = s.getBoundingClientRect();
      porRenglon.set(Math.round(r.top), r);
    }
    const c = buscar();
    const tabla = document.querySelector('table');
    // El hueco más chico entre dos interruptores del MISMO renglón.
    const porFila = new Map<number, DOMRect[]>();
    for (const s of interruptores) {
      const r = s.getBoundingClientRect();
      const fila = porFila.get(Math.round(r.top)) ?? [];
      fila.push(r);
      porFila.set(Math.round(r.top), fila);
    }
    let hueco = Infinity;
    for (const fila of porFila.values()) {
      fila.sort((a, b) => a.left - b.left);
      for (let i = 1; i < fila.length; i++) hueco = Math.min(hueco, fila[i].left - (fila[i - 1].left + fila[i - 1].width));
    }
    return {
      huecoMasChico: Number.isFinite(hueco) ? Math.round(hueco) : null,
      renglones: porRenglon.size,
      visibles: [...porRenglon.values()].filter((r) => r.top >= 0 && r.bottom <= innerHeight).length,
      tappableMasChico: Math.min(...[...porRenglon.values()].map((r) => Math.round(r.height))),
      // Desde el inicio del contenido, no desde el viewport: no depende de dónde quedó el scroll.
      empiezaEn: Math.round(enc.getBoundingClientRect().top - c.getBoundingClientRect().top + c.scrollTop),
      contenido: Math.round(c.scrollHeight),
      // El ancho útil de esta página es ~520 px (`<Page maxW="560px">` con su padding), no los 1024
      // de la tableta. Una fila que se desborda a lo ancho deja al operador tocando lo que no ve.
      desborde: tabla ? Math.round(tabla.scrollWidth - tabla.clientWidth) : 0,
    };
  }, { fuenteDelScroller: elScroller.toString() });

  expect(medida, 'no se encontró la sección de métodos de cobro').not.toBeNull();
  console.log(`[medición] /negocio: ${medida!.contenido}px de contenido · ${medida!.renglones} métodos, ` +
    `${medida!.visibles} visibles sin desplazarse · la sección arranca en y=${medida!.empiezaEn} · ` +
    `interruptor más chico ${medida!.tappableMasChico}px de alto, ${medida!.huecoMasChico}px de hueco`);

  expect(medida!.renglones, 'no se encontró ningún renglón de método').toBeGreaterThan(0);
  expect(medida!.visibles,
    'se ven menos métodos sin desplazarse que antes de la 015').toBeGreaterThanOrEqual(ANTES.metodosVisiblesEnNegocio);

  // TRES INTERRUPTORES POR RENGLÓN EXIGEN EL OBJETIVO COMPLETO. La tabla de la 015 puso «Activo»,
  // «Va al cajón» y «Automático» en la misma fila, y con el alto que Chakra le da a un Switch
  // —24 px medidos— un dedo que falla por milímetros cae en el de al lado: apagar «Activo» por
  // error saca el método del cobro a media jornada. La lista vieja tenía un solo interruptor por
  // renglón y aun así medía 20 px; esto no se hereda.
  expect(medida!.tappableMasChico,
    `un interruptor mide ${medida!.tappableMasChico}px de alto y el mínimo con el que un dedo acierta a la primera son ${MINIMO_TAPPABLE}`)
    .toBeGreaterThanOrEqual(MINIMO_TAPPABLE);

  if (medida!.huecoMasChico !== null) {
    expect(medida!.huecoMasChico,
      `entre dos interruptores del mismo renglón quedan ${medida!.huecoMasChico}px: un dedo que falla cae en el de al lado, y «Activo» apagado saca el método del cobro a media jornada`)
      .toBeGreaterThanOrEqual(MINIMO_ENTRE_CONTROLES);
  }

  expect(medida!.desborde, 'la tabla de métodos se desborda a lo ancho').toBeLessThanOrEqual(1);
});
