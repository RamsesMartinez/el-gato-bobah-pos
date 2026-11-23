import { useState } from 'react';
import { Box, Button, Flex, Spinner, Table, Text, VStack } from '@chakra-ui/react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { wasteApi, type CountVsSalesItem } from '../../api/waste';
import { backofficeApi } from '../../api/backoffice';
import { can } from '../../app/permissions';
import { useSessionStore } from '../../stores/session';
import { formatBaseQty } from '../../shared/units';

// Conteo contra ventas de un conteo cerrado (spec 096, maqueta C): agrupado por qué hacer. Revisar
// la receta o la captura cuando la diferencia es grande o errática; usar el % sugerido cuando es una
// merma estable; lo demás es una diferencia chica.
const GRUPOS: { id: CountVsSalesItem['group']; titulo: string }[] = [
  { id: 'review', titulo: 'Revisar receta o captura' },
  { id: 'suggest', titulo: 'Merma estable: usar el % sugerido' },
  { id: 'small', titulo: 'Diferencia chica' },
];

export function CountVsSalesReport({ countId }: { countId: number }) {
  const user = useSessionStore((s) => s.user);
  const qc = useQueryClient();
  const { data, isLoading } = useQuery({ queryKey: ['conteo', 'contra-ventas', countId], queryFn: () => wasteApi.countVsSales(countId) });
  const [hechos, setHechos] = useState<Record<number, string>>({});
  const usar = useMutation({
    mutationFn: (it: CountVsSalesItem) => backofficeApi.setArticleExpectedWaste(it.ingredientId, Number(it.suggestedPct)),
    onSuccess: (_r, it) => {
      setHechos((h) => ({ ...h, [it.ingredientId]: it.suggestedPct ?? '' }));
      void qc.invalidateQueries({ queryKey: ['article', it.ingredientId] });
    },
  });
  if (isLoading || !data) return <Flex justify="center" py={6}><Spinner /></Flex>;
  if (data.firstCount) {
    return <Text color="fg.muted">Es el primer conteo de este almacén: todavía no hay otro con qué comparar lo vendido.</Text>;
  }
  const items = (data.items ?? []).filter((i) => i.hasPrevious);
  const canSet = can('menu.configure', user);
  return (
    <VStack align="stretch" gap={4}>
      {GRUPOS.map((g) => {
        const del = items.filter((i) => i.group === g.id);
        if (del.length === 0) return null;
        return (
          <Box key={g.id}>
            <Text fontWeight="700" mb={1}>{g.titulo} · {del.length}</Text>
            <Box borderWidth="1px" borderRadius="lg" overflowX="auto">
              <Table.Root size="sm">
                <Table.Body>
                  {del.map((i) => (
                    <Table.Row key={i.ingredientId} aria-label={i.name}>
                      <Table.Cell><Text truncate maxW="220px">{i.name}</Text></Table.Cell>
                      <Table.Cell color="fg.muted" fontSize="sm" whiteSpace="nowrap">
                        vendido {formatBaseQty(i.sold, i.unitCode)} · contado {formatBaseQty(i.counted, i.unitCode)}
                      </Table.Cell>
                      <Table.Cell textAlign="end" fontWeight="700" whiteSpace="nowrap">{i.pct === null ? 'Sin ventas' : `${Number(i.pct)} %`}</Table.Cell>
                      <Table.Cell textAlign="end">
                        {i.group === 'suggest' && i.suggestedPct !== null && (
                          hechos[i.ingredientId]
                            ? <Text fontSize="sm" color="fg.success">Merma de {Number(hechos[i.ingredientId])} % capturada</Text>
                            : canSet
                              ? <Button minH="44px" size="sm" variant="outline" loading={usar.isPending && usar.variables?.ingredientId === i.ingredientId}
                                onClick={() => usar.mutate(i)}>Usar {Number(i.suggestedPct)} %</Button>
                              : <Text fontSize="sm" color="fg.muted">Sugerido: {Number(i.suggestedPct)} %</Text>
                        )}
                      </Table.Cell>
                    </Table.Row>
                  ))}
                </Table.Body>
              </Table.Root>
            </Box>
          </Box>
        );
      })}
      <Text fontSize="sm" color="fg.muted">
        Diferencia entre lo contado y lo que el sistema tenía, sobre lo vendido desde el conteo anterior de este almacén.
      </Text>
    </VStack>
  );
}
