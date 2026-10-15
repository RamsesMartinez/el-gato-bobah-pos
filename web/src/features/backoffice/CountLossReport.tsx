import { Box, Flex, Grid, Spinner, Text, VStack } from '@chakra-ui/react';
import { useQuery } from '@tanstack/react-query';
import { wasteApi, type LossItem } from '../../api/waste';

// Lo que se perdió desde el conteo anterior (spec 039, maqueta B): tres cifras arriba y una barra por
// artículo, en gris lo registrado como merma y en rojo lo que el conteo encontró de menos. Los pesos
// son al costo de cuando se cerró el conteo. Lo que sobraba se dice así, sin barra.

const money = (v: string) => `$${Number(v).toLocaleString('es-MX', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
const qty = (v: string) => String(Number(Number(v).toFixed(2)));

export function CountLossReport({ countId }: { countId: number }) {
  const { data, isLoading } = useQuery({ queryKey: ['merma', 'faltante', countId], queryFn: () => wasteApi.countLoss(countId) });
  if (isLoading || !data) return <Flex justify="center" py={6}><Spinner /></Flex>;
  if (data.firstCount) {
    return <Text color="fg.muted">Es el primer conteo: todavía no hay otro con qué comparar lo que se perdió.</Text>;
  }
  const items = data.items.filter((i) => i.hasPrevious);
  const max = Math.max(1, ...items.map((i) => Number(i.lostCost ?? 0)));
  return (
    <VStack align="stretch" gap={3}>
      <Grid templateColumns="repeat(3, minmax(0, 1fr))" gap={3}>
        <Figure label="Se perdió" value={money(data.totals.lost)} />
        <Figure label="Se registró" value={money(data.totals.registered)} />
        <Figure label="Sin explicar" value={money(data.totals.unexplained)} strong />
      </Grid>
      {data.totals.withoutCost > 0 && (
        <Text fontSize="sm" color="fg.muted">
          {data.totals.withoutCost} {data.totals.withoutCost === 1 ? 'artículo sin costo' : 'artículos sin costo'}: no suma a los pesos.
        </Text>
      )}
      <Box>
        {items.map((i) => <LossRow key={i.ingredientId} item={i} max={max} />)}
      </Box>
    </VStack>
  );
}

function Figure({ label, value, strong }: { label: string; value: string; strong?: boolean }) {
  return (
    <Box borderWidth={strong ? '2px' : '1px'} borderColor={strong ? 'red.500' : 'border'} borderRadius="lg" px={3} py={2}>
      <Text fontSize="sm" color="fg.muted">{label}</Text>
      <Text fontSize="xl" fontWeight="700" color={strong ? 'red.600' : undefined} fontVariantNumeric="tabular-nums">{value}</Text>
    </Box>
  );
}

function LossRow({ item, max }: { item: LossItem; max: number }) {
  if (Number(item.surplus) > 0) {
    return (
      <Flex data-testid={`faltante-${item.ingredientId}`} minH="48px" align="center" justify="space-between" borderBottomWidth="1px">
        <Text>{item.name}</Text>
        <Text color="green.700">Sobraba {qty(item.surplus)} {item.unitCode}</Text>
      </Flex>
    );
  }
  const cost = item.unitCost ? Number(item.unitCost) : null;
  const regW = cost !== null ? (Number(item.registered) * cost / max) * 100 : 0;
  const unW = cost !== null ? (Number(item.unexplained) * cost / max) * 100 : 0;
  return (
    <Flex data-testid={`faltante-${item.ingredientId}`} minH="48px" align="center" gap={3} borderBottomWidth="1px">
      <Text w="190px" flexShrink={0} lineClamp={2}>{item.name}</Text>
      <Flex flex="1" h="14px" borderRadius="sm" overflow="hidden" bg="bg.muted">
        {Number(item.registered) > 0 && <Box data-part="registrado" w={`${regW}%`} bg="gray.400" />}
        {Number(item.unexplained) > 0 && <Box data-part="sin-explicar" w={`${unW}%`} bg="red.500" />}
      </Flex>
      <Text w="240px" flexShrink={0} textAlign="right" fontSize="sm" fontVariantNumeric="tabular-nums">
        {qty(item.registered)} {item.unitCode} registrado · {qty(item.unexplained)} {item.unitCode} sin explicar
        {' · '}<b>{item.unexplainedCost ? money(item.unexplainedCost) : 'sin costo'}</b>
      </Text>
    </Flex>
  );
}
