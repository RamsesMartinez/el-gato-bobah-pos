import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import { Provider } from '../../components/ui/provider';
import { PlataformasPage } from './PlataformasPage';
import * as api from '../../api/plataformas';
import * as hookMenu from '../../hooks/useMenu';
import { ApiError } from '../../api/client';
import { useSessionStore } from '../../stores/session';

vi.mock('../../api/plataformas', async () => {
  const real = await vi.importActual<typeof api>('../../api/plataformas');
  return { ...real, listarConexiones: vi.fn(), crearConexion: vi.fn(), parejasDeLaConexion: vi.fn(), borrarConexion: vi.fn(), diferencias: vi.fn(), tiendasDisponibles: vi.fn(), estadoDeLlave: vi.fn(), guardarLlave: vi.fn(), retirarLlaveAnterior: vi.fn(), getCredentialsState: vi.fn(), saveCredentials: vi.fn() };
});
vi.mock('../../hooks/useMenu');

const montar = () =>
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter>
        <Provider>
          <PlataformasPage />
        </Provider>
      </MemoryRouter>
    </QueryClientProvider>,
  );

beforeEach(() => {
  vi.mocked(api.listarConexiones).mockResolvedValue([]);
  vi.mocked(api.diferencias).mockRejectedValue(new Error('sin lectura'));
  vi.mocked(api.estadoDeLlave).mockResolvedValue({ configured: false, rotating: false });
  vi.mocked(api.guardarLlave).mockResolvedValue(undefined);
  // Uber se puede conectar en este despliegue; Didi no (no hay con qué hablarle).
  vi.mocked(api.getCredentialsState).mockImplementation(async (id: number) =>
    id === 6
      ? { available: true, environment: 'production', configured: false, needsRecapture: false }
      : { available: false, configured: false, needsRecapture: false },
  );
  vi.mocked(api.saveCredentials).mockResolvedValue(undefined);
  useSessionStore.setState({ user: { id: 1, name: 'Admin', role: 'admin' } as never });
  vi.mocked(api.tiendasDisponibles).mockResolvedValue([
    { externalStoreId: '3806dacf-965d-4b09-91b6-6d40ad5e15db', name: 'El Gato Bobah', city: 'Monterrey', posConnected: true, alreadyAdded: false },
    { externalStoreId: '9f1c2b7e-0000-4000-8000-aaaaaaaaaaaa', name: 'El Gato Bobah Sur', city: 'Monterrey', posConnected: false, alreadyAdded: true },
  ]);
  // Los ids de ESTA empresa. En el respaldo real, Uber Eats es el 6 para el negocio y el 2 para la
  // empresa de pruebas: el id NO es universal.
  vi.mocked(hookMenu.useMenu).mockReturnValue({
    data: {
      platforms: [
        { id: 5, name: 'Didi', markupPct: '35' },
        { id: 6, name: 'Uber Eats', markupPct: '35' },
        { id: 8, name: 'Propio', markupPct: '0' },
      ],
    },
  } as never);
});

