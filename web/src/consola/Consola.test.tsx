import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { Consola } from './Consola';
import { MENSAJE_SIN_RED } from './api';

// La consola pintada, sin navegador. Lo que se prueba es lo que una barrera del backend NO puede
// atrapar: que la pantalla muestre lo que el servidor manda y que no se quede en blanco cuando la
// respuesta viene vacía o cuando el acceso deja de servir a media sesión.

// El mapa de uso vive dentro de esta pantalla, así que los stubs tienen que contestarle a ÉL
// también: una respuesta con otra forma haría que el mapa no pinte y el test miraría otra cosa.
const MAPA_VACIO = { periodo: { desde: '2026-09-01', hasta: '2026-09-12' }, pantallas: [] };

function respuesta(cuerpo: unknown, status = 200) {
  return Promise.resolve({
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(cuerpo),
  } as Response);
}

afterEach(() => {
  vi.unstubAllGlobals();
});

async function entrarComoSoporte(fetchStub: ReturnType<typeof vi.fn>) {
  vi.stubGlobal('fetch', fetchStub);
  render(<Consola />);
  await userEvent.type(screen.getByLabelText('Usuario'), 'soporte');
  await userEvent.type(screen.getByLabelText('Contraseña'), 'una-contrasena');
  await userEvent.click(screen.getByRole('button', { name: 'Entrar' }));
}

describe('la consola de plataforma', () => {
  it('arranca pidiendo entrar, no mostrando datos', () => {
    vi.stubGlobal('fetch', vi.fn());
    render(<Consola />);
    expect(screen.getByRole('button', { name: 'Entrar' })).toBeInTheDocument();
    // Y nada de la lista: una consola que pinta la tabla antes de autenticar estaría pidiéndola
    // sin token, y el 401 se vería como "no hay clientes".
    expect(screen.queryByRole('table')).not.toBeInTheDocument();
  });

  it('tras entrar, lista las empresas y dice en qué versión está la instalación', async () => {
    const fetchStub = vi.fn((url: string) => {
      if (String(url).endsWith('/platform/auth/login')) {
        return respuesta({ accessToken: 'tok', operator: { id: 1, username: 'soporte', name: 'Soporte' } });
      }
      if (String(url).includes('/platform/usage')) return respuesta(MAPA_VACIO);
      return respuesta({
        items: [
          { id: 1, slug: 'gatobobah', name: 'El Gato Bobah', activa: true, createdAt: '2026-08-26T18:21:06Z' },
          { id: 4, slug: 'otra', name: 'Otra Empresa', activa: false, createdAt: '2026-09-09T14:32:12Z' },
        ],
        schema: { version: 68 },
      });
    });
    await entrarComoSoporte(fetchStub);

    // Por CELDA y no por texto suelto: el nombre de la empresa aparece dos veces en esta pantalla
    // —en la lista y como filtro del mapa de uso— y buscarlo suelto es ambiguo.
    expect(await screen.findByRole('cell', { name: 'El Gato Bobah' })).toBeInTheDocument();
    expect(screen.getByRole('cell', { name: 'Otra Empresa (inactiva)' })).toBeInTheDocument();
    expect(screen.getByText(/versión de instalación 68/)).toBeInTheDocument();
  });

  it('con cero empresas lo dice, en vez de una tabla vacía', async () => {
    const fetchStub = vi.fn((url: string) => {
      if (String(url).endsWith('/platform/auth/login')) {
        return respuesta({ accessToken: 'tok', operator: { id: 1, username: 'soporte', name: 'Soporte' } });
      }
      if (String(url).includes('/platform/usage')) return respuesta(MAPA_VACIO);
      return respuesta({ items: [], schema: { version: 68 } });
    });
    await entrarComoSoporte(fetchStub);

    expect(await screen.findByText(/Todavía no hay ninguna empresa/)).toBeInTheDocument();
    expect(screen.queryByRole('table')).not.toBeInTheDocument();
  });

  it('si el acceso deja de servir, regresa a pedir entrar y no se queda con la pantalla muerta', async () => {
    // El caso real: alguien desactiva al operador mientras la consola está abierta. El backend
    // responde 401 en el siguiente request, y quedarse mostrando un error sin salida obligaría a
    // recargar para entender qué pasó.
    const fetchStub = vi.fn((url: string) => {
      if (String(url).endsWith('/platform/auth/login')) {
        return respuesta({ accessToken: 'tok', operator: { id: 1, username: 'soporte', name: 'Soporte' } });
      }
      return respuesta({ error: { code: 'UNAUTHORIZED', message: 'no autenticado' } }, 401);
    });
    await entrarComoSoporte(fetchStub);

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Entrar' })).toBeInTheDocument();
    });
  });

  it('una caída de red se dice en español, no con el texto del navegador', async () => {
    // El caso más común de todos y el que peor se veía: cuando `fetch` RECHAZA —wifi caído, DNS,
    // CORS— lo que llega es un TypeError con el texto que escribe el motor ("Failed to fetch"), y
    // la pantalla lo reenviaba tal cual.
    const fetchStub = vi.fn((url: string) => {
      if (String(url).endsWith('/platform/auth/login')) {
        return respuesta({ accessToken: 'tok', operator: { id: 1, username: 'soporte', name: 'Soporte' } });
      }
      return Promise.reject(new TypeError('Failed to fetch'));
    });
    await entrarComoSoporte(fetchStub);

    expect(await screen.findByText(MENSAJE_SIN_RED)).toBeInTheDocument();
    expect(screen.queryByText(/Failed to fetch/)).not.toBeInTheDocument();
  });

  it('el error se anuncia, no solo aparece', async () => {
    // Sin role="alert" el mensaje entra al DOM en silencio: quien usa lector de pantalla se queda
    // esperando a que pase algo después de tocar "Entrar".
    const fetchStub = vi.fn(() => Promise.reject(new TypeError('Failed to fetch')));
    await entrarComoSoporte(fetchStub);

    const alerta = await screen.findByRole('alert');
    expect(alerta).toHaveTextContent(MENSAJE_SIN_RED);
  });

  it('un fallo del servidor se dice en la pantalla, sin expulsar a nadie', async () => {
    const fetchStub = vi.fn((url: string) => {
      if (String(url).endsWith('/platform/auth/login')) {
        return respuesta({ accessToken: 'tok', operator: { id: 1, username: 'soporte', name: 'Soporte' } });
      }
      return respuesta({ error: { code: 'INTERNAL', message: 'algo se rompió' } }, 500);
    });
    await entrarComoSoporte(fetchStub);

    expect(await screen.findByText('algo se rompió')).toBeInTheDocument();
    // Sigue dentro: un 500 no es una sesión inválida, y sacarlo perdería el contexto de lo que
    // estaba mirando.
    expect(screen.queryByRole('button', { name: 'Entrar' })).not.toBeInTheDocument();
  });
});
