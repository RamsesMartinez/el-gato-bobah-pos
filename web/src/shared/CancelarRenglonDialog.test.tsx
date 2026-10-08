import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { Provider } from '../components/ui/provider';
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
    pintar(<CancelarRenglonDialog nombre="Alitas" pendientes={1} yaSalioACocina enviando={false}
      onCerrar={() => {}} onConfirmar={() => {}} />);
    expect(await screen.findByText(/no vuelve al almacén/i)).toBeInTheDocument();
  });

  it('avisa que sí vuelve si todavía no se prepara', async () => {
    pintar(<CancelarRenglonDialog nombre="Alitas" pendientes={1} yaSalioACocina={false} enviando={false}
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
    pintar(<CancelarRenglonDialog nombre="Alitas" pendientes={1} yaSalioACocina={false} enviando={false}
      onCerrar={() => {}} onConfirmar={onConfirmar} />);

    expect(onConfirmar).not.toHaveBeenCalled();
    await u.click(await screen.findByRole('radio', { name: 'Sin insumos' }));
    await u.click(screen.getByRole('button', { name: 'Quitar del pedido' }));
    await waitFor(() => expect(onConfirmar).toHaveBeenCalledWith('Sin insumos', 1));
  });

  // Con un motivo ya puesto, quitar es un solo toque y el motivo que queda es el que nadie eligió:
  // el reporte de cancelaciones se llena de «Ya no lo quiere» y deja de decir por qué se quita.
  it('no trae motivo puesto: «Quitar» espera a que se elija uno', async () => {
    const u = userEvent.setup();
    pintar(<CancelarRenglonDialog nombre="Alitas" pendientes={1} yaSalioACocina={false} enviando={false}
      onCerrar={() => {}} onConfirmar={() => {}} />);

    const quitar = await screen.findByRole('button', { name: 'Quitar del pedido' });
    expect(quitar).toBeDisabled();
    for (const r of screen.getAllByRole('radio')) expect(r).toHaveAttribute('aria-checked', 'false');

    await u.click(screen.getByRole('radio', { name: 'Se capturó de más' }));
    expect(quitar).toBeEnabled();
  });

  // Una sola lista de motivos para quitar, la misma que la hoja de quitar lo que falta: dos listas
  // parecidas reparten la misma causa en dos nombres y el reporte las cuenta por separado.
  it('ofrece la lista única de motivos, en filas tocables y sin texto libre', async () => {
    pintar(<CancelarRenglonDialog nombre="Alitas" pendientes={1} yaSalioACocina={false} enviando={false}
      onCerrar={() => {}} onConfirmar={() => {}} />);

    const motivos = await screen.findAllByRole('radio');
    expect(motivos.map((m) => m.textContent)).toEqual(
      ['Ya no lo quiere', 'Se capturó de más', 'Sin insumos', 'Se equivocó el pedido']);
    for (const m of motivos) expect(m).toHaveStyle({ minHeight: '44px' });
    expect(screen.queryByRole('textbox')).toBeNull();
  });

  it('se puede salir sin quitar nada', async () => {
    const u = userEvent.setup();
    const onCerrar = vi.fn();
    const onConfirmar = vi.fn();
    pintar(<CancelarRenglonDialog nombre="Alitas" pendientes={1} yaSalioACocina={false} enviando={false}
      onCerrar={onCerrar} onConfirmar={onConfirmar} />);

    await u.click(await screen.findByRole('button', { name: 'Dejarlo' }));
    expect(onCerrar).toHaveBeenCalled();
    expect(onConfirmar).not.toHaveBeenCalled();
  });

  // 44 px es el mínimo con el que un dedo acierta a la primera.
  it('los dos botones miden al menos 44 px', async () => {
    pintar(<CancelarRenglonDialog nombre="Alitas" pendientes={1} yaSalioACocina={false} enviando={false}
      onCerrar={() => {}} onConfirmar={() => {}} />);
    for (const nombre of ['Dejarlo', 'Quitar del pedido']) {
      expect(await screen.findByRole('button', { name: nombre })).toHaveStyle({ minHeight: '44px' });
    }
  });
});

