import { Button, HStack } from '@chakra-ui/react';
import { missingFor, nombreDeCuenta } from '../../domain/cuentas';
import type { AccountItem } from '../../types/pos';
import { money } from '../../utils/format';

// AccountActions son los botones con que se resuelve lo que le falta a una cuenta (spec 089): entregar
// y cobrar. Solo un pedido los tiene: una cuenta en captura se envía o se cobra desde su ticket.
//
// `onlyWhenOne`: en la columna, el renglón trae botón solo si le falta UNA cosa; con las dos, se toca
// la cuenta y su ticket ofrece las dos. Dos botones en 248 px dejaban el nombre en tres letras.
export function AccountActions({ c, onDeliver, onCharge, delivering, onlyWhenOne, size = 'md' }: {
  c: AccountItem;
  onDeliver: (c: AccountItem) => void;
  onCharge: (c: AccountItem) => void;
  delivering?: boolean;
  onlyWhenOne?: boolean;
  size?: 'sm' | 'md';
}) {
  const m = missingFor(c);
  if (onlyWhenOne && m.charge === m.deliver) return null;
  if (!m.charge && !m.deliver) return null;
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
          {onlyWhenOne ? 'Cobrar' : `Cobrar ${debe}`}
        </Button>
      )}
    </HStack>
  );
}
