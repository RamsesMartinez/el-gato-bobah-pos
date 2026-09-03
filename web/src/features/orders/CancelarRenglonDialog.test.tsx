import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { Provider } from '../../components/ui/provider';
import { CancelarRenglonDialog } from './CancelarRenglonDialog';

function pintar(nodo: React.ReactElement) {
  return render(<Provider>{nodo}</Provider>);
}

describe('quitar un renglón del pedido', () => {
  // EL AVISO ES LA MITAD DEL VALOR DE ESTA PANTALLA.
  //
  // Cancelar algo que YA salió a cocina baja el total del pedido pero NO devuelve el ingrediente,
  // porque se gastó. Sin decirlo, el operador cree que deshizo la venta entera, el almacén cuadra
  // mal y nadie sabe por qué.
  it('avisa que el ingrediente NO vuelve si ya salió a cocina', async () => {
    pintar(<CancelarRenglonDialog nombre="Alitas" yaSalioACocina enviando={false}
      onCerrar={() => {}} onConfirmar={() => {}} />);
    expect(await screen.findByText(/no vuelve al almacén/i)).toBeInTheDocument();
  });

  it('avisa que sí vuelve si todavía no se prepara', async () => {
    pintar(<CancelarRenglonDialog nombre="Alitas" yaSalioACocina={false} enviando={false}
      onCerrar={() => {}} onConfirmar={() => {}} />);
    const aviso = await screen.findByText(/vuelve al almacén/i);
    expect(aviso).toBeInTheDocument();
    expect(aviso.textContent).not.toMatch(/no vuelve/i);
  });

  // NO BORRA AL TOCAR. Es la acción destructiva de una fila apretada, al lado de "Entregar": cuando
  // no hay distancia que dé seguridad de verdad, la barrera es el paso extra.
  it('pide confirmar antes de quitar', async () => {
    const u = userEvent.setup();
    const onConfirmar = vi.fn();
    pintar(<CancelarRenglonDialog nombre="Alitas" yaSalioACocina={false} enviando={false}
      onCerrar={() => {}} onConfirmar={onConfirmar} />);

    expect(onConfirmar).not.toHaveBeenCalled();
    await u.click(await screen.findByRole('button', { name: 'Quitar del pedido' }));
    await waitFor(() => expect(onConfirmar).toHaveBeenCalledWith('Ya no lo quiere'));
  });

  it('se puede salir sin quitar nada', async () => {
    const u = userEvent.setup();
    const onCerrar = vi.fn();
    const onConfirmar = vi.fn();
    pintar(<CancelarRenglonDialog nombre="Alitas" yaSalioACocina={false} enviando={false}
      onCerrar={onCerrar} onConfirmar={onConfirmar} />);

    await u.click(await screen.findByRole('button', { name: 'Dejarlo' }));
    expect(onCerrar).toHaveBeenCalled();
    expect(onConfirmar).not.toHaveBeenCalled();
  });

  // 44 px es el mínimo con el que un dedo acierta a la primera.
  it('los dos botones miden al menos 44 px', async () => {
    pintar(<CancelarRenglonDialog nombre="Alitas" yaSalioACocina={false} enviando={false}
      onCerrar={() => {}} onConfirmar={() => {}} />);
    for (const nombre of ['Dejarlo', 'Quitar del pedido']) {
      expect(await screen.findByRole('button', { name: nombre })).toHaveStyle({ minHeight: '44px' });
    }
  });
});
