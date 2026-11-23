import type { AccountItem } from '../types/pos';
import { money } from '../utils/format';

// Cómo se nombra una cuenta viva y lo que le falta. El estado lo decide el servidor
// (domain.AccountState); aquí solo se nombra. Vive en `domain` porque lo leen Vender y el cierre de
// caja, y una pantalla no importa de otra: dos copias ya se estaban desalineando.
export function nombreDeCuenta(c: Pick<AccountItem, 'folioName' | 'customerName'>): string {
  return c.folioName || c.customerName || 'Cuenta nueva';
}

// Missing es lo que le falta a una cuenta para quitarse de la fila, dicho como lo diría alguien del
// mostrador (spec 089). `short` es para la fila angosta, donde la frase larga se corta.
export interface Missing {
  text: string;
  short: string;
  color: 'red' | 'blue' | 'gray';
  charge: boolean;
  deliver: boolean;
}

// missingFor traduce el estado que decide el servidor (domain.AccountState); no lo recalcula. El
// monto es `outstanding` del servidor, que ya descuenta lo pagado y lo condonado. Una cuenta en
// captura no ofrece acciones: todavía no es pedido, se envía o se cobra desde el ticket.
export function missingFor(c: Pick<AccountItem, 'state' | 'kind' | 'total' | 'outstanding'>): Missing {
  const debe = money(Number(c.outstanding));
  const nada = { charge: false, deliver: false };
  if (c.kind !== 'order') {
    const t = c.state === 'closed_with_new' ? 'Nuevo sin enviar' : `Sin enviar · ${money(Number(c.total))}`;
    return { text: t, short: t, color: 'gray', ...nada };
  }
  switch (c.state) {
    case 'capturing': {
      const t = `Sin enviar · ${money(Number(c.total))}`;
      return { text: t, short: t, color: 'gray', ...nada };
    }
    case 'closed_with_new':
      return { text: 'Nuevo sin enviar', short: 'Nuevo sin enviar', color: 'gray', ...nada };
    case 'in_kitchen':
    case 'partly_paid':
      return { text: `Falta cobrar ${debe} y entregar`, short: `Cobrar ${debe} · entregar`, color: 'red', charge: true, deliver: true };
    case 'paid_in_kitchen':
      return { text: 'Falta entregar', short: 'Falta entregar', color: 'blue', charge: false, deliver: true };
    case 'delivered_owes':
      return { text: `Falta cobrar ${debe}`, short: `Cobrar ${debe}`, color: 'red', charge: true, deliver: false };
  }
}
