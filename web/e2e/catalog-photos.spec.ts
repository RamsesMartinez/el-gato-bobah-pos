import { test, expect, type Page } from '@playwright/test';
import { API, tokenDeApi } from './ambiente';
import { iniciarSesion } from './sesion';

// FOTOS DEL CATÁLOGO (spec 091) de punta a punta, contra la API local y su almacenamiento S3:
// subir una foto de celular girada, verla servida desde el dominio de medios (no desde la API),
// cambiarla (la anterior deja de existir) y quitarla.
//
// La foto se arma aquí: un JPEG de 1600×1200 con la mitad de arriba roja y la de abajo azul, y la
// orientación EXIF 6 (girar 90° a la derecha), como la guarda un celular de lado. Bien leída, el rojo
// queda a la DERECHA de la foto guardada; si alguien quita la orientación, queda arriba.
//
// Capturas: en test-results/ (no se versionan).

test.use({ viewport: { width: 1024, height: 600 }, hasTouch: true });

const NOMBRE = `Foto e2e ${Date.now()}`;
let productId = 0;
let jwt = '';

function conOrientacion6(jpeg: Buffer): Buffer {
  const tiff = Buffer.from([
    0x4d, 0x4d, 0x00, 0x2a, 0x00, 0x00, 0x00, 0x08, // MM, 42, IFD en 8
    0x00, 0x01, // una entrada
    0x01, 0x12, 0x00, 0x03, 0x00, 0x00, 0x00, 0x01, 0x00, 0x06, 0x00, 0x00, // Orientation = 6
    0x00, 0x00, 0x00, 0x00, // sin más IFD
  ]);
  const body = Buffer.concat([Buffer.from('Exif\0\0', 'binary'), tiff]);
  const app1 = Buffer.concat([Buffer.from([0xff, 0xe1, (body.length + 2) >> 8, (body.length + 2) & 0xff]), body]);
  return Buffer.concat([jpeg.subarray(0, 2), app1, jpeg.subarray(2)]);
}

async function jpegDePrueba(page: Page, arriba: string, abajo: string): Promise<Buffer> {
  const b64 = await page.evaluate(([a, b]) => {
    const c = document.createElement('canvas');
    c.width = 1600; c.height = 1200;
    const x = c.getContext('2d')!;
    x.fillStyle = a; x.fillRect(0, 0, 1600, 600);
    x.fillStyle = b; x.fillRect(0, 600, 1600, 600);
    return c.toDataURL('image/jpeg', 0.9).split(',')[1];
  }, [arriba, abajo]);
  return Buffer.from(b64, 'base64');
}

async function api(path: string, init: RequestInit = {}) {
  const r = await fetch(`${API}${path}`, { ...init, headers: { Authorization: `Bearer ${jwt}`, 'Content-Type': 'application/json', ...(init.headers ?? {}) } });
  if (!r.ok) throw new Error(`${init.method ?? 'GET'} ${path}: ${r.status} ${await r.text()}`);
  return r.json();
}

test.beforeAll(async () => {
  jwt = await tokenDeApi();
  const cats = await api('/admin/categories');
  const p = await api('/admin/products/', { method: 'POST', body: JSON.stringify({ name: NOMBRE, categoryId: cats.items[0].id, price: 10 }) });
  productId = p.id;
});

test.afterAll(async () => {
  // Lo que la prueba crea lo deja como estaba: sin foto (borra los archivos) y archivado.
  if (!productId) return;
  await fetch(`${API}/admin/products/${productId}/image`, { method: 'DELETE', headers: { Authorization: `Bearer ${jwt}` } });
  await api(`/admin/products/${productId}`, { method: 'PATCH', body: JSON.stringify({ active: false }) }).catch(() => {});
});

async function abrirEditor(page: Page) {
  await page.goto('/catalogo/productos');
  await page.waitForLoadState('networkidle');
  await iniciarSesion(page);
  await page.goto('/catalogo/productos');
  await page.getByPlaceholder(/Buscar/).first().fill(NOMBRE);
  const fila = page.getByRole('row').filter({ hasText: NOMBRE });
  await expect(fila).toBeVisible({ timeout: 15_000 });
  await fila.getByRole('button', { name: 'Editar' }).click();
  await expect(page.getByRole('dialog').getByText('Editar producto')).toBeVisible();
}

