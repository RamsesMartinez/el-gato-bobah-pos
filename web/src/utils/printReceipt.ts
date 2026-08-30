import type { OrderView } from '../types/pos';
import { money } from './format';

const SERVICE: Record<string, string> = {
  mostrador: 'Mostrador',
  para_llevar: 'Para llevar',
  domicilio: 'Domicilio',
};

// Escapa datos controlados por el usuario (nombres de producto/cliente, modificadores)
// antes de interpolarlos en el HTML del ticket. Sin esto, un nombre malicioso ejecuta
// JS en la ventana same-origin del ticket (acceso a window.opener → token en localStorage).
function esc(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

// TicketBusinessInfo es la identidad del negocio tal como sale en el encabezado. Los opcionales
// llegan como string vacío (no null) y su renglón simplemente no se imprime.
export interface TicketBusinessInfo {
  businessName: string;
  address: string;
  phone: string;
  headerNote: string;
  footerNote: string;
  // Data URI, NUNCA una URL remota. Dos razones: la CSP de producción es `img-src 'self' data:` y
  // la API vive en otro dominio, así que un <img src="https://api…"> queda bloqueado solo en el
  // sitio publicado; y una carga de red puede no haber terminado cuando se dispara print(), que
  // saca el ticket con un hueco blanco donde iba el logo. Vacío = sin logo.
  logoDataUri: string;
}

// buildReceiptHtml arma el HTML del ticket (función pura y testeable). Todo string de
// datos pasa por esc(); los numéricos van por money() y son seguros.
export function buildReceiptHtml(
  order: OrderView,
  business: TicketBusinessInfo,
  opts: { reprint?: boolean; sample?: boolean; printFreeModifiers?: boolean } = {},
): string {
  // El renglón del producto muestra su BASE (cantidad × unitario) y cada adicional con costo lleva
  // el suyo debajo, así los números suman a la vista. Antes el renglón traía el total con los
  // extras adentro y el cliente no tenía cómo explicarse por qué pagó de más.
  const rows = (order.lines ?? [])
    .map((l) => {
      const qty = Number(l.quantity) || 0;
      const unit = Number(l.unitPrice) || 0;
      const mods = (l.modifiers ?? [])
        .map((m) => {
          const delta = Number(m.priceDelta) || 0;
          const veces = m.quantity > 1 ? ` x${m.quantity}` : '';
          // Sin costo: se lista sin cifra. La ausencia de número es lo que hace que los que sí
          // cuestan salten a la vista, y el negocio puede apagarlos para no alargar el papel.
          if (delta === 0) {
            if (opts.printFreeModifiers === false) return '';
            return `<tr><td class="mod">+ ${esc(m.name)}${veces}</td><td class="r mod"></td></tr>`;
          }
          const importe = delta * m.quantity * qty;
          return `<tr><td class="mod">+ ${esc(m.name)}${veces} @${money(delta)}</td>` +
                 `<td class="r mod">${money(importe)}</td></tr>`;
        })
        .join('');
      return `<tr><td>${l.quantity}x ${esc(l.productName)} @${money(unit)}</td>` +
             `<td class="r">${money(qty * unit)}</td></tr>${mods}`;
    })
    .join('');

  // Desglose solo cuando hubo envío (domicilio): el ticket muestra subtotal + envío antes del total.
  const totalsHead = Number(order.deliveryFee) > 0
    ? `<tr><td>Subtotal</td><td class="r">${money(order.subtotal)}</td></tr>` +
      `<tr><td>Envío</td><td class="r">${money(order.deliveryFee)}</td></tr>`
    : '';

  return `<!doctype html><html><head><meta charset="utf-8"><title>Ticket #${order.number}</title>
<style>
  @page { size: 80mm auto; margin: 0; }
  /* Todo en negro puro y en negritas: la térmica imprime a 1 bit, así que un gris no sale gris —
     sale como un patrón de puntos salteados que se lee desvaído, y un trazo delgado se pierde
     entre los puntos. La jerarquía se hace con tamaño y espaciado, nunca con color. */
  body { width: 80mm; margin: 0; padding: 6mm 4mm; font-family: 'Courier New', monospace; font-size: 13px; font-weight: bold; color: #000; }
  h1 { font-size: 17px; text-align: center; margin: 0 0 2px; }
  .center { text-align: center; }
  .muted { color: #000; font-size: 12px; }
  /* pre-line: los textos del negocio son bloques de varios renglones (el aviso de "sin valor
     fiscal" y sus datos de facturación). Sin esto el HTML colapsa los saltos y sale un párrafo
     corrido que nadie lee. */
  .note { margin: 3px 0; white-space: pre-line; }
  hr { border: none; border-top: 1px dashed #000; margin: 6px 0; }
  table { width: 100%; border-collapse: collapse; }
  td { vertical-align: top; padding: 2px 0; }
  .r { text-align: right; white-space: nowrap; }
  .mod { font-size: 11px; padding-left: 8px; }
  .total { font-size: 15px; font-weight: bold; }
  .logo { display: block; margin: 0 auto 4px; max-width: 40mm; max-height: 20mm; }
  .reprint { font-weight: bold; letter-spacing: 1px; margin: 4px 0; }
</style></head><body>
  ${business.logoDataUri ? `<img class="logo" src="${esc(business.logoDataUri)}" alt=""/>` : ''}
  <h1>${esc(business.businessName)}</h1>
  ${business.address ? `<div class="center muted">${esc(business.address)}</div>` : ''}
  ${business.phone ? `<div class="center muted">Tel. ${esc(business.phone)}</div>` : ''}
  ${business.headerNote ? `<div class="center note">${esc(business.headerNote)}</div>` : ''}
  <div class="center muted">Pedido #${order.number}</div>
  <div class="center muted">${new Date(order.openedAt).toLocaleString('es-MX')}</div>
  <div class="center muted">${esc(SERVICE[order.serviceType] ?? order.serviceType)}${order.customerName ? ` · ${esc(order.customerName)}` : ''}</div>
  ${opts.reprint ? '<div class="center reprint">** REIMPRESIÓN **</div>' : ''}
  ${opts.sample ? '<div class="center reprint">** TICKET DE PRUEBA **</div>' : ''}
  <hr/>
  <table>${rows}</table>
  <hr/>
  <table>${totalsHead}<tr><td class="total">TOTAL</td><td class="r total">${money(order.total)}</td></tr></table>
  <div class="center muted" style="margin-top:8px">${order.paid ? 'PAGADO' : 'POR COBRAR'}</div>
  <div class="center note" style="margin-top:10px">${business.footerNote ? esc(business.footerNote) : "¡Gracias!"}</div>
</body></html>`;
}

// Cuánto se deja el iframe en el DOM después de disparar la impresión, y cuánto se espera a que
// cargue antes de rendirse. Generosos a propósito: el costo de esperar de más es un iframe
// invisible unos segundos; el de quedarse corto es un ticket que no sale.
const FRAME_CLEANUP_MS = 10000;
const FRAME_LOAD_TIMEOUT_MS = 15000;

// Ventana del candado anti-doble-toque. No es un número mágico: es cuánto tarda en aparecer el
// diálogo (o el trabajo, con impresión directa) para que el operador vea que ya pasó algo.
const PRINT_LOCK_MS = 1000;
let printing = false;

// printFrame manda a imprimir el documento que YA está montado en el iframe de la vista previa.
// Se llama desde el padre, nunca con un <script> dentro del srcdoc: la CSP de producción es
// `script-src 'self'` y ese script quedaría bloqueado en el sitio publicado mientras funciona en
// dev, donde Vite no manda CSP — el peor tipo de falla, la que solo aparece con el cliente enfrente.
//
// Devuelve false si no hubo nada que imprimir (iframe aún sin documento) o si el candado sigue
// activo. Reemplaza al window.open de antes, que los bloqueadores de popups mataban en silencio.
export function printFrame(frame: HTMLIFrameElement | null): boolean {
  const win = frame?.contentWindow;
  if (!win || printing) return false;
  printing = true;
  setTimeout(() => {
    printing = false;
  }, PRINT_LOCK_MS);
  win.focus();
  win.print();
  return true;
}

// printHtmlOffscreen imprime un ticket SIN vista previa: es lo que usa la impresión automática al
// cerrar una venta. Monta el documento en un iframe fuera de pantalla, espera a que cargue,
// imprime y lo desmonta.
//
// Esperar el `load` no es opcional: disparar print() antes de que el documento exista saca papel en
// blanco, que es un fallo que solo se nota cuando el cliente ya tiene el ticket en la mano.
export function printHtmlOffscreen(html: string): Promise<boolean> {
  return new Promise((resolve) => {
    const frame = document.createElement('iframe');
    // Fuera de la vista pero DENTRO del layout: display:none no renderiza y algunos navegadores no
    // imprimen un frame sin caja.
    frame.setAttribute('aria-hidden', 'true');
    frame.style.cssText = 'position:fixed;left:-10000px;top:0;width:80mm;height:1px;border:0;';
    // Mismo sandbox que la vista previa: sin allow-scripts, con allow-modals para que pase print().
    frame.setAttribute('sandbox', 'allow-same-origin allow-modals');
    frame.setAttribute('srcdoc', html);

    let terminado = false;
    const cleanup = (printed: boolean) => {
      if (terminado) return;
      terminado = true;
      // El iframe NO se quita en el mismo tick en que se imprime: quitarlo ahí cancela el trabajo
      // en algunos navegadores —el diálogo alcanza a abrir y se queda sin documento— y con
      // impresión directa el trabajo se va a medias.
      setTimeout(() => frame.remove(), FRAME_CLEANUP_MS);
      resolve(printed);
    };

    frame.addEventListener('load', () => {
      // Un iframe recién insertado dispara `load` por su about:blank inicial ANTES de cargar el
      // srcdoc. Imprimir en ese momento saca papel en blanco y desmonta el marco antes de que
      // llegue el ticket, así que se espera hasta que el documento traiga contenido de verdad.
      const doc = frame.contentDocument;
      if (!doc?.body?.hasChildNodes()) return;
      try {
        cleanup(printFrame(frame));
      } catch {
        // Una impresora apagada no debe tumbar la venta: el pedido ya está registrado y el
        // operador puede reimprimir desde el tablero.
        cleanup(false);
      }
    });

    // Red de seguridad: si el documento nunca carga (srcdoc bloqueado, navegador raro), no se deja
    // un iframe colgado en el DOM del POS ni una promesa sin resolver por el resto del turno.
    setTimeout(() => cleanup(false), FRAME_LOAD_TIMEOUT_MS);
    document.body.appendChild(frame);
  });
}

// sampleTicketOrder arma un pedido de muestra para el ticket de prueba de la pantalla de
// configuración: sirve para ver en papel cómo quedó el logo y los textos sin tener que cobrar una
// venta de mentiras, que ensuciaría los reportes y el corte de caja.
export function sampleTicketOrder(): OrderView {
  return {
    id: 0,
    number: 0,
    status: 'entregada',
    serviceType: 'mostrador',
    customerName: null,
    subtotal: '250',
    deliveryFee: '0',
    total: '250',
    currency: 'MXN',
    paid: true,
    // Fecha fija: el ticket de prueba se compara contra el papel anterior al ajustar la impresora,
    // y una hora que cambia en cada impresión estorba esa comparación.
    openedAt: '2026-01-01T12:00:00.000Z',
    lines: [
      {
        productName: 'Producto de ejemplo',
        quantity: '2',
        unitPrice: '80',
        lineTotal: '160',
        modifiers: [{ name: 'Con modificador', quantity: 1, priceDelta: '0' }],
      },
      { productName: 'Otro producto', quantity: '1', unitPrice: '90', lineTotal: '90', modifiers: [] },
    ],
  };
}

// Ancho útil del ticket en caracteres. Sale de la geometría del documento: 80mm de papel menos
// 4mm de margen por lado son 72mm ≈ 272px a 96dpi, y Courier New avanza 0.6em ≈ 7.8px a 13px, así
// que caben ~34. Se deja en 32 para no quedar al filo: un renglón que se pasa por uno no "se ve
// apretado", se parte en dos y desacomoda el bloque entero.
export const TICKET_COLUMNS = 32;

// overflowingLines devuelve los números de renglón (1-based) que no caben a lo ancho del papel.
// Es lo que deja avisar al operador MIENTRAS escribe, en vez de que lo descubra imprimiendo.
export function overflowingLines(text: string, width = TICKET_COLUMNS): number[] {
  if (!text) return [];
  return text
    .split('\n')
    // Array.from y no .length: "ñ" son dos bytes pero un carácter, y medir en bytes marcaría como
    // largo un renglón que sí cabe.
    .map((line, i) => (Array.from(line).length > width ? i + 1 : 0))
    .filter((n) => n > 0);
}
