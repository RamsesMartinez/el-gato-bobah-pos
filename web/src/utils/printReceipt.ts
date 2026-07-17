import type { OrderView } from '../types/pos';
import { money } from './format';

const SERVICE: Record<string, string> = {
  mostrador: 'Mostrador',
  para_llevar: 'Para llevar',
  domicilio: 'Domicilio',
};

// Imprime un ticket de 80mm usando el diálogo del navegador. MVP: el driver de la
// impresora térmica (Android/iOS) hace el resto. La versión ESC/POS con print-agent
// queda para fase 2.
export function printReceipt(order: OrderView) {
  const rows = (order.lines ?? [])
    .map((l) => {
      const mods = (l.modifiers ?? [])
        .map((m) => `<div class="mod">+ ${m.name}${m.quantity > 1 ? ` x${m.quantity}` : ''}</div>`)
        .join('');
      return `<tr><td>${l.quantity}x ${l.productName}${mods}</td><td class="r">${money(l.lineTotal)}</td></tr>`;
    })
    .join('');

  const html = `<!doctype html><html><head><meta charset="utf-8"><title>Ticket #${order.number}</title>
<style>
  @page { size: 80mm auto; margin: 0; }
  body { width: 80mm; margin: 0; padding: 6mm 4mm; font-family: 'Courier New', monospace; font-size: 12px; color: #000; }
  h1 { font-size: 16px; text-align: center; margin: 0 0 2px; }
  .center { text-align: center; }
  .muted { color: #333; }
  hr { border: none; border-top: 1px dashed #000; margin: 6px 0; }
  table { width: 100%; border-collapse: collapse; }
  td { vertical-align: top; padding: 2px 0; }
  .r { text-align: right; white-space: nowrap; }
  .mod { font-size: 11px; padding-left: 8px; }
  .total { font-size: 15px; font-weight: bold; }
</style></head><body>
  <h1>El Gato Bobah</h1>
  <div class="center muted">Pedido #${order.number}</div>
  <div class="center muted">${new Date(order.openedAt).toLocaleString('es-MX')}</div>
  <div class="center muted">${SERVICE[order.serviceType] ?? order.serviceType}${order.customerName ? ` · ${order.customerName}` : ''}</div>
  <hr/>
  <table>${rows}</table>
  <hr/>
  <table><tr><td class="total">TOTAL</td><td class="r total">${money(order.total)}</td></tr></table>
  <div class="center muted" style="margin-top:8px">${order.paid ? 'PAGADO' : 'POR COBRAR'}</div>
  <div class="center" style="margin-top:10px">¡Gracias!</div>
</body></html>`;

  const w = window.open('', '_blank', 'width=380,height=600');
  if (!w) return;
  w.document.write(html);
  w.document.close();
  w.focus();
  w.print();
}
