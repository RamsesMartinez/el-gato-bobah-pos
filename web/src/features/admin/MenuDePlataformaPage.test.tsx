import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../../components/ui/provider';
import { MenuDePlataformaPage } from './MenuDePlataformaPage';
import * as api from '../../api/plataformas';

vi.mock('../../api/plataformas', async () => {
  const real = await vi.importActual<typeof api>('../../api/plataformas');
  return { ...real, listarConexiones: vi.fn(), diferencias: vi.fn(), leerMenu: vi.fn() };
});

const conexion = (over: Partial<api.ConexionDePlataforma> = {}): api.ConexionDePlataforma => ({
  id: 1,
  platformId: 2,
  platformName: 'Uber Eats',
  externalStoreId: 'abc',
  label: 'Sucursal Centro',
  active: true,
  credentialsConfigured: true,
  lastRead: {
    id: 9,
    status: 'ok',
    startedAt: '2026-09-15T10:00:00Z',
    finishedAt: '2026-09-15T10:00:04Z',
    itemCount: 222,
    failureKind: null,
    stale: false,
  },
  ...over,
});

// La pantalla usa el formateador de hora del negocio, que consulta la zona por react-query.
const montar = () =>
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <Provider>
        <MenuDePlataformaPage />
      </Provider>
    </QueryClientProvider>,
  );

beforeEach(() => {
  vi.mocked(api.listarConexiones).mockResolvedValue([conexion()]);
  vi.mocked(api.diferencias).mockResolvedValue({
    readAt: '2026-09-15T10:00:00Z',
    stale: false,
    differences: [
      {
        kind: 'precio',
        externalId: 'Sodas_explosivas',
        platformName: 'Sodas explosivas',
        localId: 3,
        localName: 'Soda Explosiva',
        platformPrice: '110.00',
        catalogPrice: '34.75',
      },
    ],
    unpaired: 109,
  });
});

describe('MenuDePlataformaPage', () => {
  // Abre con lo ACCIONABLE. Con 109 sin pareja, mezclarlos ahogaría todos los días lo que sí hay
  // que corregir — y la mayoría nunca se van a emparejar a propósito.
  it('pide solo precio y disponibilidad al abrir', async () => {
    montar();
    await screen.findByText(/Sodas explosivas/);
    expect(api.diferencias).toHaveBeenCalledWith(1, ['precio', 'disponibilidad']);
  });

  it('el conteo de sin emparejar vive en la pestaña, no en un aviso propio', async () => {
    montar();
    expect(await screen.findByRole('button', { name: /Solo en un lado \(109\)/ })).toBeInTheDocument();
  });

  // Manda lo publicado: la acción normal es actualizar el POS, y el renglón se lee en ese orden.
  it('nombra primero el precio de la app y después el del sistema', async () => {
    montar();
    const t = await screen.findByText(/En la app cuesta/);
    expect(t.textContent).toMatch(/110\.00.*34\.75/);
  });

  // La constitución prohíbe nombrar internals en pantalla.
  it('traduce el fallo y no muestra el nombre del campo', async () => {
    vi.mocked(api.listarConexiones).mockResolvedValue([
      conexion({
        lastRead: {
          id: 9,
          status: 'fallida',
          startedAt: '2026-09-15T10:00:00Z',
          finishedAt: '2026-09-15T10:00:04Z',
          itemCount: null,
          failureKind: 'tiempo_agotado',
          stale: false,
        },
      }),
    ]);
    montar();
    expect(await screen.findByText(/tardó demasiado en responder/)).toBeInTheDocument();
    expect(screen.queryByText(/tiempo_agotado/)).not.toBeInTheDocument();
  });

  // Los tres estados significan cosas opuestas y una pantalla mal hecha los muestra igual.
  it('distingue «nunca se ha leído» de «sin diferencias»', async () => {
    vi.mocked(api.listarConexiones).mockResolvedValue([conexion({ lastRead: null })]);
    montar();
    expect(await screen.findByText(/Nunca se ha leído/)).toBeInTheDocument();
  });

  it('dice cuando la tienda no está conectada', async () => {
    vi.mocked(api.listarConexiones).mockResolvedValue([
      conexion({ credentialsConfigured: false, lastRead: null }),
    ]);
    montar();
    expect(await screen.findByText(/todavía no está conectada/)).toBeInTheDocument();
  });

  // `overflowY` SIN ALTO NO HACE SCROLL: la caja crece con el contenido y empuja hacia abajo todo
  // lo que sigue en la pantalla —la lista de tiendas y el formulario de alta— hasta sacarlo de una
  // tableta de 600 px. Con 174 productos contra 65 platillos publicados, la lista larga es lo
  // normal. Si esta prueba se cae, la lista volvió a crecer sin tope.
  it('la lista de diferencias hace scroll en su propia caja, no empuja la pantalla', async () => {
    montar();
    const caja = await screen.findByTestId('lista-de-diferencias');
    const estilo = getComputedStyle(caja);
    expect(estilo.overflowY).toBe('auto');
    expect(estilo.maxHeight).not.toBe('');
    expect(estilo.maxHeight).not.toBe('none');
  });
});
