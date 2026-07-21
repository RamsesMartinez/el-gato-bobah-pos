import {
  Box, Heading, Table, Center, Spinner, Badge, Tabs, Text,
} from '@chakra-ui/react';
import { useQuery } from '@tanstack/react-query';
import { backofficeApi } from '../../api/backoffice';
import { Page } from '../../components/Page';

export function StockPage() {
  const levels = useQuery({ queryKey: ['stock', 'levels'], queryFn: backofficeApi.stockLevels });
  const moves = useQuery({ queryKey: ['stock', 'moves'], queryFn: backofficeApi.stockMovements });

  if (levels.isLoading) return <Center h="60vh"><Spinner size="xl" /></Center>;

  return (
    <Page maxW="1150px">
      <Heading size="lg" mb={4}>Almacén</Heading>
      <Tabs.Root defaultValue="existencias">
        <Tabs.List>
          <Tabs.Trigger value="existencias">Existencias</Tabs.Trigger>
          <Tabs.Trigger value="movimientos">Movimientos</Tabs.Trigger>
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
                    <Table.Cell>{new Date(m.created_at).toLocaleString('es-MX')}</Table.Cell>
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
      </Tabs.Root>
    </Page>
  );
}