test('F1 · subir, cambiar y quitar la foto de un producto', async ({ page }) => {
  const peticiones: string[] = [];
  page.on('request', (r) => { if (r.resourceType() === 'image') peticiones.push(r.url()); });

  await abrirEditor(page);
  const editor = page.getByRole('dialog');
  await expect(editor.getByText('Sin foto')).toBeVisible();
  await page.screenshot({ path: test.info().outputPath('1-sin-foto.png') });

  // Subir: el botón grande abre el selector (el input es invisible); se ve «Así se verá».
  const foto = conOrientacion6(await jpegDePrueba(page, '#ff0000', '#0000ff'));
  await page.locator('input[type=file]').setInputFiles({ name: 'celular.jpg', mimeType: 'image/jpeg', buffer: foto });
  await expect(page.getByText('Así se verá')).toBeVisible();
  await page.screenshot({ path: test.info().outputPath('2-asi-se-vera.png') });
  // «Guardar foto» siempre a la vista en 600 px de alto, también con el ajuste abierto.
  await page.getByRole('button', { name: /Ajustar encuadre/ }).click();
  const guardar = page.getByRole('button', { name: 'Guardar foto' });
  const caja = await guardar.boundingBox();
  expect(caja && caja.y + caja.height <= 600, `«Guardar foto» queda fuera de la pantalla: ${JSON.stringify(caja)}`).toBeTruthy();
  await page.screenshot({ path: test.info().outputPath('3-ajustar.png') });
  await guardar.click();
  await expect(editor.getByText('Foto guardada')).toBeVisible({ timeout: 20_000 });

  const prod = await api(`/admin/products/${productId}/image`);
  const url1: string = prod.imageUrl;
  expect(url1).toMatch(/-1200\.webp$/);
  expect(new URL(url1).origin, 'la foto no puede salir de la API').not.toBe(new URL(API).origin);

  // La guardada sale derecha: el rojo de arriba quedó a la derecha.
  // Se baja desde la prueba y no desde la página: el bucket no manda CORS, ni le hace falta a un <img>.
  const chica = Buffer.from(await (await fetch(url1.replace(/-1200\.webp$/, '-400.webp'))).arrayBuffer()).toString('base64');
  const lados = await page.evaluate(async (b64) => {
    const bytes = Uint8Array.from(atob(b64), (c) => c.charCodeAt(0));
    const img = await createImageBitmap(new Blob([bytes], { type: 'image/webp' }));
    const c = new OffscreenCanvas(img.width, img.height);
    const x = c.getContext('2d')!;
    x.drawImage(img, 0, 0);
    const px = (a: number, b: number) => Array.from(x.getImageData(a, b, 1, 1).data.slice(0, 3));
    return { w: img.width, h: img.height, izq: px(20, 200), der: px(380, 200) };
  }, chica);
  expect(lados.w).toBe(400);
  expect(lados.h).toBe(400);
  expect(lados.der[0], `a la derecha debía quedar rojo: ${lados.der}`).toBeGreaterThan(180);
  expect(lados.izq[2], `a la izquierda debía quedar azul: ${lados.izq}`).toBeGreaterThan(180);

  // Cambiar: la anterior deja de existir en el almacenamiento.
  await editor.getByRole('button', { name: 'Cambiar foto' }).click({ trial: true });
  await page.locator('input[type=file]').setInputFiles({ name: 'otra.jpg', mimeType: 'image/jpeg', buffer: await jpegDePrueba(page, '#00aa00', '#ffff00') });
  await page.getByRole('button', { name: 'Guardar foto' }).click();
  await expect(editor.getByText('Foto guardada')).toBeVisible({ timeout: 20_000 });
  const url2: string = (await api(`/admin/products/${productId}/image`)).imageUrl;
  expect(url2).not.toBe(url1);
  expect((await fetch(url1)).status, 'la foto anterior sigue en el almacenamiento').toBe(404);

  // Quitar, con confirmación.
  await editor.getByRole('button', { name: 'Quitar foto' }).click();
  await page.getByRole('button', { name: 'Quitar', exact: true }).click();
  await expect(editor.getByText('Sin foto')).toBeVisible();
  expect((await fetch(url2)).status).toBe(404);
  await page.screenshot({ path: test.info().outputPath('4-quitada.png') });

  // Ninguna imagen se pidió a la API.
  const aLaApi = peticiones.filter((u) => u.startsWith(new URL(API).origin));
  expect(aLaApi, 'imágenes servidas por la API').toEqual([]);
});

test('F2 · la pestaña Categorías abre en mosaico y cambia de vista', async ({ page }) => {
  await page.goto('/catalogo/categorias');
  await page.waitForLoadState('networkidle');
  await iniciarSesion(page);
  await page.goto('/catalogo/categorias');
  await expect(page.getByRole('button', { name: 'Ver como mosaico' })).toHaveAttribute('aria-pressed', 'true');
  await page.screenshot({ path: test.info().outputPath('5-categorias-mosaico.png') });
  // Una descarga del sprite para toda la pantalla, no una por categoría.
  const sprites: string[] = [];
  page.on('request', (r) => { if (r.url().includes('food-families')) sprites.push(r.url()); });
  await page.getByRole('button', { name: 'Ver como tabla' }).click();
  await expect(page.getByRole('columnheader', { name: 'Familia' })).toBeVisible();
  await page.screenshot({ path: test.info().outputPath('6-categorias-tabla.png') });
  await page.getByRole('button', { name: 'Ver como lista y panel' }).click();
  await expect(page.getByText('Familia de comida')).toBeVisible();
  await page.screenshot({ path: test.info().outputPath('7-categorias-panel.png') });
  expect(new Set(sprites).size, `sprite pedido varias veces: ${sprites.join(', ')}`).toBeLessThanOrEqual(1);
  // Toda pieza tocable de la barra mide al menos 44 px.
  for (const name of ['Ver como mosaico', 'Ver como tabla', 'Ver como lista y panel']) {
    const b = await page.getByRole('button', { name }).boundingBox();
    expect(b!.height).toBeGreaterThanOrEqual(44);
  }
});
