import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';
import { Provider } from '../../components/ui/provider';
import { TicketPreviewDialog } from './TicketPreview';

// printFrame ya tiene sus propios tests (candado anti-doble-toque incluido); aquí lo que importa es
// el cableado: que el botón imprima EL MISMO iframe que el operador está viendo.
const printFrame = vi.hoisted(() => vi.fn(() => true));
vi.mock('../../utils/printReceipt', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../utils/printReceipt')>()),
  printFrame,
}));

function wrap(ui: React.ReactElement) {
  return render(<Provider>{ui}</Provider>);
}

const html = '<!doctype html><html><body><h1>El Gato Bobah</h1>Pedido #42</body></html>';

beforeEach(() => printFrame.mockClear());

test('el iframe lleva el documento del ticket y el botón imprime ESE iframe', async () => {
  const u = userEvent.setup();
  const { container } = wrap(<TicketPreviewDialog html={html} isOpen onClose={vi.fn()} />);

  const frame = container.ownerDocument.querySelector('iframe');
  expect(frame).not.toBeNull();
  // Lo que se ve y lo que se imprime son el mismo documento: no hay dos renders que puedan
  // divergir (FR-002).
  expect(frame!.getAttribute('srcdoc')).toBe(html);

  await u.click(screen.getByRole('button', { name: /imprimir/i }));
  expect(printFrame).toHaveBeenCalledTimes(1);
  expect(printFrame).toHaveBeenCalledWith(frame);
});

test('cerrar sin imprimir no gasta papel', async () => {
  const u = userEvent.setup();
  const onClose = vi.fn();
  wrap(<TicketPreviewDialog html={html} isOpen onClose={onClose} />);

  await u.click(screen.getByRole('button', { name: /cerrar/i }));
  expect(onClose).toHaveBeenCalled();
  expect(printFrame).not.toHaveBeenCalled();
});

test('cerrado no monta el documento del ticket', () => {
  const { container } = wrap(<TicketPreviewDialog html={html} isOpen={false} onClose={vi.fn()} />);
  expect(container.ownerDocument.querySelector('iframe')).toBeNull();
});

test('el ancho del ticket no se comprime: se escala, no se aplasta', () => {
  // Comprimir el iframe a un ancho menor que los 80mm del documento deja al ticket con scroll
  // horizontal dentro del marco. En una tablet, arrastrar esa barra es un toque perdido y, peor,
  // el arrastre cuenta como "fuera del diálogo" y lo cierra.
  const { container } = wrap(<TicketPreviewDialog html={html} isOpen onClose={vi.fn()} />);
  const frame = container.ownerDocument.querySelector('iframe')!;
  expect(frame.style.width).toMatch(/\d+px$/);
  expect(frame.style.width).not.toBe('100%');
});
