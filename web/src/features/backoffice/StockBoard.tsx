import { Badge, Box, Button, HStack, SimpleGrid, Text, VStack } from '@chakra-ui/react';
import type { StockLevel } from '../../api/backoffice';
import { formatBaseQty } from '../../shared/units';

// La vista sencilla de Existencias (spec 096, maqueta B «Tablero por estado»): qué se acabó, qué
// queda poco y qué hay, sin valor ni costos. Lo que no tiene mínimo nunca está «poco»: el sistema no
// sabe cuánto es poco para ese artículo, y adivinarlo llenaría la columna de falsas alarmas.
type StockState = 'agotado' | 'poco' | 'hay';

function stockState(s: StockLevel): StockState {
  const onHand = Number(s.on_hand);
  if (onHand <= 0) return 'agotado';
  if (s.min_stock != null && onHand <= Number(s.min_stock)) return 'poco';
  return 'hay';
}

const COLUMNS: { state: StockState; title: string; palette: string }[] = [
  { state: 'agotado', title: 'Se acabó', palette: 'red' },
  { state: 'poco', title: 'Queda poco', palette: 'orange' },
  { state: 'hay', title: 'Hay', palette: 'green' },
];

export function StockBoard({ items, bringFrom, onBring }: {
  items: StockLevel[];
  // bringFrom dice de dónde se puede traer un negativo (solo en el de venta y con permiso de traspasar).
  bringFrom?: (s: StockLevel) => string | null;
  onBring?: (s: StockLevel) => void;
}) {
  return (
    <SimpleGrid columns={3} gap={3} flex="1" minH={0} data-testid="tablero-existencias">
      {COLUMNS.map(({ state, title, palette }) => {
        const of = items.filter((s) => stockState(s) === state);
        return (
          <VStack key={state} align="stretch" gap={0} minH={0} borderWidth="1px" borderRadius="lg" bg="bg.panel"
            role="region" aria-label={title}>
            <HStack px={3} py={2} borderBottomWidth="1px">
              <Text fontWeight="700" flex="1">{title}</Text>
              <Badge colorPalette={palette}>{of.length}</Badge>
            </HStack>
            <Box flex="1" minH={0} maxH="calc(100dvh - 260px)" overflowY="auto">
              {of.length === 0 && <Text px={3} py={2} color="fg.subtle">Nada</Text>}
              {of.map((s, i) => {
                const from = state === 'agotado' && Number(s.on_hand) < 0 ? bringFrom?.(s) : null;
                return (
                  <HStack key={i} px={3} py={2} minH="44px" borderBottomWidth="1px" gap={2} flexWrap="wrap" data-testid="ficha-existencia">
                    <Text flex="1" minW={0} truncate>{s.item_name}</Text>
                    <Text flexShrink={0} fontWeight="600" color={Number(s.on_hand) < 0 ? 'fg.error' : undefined}>
                      {formatBaseQty(s.on_hand, s.unit_code)}
                    </Text>
                    {state === 'poco' && <Text flexBasis="100%" fontSize="xs" color="fg.muted">mínimo {formatBaseQty(s.min_stock!, s.unit_code)}</Text>}
                    {from && onBring && (
                      <Button size="sm" minH="44px" variant="outline" onClick={() => onBring(s)}>Traer de {from}</Button>
                    )}
                  </HStack>
                );
              })}
            </Box>
          </VStack>
        );
      })}
    </SimpleGrid>
  );
}
