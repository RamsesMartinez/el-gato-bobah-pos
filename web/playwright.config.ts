import { defineConfig, devices } from '@playwright/test';

// Los E2E corren contra el AMBIENTE DE PRUEBAS desplegado, no contra un build local.
//
// Es a propósito: lo que estas pruebas tienen que atrapar es el desacuerdo entre el front y el
// servidor —una cifra que la pantalla calcula y el servidor no cobra, un método que rebota, un
// pedido que queda creado y sin cobrar—, y eso solo aparece con los dos hablando de verdad. Un
// mock del backend probaría que la pantalla es consistente consigo misma, que es justo lo que ya
// prueban los tests de vitest.
//
// La VM de pruebas es spot: Google puede reclamarla y aparece apagada sin que nadie la apague. Si
// la suite falla en el primer `goto`, revisa que esté prendida antes de buscar el defecto en el
// código.
const BASE = process.env.E2E_BASE_URL ?? 'https://app-dev.elgatobobah.com';

export default defineConfig({
  testDir: './e2e',
  // Sin paralelo: los tests comparten la base de pruebas y el turno de caja abierto. Dos que cobren
  // a la vez sobre la misma caja se estorban, y un fallo por carrera se lee como un defecto real.
  workers: 1,
  fullyParallel: false,
  // Un reintento: la VM spot y la red de por medio producen fallos que no son del código. Más de
  // uno escondería un defecto intermitente, que es exactamente lo que hay que ver.
  retries: 1,
  reporter: [['list']],
  timeout: 60_000,
  expect: { timeout: 15_000 },
  use: {
    baseURL: BASE,
    // La tableta real, no un escritorio: el presupuesto de 1024x600 es requisito funcional y varios
    // defectos de esta pantalla solo se ven a ese ancho.
    viewport: { width: 1024, height: 600 },
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    ...devices['Desktop Chrome'],
  },
  projects: [{ name: 'tableta', use: { viewport: { width: 1024, height: 600 } } }],
});
