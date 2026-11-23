import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { Provider } from '../../components/ui/provider';
import type { PedidoEntrante } from '../../api/pedidosEntrantes';
import type { Alarma, EstadoAlarma } from './alarma';
import { AlertaDeEntrantes } from './AlertaDeEntrantes';
import { useColumnaPorAceptar } from './estado';
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
  it('un toque en cualquier parte o volver a primer plano intenta desbloquear el sonido', () => {
    const { alarma } = montar('/almacen', [pedido], alarmaFalsa('bloqueada'));
    fireEvent.pointerDown(document.body);
    expect(alarma.desbloquear).toHaveBeenCalledTimes(1);
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' });
    fireEvent(document, new Event('visibilitychange'));
    expect(alarma.desbloquear).toHaveBeenCalledTimes(2);
  });

  // Al salir de la sesión se desmonta el shell: la alarma no se queda sonando en el login.
  it('al desmontar se calla', () => {
    const { alarma, unmount } = montar('/almacen', [pedido]);
    unmount();
    expect(alarma.sonar).toHaveBeenLastCalledWith(false);
  });
});
