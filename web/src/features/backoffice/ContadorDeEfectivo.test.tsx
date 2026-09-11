import { vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ChakraProvider, defaultSystem } from '@chakra-ui/react';
import type { ReactNode } from 'react';

const cashDenominations = vi.hoisted(() => vi.fn());
vi.mock('../../api/backoffice', () => ({ backofficeApi: { cashDenominations } }));

import { ContadorDeEfectivo } from './ContadorDeEfectivo';

function pinta(nodo: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <ChakraProvider value={defaultSystem}>
      <QueryClientProvider client={qc}>{nodo}</QueryClientProvider>
    </ChakraProvider>,
  );
}

const catalogo = [
  { id: 11, value: '1000.00', isCoin: false },
  { id: 6, value: '50.00', isCoin: false },
  { id: 5, value: '10.00', isCoin: true },
  { id: 1, value: '0.50', isCoin: true },
];

function hoja(onConfirmar = vi.fn(), onClose = vi.fn()) {
  pinta(
    <ContadorDeEfectivo isOpen onClose={onClose} titulo="Fondo inicial" currency="MXN"
      etiquetaConfirmar="Abrir caja" guardando={false} onConfirmar={onConfirmar} />,
  );
  return onConfirmar;
}

beforeEach(() => {
  cashDenominations.mockReset();
  cashDenominations.mockResolvedValue({ items: catalogo });
});

// El caso de la operación diaria: se cuenta el montón y se escribe. Si esto exige tocar `+`, la
// pantalla obliga a 40 taps para 40 monedas y SC-003 no se cumple.
test('un cajón completo se captura escribiendo, sin tocar ni una vez + o −', async () => {
  const onConfirmar = hoja();
  const u = userEvent.setup();
  await waitFor(() => expect(screen.getByLabelText('Piezas de $10')).toBeInTheDocument());

  await u.type(screen.getByLabelText('Piezas de $10'), '40');
  await u.type(screen.getByLabelText('Piezas de $50'), '3');

  // 40 × $10 + 3 × $50 = $550, y el total se ve sin haber confirmado nada.
  expect(screen.getByLabelText('Total contado')).toHaveTextContent('$550');

  await u.click(screen.getByText('Abrir caja'));
  expect(onConfirmar).toHaveBeenCalledWith({
    counts: [{ denominationId: 6, pieces: 3 }, { denominationId: 5, pieces: 40 }],
    total: 550,
  });
});

test('el total se actualiza en cada tecla, no al confirmar', async () => {
  hoja();
  const u = userEvent.setup();
  await waitFor(() => expect(screen.getByLabelText('Piezas de 50¢')).toBeInTheDocument());

  await u.type(screen.getByLabelText('Piezas de 50¢'), '7');
  // 7 monedas de 50¢ son $3.50: truncar el medio peso es el error que la feature viene a quitar.
  expect(screen.getByLabelText('Total contado')).toHaveTextContent('$3.5');

  await u.type(screen.getByLabelText('Piezas de $1,000'), '2');
  expect(screen.getByLabelText('Total contado')).toHaveTextContent('$2,003.5');
});

test('el + ajusta una pieza sobre lo que ya se escribió', async () => {
  hoja();
  const u = userEvent.setup();
  await waitFor(() => expect(screen.getByLabelText('Piezas de $50')).toBeInTheDocument());

  await u.type(screen.getByLabelText('Piezas de $50'), '3');
  await u.click(screen.getByLabelText('Una más de $50'));
  expect(screen.getByLabelText('Piezas de $50')).toHaveValue('4');
  expect(screen.getByLabelText('Total contado')).toHaveTextContent('$200');
});

test('una denominación en cero no manda renglón', async () => {
  const onConfirmar = hoja();
  const u = userEvent.setup();
  await waitFor(() => expect(screen.getByLabelText('Piezas de $50')).toBeInTheDocument());

  await u.type(screen.getByLabelText('Piezas de $1,000'), '0');
  await u.type(screen.getByLabelText('Piezas de $50'), '2');
  await u.click(screen.getByText('Abrir caja'));

  expect(onConfirmar).toHaveBeenCalledWith({ counts: [{ denominationId: 6, pieces: 2 }], total: 100 });
});

