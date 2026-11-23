import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Provider } from '../../../components/ui/provider';
import { ExpectedWasteField } from './ExpectedWasteField';

const back = vi.hoisted(() => ({ setArticleExpectedWaste: vi.fn() }));
vi.mock('../../../api/backoffice', async (orig) => ({ ...(await orig<object>()), backofficeApi: back }));

beforeEach(() => back.setArticleExpectedWaste.mockResolvedValue({ pct: null }));
afterEach(() => vi.clearAllMocks());

function montar(value: string | null, canEdit = true) {
  render(<QueryClientProvider client={new QueryClient()}><Provider>
    <ExpectedWasteField id={7} value={value} generalPct="5.00" canEdit={canEdit} />
  </Provider></QueryClientProvider>);
}

// El % del artículo (spec 096, maqueta A2): «sin capturar» y «capturada en 0» son dos estados que se
// ven distintos y viajan distinto (null contra 0).
describe('Merma al preparar en la ficha del artículo', () => {
  test('sin captura dice de dónde sale la que se usa y no muestra campo', () => {
    montar(null);
    expect(screen.getByRole('button', { name: 'Sin capturar' })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByText('Usa la del producto o la general (5 %)')).toBeInTheDocument();
    expect(screen.queryByLabelText('Merma del artículo, en porcentaje')).toBeNull();
  });

  test('capturar 0 manda 0 y dice que aquí no se pierde nada', async () => {
    montar(null);
    fireEvent.click(screen.getByRole('button', { name: 'Capturada' }));
    const input = screen.getByLabelText('Merma del artículo, en porcentaje');
    fireEvent.change(input, { target: { value: '0' } });
    fireEvent.blur(input);
    await waitFor(() => expect(back.setArticleExpectedWaste).toHaveBeenCalledWith(7, 0));
    expect(screen.getByText('Aquí no se pierde nada')).toBeInTheDocument();
  });

  test('volver a «Sin capturar» manda null', async () => {
    montar('15.00');
    expect(screen.getByLabelText('Merma del artículo, en porcentaje')).toHaveValue('15');
    fireEvent.click(screen.getByRole('button', { name: 'Sin capturar' }));
    await waitFor(() => expect(back.setArticleExpectedWaste).toHaveBeenCalledWith(7, null));
  });

  test('«Capturada» con el campo vacío no manda nada: vacío nunca es 0', () => {
    montar(null);
    fireEvent.click(screen.getByRole('button', { name: 'Capturada' }));
    fireEvent.blur(screen.getByLabelText('Merma del artículo, en porcentaje'));
    expect(back.setArticleExpectedWaste).not.toHaveBeenCalled();
  });

  test('sin permiso se ve pero no se cambia', () => {
    montar('15.00', false);
    expect(screen.getByLabelText('Merma del artículo, en porcentaje')).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Sin capturar' })).toBeDisabled();
  });
});
