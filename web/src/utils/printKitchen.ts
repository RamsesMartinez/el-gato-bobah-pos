import type { ReceiptOrder } from '../types/pos';
import { soloHora } from './horaDelNegocio';
import { DEFAULT_TIMEZONE } from './zonaPorDefecto';

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

// `soloLineas` limita el papel a esos renglones: es la comanda de un AGREGADO.
//
// Cocina ya está preparando lo anterior, así que reimprimir el pedido entero la haría preparar dos
// veces lo mismo. El folio es el mismo en los dos papeles: es con lo que los junta.
export function buildKitchenHtml(order: ReceiptOrder, soloLineas?: number[], zona?: string): string {
  const hora = new Date(order.openedAt);
  // La hora del NEGOCIO, no la de la tableta: dos estaciones con el reloj distinto sacarían
  // comandas con horas distintas del mismo pedido, y cocina las junta por hora.
  const horaTxt = Number.isNaN(hora.getTime()) ? '' : soloHora(hora, zona ?? DEFAULT_TIMEZONE);

  const esAgregado = soloLineas !== undefined && soloLineas.length > 0;
  const aImprimir = esAgregado
    ? (order.lines ?? []).filter((l) => l.id !== undefined && soloLineas.includes(l.id))
    : (order.lines ?? []);

  const lineas = aImprimir
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
  return `<!doctype html><html><head><meta charset="utf-8"><title>Comanda ${esAgregado ? 'agregado ' : ''}${order.number}</title>
<style>
  @page { margin: 4mm; }
  body { font-family: 'Courier New', monospace; font-size: 15px; margin: 0; }
  .folio { font-size: 46px; font-weight: 800; line-height: 1; }
  .num { font-size: 15px; font-weight: 600; color: #444; }
  .cab { display: flex; justify-content: space-between; align-items: baseline;
         border-bottom: 2px dashed #000; padding-bottom: 6px; margin-bottom: 8px; }
  .hora { font-size: 16px; }
  .quien { font-size: 18px; font-weight: 700; margin-bottom: 8px; }
  .linea { margin-bottom: 10px; }
  .prod { font-size: 20px; font-weight: 700; }
  .mod { font-size: 16px; padding-left: 14px; }
  .nota { font-size: 16px; font-weight: 700; padding-left: 14px; text-transform: uppercase; }
  /* Lo primero que se lee del papel: sin esto, cocina no distingue un agregado de un pedido nuevo
     con el mismo nombre y prepara dos veces. */
  .agregado { font-size: 22px; font-weight: 800; letter-spacing: 2px; }
</style></head><body>
<div class="cab">
  <div>
    ${esAgregado ? '<div class="agregado">AGREGADO</div>' : ''}
    <div class="folio">${order.folioName || `#${order.number}`}</div>
    ${order.folioName ? `<div class="num">#${order.number}</div>` : ''}
  </div>
  <div class="hora">${horaTxt}</div>
</div>
${quien}${lineas}
</body></html>`;
}