describe('PlataformasPage', () => {
  // EL ID DE PLATAFORMA ES POR EMPRESA. Una lista fija en el front manda el id de otra empresa: la
  // FK compuesta lo rechaza —falla seguro— pero el formulario deja de servir sin decir por qué.
  it('ofrece las plataformas de la empresa, con sus ids reales', async () => {
    montar();
    await userEvent.click(await screen.findByRole('button', { name: /De qué plataforma/ }));
    expect(await screen.findByRole('button', { name: 'Uber Eats' })).toBeInTheDocument();
    // «Propio» es reparto del propio negocio: no tiene menú publicado que leer.
    expect(screen.queryByText('Propio')).not.toBeInTheDocument();
  });

  it('sin tiendas dadas de alta lo dice, en vez de una pantalla vacía', async () => {
    montar();
    expect(await screen.findByText(/Todavía no hay ninguna/)).toBeInTheDocument();
  });

  // Desconectar se lleva las lecturas Y el emparejamiento: hasta una sesión completa de trabajo
  // manual. La cuenta va en el diálogo, antes de confirmar.
  it('al desconectar dice cuántos platillos emparejados se pierden', async () => {
    vi.mocked(api.listarConexiones).mockResolvedValue([
      {
        id: 1, platformId: 6, platformName: 'Uber Eats', externalStoreId: 'abc',
        label: 'Sucursal Centro', active: true, credentialsConfigured: true, lastRead: null,
      },
    ]);
    vi.mocked(api.parejasDeLaConexion).mockResolvedValue(41);
    montar();
    await screen.findByText(/Sucursal Centro/);
    await userEvent.click(screen.getByRole('button', { name: /Desconectar Uber Eats/ }));
    expect(await screen.findByText(/Se pierden 41 platillos/)).toBeInTheDocument();
  });

  // EL UUID NO SE TECLEA. Es el único dato del alta que una persona no puede producir ni deducir, y
  // pedirlo escrito es lo que deja fuera a quien nunca ha usado el sistema. Si esta prueba se cae,
  // el alta volvió a ser un campo de texto.
  it('ofrece las tiendas por nombre y ciudad, no un campo para escribir el id', async () => {
    montar();
    await userEvent.click(await screen.findByRole('button', { name: /De qué plataforma/ }));
    await userEvent.click(await screen.findByRole('button', { name: 'Uber Eats' }));
    await userEvent.click(await screen.findByRole('button', { name: /Elige tu tienda/ }));
    expect(await screen.findByText('El Gato Bobah — Monterrey')).toBeInTheDocument();
    expect(api.tiendasDisponibles).toHaveBeenCalledWith(6);
  });

  // Marcada y NO escondida: quien la busca y no la encuentra cree que se equivocó de tienda.
  it('una tienda ya conectada se marca y no deja volver a conectarla', async () => {
    montar();
    await userEvent.click(await screen.findByRole('button', { name: /De qué plataforma/ }));
    await userEvent.click(await screen.findByRole('button', { name: 'Uber Eats' }));
    await userEvent.click(await screen.findByRole('button', { name: /Elige tu tienda/ }));
    await userEvent.click(await screen.findByText('El Gato Bobah Sur — Monterrey'));
    expect(await screen.findByText(/ya está conectada/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Conectar$/ })).toBeDisabled();
  });

  // Cambiar de app dos veces seguidas puede hacer que la respuesta de la PRIMERA llegue al final.
  // Sin descartarla, la pantalla queda mostrando las tiendas de una app distinta de la elegida —con
  // sus nombres reales, así que se ve bien— y el alta conecta la tienda a la plataforma equivocada.
  it('descarta las tiendas de una app que ya no está elegida', async () => {
    let responderUber: (t: api.TiendaDePlataforma[]) => void = () => {};
    vi.mocked(api.tiendasDisponibles).mockImplementation((id) =>
      id === 6
        ? new Promise((res) => {
            responderUber = res;
          })
        : Promise.resolve([
            { externalStoreId: 'didi-1', name: 'Bobah en Didi', city: 'Monterrey', posConnected: false, alreadyAdded: false },
          ]),
    );
    montar();
    await userEvent.click(await screen.findByRole('button', { name: /De qué plataforma/ }));
    await userEvent.click(await screen.findByRole('button', { name: 'Uber Eats' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Uber Eats' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Didi' }));
    await screen.findByRole('button', { name: /Elige tu tienda/ });
    responderUber([
      { externalStoreId: 'uber-1', name: 'Bobah en Uber', city: 'Monterrey', posConnected: false, alreadyAdded: false },
    ]);
    await userEvent.click(screen.getByRole('button', { name: /Elige tu tienda/ }));
    expect(await screen.findByText('Bobah en Didi — Monterrey')).toBeInTheDocument();
    expect(screen.queryByText('Bobah en Uber — Monterrey')).not.toBeInTheDocument();
  });

  // La lista puede no cargar —credenciales sin configurar, la plataforma caída— y el alta no puede
  // quedar bloqueada por eso: escribir el id a mano sigue siendo la salida.
  it('si no se pueden traer las tiendas, deja escribir el id a mano', async () => {
    vi.mocked(api.tiendasDisponibles).mockRejectedValue(new Error('sin credenciales'));
    montar();
    await userEvent.click(await screen.findByRole('button', { name: /De qué plataforma/ }));
    await userEvent.click(await screen.findByRole('button', { name: 'Uber Eats' }));
    expect(await screen.findByText(/No se pudo traer la lista/)).toBeInTheDocument();
  });

  // DOS `Page` ANIDADOS SON 48 px DE RELLENO QUE NO SEPARA NADA. La comparación de menú se pinta
  // dentro de esta pantalla, que ya está en un `Page`; cuando ella traía el suyo, la tableta perdía
  // el 8% de su alto en márgenes y el ancho se recortaba dos veces. `maxW` es la firma del `Page`.
  it('no envuelve la comparación de menú en un segundo contenedor de página', async () => {
    vi.mocked(api.listarConexiones).mockResolvedValue([
      {
        id: 1, platformId: 6, platformName: 'Uber Eats', externalStoreId: 'abc',
        label: 'Sucursal Centro', active: true, credentialsConfigured: true, lastRead: null,
      },
    ]);
    const { container } = montar();
    await screen.findByText(/Sucursal Centro/);
    const contenedores = [...container.querySelectorAll('div')].filter(
      (d) => getComputedStyle(d).maxWidth === '1150px',
    );
    expect(contenedores).toHaveLength(1);
  });

  // Conectar una tienda se hace UNA VEZ por sucursal; revisar diferencias, todos los días. El
  // formulario desplegado le cobraba ~250 px de una tableta de 600 a la tarea que sí se usa.
  it('con una tienda ya conectada el alta se guarda detrás de un botón', async () => {
    vi.mocked(api.listarConexiones).mockResolvedValue([
      {
        id: 1, platformId: 6, platformName: 'Uber Eats', externalStoreId: 'abc',
        label: 'Sucursal Centro', active: true, credentialsConfigured: true, lastRead: null,
      },
    ]);
    montar();
    await screen.findByText(/Sucursal Centro/);
    expect(screen.queryByText('Conectar una tienda')).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: /Conectar otra tienda/ }));
    expect(await screen.findByText('Conectar una tienda')).toBeInTheDocument();
  });

  // LA LLAVE DE FIRMA (spec 021). Sin ella ningún pedido de la app entra, y ninguna otra pantalla
  // lo dice: la tienda aparece «Conectada» porque para leer el menú sí lo está.
  describe('la llave para recibir pedidos', () => {
    const uber = {
      id: 1, platformId: 6, platformName: 'Uber Eats', externalStoreId: 'abc',
      label: 'Sucursal Centro', active: true, credentialsConfigured: true, lastRead: null,
    };
    const LLAVE = 'whsec_una-llave-de-verdad-larga';

    it('dice que falta, y se captura pegándola', async () => {
      vi.mocked(api.listarConexiones).mockResolvedValue([uber]);
      montar();
      expect(await screen.findByText(/Falta la llave para recibir pedidos/)).toBeInTheDocument();
      // Se pide por el id de la plataforma DE ESTA EMPRESA, no por el de la tienda.
      expect(api.estadoDeLlave).toHaveBeenCalledWith(6);

      await userEvent.click(screen.getByRole('button', { name: /Poner la llave/ }));
      await userEvent.type(screen.getByLabelText(/Llave de firma de Uber Eats/), LLAVE);
      vi.mocked(api.estadoDeLlave).mockResolvedValue({ configured: true, rotating: false });
      await userEvent.click(screen.getByRole('button', { name: /^Guardar la llave/ }));

      expect(api.guardarLlave).toHaveBeenCalledWith(6, LLAVE);
      expect(await screen.findByText(/Recibe pedidos/)).toBeInTheDocument();
    });

    // UNA VEZ GUARDADA NO SE VUELVE A VER. Ni en el campo ni en texto: lo que se puede releer desde
    // la pantalla se filtra por una foto de la tableta, y con esa llave se meten pedidos a la cocina.
    it('una vez guardada, la llave no queda en ningún lado de la pantalla', async () => {
      vi.mocked(api.listarConexiones).mockResolvedValue([uber]);
      montar();
      await userEvent.click(await screen.findByRole('button', { name: /Poner la llave/ }));
      const campo = screen.getByLabelText(/Llave de firma de Uber Eats/);
      // Mientras se pega tampoco se lee por encima del hombro.
      expect(campo).toHaveAttribute('type', 'password');
      await userEvent.type(campo, LLAVE);
      vi.mocked(api.estadoDeLlave).mockResolvedValue({ configured: true, rotating: false });
      await userEvent.click(screen.getByRole('button', { name: /^Guardar la llave/ }));
      await screen.findByText(/Recibe pedidos/);

      expect(screen.queryByDisplayValue(LLAVE)).not.toBeInTheDocument();
      expect(document.body.innerHTML).not.toContain(LLAVE);

      // Y al volver a abrir para cambiarla, el campo nace vacío: no trae la anterior.
      await userEvent.click(screen.getByRole('button', { name: /Cambiar la llave/ }));
      expect(screen.getByLabelText(/Llave de firma de Uber Eats/)).toHaveValue('');
    });

    it('si el servidor la rechaza lo dice, y no la da por guardada', async () => {
      vi.mocked(api.listarConexiones).mockResolvedValue([uber]);
      vi.mocked(api.guardarLlave).mockRejectedValue(new Error('400'));
      montar();
      await userEvent.click(await screen.findByRole('button', { name: /Poner la llave/ }));
      await userEvent.type(screen.getByLabelText(/Llave de firma de Uber Eats/), 'corta');
      await userEvent.click(screen.getByRole('button', { name: /^Guardar la llave/ }));
      expect(await screen.findByText(/No se pudo guardar/)).toBeInTheDocument();
      expect(screen.getByText(/Falta la llave para recibir pedidos/)).toBeInTheDocument();
    });

    // Cambiar la llave deja la anterior sirviendo; si nadie la retira sirve para siempre. La
    // pantalla lo dice y da con qué retirarla.
    it('a media rotación ofrece retirar la anterior', async () => {
      vi.mocked(api.listarConexiones).mockResolvedValue([uber]);
      vi.mocked(api.estadoDeLlave).mockResolvedValue({ configured: true, rotating: true, rotatedAt: '2026-09-20T12:00:00Z' });
      vi.mocked(api.retirarLlaveAnterior).mockResolvedValue(undefined);
      montar();
      await userEvent.click(await screen.findByRole('button', { name: /Retirar la anterior/ }));
      expect(api.retirarLlaveAnterior).toHaveBeenCalledWith(6);
    });

    // Dos sucursales de la misma app comparten llave: se pide UNA vez, no una por tienda.
    it('dos tiendas de la misma app piden una sola llave', async () => {
      vi.mocked(api.listarConexiones).mockResolvedValue([uber, { ...uber, id: 2, label: 'Sucursal Sur', externalStoreId: 'def' }]);
      montar();
      expect(await screen.findAllByText(/Falta la llave para recibir pedidos/)).toHaveLength(1);
    });
  });
});

