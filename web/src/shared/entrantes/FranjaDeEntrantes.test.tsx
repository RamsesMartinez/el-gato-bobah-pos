import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Provider } from '../../components/ui/provider';
import type { PedidoEntrante } from '../../api/pedidosEntrantes';
import { FranjaDeEntrantes } from './FranjaDeEntrantes';
import { CAPA_FRANJA } from './capas';
import type { EstadoAlarma } from './alarma';

const ahora = Date.parse('2026-11-23T19:00:00Z');
const pedido = (id: number, extra: Partial<PedidoEntrante> = {}): PedidoEntrante => ({
  source: 'platform', id, sourceName: 'Uber Eats', displayId: `F${id}`, customerName: 'Daniela', customerPhone: '',
  address: '', serviceType: 'domicilio', total: '312.00', placedAt: '2026-11-23T18:58:00Z',
  decideBefore: '2026-11-23T19:08:30Z', lines: [], ...extra,
});

function montar(props: Partial<{ pedidos: PedidoEntrante[]; alarma: EstadoAlarma; bloqueada: boolean }> = {}) {
  const onVer = vi.fn();
  const onActivar = vi.fn();
  render(
    <Provider>
      <FranjaDeEntrantes pedidos={props.pedidos ?? [pedido(1)]} alarma={props.alarma ?? 'sonando'}
        tabletaBloqueada={props.bloqueada ?? false} ahora={ahora} onVer={onVer} onActivarSonido={onActivar} />
    </Provider>,
  );
  return { onVer, onActivar };
}

describe('FranjaDeEntrantes (M-Indicador)', () => {
  it('con un pedido dice cuál y lleva a verlo', async () => {
    const { onVer } = montar();
    expect(screen.getByText('1 pedido por aceptar')).toBeInTheDocument();
    expect(screen.getByText(/Uber Eats · Daniela · quedan 8 min/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Ver pedido' }));
    expect(onVer).toHaveBeenCalled();
  });

  it('con varios dice cuántos y nombra al más urgente', () => {
    montar({ pedidos: [pedido(1), pedido(2), pedido(3)] });
    expect(screen.getByText('3 pedidos por aceptar')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Ver pedidos' })).toBeInTheDocument();
  });

  // Un origen sin plazo (el pedido web) dice hace cuánto llegó: «quedan 0 min» mentiría.
  it('sin plazo dice hace cuánto llegó', () => {
    montar({ pedidos: [pedido(1, { decideBefore: null, sourceName: 'Web' })] });
    expect(screen.getByText(/Web · Daniela · hace 2 min/)).toBeInTheDocument();
  });

  it('sin pendientes no ocupa alto', () => {
    montar({ pedidos: [] });
    expect(screen.queryByTestId('franja-de-entrantes')).not.toBeInTheDocument();
  });

  // ENCIMA DE TODO: una hoja, el toaster y el bloqueo por PIN quedan debajo.
  it('va en la capa de la franja', () => {
    montar();
    const franja = screen.getByTestId('franja-de-entrantes');
    expect(Number(getComputedStyle(franja).zIndex)).toBe(CAPA_FRANJA);
    expect(getComputedStyle(franja).position).toBe('relative');
  });

  // SONIDO BLOQUEADO (US5): se ve y se activa con un toque. Si sigue bloqueado, el botón sigue.
  it('con el sonido bloqueado ofrece activarlo', async () => {
    const { onActivar } = montar({ alarma: 'bloqueada' });
    await userEvent.click(screen.getByRole('button', { name: /Toca para activar el sonido/ }));
    expect(onActivar).toHaveBeenCalled();
    expect(screen.getByRole('button', { name: /Toca para activar el sonido/ })).toBeInTheDocument();
  });

  it('sonando no ofrece activar nada', () => {
    montar({ alarma: 'sonando' });
    expect(screen.queryByText(/activar el sonido/)).not.toBeInTheDocument();
  });

  // TABLETA BLOQUEADA POR PIN: dice cuántos y nada del cliente, y no ofrece entrar sin desbloquear.
  it('con la tableta bloqueada solo dice cuántos', () => {
    montar({ bloqueada: true, alarma: 'bloqueada' });
    expect(screen.getByText('1 pedido por aceptar')).toBeInTheDocument();
    expect(screen.queryByText(/Daniela/)).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Ver pedido/ })).not.toBeInTheDocument();
    expect(screen.getByText(/Toca la pantalla para activar el sonido/)).toBeInTheDocument();
  });
});
