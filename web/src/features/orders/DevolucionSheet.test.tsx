import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { Provider } from '../../components/ui/provider';
import { DevolucionSheet } from './DevolucionSheet';
import type { BoardOrder } from '../../types/pos';

const pedido = (over: Partial<BoardOrder> = {}): BoardOrder => ({
  id: 1, number: 7, folioName: 'Tigre', status: 'entregada', serviceType: 'mostrador',
  deliveryPlatformId: null, customerName: null, total: '500', outstanding: '0',
  currency: 'MXN', paid: true, refund: '0', enPreparacion: false, renglones: 1,
  openedAt: '2026-09-03T18:00:00Z', lines: [], ...over,
});

function pintar(nodo: React.ReactElement) {
  return render(<Provider>{nodo}</Provider>);
}

describe('la hoja de devolución', () => {
  // ARRANCA CON TODO LO QUE QUEDA, que es el caso de casi siempre. Obligar a teclear el monto en la
  // devolución completa sería un paso de más en el gesto más común.
  it('propone devolver todo lo que queda', async () => {
    pintar(<DevolucionSheet pedido={pedido()} enviando={false} onCerrar={() => {}} onConfirmar={() => {}} />);
    expect(await screen.findByLabelText('Cuánto se devuelve')).toHaveValue('500');
  });

  // Y descuenta lo ya devuelto: sin eso ofrecería devolver dos veces lo mismo y el servidor la
  // rebotaría con el cliente enfrente.
  it('descuenta lo que ya se había devuelto', async () => {
    pintar(<DevolucionSheet pedido={pedido({ refund: '440' })} enviando={false}
      onCerrar={() => {}} onConfirmar={() => {}} />);
    expect(await screen.findByLabelText('Cuánto se devuelve')).toHaveValue('60');
    expect(screen.getByText('$440')).toBeInTheDocument();
  });

  // UN PEDIDO SIN COBROS NO SE DEVUELVE, Y SE DICE POR QUÉ.
  //
  // El tablero pintaba "Reembolsar" junto a "Cobrar $220" en la misma tarjeta, y tocarlo anotaba
  // $220 de pérdida por un ingreso que nunca ocurrió.
  it('un pedido sin cobrar no deja devolver, y dice por qué', async () => {
    pintar(<DevolucionSheet pedido={pedido({ outstanding: '500', paid: false })} enviando={false}
      onCerrar={() => {}} onConfirmar={() => {}} />);
    expect(await screen.findByText(/no se ha cobrado/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Devolver/ })).toBeDisabled();
  });

  it('no deja devolver más de lo que entró', async () => {
    const u = userEvent.setup();
    pintar(<DevolucionSheet pedido={pedido()} enviando={false} onCerrar={() => {}} onConfirmar={() => {}} />);
    const campo = await screen.findByLabelText('Cuánto se devuelve');
    await u.clear(campo);
    await u.type(campo, '600');

    expect(await screen.findByText(/No puedes devolver más/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Devolver/ })).toBeDisabled();
  });

  it('devuelve el monto y el motivo que se eligieron', async () => {
    const u = userEvent.setup();
    const onConfirmar = vi.fn();
    pintar(<DevolucionSheet pedido={pedido()} enviando={false} onCerrar={() => {}} onConfirmar={onConfirmar} />);
    const campo = await screen.findByLabelText('Cuánto se devuelve');
    await u.clear(campo);
    await u.type(campo, '60');
    await u.click(screen.getByRole('button', { name: /^Devolver/ }));

    await waitFor(() => expect(onConfirmar).toHaveBeenCalledWith(60, 'Producto en mal estado'));
  });

  // CANCELAR UN PEDIDO SIN COBROS NO PIDE MONTO.
  //
  // Exigirle un monto sería pedirle al cajero que devuelva un dinero que nunca entró, y el botón se
  // quedaría apagado sin que él pueda hacer nada al respecto.
  it('cancelar un pedido sin cobros solo pide el motivo', async () => {
    const onConfirmar = vi.fn();
    const u = userEvent.setup();
    pintar(<DevolucionSheet pedido={pedido({ outstanding: '500', paid: false })} cancelando
      enviando={false} onCerrar={() => {}} onConfirmar={onConfirmar} />);

    expect(await screen.findByRole('button', { name: 'Cancelar pedido' })).toBeEnabled();
    expect(screen.queryByLabelText('Cuánto se devuelve')).toBeNull();

    await u.click(screen.getByRole('button', { name: 'Cancelar pedido' }));
    await waitFor(() => expect(onConfirmar).toHaveBeenCalledWith(0, 'Producto en mal estado'));
  });

  // Y cancelar uno YA COBRADO sí lo pide: es la devolución que el arqueo necesita para cuadrar.
  it('cancelar un pedido cobrado pide cuánto devolver', async () => {
    pintar(<DevolucionSheet pedido={pedido({ status: 'abierta' })} cancelando
      enviando={false} onCerrar={() => {}} onConfirmar={() => {}} />);
    expect(await screen.findByLabelText('Cuánto se devuelve')).toHaveValue('500');
    expect(screen.getByRole('button', { name: /Cancelar y devolver/ })).toBeEnabled();
  });

  // 44 px es el mínimo con el que un dedo acierta a la primera.
  it('el campo del monto mide al menos 44 px', async () => {
    pintar(<DevolucionSheet pedido={pedido()} enviando={false} onCerrar={() => {}} onConfirmar={() => {}} />);
    expect(await screen.findByLabelText('Cuánto se devuelve')).toHaveStyle({ minHeight: '44px' });
  });
});
