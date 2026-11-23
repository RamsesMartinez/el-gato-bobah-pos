import { Box, HStack, Spinner, Text, VStack } from '@chakra-ui/react';
import { useQuery } from '@tanstack/react-query';
import { LuTriangleAlert } from 'react-icons/lu';
import { backofficeApi } from '../../../api/backoffice';
import type { RealWaste } from '../../../api/admin';

// La merma real de los últimos 90 días por almacén (spec 096, maqueta A2): lo mermado entre lo que
// las ventas consumieron. Solo informa: no cambia el costo. El aviso va en el mismo renglón del
// almacén para no comerse el historial de compras a 1024×600.
export function RealWasteCard({ id, expected, unit }: { id: number; expected: number | null; unit: string }) {
  const { data, isLoading } = useQuery({ queryKey: ['article', id, 'real-waste'], queryFn: () => backofficeApi.articleRealWaste(id) });
  return (
    <Box bg="bg.muted" borderRadius="lg" p={3} flex="1" minW={0}>
      <Text fontSize="sm" color="fg.muted">Merma real, últimos 90 días</Text>
      {isLoading ? <Spinner size="sm" mt={2} /> : <RealWasteRows items={data?.items ?? []} expected={expected} unit={unit} />}
    </Box>
  );
}

export function RealWasteRows({ items, expected, unit }: { items: RealWaste[]; expected: number | null; unit: string }) {
  if (items.length === 0) return <Text fontSize="sm" mt={1}>Sin datos en 90 días</Text>;
  return (
    <VStack align="stretch" gap={0} mt={1}>
      {items.map((r) => {
        const over = r.state === 'ratio' && r.ratio !== null && expected !== null && r.ratio > expected;
        return (
          <HStack key={r.warehouseId} gap={2} minH="32px">
            <Box w="16px" color="fg.warning" flexShrink={0}>{over && <LuTriangleAlert aria-hidden="true" />}</Box>
            <Text flex="1" truncate>{r.warehouseName}</Text>
            {over && <Text fontSize="sm" color="fg.warning">Pasa lo esperado</Text>}
            <Text fontWeight="700" title={r.state === 'no_sales' ? `${Number(r.wasted)} ${unit} mermados` : undefined}>
              {r.state === 'ratio' ? `${r.ratio} %` : r.state === 'no_sales' ? 'Sin ventas' : 'Sin datos'}
            </Text>
          </HStack>
        );
      })}
    </VStack>
  );
}