// EL ACCESO A LA APP SE CAPTURA AQUÍ, Y ANTES QUE CUALQUIER TIENDA: sin él no se puede ni pedir la
// lista de tiendas. Lo que se prueba es lo que ve quien entra por primera vez y lo que pasa cuando
// la plataforma dice que no.
describe('Acceso a la app de la plataforma', () => {
  const capture = async (id = 'client-id-de-la-app', secreto = 'el-secreto-de-la-app-01') => {
    await userEvent.click(await screen.findByRole('button', { name: /Conectar con Uber Eats/ }));
    await userEvent.type(screen.getByLabelText(/Client ID de Uber Eats/), id);
    await userEvent.type(screen.getByLabelText(/Client Secret de Uber Eats/), secreto);
    await userEvent.click(screen.getByRole('button', { name: /Comprobar y guardar/ }));
  };

  it('sin credenciales lo dice antes que nada, y solo para las apps que este sistema sabe usar', async () => {
    montar();
    expect(await screen.findByText(/· Falta conectar$/)).toBeInTheDocument();
    // Didi no está disponible: pedirle credenciales que nadie va a usar solo confunde.
    expect(screen.queryByRole('button', { name: /Conectar con Didi/ })).not.toBeInTheDocument();
  });

  it('guarda, y el secreto no vuelve a la pantalla', async () => {
    let guardado = false;
    vi.mocked(api.saveCredentials).mockImplementation(async () => { guardado = true; });
    vi.mocked(api.getCredentialsState).mockImplementation(async (id: number) =>
      id !== 6
        ? { available: false, configured: false, needsRecapture: false }
        : guardado
          ? {
              available: true, environment: 'production', configured: true, needsRecapture: false,
              clientId: 'client-id-de-la-app', updatedBy: 'Admin', updatedAt: '2026-09-27T10:00:00Z',
            }
          : { available: true, environment: 'production', configured: false, needsRecapture: false },
    );
    montar();
    await capture();
    expect(api.saveCredentials).toHaveBeenCalledWith(6, { clientId: 'client-id-de-la-app', clientSecret: 'el-secreto-de-la-app-01' });
    expect(await screen.findByText(/Conectada/)).toBeInTheDocument();
    expect(screen.queryByLabelText(/Client Secret de Uber Eats/)).not.toBeInTheDocument();
    expect(screen.queryByDisplayValue('el-secreto-de-la-app-01')).not.toBeInTheDocument();
  });

  it('el secreto se escribe en un campo que no se lee por encima del hombro', async () => {
    montar();
    await userEvent.click(await screen.findByRole('button', { name: /Conectar con Uber Eats/ }));
    expect(screen.getByLabelText(/Client Secret de Uber Eats/)).toHaveAttribute('type', 'password');
  });

  // Cada rechazo pide corregir algo distinto; un «no se pudo» genérico manda a adivinar.
  it.each([
    ['PLATFORM_CREDENTIALS_REJECTED', 422, /no reconoce/],
    ['PLATFORM_CREDENTIALS_MISSING_SCOPES', 422, /permiso/],
    ['PLATFORM_UNAVAILABLE', 503, /no respondió/],
    // KMS caído al guardar: no es culpa de quien captura, y tiene que saber que puede reintentar.
    ['KEY_SERVICE_UNAVAILABLE', 503, /servicio de seguridad no respondió/],
    ['INTERNAL', 500, /No se pudo guardar/],
  ])('si la plataforma responde %s, lo dice en palabras de quien opera', async (code, status, frase) => {
    vi.mocked(api.saveCredentials).mockRejectedValue(new ApiError(status, code, 'x', 'req'));
    montar();
    await capture();
    expect(await screen.findByText(frase)).toBeInTheDocument();
    // Nada se guardó, y lo que se tecleó sigue ahí para corregirlo sin volver a copiar todo.
    expect(screen.getByLabelText(/Client ID de Uber Eats/)).toHaveValue('client-id-de-la-app');
  });

  it('con Uber sin responder deja claro que no se guardó nada', async () => {
    vi.mocked(api.saveCredentials).mockRejectedValue(new ApiError(503, 'PLATFORM_UNAVAILABLE', 'x', 'req'));
    montar();
    await capture();
    expect(await screen.findByText(/No se guardó nada/)).toBeInTheDocument();
  });

  // Al restaurar un respaldo de otro ambiente: hay credenciales, pero ilegibles aquí. No es «nunca
  // se capturaron» y no es un error: es «vuelve a capturarlas».
  it('distingue «vuelve a capturarlas» de «falta conectarla»', async () => {
    vi.mocked(api.getCredentialsState).mockImplementation(async (id: number) =>
      id === 6
        ? { available: true, environment: 'production', configured: true, needsRecapture: true, clientId: 'client-id-de-la-app' }
        : { available: false, configured: false, needsRecapture: false },
    );
    montar();
    expect(await screen.findByText(/vuelve a capturarlas/)).toBeInTheDocument();
    // El botón dice lo que el estado pide, no «cambiar».
    expect(screen.getByRole('button', { name: /Volver a capturar/ })).toBeInTheDocument();
    expect(screen.queryByText(/· Falta conectar$/)).not.toBeInTheDocument();
  });

  // El servidor solo deja escribir al administrador; el botón no se ofrece a quien lo verá fallar.
  it('un gerente ve el estado pero no puede cambiar el acceso', async () => {
    useSessionStore.setState({ user: { id: 2, name: 'Gerente', role: 'gerente' } as never });
    montar();
    expect(await screen.findByText(/· Falta conectar$/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Conectar con Uber Eats/ })).not.toBeInTheDocument();
    // Sin botón, tiene que decir a quién pedírselo: si no, parece un error del sistema.
    expect(screen.getByText(/Pídele a un administrador/)).toBeInTheDocument();
  });

  // Sin credenciales no hay lista de tiendas. Decir «no se pudo traer la lista, escribe el id» manda
  // a buscar un UUID que nadie sabe de dónde sacar; lo que falta es conectar la app.
  it('al dar de alta una tienda sin acceso a la app, manda a conectarla primero', async () => {
    vi.mocked(api.tiendasDisponibles).mockRejectedValue(new ApiError(412, 'PLATFORM_NOT_CONFIGURED', 'x', 'req'));
    montar();
    await userEvent.click(await screen.findByRole('button', { name: /De qué plataforma/ }));
    await userEvent.click(await screen.findByRole('button', { name: 'Uber Eats' }));
    expect(await screen.findByText(/Primero conecta con Uber Eats/)).toBeInTheDocument();
    // Y no se ofrece terminar un alta que no puede terminar: el nombre y el botón esperan.
    expect(screen.queryByPlaceholderText('Sucursal Centro')).not.toBeInTheDocument();
  });

  // Al escribir, el placeholder desaparece; con un mensaje que dice «cada uno en su campo», el
  // nombre del campo tiene que seguir a la vista.
  it('cada campo conserva su nombre a la vista', async () => {
    montar();
    await userEvent.click(await screen.findByRole('button', { name: /Conectar con Uber Eats/ }));
    expect(screen.getByText('Client ID', { selector: 'p, label, span' })).toBeInTheDocument();
    expect(screen.getByText('Client Secret', { selector: 'p, label, span' })).toBeInTheDocument();
  });

  it('una vez conectada, dice con cuál app sin que haga falta saber qué es «…k9Qz»', async () => {
    vi.mocked(api.getCredentialsState).mockImplementation(async (id: number) =>
      id === 6
        ? { available: true, environment: 'production', configured: true, needsRecapture: false, clientId: 'aB3dEfGhk9Qz' }
        : { available: false, configured: false, needsRecapture: false },
    );
    montar();
    expect(await screen.findByText(/Client ID termina en k9Qz/)).toBeInTheDocument();
  });

  // CON UNA TIENDA Y SIN CONEXIÓN, LO QUE FALTA VA ARRIBA. Debajo de una comparación que no puede
  // funcionar, el único botón que sirve quedaba al fondo de la pantalla.
  it('con una tienda y sin conexión, la conexión va antes que la tienda', async () => {
    vi.mocked(api.listarConexiones).mockResolvedValue([
      {
        id: 1, platformId: 6, platformName: 'Uber Eats', externalStoreId: 'abc',
        label: 'Sucursal Centro', active: true, credentialsConfigured: false, lastRead: null,
      },
    ]);
    montar();
    const conexion = await screen.findByText('Conexión con las plataformas');
    const tiendas = screen.getByText('Tus tiendas');
    expect(conexion.compareDocumentPosition(tiendas) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    // La llave de pedidos es su propio renglón, no un «Uber Eats» repetido.
    expect(await screen.findByText('Pedidos de Uber Eats')).toBeInTheDocument();
  });

  // La llave de pedidos también es solo del administrador (el servidor responde 403 al gerente).
  it('un gerente ve la llave de pedidos pero no puede ponerla ni retirar la anterior', async () => {
    useSessionStore.setState({ user: { id: 2, name: 'Gerente', role: 'gerente' } as never });
    vi.mocked(api.listarConexiones).mockResolvedValue([
      {
        id: 1, platformId: 6, platformName: 'Uber Eats', externalStoreId: 'abc',
        label: 'Sucursal Centro', active: true, credentialsConfigured: true, lastRead: null,
      },
    ]);
    vi.mocked(api.estadoDeLlave).mockResolvedValue({ configured: true, rotating: true });
    montar();
    expect(await screen.findByText('Pedidos de Uber Eats')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Cambiar la llave|Poner la llave/ })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Retirar la anterior/ })).not.toBeInTheDocument();
  });
});
