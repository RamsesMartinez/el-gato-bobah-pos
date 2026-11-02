import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { Provider } from '../../../components/ui/provider';
import { ApiError } from '../../../api/client';
import { DensitySheet } from './DensitySheet';

const back = vi.hoisted(() => ({
  setDensity: vi.fn(() => Promise.resolve({ density: { value: '1.1', method: 'weighing', setByName: 'Ana', setAt: '2026-11-02T10:00:00Z' } })),
  clearDensity: vi.fn(() => Promise.resolve()),
}));
vi.mock('../../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));

const salsa = { id: 7, name: 'Salsa de chocolate', density: null };
const conDensidad = { ...salsa, density: { value: '1.1', method: 'weighing' as const, setByName: 'Ana', setAt: '2026-11-02T10:00:00Z' } };

function montar(article: typeof salsa | typeof conDensidad = salsa) {
  const onDone = vi.fn();
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><Provider><DensitySheet article={article} onClose={vi.fn()} onDone={onDone} /></Provider></QueryClientProvider>);
  return onDone;
}

// teclear escribe con el teclado de la app: toca la casilla y luego cada tecla.
async function teclear(label: string, value: string) {
  fireEvent.click(await screen.findByLabelText(label));
  for (const ch of value) fireEvent.click(screen.getByRole('button', { name: ch === '.' ? 'punto' : ch }));
}
const listo = () => fireEvent.click(screen.getByRole('button', { name: 'Listo' }));
const guardar = () => screen.getByRole('button', { name: 'Guardar' });

async function pesada(full: string, empty: string, content: string) {
  await teclear('Pesa lleno (g)', full);
  await teclear('Pesa vacío (g)', empty);
  await teclear('Contiene (ml, según la etiqueta)', content);
  listo();
}

// US1 (FR-002, FR-003): el resultado se ve antes de guardar, y lo imposible no se guarda.
describe('Hoja de densidad', () => {
  beforeEach(() => vi.clearAllMocks());

  it('con una pesada muestra la cuenta y guarda lo pesado', async () => {
    const onDone = montar();
    expect(await screen.findByText('Opcional')).toBeInTheDocument();
    await pesada('1250', '150', '1000');
    expect(screen.getByText('(1,250 g − 150 g) ÷ 1,000 ml = 1.10 gramos por mililitro')).toBeInTheDocument();
    fireEvent.click(guardar());
    await waitFor(() => expect(back.setDensity).toHaveBeenCalledWith(7, { method: 'weighing', fullG: '1250', emptyG: '150', contentMl: '1000' }));
    await waitFor(() => expect(onDone).toHaveBeenCalled());
  });

  it('el (i) dice para qué sirve con el ejemplo del galón', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: '¿Para qué sirve la densidad?' }));
    expect(screen.getByText(/4,300 g ÷ 1.10 = 3,909 ml/)).toBeInTheDocument();
    expect(screen.getByText(/Sin ella todo funciona igual que hoy/)).toBeInTheDocument();
  });

  it.each([
    ['vacío igual a lleno', '150', '150', '1000', /vacío tiene que pesar menos/],
    ['vacío más que lleno', '150', '1250', '1000', /vacío tiene que pesar menos/],
    ['contenido cero', '1250', '150', '0', /mayor que cero/],
  ])('%s: «Guardar» no se puede tocar y dice qué revisar', async (_n, full, empty, content, msg) => {
    montar();
    await pesada(full, empty, content);
    expect(screen.getByText(msg)).toBeInTheDocument();
    expect(guardar()).toBeDisabled();
  });

  it('fuera de 0.30–3.00 dice cuánto da y no guarda', async () => {
    montar();
    await pesada('5000', '100', '1000');
    expect(screen.getByText(/Da 4.90 gramos por mililitro/)).toBeInTheDocument();
    expect(guardar()).toBeDisabled();
  });

  // Borrar un campo es «sin capturar», nunca cero: con vacío en 0 la cuenta daba una densidad falsa.
  it('borrar «Pesa vacío» lo deja vacío, no en cero', async () => {
    montar();
    await teclear('Pesa lleno (g)', '1100');
    await teclear('Pesa vacío (g)', '15');
    fireEvent.click(screen.getByRole('button', { name: 'Borrar' }));
    fireEvent.click(screen.getByRole('button', { name: 'Borrar' }));
    await teclear('Contiene (ml, según la etiqueta)', '1000');
    listo();
    expect(screen.getByLabelText('Pesa vacío (g)')).toHaveValue('');
    expect(screen.queryByText(/gramos por mililitro$/)).toBeNull();
    expect(guardar()).toBeDisabled();
  });

  it('escrita a mano manda el valor', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Escribir el valor' }));
    await teclear('Gramos por mililitro', '1.1');
    listo();
    expect(screen.getByText('1.10 gramos por mililitro')).toBeInTheDocument();
    fireEvent.click(guardar());
    await waitFor(() => expect(back.setDensity).toHaveBeenCalledWith(7, { method: 'manual', density: '1.1' }));
  });

  it('con la etiqueta divide el peso neto entre el contenido', async () => {
    montar();
    fireEvent.click(await screen.findByRole('button', { name: 'Escribir el valor' }));
    fireEvent.click(screen.getByRole('button', { name: 'Con la etiqueta' }));
    await teclear('Peso neto (g)', '4300');
    await teclear('Contenido (ml)', '3785');
    listo();
    expect(screen.getByText('4,300 g ÷ 3,785 ml = 1.1361 gramos por mililitro')).toBeInTheDocument();
    fireEvent.click(guardar());
    await waitFor(() => expect(back.setDensity).toHaveBeenCalledWith(7, { method: 'label', netG: '4300', contentMl: '3785' }));
  });

  it('«Quitar densidad» pregunta en el mismo lugar antes de quitarla', async () => {
    const onDone = montar(conDensidad);
    fireEvent.click(await screen.findByRole('button', { name: 'Quitar densidad' }));
    expect(back.clearDensity).not.toHaveBeenCalled();
    expect(screen.getByText('¿Quitar?')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'No' }));
    expect(screen.getByRole('button', { name: 'Quitar densidad' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Quitar densidad' }));
    fireEvent.click(screen.getByRole('button', { name: 'Sí' }));
    await waitFor(() => expect(back.clearDensity).toHaveBeenCalledWith(7));
    await waitFor(() => expect(onDone).toHaveBeenCalled());
  });

  it('sin densidad no ofrece quitarla', async () => {
    montar();
    await screen.findByText('Opcional');
    expect(screen.queryByRole('button', { name: 'Quitar densidad' })).toBeNull();
  });

  it('quitarla con una compra por recibir que la necesita lo dice', async () => {
    back.clearDensity.mockRejectedValueOnce(new ApiError(409, 'DENSITY_IN_USE', 'conflicto: hay una compra…', 'r'));
    const onDone = montar(conDensidad);
    fireEvent.click(await screen.findByRole('button', { name: 'Quitar densidad' }));
    fireEvent.click(screen.getByRole('button', { name: 'Sí' }));
    expect(await screen.findByText('Hay una compra por recibir que la necesita')).toBeInTheDocument();
    expect(onDone).not.toHaveBeenCalled();
  });
});
