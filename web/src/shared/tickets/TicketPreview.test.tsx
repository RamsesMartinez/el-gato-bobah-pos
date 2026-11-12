import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';
import { Provider } from '../../components/ui/provider';
import { TicketPreviewDialog } from './TicketPreview';
import { buildReceiptHtml, sampleTicketOrder, type TicketBusinessInfo } from '../../utils/printReceipt';

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

const negocio: TicketBusinessInfo = {
  businessName: 'El Gato Bobah', address: '', phone: '', headerNote: '', footerNote: '', logoDataUri: '', timezone: 'America/Mexico_City',
};

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

// El ancho exterior del documento del ticket en px de pantalla, leído de SU propia hoja de estilo:
// el ancho del cuerpo más su relleno lateral. El cuerpo es content-box, así que el relleno se suma.
function anchoDelDocumento(doc: string): number {
  const body = /body\s*\{([^}]*)\}/.exec(doc)?.[1] ?? '';
  const ancho = Number(/(?:^|;)\s*width:\s*([\d.]+)mm/.exec(body)?.[1]);
  const [, , lateral = 0] = /padding:\s*([\d.]+)mm\s+([\d.]+)mm/.exec(body)?.map(Number) ?? [];
  expect(ancho, 'no encontré el ancho del cuerpo del ticket en su hoja de estilo').toBeGreaterThan(0);
  return ((ancho + 2 * lateral) * 96) / 25.4;
}

test('el marco deja ver el ticket completo: la columna de importes y el TOTAL no quedan fuera (#233)', () => {
  // Medido a 1024×600: el documento mide ~333 px (80 mm de papel + 4 mm de relleno por lado) y el
  // marco medía 302 px (solo los 80 mm). Los ~31 px que sobraban eran la columna de importes: el
  // operador veía «TOTAL» sin cifra y «@$12» sin el importe del renglón.
  const doc = buildReceiptHtml(sampleTicketOrder(), negocio, {});
  const { container } = wrap(<TicketPreviewDialog html={doc} isOpen onClose={vi.fn()} />);
  const frame = container.ownerDocument.querySelector('iframe')!;
  const documento = anchoDelDocumento(doc);
  expect(
    Number.parseInt(frame.style.width, 10),
    `el marco mide ${frame.style.width} y el ticket ${documento.toFixed(1)} px: se corta la columna de importes`,
  ).toBeGreaterThanOrEqual(Math.ceil(documento));
});