// QUITAR UNA DE DOS (spec 027, US7).
//
// Sin contador, el bote quitaba todas las piezas que faltan: con «2 Taro» y uno que ya no se quiere,
// la única salida era quitar los dos y volver a capturar uno, que manda otra comanda a cocina.
describe('quitar solo algunas piezas', () => {
  const contador = () => screen.findByText(/^\d+ de \d+$/);

  it('con dos o más pendientes arranca en 1 y manda esa cantidad', async () => {
    const u = userEvent.setup();
    const onConfirmar = vi.fn();
    pintar(<CancelarRenglonDialog nombre="Taro" pendientes={3} yaSalioACocina={false} enviando={false}
      onCerrar={() => {}} onConfirmar={onConfirmar} />);

    expect((await contador()).textContent).toBe('1 de 3');
    await u.click(screen.getByRole('radio', { name: 'Ya no lo quiere' }));
    await u.click(screen.getByRole('button', { name: 'Quitar del pedido' }));
    await waitFor(() => expect(onConfirmar).toHaveBeenCalledWith('Ya no lo quiere', 1));
  });

  // Los topes son el borde: bajar de 1 no quita nada y pasar de lo pendiente el servidor lo rechaza.
  it('no baja de 1 ni pasa de lo pendiente', async () => {
    const u = userEvent.setup();
    const onConfirmar = vi.fn();
    pintar(<CancelarRenglonDialog nombre="Taro" pendientes={2} yaSalioACocina={false} enviando={false}
      onCerrar={() => {}} onConfirmar={onConfirmar} />);

    const menos = await screen.findByRole('button', { name: 'Una menos' });
    const mas = screen.getByRole('button', { name: 'Una más' });
    expect(menos).toBeDisabled();
    await u.click(mas);
    expect((await contador()).textContent).toBe('2 de 2');
    expect(mas).toBeDisabled();
    expect(menos).toBeEnabled();

    await u.click(screen.getByRole('radio', { name: 'Sin insumos' }));
    await u.click(screen.getByRole('button', { name: 'Quitar del pedido' }));
    await waitFor(() => expect(onConfirmar).toHaveBeenCalledWith('Sin insumos', 2));
  });

  it('los botones − y + miden al menos 44 px', async () => {
    pintar(<CancelarRenglonDialog nombre="Taro" pendientes={2} yaSalioACocina={false} enviando={false}
      onCerrar={() => {}} onConfirmar={() => {}} />);
    for (const nombre of ['Una menos', 'Una más']) {
      const b = await screen.findByRole('button', { name: nombre });
      expect(b, nombre).toHaveStyle({ minHeight: '44px', minWidth: '44px' });
    }
  });

  // Con una sola pieza el contador es un control que no decide nada.
  it('con una sola pieza pendiente no muestra contador', async () => {
    pintar(<CancelarRenglonDialog nombre="Taro" pendientes={1} yaSalioACocina={false} enviando={false}
      onCerrar={() => {}} onConfirmar={() => {}} />);
    await screen.findByRole('button', { name: 'Quitar del pedido' });
    expect(screen.queryByRole('button', { name: 'Una más' })).toBeNull();
    expect(screen.queryByText(/^\d+ de \d+$/)).toBeNull();
  });
});

// Quitar el renglón entero no puede costar un toque por pieza: «Todas» lo pone de un toque.
test('«Todas» pone todas las piezas pendientes', async () => {
  const u = userEvent.setup();
  pintar(<CancelarRenglonDialog nombre="Alitas" pendientes={5} yaSalioACocina={false} enviando={false}
    onCerrar={() => {}} onConfirmar={() => {}} />);
  await u.click(screen.getByRole('button', { name: 'Todas' }));
  expect(screen.getByText('5 de 5')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Todas' })).toBeDisabled();
});
