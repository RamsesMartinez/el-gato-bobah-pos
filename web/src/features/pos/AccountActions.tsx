import { Button, HStack } from '@chakra-ui/react';
import { missingFor, nombreDeCuenta } from '../../domain/cuentas';
import type { AccountItem } from '../../types/pos';
import { money } from '../../utils/format';

// AccountActions son los botones con que se resuelve lo que le falta a una cuenta (spec 089): entregar
// y cobrar. Solo un pedido los tiene: una cuenta en captura se envía o se cobra desde su ticket.
//
// Solo hay botón cuando le falta UNA cosa. Con las dos se abre la cuenta: su ticket ofrece entregar
// arriba y cobrar abajo. «Entregar» pegado a «Cobrar» invitaba a entregar sin cobrar queriendo
// cobrar, y dos botones en 248 px dejaban el nombre en tres letras. `compact` (la columna) dice
// «Cobrar» sin la cifra, que ya va en el renglón.
export function AccountActions({ c, onDeliver, onCharge, delivering, compact, size = 'md' }: {
  c: AccountItem;
  onDeliver: (c: AccountItem) => void;
  onCharge: (c: AccountItem) => void;
  delivering?: boolean;
  compact?: boolean;
  size?: 'sm' | 'md';
}) {
  const m = missingFor(c);
  if (m.charge === m.deliver) return null;
  const nombre = nombreDeCuenta(c);
  const debe = money(Number(c.outstanding));
  return (
    <HStack gap={2} flexShrink={0}>
      {m.deliver && (
        <Button size={size} minH="44px" variant="outline" colorPalette="blue" aria-label={`Entregar ${nombre}`}
          loading={delivering} disabled={delivering}
          onClick={(e) => { e.stopPropagation(); onDeliver(c); }}>
          Entregar
        </Button>
      )}
      {m.charge && (
        <Button size={size} minH="44px" colorPalette="green" aria-label={`Cobrar ${debe} ${nombre}`}
          onClick={(e) => { e.stopPropagation(); onCharge(c); }}>
          {compact ? 'Cobrar' : `Cobrar ${debe}`}
        </Button>
      )}
    </HStack>
  );
}
