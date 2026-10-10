import { round2 } from '../../domain/numeros';
import type { ReceiptLine, ReceiptOrder } from '../../types/pos';
import type { RenglonNuevo, VistaCuenta } from './cuentaEnPantalla';

// EL PAPEL DE LA CUENTA (spec 012), desde el ticket (spec 030).
//
// Con la puerta única la hoja de cobro solo se abre sobre un pedido, así que la cuenta que todavía
// no se manda se imprime desde el ⋮ del ticket:
//   - una cuenta en captura sale como PRE-CUENTA, con el nombre AMARRADO al nacer y sin número —no
//     existe hasta mandarla a cocina (D-2)—;
//   - un pedido con algo «Nuevo» lleva lo enviado y lo nuevo, con su número y la suma de los dos.
// Pura a propósito: convertir una cuenta en un papel es una decisión sobre dinero, y se prueba.

function renglonImpreso(l: RenglonNuevo): ReceiptLine {
  return {
    productName: l.name,
    quantity: String(l.qty),
    unitPrice: (l.qty > 0 ? round2(l.lineTotal / l.qty) : 0).toFixed(2),
    lineTotal: l.lineTotal.toFixed(2),
    notes: l.notes || undefined,
    modifiers: l.modifiers.map((m) => ({ name: m.name, quantity: m.qty, priceDelta: m.priceDelta })),
  };
}

export function cuentaImpresa(v: VistaCuenta, ahora: Date): { order: ReceiptOrder; preCuenta: boolean } | null {
  // Lo que el servidor no confirmó o ya no vende no se cobra: no va en el papel.
  const nuevos = v.nuevos.filter((l) => !l.guardando && l.available).map(renglonImpreso);
  const enviados = (v.pedido?.lines ?? []).filter((l) => !l.cancelled);
  if (nuevos.length + enviados.length === 0) return null;

  const descuento = round2(Number(v.pedido?.discount ?? 0) + v.descuentoNuevo);
  const subtotal = round2(Number(v.pedido?.subtotal ?? 0) + v.subtotalNuevo);
  const total = round2(v.totalPedido + v.totalNuevo);
  const envio = round2(total - subtotal + descuento);
  const base = {
    id: v.pedido?.id ?? 0,
    folioName: v.nombre,
    status: 'abierta' as const,
    serviceType: v.serviceType,
    customerName: v.customerName || null,
    subtotal: subtotal.toFixed(2),
    discount: descuento.toFixed(2),
    deliveryFee: envio.toFixed(2),
    total: total.toFixed(2),
    currency: v.pedido?.currency ?? 'MXN',
    refund: '0',
    openedAt: v.pedido?.openedAt ?? ahora.toISOString(),
    lines: [...enviados, ...nuevos] as ReceiptLine[],
  };
  if (v.pedido) {
    return { order: { ...base, number: v.pedido.number, paid: v.falta <= 0 }, preCuenta: false };
  }
  // El número no existe todavía: se manda en cero y la pre-cuenta no lo pinta.
  return { order: { ...base, number: 0, paid: false }, preCuenta: true };
}
