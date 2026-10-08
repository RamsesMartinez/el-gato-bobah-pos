import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../../components/ui/provider';
import { PlatformPicker } from './PlatformPicker';
import type { Menu } from '../../types/pos';

const menu: Menu = {
  categories: [],
  products: [],
  platforms: [
    { id: 5, name: 'Didi', markupPct: 35 },
    { id: 6, name: 'Uber Eats', markupPct: 35 },
    { id: 7, name: 'Rappi', markupPct: 35 },
  ],
  platformPrices: {},
  platformModPrices: {},
} as unknown as Menu;

// La cuenta vive en el servidor (spec 030): el selector solo pinta la lista de la cuenta abierta y
// avisa qué se eligió. Quién guarda y quién tira el folio al cambiar de lista es la cuenta.
let estado: { platformId: number | null; ref: string };
const onCambiar = vi.fn((id: number | null) => { estado = { platformId: id, ref: '' }; });
const onFolio = vi.fn((ref: string) => { estado = { ...estado, ref }; });
const onFolioListo = vi.fn();

function montar(bloqueado = false) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(['menu'], menu);
  const arbol = () => (
    <QueryClientProvider client={qc}>
      <Provider>
        <PlatformPicker platformId={estado.platformId} platformOrderRef={estado.ref} bloqueado={bloqueado}
          onCambiar={(id) => { onCambiar(id); r.rerender(arbol()); }}
          onFolio={(v) => { onFolio(v); r.rerender(arbol()); }} onFolioListo={onFolioListo} />
      </Provider>
    </QueryClientProvider>
  );
  const r = render(arbol());
  return r;
}

function campoDeFolio() {
  return screen.queryByLabelText(/folio/i);
}

describe('PlatformPicker · el folio de la plataforma', () => {
  beforeEach(() => {
    // Cada caso arranca en mostrador, como una cuenta recién abierta.
    estado = { platformId: null, ref: '' };
    vi.clearAllMocks();
  });

  // El campo NO existe en el árbol, no "existe oculto". A 1024×600 el presupuesto del mosaico es de
  // 25 px sobre el último renglón: un control montado y escondido sigue costando alto si algún día
  // alguien le quita el display:none, y el spec pide que no exista.
  it('no existe con la lista en Mostrador', () => {
    montar();
    expect(campoDeFolio()).toBeNull();
  });

  it('aparece al elegir una plataforma', () => {
    montar();
    fireEvent.click(screen.getByRole('button', { name: /Uber Eats/ }));
    expect(campoDeFolio()).not.toBeNull();
  });

  // El rótulo nombra la PLATAFORMA. En esta pantalla "folio" ya significa otra cosa —el número del
  // turno, el que se canta como "Tigre"—, así que un campo que dijera "Folio" a secas manda a
  // teclear el dato equivocado con el documento de pago en la mano.
  it('el rótulo dice de qué plataforma es el folio, no solo "folio"', () => {
    montar();
    fireEvent.click(screen.getByRole('button', { name: /Uber Eats/ }));
    expect(screen.getByLabelText(/Folio de Uber Eats/i)).toBeInTheDocument();
  });

  it('lo que se teclea va a la cuenta abierta, y se guarda al salir del campo', () => {
    montar();
    fireEvent.click(screen.getByRole('button', { name: /Rappi/ }));
    fireEvent.change(campoDeFolio()!, { target: { value: '1234567890' } });
    expect(onFolio).toHaveBeenLastCalledWith('1234567890');
    fireEvent.blur(campoDeFolio()!);
    expect(onFolioListo).toHaveBeenCalled();
  });

  // Un pedido ya enviado no cambia de lista: los precios ya se cobraron con ella.
  it('con la cuenta ya enviada la lista no se puede cambiar', () => {
    estado = { platformId: 6, ref: 'U1' };
    montar(true);
    expect(screen.getByRole('button', { name: /Mostrador/ })).toBeDisabled();
    expect(screen.getByRole('button', { name: /Rappi/ })).toBeDisabled();
  });

  // Cambiar de plataforma con un folio puesto deja basura silenciosa: un folio de Uber colgando de
  // Rappi. La cuenta lo tira, y por eso el servidor nunca lo ve.
  it('cambiar de plataforma borra el folio que había', () => {
    montar();
    fireEvent.click(screen.getByRole('button', { name: /Uber Eats/ }));
    fireEvent.change(campoDeFolio()!, { target: { value: 'UBER-1' } });
    fireEvent.click(screen.getByRole('button', { name: /Rappi/ }));
    expect(onCambiar).toHaveBeenLastCalledWith(7);
    expect(campoDeFolio()).toHaveValue('');
  });

  it('volver a Mostrador también lo borra', () => {
    montar();
    fireEvent.click(screen.getByRole('button', { name: /Didi/ }));
    fireEvent.change(campoDeFolio()!, { target: { value: 'DIDI-1' } });
    fireEvent.click(screen.getByRole('button', { name: /Mostrador/ }));
    expect(onCambiar).toHaveBeenLastCalledWith(null);
    expect(campoDeFolio()).toBeNull();
  });
});
