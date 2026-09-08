import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../../components/ui/provider';
import { LiquidacionSheet } from './LiquidacionSheet';

const api = vi.hoisted(() => ({
  get: vi.fn(),
  save: vi.fn(() => Promise.resolve({})),
  summary: vi.fn(),
}));
vi.mock('../../api/settlements', async (orig) => ({
  ...(await orig<object>()), settlementsApi: api,
}));

function montar() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <Provider>
        <LiquidacionSheet orderId={187} plataforma="Uber Eats" isOpen onClose={() => {}} />
      </Provider>
    </QueryClientProvider>,
  );
}

// Los nueve campos del documento de pago, más el derivado.
const CAMPOS = [
  'Venta que reporta', 'Comisión', 'Tasa de comisión (%)', 'Retenciones',
  'Descuento total', 'Lo puso la plataforma', 'Depositado (neto)',
  'Referencia del depósito', 'Documento',
];

describe('LiquidacionSheet', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Sin liquidación capturada: el 404 es la respuesta legítima, no un fallo.
    api.get.mockRejectedValue(new Error('no encontrado'));
  });

  it('pide los nueve campos que trae el documento de pago', async () => {
    montar();
    for (const c of CAMPOS) {
      expect(await screen.findByLabelText(c)).toBeInTheDocument();
    }
  });

  it('todos los controles miden al menos 44 px', async () => {
    montar();
    for (const c of CAMPOS) {
      const el = await screen.findByLabelText(c);
      expect(parseInt(getComputedStyle(el).minHeight || '0', 10)).toBeGreaterThanOrEqual(44);
    }
  });

  // Lo que puso el restaurante es la cifra por la que existe esta pantalla: es lo que costó la
  // promoción. Se DERIVA al teclear, no se captura, porque guardar los tres números serían dos
  // verdades sobre el mismo hecho.
  it('recalcula lo que puso el restaurante al teclear', async () => {
    montar();
    fireEvent.change(await screen.findByLabelText('Descuento total'), { target: { value: '110' } });
    fireEvent.change(screen.getByLabelText('Lo puso la plataforma'), { target: { value: '40' } });
    await waitFor(() => {
      expect(screen.getByLabelText('Lo puso el restaurante').textContent).toContain('70');
    });
  });

  it('manda al servidor lo que dice el documento, sin calcular nada', async () => {
    montar();
    fireEvent.change(await screen.findByLabelText('Venta que reporta'), { target: { value: '220.00' } });
    fireEvent.change(screen.getByLabelText('Comisión'), { target: { value: '33.00' } });
    fireEvent.change(screen.getByLabelText('Depositado (neto)'), { target: { value: '51.77' } });
    fireEvent.click(screen.getByRole('button', { name: /Guardar liquidación/i }));
    await waitFor(() => {
      expect(api.save).toHaveBeenCalledWith(187, expect.objectContaining({
        reportedGross: '220.00', commissionAmount: '33.00', netAmount: '51.77',
      }));
    });
  });

  // La tasa VACÍA viaja como null y no como "0": un cero afirmaría que la plataforma cobró 0%, que
  // es medible y falso.
  it('una tasa sin capturar viaja como ausente, no como cero', async () => {
    montar();
    fireEvent.change(await screen.findByLabelText('Venta que reporta'), { target: { value: '220' } });
    fireEvent.click(screen.getByRole('button', { name: /Guardar liquidación/i }));
    await waitFor(() => {
      expect(api.save).toHaveBeenCalledWith(187, expect.objectContaining({ commissionPct: null }));
    });
  });

  // El neto negativo se puede teclear: con una promoción que financió el restaurante, es lo que
  // de verdad pasó. Rechazarlo aquí obligaría a capturar una mentira.
  it('deja capturar un neto negativo', async () => {
    montar();
    fireEvent.change(await screen.findByLabelText('Depositado (neto)'), { target: { value: '-31.20' } });
    fireEvent.click(screen.getByRole('button', { name: /Guardar liquidación/i }));
    await waitFor(() => {
      expect(api.save).toHaveBeenCalledWith(187, expect.objectContaining({ netAmount: '-31.20' }));
    });
  });
});