// Un campo con basura no puede valer cero: sería declarar que no había nada de esa denominación.
test('un campo mal capturado se nombra y apaga el botón', async () => {
  const onConfirmar = hoja();
  const u = userEvent.setup();
  await waitFor(() => expect(screen.getByLabelText('Piezas de $50')).toBeInTheDocument());

  await u.type(screen.getByLabelText('Piezas de $50'), '1,000');

  expect(screen.getByText(/Revisa las piezas de \$50/)).toBeInTheDocument();
  expect(screen.getByText('Abrir caja')).toBeDisabled();
  // Y el ajuste tampoco puede "arreglarlo" sumándole uno: descartaría lo tecleado en silencio.
  expect(screen.getByLabelText('Una más de $50')).toBeDisabled();
  expect(onConfirmar).not.toHaveBeenCalled();
});

// FR-015: los dos caminos no pueden coexistir, así que cambiar descarta lo capturado — y eso se
// avisa ANTES. Un cajón contado que desaparece por un tap se vuelve a contar a mano.
test('cambiar de camino con algo capturado advierte antes de descartarlo', async () => {
  hoja();
  const u = userEvent.setup();
  await waitFor(() => expect(screen.getByLabelText('Piezas de $50')).toBeInTheDocument());

  await u.type(screen.getByLabelText('Piezas de $50'), '3');
  await u.click(screen.getByText('Escribir el total'));

  expect(screen.getByText('Al cambiar se borra lo que ya capturaste.')).toBeInTheDocument();
  // Todavía no cambió: lo capturado sigue ahí.
  expect(screen.getByLabelText('Piezas de $50')).toHaveValue('3');

  await u.click(screen.getByText('Seguir aquí'));
  expect(screen.getByLabelText('Piezas de $50')).toHaveValue('3');

  await u.click(screen.getByText('Escribir el total'));
  await u.click(screen.getByText('Borrar y cambiar'));
  expect(screen.getByLabelText('Efectivo')).toBeInTheDocument();
});

test('sin nada capturado el interruptor cambia directo', async () => {
  hoja();
  const u = userEvent.setup();
  await waitFor(() => expect(screen.getByLabelText('Piezas de $50')).toBeInTheDocument());

  await u.click(screen.getByText('Escribir el total'));
  expect(screen.getByLabelText('Efectivo')).toBeInTheDocument();
});

// FR-016: el total a mano exige motivo. Sin él quedan arqueos con una cifra que nadie puede
// reconstruir ni justificar, que es el problema que esta feature viene a cerrar.
test('el total escrito a mano no se puede confirmar sin motivo', async () => {
  const onConfirmar = hoja();
  const u = userEvent.setup();
  await waitFor(() => expect(screen.getByLabelText('Piezas de $50')).toBeInTheDocument());

  await u.click(screen.getByText('Escribir el total'));
  await u.type(screen.getByLabelText('Efectivo'), '2350');
  expect(screen.getByText('Abrir caja')).toBeDisabled();

  await u.type(screen.getByLabelText('Por qué no se contó pieza por pieza'), 'un billete que no está en la lista');
  await u.click(screen.getByText('Abrir caja'));

  expect(onConfirmar).toHaveBeenCalledWith({
    total: 2350, manualReason: 'un billete que no está en la lista',
  });
});

// La misma pérdida que protege el cambio de camino, por el otro lado: «Cancelar» está a 12 px del
// botón de confirmar y descartaba el conteo entero sin preguntar. Un tap fallido costaba recontar
// hasta once denominaciones — hasta los 2 minutos que SC-003 viene a defender.
test('Cancelar con algo capturado advierte antes de descartarlo', async () => {
  const onClose = vi.fn();
  hoja(vi.fn(), onClose);
  const u = userEvent.setup();
  await waitFor(() => expect(screen.getByLabelText('Piezas de $50')).toBeInTheDocument());

  await u.type(screen.getByLabelText('Piezas de $50'), '3');
  await u.click(screen.getByText('Cancelar'));

  expect(onClose).not.toHaveBeenCalled();
  expect(screen.getByText('Si sales ahora se borra lo que ya capturaste.')).toBeInTheDocument();
  expect(screen.getByLabelText('Piezas de $50')).toHaveValue('3');

  await u.click(screen.getByText('Seguir contando'));
  expect(onClose).not.toHaveBeenCalled();
  expect(screen.getByLabelText('Piezas de $50')).toHaveValue('3');

  await u.click(screen.getByText('Cancelar'));
  await u.click(screen.getByText('Salir sin guardar'));
  expect(onClose).toHaveBeenCalled();
});

test('Cancelar sin nada capturado cierra sin estorbar', async () => {
  const onClose = vi.fn();
  hoja(vi.fn(), onClose);
  const u = userEvent.setup();
  await waitFor(() => expect(screen.getByLabelText('Piezas de $50')).toBeInTheDocument());

  await u.click(screen.getByText('Cancelar'));
  expect(onClose).toHaveBeenCalled();
});
