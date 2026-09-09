import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../../components/ui/provider';
import { PlatformPicker } from './PlatformPicker';
import { useTicketStore } from '../../stores/ticket';
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

function montar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(['menu'], menu);
  return render(
    <QueryClientProvider client={qc}>
      <Provider><PlatformPicker /></Provider>
    </QueryClientProvider>,
  );
}

function campoDeFolio() {
  return screen.queryByLabelText(/folio/i);
}

describe('PlatformPicker · el folio de la plataforma', () => {
  beforeEach(() => {
    // Cada caso arranca en mostrador, como una cuenta recién abierta.
    useTicketStore.getState().setPlatform(null);
    useTicketStore.getState().setPlatformOrderRef('');
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

  it('lo que se teclea queda en la cuenta activa', () => {
    montar();
    fireEvent.click(screen.getByRole('button', { name: /Rappi/ }));
    fireEvent.change(campoDeFolio()!, { target: { value: '1234567890' } });
    expect(useTicketStore.getState().tabs[0].platformOrderRef).toBe('1234567890');
  });

  // Cambiar de plataforma con un folio puesto deja basura silenciosa: un folio de Uber colgando de
  // Rappi. La cuenta lo tira, y por eso el servidor nunca lo ve.
  it('cambiar de plataforma borra el folio que había', () => {
    montar();
    fireEvent.click(screen.getByRole('button', { name: /Uber Eats/ }));
    fireEvent.change(campoDeFolio()!, { target: { value: 'UBER-1' } });
    fireEvent.click(screen.getByRole('button', { name: /Rappi/ }));
    expect(useTicketStore.getState().tabs[0].platformOrderRef).toBe('');
  });

  it('volver a Mostrador también lo borra', () => {
    montar();
    fireEvent.click(screen.getByRole('button', { name: /Didi/ }));
    fireEvent.change(campoDeFolio()!, { target: { value: 'DIDI-1' } });
    fireEvent.click(screen.getByRole('button', { name: /Mostrador/ }));
    expect(useTicketStore.getState().tabs[0].platformOrderRef).toBe('');
    expect(campoDeFolio()).toBeNull();
  });
});
