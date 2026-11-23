import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { Provider } from '../../components/ui/provider';
import type { PedidoEntrante } from '../../api/pedidosEntrantes';
import type { Alarma, EstadoAlarma } from './alarma';
import { AlertaDeEntrantes } from './AlertaDeEntrantes';
import { useColumnaPorAceptar, usePestanaDePedidos } from './estado';
import * as hook from './useEntrantes';

vi.mock('./useEntrantes', () => ({ useEntrantes: vi.fn(), CLAVE_ENTRANTES: ['pedidos-entrantes'] }));

const pedido: PedidoEntrante = {
  source: 'platform', id: 1, sourceName: 'Uber Eats', displayId: 'F1', customerName: 'Daniela', customerPhone: '',
  address: '', serviceType: 'domicilio', total: '312.00', placedAt: null, decideBefore: null, lines: [],
};

function alarmaFalsa(estado: EstadoAlarma = 'sonando') {
  const a = {
    sonar: vi.fn<(debe: boolean) => void>(),
    desbloquear: vi.fn<() => void>(),
    estado: () => estado,
    suscribir: () => () => {},
  };
  return a satisfies Alarma;
}

function montar(ruta: string, pedidos: PedidoEntrante[], alarma = alarmaFalsa()) {
  vi.mocked(hook.useEntrantes).mockReturnValue({ pedidos, refrescar: vi.fn() });
  const r = render(
    <Provider>
      <MemoryRouter initialEntries={[ruta]}><AlertaDeEntrantes alarma={alarma} /></MemoryRouter>
    </Provider>,
  );
  return { alarma, ...r };
}

describe('AlertaDeEntrantes', () => {
  beforeEach(() => useColumnaPorAceptar.setState({ visible: false }));

  // EN CUALQUIER PANTALLA: suena y se ve.
  it('en otra pantalla, con un pendiente, suena y pinta la franja', () => {
    const { alarma } = montar('/almacen', [pedido]);
    expect(alarma.sonar).toHaveBeenLastCalledWith(true);
    expect(screen.getByTestId('franja-de-entrantes')).toBeInTheDocument();
  });

  it('sin pendientes se calla y no pinta nada', () => {
    const { alarma } = montar('/almacen', []);
    expect(alarma.sonar).toHaveBeenLastCalledWith(false);
    expect(screen.queryByTestId('franja-de-entrantes')).not.toBeInTheDocument();
  });

  // En Pedidos la columna ya lo muestra: la franja se quita, pero la alarma SIGUE sonando.
  it('en Pedidos con la columna a la vista, sigue sonando y no duplica el aviso', () => {
    useColumnaPorAceptar.setState({ visible: true });
    const { alarma } = montar('/pedidos', [pedido]);
    expect(alarma.sonar).toHaveBeenLastCalledWith(true);
    expect(screen.queryByTestId('franja-de-entrantes')).not.toBeInTheDocument();
  });

  // En la pestaña «Cerrados» la columna no está: sin franja, la alarma sonaría sin decir por qué.
  it('en Pedidos sin la columna a la vista, pinta la franja', () => {
    montar('/pedidos', [pedido]);
    expect(screen.getByTestId('franja-de-entrantes')).toBeInTheDocument();
  });

  // El primer toque en cualquier parte desbloquea el sonido (FR-012), y volver a primer plano lo
  // intenta de nuevo (una tableta que se suspendió puede volver con el audio suspendido).
  // Con el dedo, el navegador cuenta como gesto el `pointerup`, el `touchend` y el `click`, no el
  // `pointerdown`: escuchar solo éste dejaba la tableta muda tras el primer toque (en iOS, siempre).
  it('el toque que cuenta como gesto desbloquea el sonido, y también volver a primer plano', () => {
    const { alarma } = montar('/almacen', [pedido], alarmaFalsa('bloqueada'));
    for (const toque of [() => fireEvent.pointerUp(document.body), () => fireEvent.touchEnd(document.body),
      () => fireEvent.click(document.body), () => fireEvent.keyDown(document.body, { key: 'a' })]) {
      alarma.desbloquear.mockClear();
      toque();
      expect(alarma.desbloquear).toHaveBeenCalled();
    }
    alarma.desbloquear.mockClear();
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' });
    fireEvent(document, new Event('visibilitychange'));
    expect(alarma.desbloquear).toHaveBeenCalledTimes(1);
  });

  // En Pedidos con «Cerrados» abierto, «Ver pedido» lleva a la misma ruta: tiene que volver a
  // «En curso», donde está la columna, o el botón no hace nada.
  it('«Ver pedido» regresa a la pestaña En curso', () => {
    usePestanaDePedidos.setState({ pestana: 'cerrados' });
    montar('/pedidos', [pedido]);
    fireEvent.click(screen.getByRole('button', { name: 'Ver pedido' }));
    expect(usePestanaDePedidos.getState().pestana).toBe('curso');
  });

  // Al salir de la sesión se desmonta el shell: la alarma no se queda sonando en el login.
  it('al desmontar se calla', () => {
    const { alarma, unmount } = montar('/almacen', [pedido]);
    unmount();
    expect(alarma.sonar).toHaveBeenLastCalledWith(false);
  });
});
