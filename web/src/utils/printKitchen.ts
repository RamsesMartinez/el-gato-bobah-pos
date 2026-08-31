import type { OrderView } from '../types/pos';

// La comanda de cocina: el papel que reemplaza a la libreta.
//
// Es OTRO documento que el ticket del cliente, no el mismo con menos cosas. Lleva folio grande, qué
// preparar y las notas; NO lleva precios ni total. Si llevara importes y alguien se lo entregara al
// cliente, se volvería un comprobante que el negocio no emitió.
//
// Los adicionales sin costo SÍ salen, al revés que en el ticket del cliente: "sin cebolla" no
// cuesta y es justo lo que cambia cómo se prepara el plato. El ajuste que los oculta existe para
// acortar el papel del cliente, no el de cocina.

function esc(s: string): string {
  return s.replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c] as string);
}

export function buildKitchenHtml(order: OrderView): string {
  const hora = new Date(order.openedAt);
  const horaTxt = Number.isNaN(hora.getTime())
    ? ''
    : hora.toLocaleTimeString('es-MX', { hour: '2-digit', minute: '2-digit' });

  const lineas = (order.lines ?? [])
    .map((l) => {
      const mods = (l.modifiers ?? [])
        .map((m) => `<div class="mod">+ ${esc(m.name)}${m.quantity > 1 ? ` x${m.quantity}` : ''}</div>`)
        .join('');
      const nota = l.notes ? `<div class="nota">${esc(l.notes)}</div>` : '';
      return `<div class="linea"><div class="prod">${l.quantity}x ${esc(l.productName)}</div>${mods}${nota}</div>`;
    })
    .join('');

  const quien = order.customerName ? `<div class="quien">${esc(order.customerName)}</div>` : '';

  // Tipografía grande y de ancho fijo: se lee de lejos, con las manos ocupadas y bajo la luz de una
  // cocina. El folio es lo más grande del papel porque es con lo que se canta el pedido.
  return `<!doctype html><html><head><meta charset="utf-8"><title>Comanda ${order.number}</title>
<style>
  @page { margin: 4mm; }
  body { font-family: 'Courier New', monospace; font-size: 15px; margin: 0; }
  .folio { font-size: 46px; font-weight: 800; line-height: 1; }
  .cab { display: flex; justify-content: space-between; align-items: baseline;
         border-bottom: 2px dashed #000; padding-bottom: 6px; margin-bottom: 8px; }
  .hora { font-size: 16px; }
  .quien { font-size: 18px; font-weight: 700; margin-bottom: 8px; }
  .linea { margin-bottom: 10px; }
  .prod { font-size: 20px; font-weight: 700; }
  .mod { font-size: 16px; padding-left: 14px; }
  .nota { font-size: 16px; font-weight: 700; padding-left: 14px; text-transform: uppercase; }
</style></head><body>
<div class="cab"><div class="folio">#${order.number}</div><div class="hora">${horaTxt}</div></div>
${quien}${lineas}
</body></html>`;
}
