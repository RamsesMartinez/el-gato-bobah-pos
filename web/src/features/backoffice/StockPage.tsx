import { useState } from 'react';
import {
  Box, Button, Heading, Table, Center, Spinner, Badge, Tabs, Text,
} from '@chakra-ui/react';
import { useQuery } from '@tanstack/react-query';
import { CompositionSheet } from '../../shared/CompositionSheet';
import { backofficeApi } from '../../api/backoffice';
import { Page } from '../../components/Page';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';

export function StockPage() {
  const horaNegocio = useHoraDelNegocio();
  const levels = useQuery({ queryKey: ['stock', 'levels'], queryFn: backofficeApi.stockLevels });
  const moves = useQuery({ queryKey: ['stock', 'moves'], queryFn: backofficeApi.stockMovements });
  const ingredients = useQuery({ queryKey: ['ingredients', 'stock'], queryFn: () => backofficeApi.ingredients() });
  const [composing, setComposing] = useState<{ id: number; name: string } | null>(null);

  if (levels.isLoading) return <Center h="60vh"><Spinner size="xl" /></Center>;

  return (
    <Page maxW="1150px">
      <Heading size="lg" mb={4}>Almacén</Heading>
      <Tabs.Root defaultValue="existencias">
        <Tabs.List>
          <Tabs.Trigger value="existencias">Existencias</Tabs.Trigger>
          <Tabs.Trigger value="movimientos">Movimientos</Tabs.Trigger>
          <Tabs.Trigger value="insumos">Insumos</Tabs.Trigger>
        </Tabs.List>
        <Tabs.Content value="existencias" px={0}>
          <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" overflowX="auto">
            <Table.Root size="sm">
              <Table.Header><Table.Row><Table.ColumnHeader>Artículo</Table.ColumnHeader><Table.ColumnHeader>Tipo</Table.ColumnHeader><Table.ColumnHeader textAlign="end">Existencia</Table.ColumnHeader><Table.ColumnHeader textAlign="end">Mínimo</Table.ColumnHeader></Table.Row></Table.Header>
              <Table.Body>
                {(levels.data?.items ?? []).length === 0 && (
                  <Table.Row><Table.Cell colSpan={4}><Text color="fg.subtle">Sin movimientos aún</Text></Table.Cell></Table.Row>
                )}
                {(levels.data?.items ?? []).map((s, i) => {
                  const low = s.min_stock != null && Number(s.on_hand) <= Number(s.min_stock);
                  return (
                    <Table.Row key={i} bg={Number(s.on_hand) < 0 ? 'red.50' : undefined}>
                      <Table.Cell>{s.item_name}</Table.Cell>
                      <Table.Cell>{s.item_type}</Table.Cell>
                      <Table.Cell textAlign="end" color={Number(s.on_hand) < 0 ? 'red.600' : undefined}>
                        {s.on_hand} {s.unit_code}{low && <Badge ml={2} colorPalette="orange">bajo</Badge>}
                      </Table.Cell>
                      <Table.Cell textAlign="end">{s.min_stock ?? '—'}</Table.Cell>
                    </Table.Row>
                  );
                })}
              </Table.Body>
            </Table.Root>
          </Box>
        </Tabs.Content>
        <Tabs.Content value="movimientos" px={0}>
          <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" overflowX="auto">
            <Table.Root size="sm">
              <Table.Header><Table.Row><Table.ColumnHeader>Fecha</Table.ColumnHeader><Table.ColumnHeader>Artículo</Table.ColumnHeader><Table.ColumnHeader>Tipo</Table.ColumnHeader><Table.ColumnHeader textAlign="end">Cantidad</Table.ColumnHeader><Table.ColumnHeader>Motivo</Table.ColumnHeader></Table.Row></Table.Header>
              <Table.Body>
                {(moves.data?.items ?? []).map((m) => (
                  <Table.Row key={m.id}>
                    <Table.Cell>{horaNegocio.fechaYHora(m.created_at)}</Table.Cell>
                    <Table.Cell>{m.item_name}</Table.Cell>
                    <Table.Cell>{m.movement_type}</Table.Cell>
                    <Table.Cell textAlign="end" color={Number(m.quantity) < 0 ? 'red.600' : 'green.600'}>{m.quantity}</Table.Cell>
                    <Table.Cell>{m.reason ?? '—'}</Table.Cell>
                  </Table.Row>
                ))}
              </Table.Body>
            </Table.Root>
          </Box>
        </Tabs.Content>
        {/* Qué insumos se preparan en el local y qué llevan. Lo que vino estimado de FUDO se revisa
            aquí: no había otro lugar donde verlo. */}
        <Tabs.Content value="insumos" px={0}>
          <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" overflowX="auto">
            <Table.Root size="sm">
              <Table.Header><Table.Row><Table.ColumnHeader>Insumo</Table.ColumnHeader><Table.ColumnHeader textAlign="end">Existencia</Table.ColumnHeader><Table.ColumnHeader>Qué lleva</Table.ColumnHeader></Table.Row></Table.Header>
              <Table.Body>
                {(ingredients.data?.items ?? []).map((i) => (
                  <Table.Row key={i.id}>
                    <Table.Cell>{i.name}</Table.Cell>
                    <Table.Cell textAlign="end">{Number(i.onHand)} {i.baseUnitCode}</Table.Cell>
                    <Table.Cell>
                      <Button aria-label={`Qué lleva ${i.name}`} size="sm" variant="ghost" minH="44px" px={2}
                        onClick={() => setComposing({ id: i.id, name: i.name })}>
                        {i.isPrep ? 'Se prepara aquí' : 'Se compra'}
                        {i.compositionStatus === 'estimated' && <Badge ml={2} colorPalette="orange">por revisar</Badge>}
                        {' ›'}
                      </Button>
                    </Table.Cell>
                  </Table.Row>
                ))}
              </Table.Body>
            </Table.Root>
          </Box>
        </Tabs.Content>
      </Tabs.Root>
      {composing && (
        <CompositionSheet kind="ingredient" id={composing.id} name={composing.name} open
          onClose={() => setComposing(null)} />
      )}
    </Page>
  );
}
