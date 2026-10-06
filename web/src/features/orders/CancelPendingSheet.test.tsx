import { describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { Provider } from '../../components/ui/provider';
import { ApiError } from '../../api/client';
import type { BoardLine, BoardOrder } from '../../types/pos';
import { CancelPendingSheet } from './CancelPendingSheet';

const linea = (id: number, qty: number, delivered = 0): BoardLine => ({
  id, name: `Producto ${id}`, qty: String(qty), delivered: String(delivered),
});

// La mesa del incidente: once productos, uno ya entregado.
const persa = (over: Partial<BoardOrder> = {}): BoardOrder => ({
  id: 9, number: 3, folioName: 'Persa', status: 'abierta', serviceType: 'mostrador',
  deliveryPlatformId: null, customerName: null, total: '857', currency: 'MXN' as const,
  paid: false, outstanding: '857', openedAt: new Date().toISOString(),
  enPreparacion: true, renglones: 11,
  lines: [linea(1, 1, 1), ...Array.from({ length: 10 }, (_, i) => linea(i + 2, 1))],
  ...over,
});

function pintar(onConfirm: (reason: string) => Promise<unknown>, onClose = vi.fn(), order = persa()) {
  render(<Provider><CancelPendingSheet order={order} onClose={onClose} onConfirm={onConfirm} /></Provider>);
  return onClose;
}

describe('quitar lo que falta', () => {
  it('dice cuántos productos se quitan y que lo entregado se queda', async () => {
    pintar(vi.fn());
    expect(await screen.findByRole('button', { name: 'Quitar los 10 que faltan' })).toBeInTheDocument();
    expect(screen.getByText('Persa ya entregó 1 producto')).toBeInTheDocument();
  });

  // Sin motivo puesto: con uno preseleccionado, quitar diez productos es un solo toque y el motivo
  // guardado es el que nadie eligió.
  it('los motivos van en filas de 44 px sin ninguno elegido, y quitar espera a que se elija', async () => {
    const u = userEvent.setup();
    pintar(vi.fn());

    const quitar = await screen.findByRole('button', { name: 'Quitar los 10 que faltan' });
    expect(quitar).toBeDisabled();
    const motivos = screen.getAllByRole('radio');
    expect(motivos).toHaveLength(4);
    for (const m of motivos) {
      expect(m).toHaveAttribute('aria-checked', 'false');
      expect(m).toHaveStyle({ minHeight: '44px' });
    }

    await u.click(screen.getByRole('radio', { name: 'Sin insumos' }));
    expect(quitar).toBeEnabled();
  });

  // La acción destructiva no va pegada a la que deja todo como estaba: un dedo que erra por unos
  // píxeles quita diez productos en vez de salir.
  it('«Dejarlo» y «Quitar» van separados, y «Dejarlo» no quita nada', async () => {
    const u = userEvent.setup();
    const onConfirm = vi.fn();
    const onClose = pintar(onConfirm);

    const dejarlo = await screen.findByRole('button', { name: 'Dejarlo' });
    const quitar = screen.getByRole('button', { name: 'Quitar los 10 que faltan' });
    expect(dejarlo).toHaveStyle({ minHeight: '44px' });
    expect(quitar).toHaveStyle({ minHeight: '44px' });
    expect(dejarlo.nextElementSibling).not.toBe(quitar);

    await u.click(dejarlo);
    expect(onClose).toHaveBeenCalled();
    expect(onConfirm).not.toHaveBeenCalled();
  });

  // Cerrar antes de que conteste el servidor le dice al operador que ya se quitó; si después el
  // servidor lo rechaza, el aviso sale en una pantalla que ya no está y el pedido sigue igual.
  it('con la petición en curso la hoja sigue abierta, y cierra cuando el servidor responde', async () => {
    const u = userEvent.setup();
    let responder!: (v: unknown) => void;
    const onConfirm = vi.fn(() => new Promise((r) => { responder = r; }));
    const onClose = pintar(onConfirm);

    await u.click(await screen.findByRole('radio', { name: 'Ya no lo quiere' }));
    await u.click(screen.getByRole('button', { name: 'Quitar los 10 que faltan' }));

    expect(onConfirm).toHaveBeenCalledWith('Ya no lo quiere');
    expect(onClose).not.toHaveBeenCalled();
    expect(document.querySelector('[role="dialog"]')).not.toBeNull();

    responder({ removed: 10, restocked: 3 });
    await waitFor(() => expect(onClose).toHaveBeenCalled());
  });

  it('si el servidor lo rechaza, se queda abierta con su texto', async () => {
    const u = userEvent.setup();
    const onConfirm = vi.fn(() => Promise.reject(new ApiError(409, 'CONFLICT',
      'Ya se cobró más de lo que quedaría. Primero hay que devolver un pago', 'r1')));
    const onClose = pintar(onConfirm);

    await u.click(await screen.findByRole('radio', { name: 'Se equivocó el pedido' }));
    await u.click(screen.getByRole('button', { name: 'Quitar los 10 que faltan' }));

    expect(await screen.findByText('Ya se cobró más de lo que quedaría. Primero hay que devolver un pago'))
      .toBeInTheDocument();
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByRole('button', { name: 'Quitar los 10 que faltan' })).toBeEnabled();
  });
});
