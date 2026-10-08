import { useMemo, useState } from 'react';
import { Box, Button, HStack, Input, Text, VStack } from '@chakra-ui/react';
import { LuArrowLeft, LuPlus } from 'react-icons/lu';
import { useQuery } from '@tanstack/react-query';
import { posApi } from '../../api/pos';
import { ListRow } from '../../components/ListRow';
import type { BoardOrder, Currency } from '../../types/pos';
import { money } from '../../utils/format';

// NEW_ORDER es el destino «pedido nuevo» en el estado de la vista.
export const NEW_ORDER = 'new' as const;
export type MoveTarget = number | typeof NEW_ORDER;

interface Props {
  fromOrderId: number;
  // Cuántos productos y cuáles, para el encabezado.
  count: number;
  names: string[];
  currency: Currency;
  // Con todo seleccionado, «Pedido nuevo» no hace nada que el pedido no sea ya.
  everything: boolean;
  moving: boolean;
  onBack: () => void;
  onConfirm: (target: MoveTarget, label: string) => void;
}

const SERVICE: Record<string, string> = { mostrador: 'Mostrador', domicilio: 'Domicilio', llevar: 'Para llevar' };

// MoveToOrder es «Pasar a otro pedido»: la lista de pedidos abiertos con «+ Pedido nuevo» arriba.
// Tocar un destino lo marca y el pie pide confirmar con su nombre: un toque solo no pasa nada, porque
// la lista se recorre con prisa y un dedo de más mandaría los productos a la mesa equivocada.
//
// Es una vista de la misma hoja y no otra hoja encima, y el buscador no se enfoca solo: abriría el
// teclado del sistema sobre la lista.
export function MoveToOrder({ fromOrderId, count, names, currency, everything, moving, onBack, onConfirm }: Props) {
  const [query, setQuery] = useState('');
  const [target, setTarget] = useState<{ id: MoveTarget; label: string } | null>(null);
  const { data } = useQuery({ queryKey: ['orders', 'active'], queryFn: posApi.activeOrders });
  const candidates = useMemo(() => {
    const q = query.trim().toLowerCase();
    return (data?.items ?? [])
      .filter((o: BoardOrder) => o.id !== fromOrderId && o.deliveryPlatformId === null
        && (o.status === 'abierta' || o.status === 'lista'))
      .filter((o) => q === '' || o.folioName.toLowerCase().includes(q) || String(o.number) === q.replace('#', ''));
  }, [data, fromOrderId, query]);
  const label = (o: BoardOrder) => o.folioName || `#${o.number}`;

  return (
    <VStack align="stretch" gap={2}>
      <HStack gap={2}>
        <Button aria-label="Volver" minH="44px" minW="44px" variant="ghost" colorPalette="gray" onClick={onBack}>
          <LuArrowLeft />
        </Button>
        <VStack align="start" gap={0} minW={0}>
          <Text fontWeight="800" fontSize="lg">Pasar {count} producto{count === 1 ? '' : 's'} a…</Text>
          <Text fontSize="sm" color="fg.muted" lineClamp={1}>{names.join(' · ')}</Text>
        </VStack>
      </HStack>
      <Text fontSize="sm" color="fg.muted">Lo que ya salió a cocina no se vuelve a preparar.</Text>
      <Input minH="44px" placeholder="Buscar pedido por nombre o número" aria-label="Buscar pedido"
        value={query} onChange={(e) => setQuery(e.target.value)} />
      <VStack align="stretch" gap={1} maxH="38dvh" overflowY="auto">
        <Box>
          <Button w="100%" minH="56px" justifyContent="flex-start" variant={target?.id === NEW_ORDER ? 'subtle' : 'ghost'}
            colorPalette="gray" disabled={everything} aria-pressed={target?.id === NEW_ORDER}
            onClick={() => setTarget({ id: NEW_ORDER, label: 'un pedido nuevo' })}>
            <LuPlus /> Pedido nuevo
          </Button>
          {everything && (
            <Text fontSize="xs" color="fg.muted" px={3}>Ya es su propio pedido; no hace falta pasarlo</Text>
          )}
        </Box>
        {candidates.map((o) => (
          <ListRow key={o.id} minH={56} selected={target?.id === o.id}
            label={label(o)}
            hint={`#${o.number} · ${SERVICE[o.serviceType] ?? o.serviceType} · ${o.renglones} producto${o.renglones === 1 ? '' : 's'} · ${money(o.total, currency)}`}
            onClick={() => setTarget({ id: o.id, label: label(o) })} />
        ))}
        {candidates.length === 0 && (
          <Text fontSize="sm" color="fg.muted" px={3}>No hay otros pedidos abiertos.</Text>
        )}
      </VStack>
      <Button minH="56px" colorPalette="brand" disabled={target === null} loading={moving}
        onClick={() => target && onConfirm(target.id, target.label)}>
        {target ? `Pasar a ${target.label}` : 'Elige a qué pedido'}
      </Button>
    </VStack>
  );
}
