import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Provider } from '../../components/ui/provider';
import { EmparejarPage } from './EmparejarPage';
import * as api from '../../api/plataformas';

vi.mock('../../api/plataformas', async () => {
  const real = await vi.importActual<typeof api>('../../api/plataformas');
  return { ...real, emparejamiento: vi.fn(), guardarPareja: vi.fn(), borrarPareja: vi.fn() };
});

const datos = (): api.Emparejamiento => ({
  readAt: '2026-09-15T10:00:00Z',
  items: [
    // Dos ya confirmadas, en orden ALFABÉTICO y no cronológico: la lista llega ordenada por nombre
    // y «Aguas frescas» se confirmó ANTES que «Zarzamora». Es el caso que rompía el deshacer.
    {
      externalId: 'Aguas_frescas',
      kind: 'platillo' as const,
      name: 'Aguas frescas',
      priceCents: 4500,
      available: true,
      link: { localId: 2, localName: 'Agua', localKind: 'producto' as const, confirmed: true, confirmedAt: '2026-09-15T09:00:00Z' },
    },
    {
      externalId: 'Zarzamora',
      kind: 'platillo' as const,
      name: 'Zarzamora',
      priceCents: 5500,
      available: true,
      link: { localId: 3, localName: 'Zarza', localKind: 'producto' as const, confirmed: true, confirmedAt: '2026-09-15T11:00:00Z' },
    },
    {
      externalId: 'Chamoyada_de_Mango',
      kind: 'platillo',
      name: 'Chamoyada de Mango 🥭',
      priceCents: 9900,
      available: true,
      link: { localId: 7, localName: 'Chamoyada', localKind: 'producto', confirmed: false },
    },
    {
      externalId: 'Dedos_de_queso',
      kind: 'platillo',
      name: 'Dedos de queso',
      priceCents: 7500,
      available: true,
      link: null,
    },
  ],
  unlinkedLocal: [
    { id: 7, name: 'Chamoyada' },
    { id: 9, name: 'Papas Fritas - Corte Gajo 270g' },
  ],
});

const montar = () =>
  render(
    <Provider>
      <EmparejarPage conexionId={1} />
    </Provider>,
  );

beforeEach(() => {
  vi.mocked(api.emparejamiento).mockResolvedValue(datos());
  vi.mocked(api.guardarPareja).mockResolvedValue(undefined as never);
});

describe('EmparejarPage', () => {
  // FR-011: una propuesta es una propuesta y se ve así. Aceptada en silencio produce
  // comparaciones falsas que nadie puede auditar — medido, el nombre acierta 6 de 65.
  it('marca la sugerencia como propuesta, no como hecho', async () => {
    montar();
    expect(await screen.findByText('Propuesta')).toBeInTheDocument();
  });

  // El nombre completo del platillo tiene que estar: es lo único que lo distingue de sus hermanos.
  it('muestra el nombre completo del platillo de la plataforma', async () => {
    montar();
    expect(await screen.findByText('Chamoyada de Mango 🥭')).toBeInTheDocument();
  });

  // Una decisión a la vez: el segundo pendiente no se pinta hasta resolver el primero. Con dos
  // listas lado a lado a 1024 px los nombres se truncan justo por donde se distinguen.
  it('presenta un solo pendiente a la vez', async () => {
    montar();
    await screen.findByText('Chamoyada de Mango 🥭');
    expect(screen.queryByText('Dedos de queso')).not.toBeInTheDocument();
  });

  it('confirmar guarda la pareja y vuelve a cargar', async () => {
    montar();
    await userEvent.click(await screen.findByRole('button', { name: /es el mismo/i }));
    await waitFor(() =>
      expect(api.guardarPareja).toHaveBeenCalledWith(1, 'Chamoyada_de_Mango', {
        localId: 7,
        localKind: 'producto',
        kind: 'platillo',
      }),
    );
  });

  // El POS vive en tabletas: el desplegable nativo lo pinta el sistema con renglones de ~20 px y no
  // se acierta con el dedo. Va el Picker, que además trae buscador.
  it('no usa ningún select nativo', async () => {
    const { container } = montar();
    await screen.findByText('Chamoyada de Mango 🥭');
    expect(container.querySelector('select')).toBeNull();
  });
});

describe('EmparejarPage — lo que la revisión encontró', () => {
  // DESHACER TIENE QUE DESHACER EL ÚLTIMO, no el primero alfabético.
  //
  // La lista llega ordenada por nombre, así que un `.find()` devolvía «Aguas frescas» (confirmada a
  // las 09:00) en vez de «Zarzamora» (a las 11:00). El operador se equivocaba en la última, tocaba
  // deshacer, y el sistema borraba en silencio otra pareja: un error nuevo en vez de la corrección
  // que pidió, y encima invisible hasta semanas después.
  it('deshace la pareja más reciente, no la primera de la lista', async () => {
    vi.mocked(api.borrarPareja).mockResolvedValue(undefined as never);
    montar();
    const boton = await screen.findByRole('button', { name: /Deshacer «Zarzamora»/ });
    await userEvent.click(boton);
    await waitFor(() => expect(api.borrarPareja).toHaveBeenCalledWith(1, 'Zarzamora'));
  });

  // UN PLATILLO SIN PAREJA POSIBLE NO PUEDE ATORAR LA SESIÓN.
  //
  // Con 65 platillos y 109 productos sin pareja es seguro toparse con uno que no corresponde a
  // nada. Sin salida, el operador solo podía confirmar algo que sabía mal o abandonar las decisiones
  // que faltaban — y el spec pide terminarlas de un tirón.
  it('deja saltar un platillo y pasa al siguiente', async () => {
    montar();
    await screen.findByText('Chamoyada de Mango 🥭');
    await userEvent.click(screen.getByRole('button', { name: /Más tarde/ }));
    expect(await screen.findByText('Dedos de queso')).toBeInTheDocument();
  });
});
