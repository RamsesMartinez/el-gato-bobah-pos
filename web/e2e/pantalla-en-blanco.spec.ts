import { test, expect, type Page } from '@playwright/test';

import { API, USUARIO, EMPRESA, PASSWORD } from './ambiente';

// NINGUNA RUTA SE QUEDA EN BLANCO, Y NINGUNA TIRA UNA EXCEPCIÓN AL PINTARSE.
//
// La clase de fallo que vigila: un throw durante el render deja el `#root` VACÍO. No hay error
// boundary en la app, así que el operador no ve un mensaje ni un botón — ve una pantalla blanca, y
// desde ahí no puede ni aplicar la versión que lo arregla (el aviso de "Nueva versión disponible"
// lo pinta el toaster, que vive dentro del árbol que acaba de morir). Ya pasó una vez, con el folio
// de plataforma: ver Z4 en docs/matriz-de-pantallas.md.
//
// SE RECORRE CON EL ESTADO DE UNA TABLETA QUE YA SE USÓ, no con un perfil limpio. Dos llaves, y las
// dos hacen falta:
//   - `egb:ticket:v2` con una cuenta guardada por una versión ANTERIOR (le falta un campo que hoy
//     existe): es la forma que tiene el almacén de un operador que no ha cerrado sus cuentas.
//   - `sesion.ultimaEmpresa`: sin ella, `hayQueLimpiar` trata el perfil limpio de Playwright como
//     cambio de empresa y el login llama a `descartarTodo()`, que tira lo sembrado ANTES de que el
//     POS renderice. Sembrar solo el carrito deja este caso pasando en verde con el defecto puesto.
//
// Lo que este caso NO cubre: que la ruta muestre lo CORRECTO. Eso es de cada spec. Aquí solo se
// exige que muestre algo y que no truene.
const RUTAS = [
  '/pos', '/pedidos', '/ventas', '/caja', '/gastos', '/almacen', '/reportes',
  '/catalogo/productos', '/catalogo/opciones', '/empleados', '/negocio', '/impresion',
  '/apariencia', '/cuenta',
];

// loQuePinta devuelve cuántos caracteres visibles tiene el árbol. Cero = pantalla en blanco, que es
// exactamente lo que ve el operador cuando el render muere.
async function loQuePinta(page: Page): Promise<number> {
  return page.evaluate(() => (document.getElementById('root')?.innerText ?? '').trim().length);
}

test('Z6 · ninguna pantalla se queda en blanco con una cuenta guardada por la versión anterior',
  async ({ page, request }) => {
    const r = await request.post(`${API}/auth/login`, {
      data: { username: USUARIO, slug: EMPRESA, password: PASSWORD },
    });
    expect(r.ok(), 'el login del ambiente de pruebas falló').toBeTruthy();
    const companyId: number = (await r.json()).user.companyId;

    const tronó: string[] = [];
    let dónde = '/';
    page.on('pageerror', (e) => tronó.push(`${dónde}: ${e.message.slice(0, 160)}`));

    await page.addInitScript((empresa: number) => {
      localStorage.setItem('sesion.ultimaEmpresa', String(empresa));
      localStorage.setItem('egb:ticket:v2', JSON.stringify({
        state: {
          tabs: [{
            id: 'vieja-1', num: 1, folioName: 'Tigre', lines: [], envio: '',
            serviceType: 'mostrador', customerName: '', platformId: null,
            // le falta platformOrderRef, como a toda cuenta guardada antes del folio de plataforma
          }],
          activeId: 'vieja-1', seq: 2,
        },
        version: 0,
      }));
    }, companyId);

    await page.goto('/');
    await page.waitForLoadState('networkidle');
    const usuario = page.getByPlaceholder('usuario@empresa');
    if (await usuario.isVisible().catch(() => false)) {
      await usuario.fill(`${USUARIO}@${EMPRESA}`);
      await page.getByPlaceholder('Contraseña').fill(PASSWORD);
      await page.getByRole('button', { name: 'Entrar', exact: true }).click();
      await page.waitForURL(/\/(pos)?$/);
    }

    // La cuenta sembrada tiene que seguir ahí: si el login la tiró, el recorrido se está haciendo
    // con un perfil limpio y este caso no prueba lo que dice probar.
    await expect(page.getByRole('button', { name: /Tigre/ }),
      'el carrito sembrado no sobrevivió al login: el recorrido no representa la tableta de un operador')
      .toBeVisible({ timeout: 30_000 });

    const vacías: string[] = [];
    for (const ruta of RUTAS) {
      dónde = ruta;
      await page.goto(ruta);
      await page.waitForLoadState('networkidle');
      const pintado = await loQuePinta(page);
      if (pintado === 0) vacías.push(ruta);
    }

    expect(tronó, `una excepción durante el render deja la pantalla en blanco:\n${tronó.join('\n')}`)
      .toHaveLength(0);
    expect(vacías, `estas rutas no pintaron nada: ${vacías.join(', ')}`).toHaveLength(0);
    console.log(`[medido 1024×600] Z6 · ${RUTAS.length} rutas recorridas con una cuenta guardada ` +
      'por la versión anterior: ninguna en blanco, ninguna excepción');
  });
