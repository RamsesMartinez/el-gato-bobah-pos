import type { ReceiptLine } from '../types/pos';

// printableLines decide qué renglones de un pedido van al papel: ticket, pre-cuenta, reimpresión
// y comanda pasan por aquí, y en ningún otro lado se filtra.
//
// El servidor manda los cancelados en el pedido (la pantalla los tacha), pero en el papel un
// renglón cancelado se lee como cobrado o como algo por preparar. Pasó el 2026-11-02: unos boneless
// cancelados y otros agregados salieron los dos en el ticket, con el total contando uno.
export function printableLines<T extends ReceiptLine>(lines: T[] | undefined | null): T[] {
  return (lines ?? []).filter((l) => !l.cancelled);
}
