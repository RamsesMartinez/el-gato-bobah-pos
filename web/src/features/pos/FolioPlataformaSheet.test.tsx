import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

import { Provider } from '../../components/ui/provider';
import { FolioPlataformaSheet } from './FolioPlataformaSheet';

const guardar = vi.fn();
const sinFolio = vi.fn();
const cancelar = vi.fn();

function montar(valorInicial = '') {
  return render(
    <Provider>
      <FolioPlataformaSheet
        isOpen plataforma="Uber Eats" valorInicial={valorInicial}
        onGuardarYMandar={guardar} onMandarSinFolio={sinFolio} onCancelar={cancelar}
      />
    </Provider>,
  );
}

describe('la hoja que pide el folio al mandar', () => {
  beforeEach(() => vi.clearAllMocks());

  it('nombra la plataforma en la pregunta y en el campo', () => {
    montar();
    expect(screen.getByText(/¿Con qué folio llegó de Uber Eats\?/)).toBeInTheDocument();
    expect(screen.getByLabelText(/Folio de Uber Eats/i)).toBeInTheDocument();
  });

  // UNA sola salida explícita, y dice lo que va a pasar. Dos salidas serían dos formas de mandar
  // sin folio, y la falta dejaría de leerse como una decisión.
  it('ofrece exactamente una salida para mandar sin folio', () => {
    montar();
    expect(screen.getAllByRole('button', { name: /sin folio/i })).toHaveLength(1);
  });

  it('no deja guardar un folio vacío ni de puros espacios', () => {
    montar();
    const guardarBtn = screen.getByRole('button', { name: /Guardar y mandar/i });
    expect(guardarBtn).toBeDisabled();
    fireEvent.change(screen.getByLabelText(/Folio de Uber Eats/i), { target: { value: '   ' } });
    expect(guardarBtn).toBeDisabled();
  });

  it('guarda el folio recortado y sigue con el envío', () => {
    montar();
    fireEvent.change(screen.getByLabelText(/Folio de Uber Eats/i), { target: { value: '  UBER-77  ' } });
    fireEvent.click(screen.getByRole('button', { name: /Guardar y mandar/i }));
    expect(guardar).toHaveBeenCalledWith('UBER-77');
    expect(sinFolio).not.toHaveBeenCalled();
  });

  // La salida cuesta UN toque, que es lo que declara SC-003. Si algún día pide confirmación, este
  // test dice qué se rompió.
  it('la salida manda el pedido con un solo toque', () => {
    montar();
    fireEvent.click(screen.getByRole('button', { name: /sin folio/i }));
    expect(sinFolio).toHaveBeenCalledTimes(1);
    expect(guardar).not.toHaveBeenCalled();
  });

  // Los tres controles son tappables con el dedo. En jsdom no hay layout, así que se comprueba la
  // intención declarada (minH ≥ 44) y la medición real vive en el e2e a 1024×600.
  it('los controles declaran al menos 44 px de alto', () => {
    montar();
    for (const el of [
      screen.getByLabelText(/Folio de Uber Eats/i),
      screen.getByRole('button', { name: /Guardar y mandar/i }),
      screen.getByRole('button', { name: /sin folio/i }),
    ]) {
      const alto = parseInt(getComputedStyle(el).minHeight || '0', 10);
      expect(alto).toBeGreaterThanOrEqual(44);
    }
  });
});
