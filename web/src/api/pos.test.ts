import { vi } from 'vitest';
import { posApi } from './pos';

// LA HOJA DEL POS PIDE LA LISTA YA RECORTADA.
//
// Sin el parámetro, `/orders/open` devuelve la unión entera —lo que sigue en cocina MÁS lo que debe
// dinero— y la hoja abre con todo mezclado: medido en el ambiente de pruebas, 30 renglones con 14 ya
// cobrados, sobre una pantalla donde caben cinco.
//
// Este test existe porque perder el parámetro es invisible: la pantalla sigue funcionando, solo
// vuelve a abrir gigante, y no hay nada en rojo que lo delate.
test('la barra pide solo los pedidos que faltan por cobrar', async () => {
  const fetchMock = vi.fn(async (_url: RequestInfo | URL) => new Response(JSON.stringify({ items: [], outstanding: '0' }), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  }));
  vi.stubGlobal('fetch', fetchMock);

  await posApi.openOrders();

  const url = String(fetchMock.mock.calls[0][0]);
  expect(url).toContain('/orders/open');
  expect(url, 'sin porCobrar el servidor devuelve también los ya saldados').toContain('porCobrar=true');

  vi.unstubAllGlobals();
});
