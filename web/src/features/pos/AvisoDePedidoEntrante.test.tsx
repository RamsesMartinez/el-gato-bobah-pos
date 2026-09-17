import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Provider } from '../../components/ui/provider';
import { AvisoDePedidoEntrante, CAPA_DEL_AVISO } from './AvisoDePedidoEntrante';
import type { PedidoDePlataforma } from '../../api/pedidosDePlataforma';

const enMinutos = (m: number) => new Date(Date.now() + m * 60000).toISOString();

const pedido = (over: Partial<PedidoDePlataforma> = {}): PedidoDePlataforma => ({
  id: 1,
  platformName: 'Uber Eats',
  displayId: 'K4T2',
  placedAt: new Date().toISOString(),
  decideBefore: enMinutos(11),
  serviceType: 'domicilio',
  customerName: 'Ana',
  total: '342.00',
  lines: [{ externalName: 'Crepa de Nutella', quantity: '2', unitPrice: '129', productId: 8, matched: true }],
  ...over,
});

const montar = (props: Partial<Parameters<typeof AvisoDePedidoEntrante>[0]> = {}) =>
  render(
    <Provider>
      <AvisoDePedidoEntrante
        pedidos={[pedido()]}
        onAceptar={() => {}}
        onVerTodos={() => {}}
        aceptando={false}
        {...props}
      />
    </Provider>,
  );

describe('AvisoDePedidoEntrante', () => {
  // EL AVISO SE VE AUNQUE HAYA UNA HOJA ABIERTA ENCIMA.
  //
  // Es el escenario que más importa y el que se pierde si se pinta como una parte más de la
  // pantalla: las hojas del POS se montan en un portal, así que un aviso dentro del árbol de la
  // página queda DEBAJO de la que se abra después — tapado justo mientras alguien captura una
  // venta, que es cuando llega el pedido.
  it('vive en su propio portal, por encima de las hojas del POS', () => {
    montar();
    const aviso = screen.getByTestId('aviso-de-pedido-entrante');
    // Fuera del contenedor que renderiza la página: colgado de body.
    expect(aviso.closest('[data-testid="aviso-de-pedido-entrante"]')).toBe(aviso);
    const capa = Number(getComputedStyle(aviso).zIndex);
    expect(capa).toBeGreaterThanOrEqual(CAPA_DEL_AVISO);
    // Chakra pinta sus hojas por debajo de 1500. Si alguien baja este número, el aviso vuelve a
    // quedar tapado y esta prueba se cae antes que la tableta.
    expect(capa).toBeGreaterThan(1500);
    expect(getComputedStyle(aviso).position).toBe('fixed');
  });

  // ACEPTAR CUESTA UN TOQUE, y es el criterio SC-002.
  it('aceptar el más urgente es un solo toque', async () => {
    const onAceptar = vi.fn();
    montar({ onAceptar });
    await userEvent.click(screen.getByRole('button', { name: /Aceptar/ }));
    expect(onAceptar).toHaveBeenCalledTimes(1);
  });

  // NO HAY UN «RECHAZAR» DEL MISMO TAMAÑO AL LADO. Con la cuenta corriendo el operador va rápido, y
  // dos botones iguales hacen que el toque caiga en el equivocado.
  it('no pone rechazar junto a aceptar', () => {
    montar();
    expect(screen.queryByRole('button', { name: /Rechazar/ })).not.toBeInTheDocument();
  });

  // CON VARIOS PENDIENTES SE VE EL MÁS URGENTE Y UN CONTADOR, no tres tarjetas apiladas que taparían
  // la pantalla entera.
  it('con tres pendientes muestra uno y cuenta los otros', async () => {
    const onVerTodos = vi.fn();
    montar({
      pedidos: [
        pedido({ id: 1, displayId: 'AAA1' }),
        pedido({ id: 2, displayId: 'BBB2' }),
        pedido({ id: 3, displayId: 'CCC3' }),
      ],
      onVerTodos,
    });
    expect(screen.getByText(/AAA1/)).toBeInTheDocument();
    expect(screen.queryByText(/BBB2/)).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: /\+2 más/ }));
    expect(onVerTodos).toHaveBeenCalled();
  });

  // MINUTOS Y COLOR, NUNCA SEGUNDOS: entre el segundo 47 y el 46 no hay ninguna decisión distinta.
  it('muestra minutos y jamás un segundero', () => {
    montar();
    expect(screen.getByText(/Quedan \d+ min/)).toBeInTheDocument();
    expect(screen.queryByText(/\d+:\d\d/)).not.toBeInTheDocument();
  });

  it('cerca del final deja de prometer tiempo', () => {
    montar({ pedidos: [pedido({ decideBefore: enMinutos(1) })] });
    expect(screen.getByText(/Se cancela en cualquier momento/)).toBeInTheDocument();
  });

  // UN PEDIDO PARA RECOGER SE DISTINGUE: nadie sale a repartirlo, y confundirlo manda a un
  // repartidor a una dirección que no existe.
  it('dice si el cliente pasa por él', () => {
    montar({ pedidos: [pedido({ serviceType: 'para_llevar' })] });
    expect(screen.getByText(/Pasan por él/)).toBeInTheDocument();
  });

  // `lines` puede llegar como null por un camino nuevo. `.length` sobre null tumba la pantalla
  // entera con la API respondiendo 200: ya pasó con la pantalla de pedidos de producción.
  it('no se cae si el pedido llega sin renglones', () => {
    montar({ pedidos: [pedido({ lines: undefined })] });
    expect(screen.getByTestId('aviso-de-pedido-entrante')).toBeInTheDocument();
  });
});
