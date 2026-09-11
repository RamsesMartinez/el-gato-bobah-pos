import { test, expect, type Page } from '@playwright/test';
import { USUARIO, EMPRESA, PASSWORD } from './ambiente';

// NINGUNA PANTALLA PUEDE ESTAR ROTA SIN QUE LA SUITE SE ENTERE.
//
// Existe por un defecto que vivió HORAS en producción sin que nada lo detectara:
// `/catalogo/opciones` respondía 500 —`string_agg` sobre un conjunto vacío devolvía NULL y sqlc
// había tipado la columna como `string` no nulable— y un solo grupo de modificadores sin opciones
// activas tumbaba la consulta COMPLETA.
//
// La suite ya visitaba esa ruta: `pantalla-en-blanco.spec.ts` › Z6 la recorre. Pero Z6 solo exige
// que la pantalla PINTE algo y que no tire una excepción, y un 500 de la API no deja la pantalla en
// blanco —TanStack Query lo atrapa y la pantalla se queda con su encabezado y sin datos—. Z6 pasaba
// en verde con la pantalla rota, que es el peor resultado posible: un caso que se ve cubierto y no
// cubre.
//
// Este caso mira lo que Z6 no puede ver: el CÓDIGO DE RESPUESTA de todo lo que la pantalla pide.
// Es la guardia de la CLASE, no la de este defecto: cualquier 5xx nuevo, en cualquier ruta, sale
// aquí con el endpoint que lo devolvió.
const RUTAS = [
  '/pos', '/pedidos', '/ventas', '/caja', '/gastos', '/almacen', '/reportes',
  '/catalogo/productos', '/catalogo/opciones', '/empleados', '/negocio', '/impresion',
  '/apariencia', '/cuenta',
];

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

test('N1 · ninguna pantalla recibe un 5xx de la API', async ({ page }) => {
  const rotos: string[] = [];
  let donde = '/';
  page.on('response', (r) => {
    if (r.status() >= 500 && r.url().includes('/api/')) {
      rotos.push(`${donde} → ${r.request().method()} ${new URL(r.url()).pathname} devolvió ${r.status()}`);
    }
  });

  await entrar(page);
  for (const ruta of RUTAS) {
    donde = ruta;
    await page.goto(ruta);
    await page.waitForLoadState('networkidle');
    // Un respiro para las consultas que arrancan después del primer render.
    await page.waitForTimeout(400);
  }

  expect(rotos, `hay pantallas rotas:\n  ${rotos.join('\n  ')}`).toEqual([]);
});

// Y EL CAMINO CONCRETO QUE ESTUVO ROTO, porque recorrer la ruta no bastaba: el defecto solo salía
// en dos de las tres pestañas. «Activos» respondía 200 —todos los grupos activos tenían al menos
// una opción activa— mientras «Inactivos» y «Todos» daban 500. Una prueba que solo abre la pantalla
// habría pasado en verde con el catálogo caído.
test('N2 · las tres pestañas del catálogo de modificadores responden', async ({ page }) => {
  const rotos: string[] = [];
  page.on('response', (r) => {
    if (r.status() >= 500 && r.url().includes('/admin/groups')) {
      rotos.push(`${new URL(r.url()).search || '(sin filtro)'} devolvió ${r.status()}`);
    }
  });

  await entrar(page);
  await page.goto('/catalogo/opciones');
  await page.waitForLoadState('networkidle');

  for (const pestana of ['Activos', 'Inactivos', 'Todos']) {
    await page.getByRole('button', { name: new RegExp(`^${pestana}`) }).first().click();
    await page.waitForLoadState('networkidle');
    await page.waitForTimeout(400);
  }

  expect(rotos, `el catálogo de modificadores se rompió en:\n  ${rotos.join('\n  ')}`).toEqual([]);
  // Y la pantalla llegó a pintar grupos, para que esto no pase en vacío el día que la lista venga
  // sin nada: sin esto, "cero 500" y "cero peticiones" se ven igual.
  await expect(page.getByText(/opciones · por defecto elige/).first()).toBeVisible({ timeout: 15_000 });
});
