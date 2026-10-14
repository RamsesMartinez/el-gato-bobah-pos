import { useState } from 'react';
import { useSearchParams } from 'react-router';
import {
  Box, Button, Heading, Table, Center, Spinner, Badge, Tabs, Text,
} from '@chakra-ui/react';
import { useQuery } from '@tanstack/react-query';
import { CompositionSheet } from '../../shared/CompositionSheet';
import { backofficeApi } from '../../api/backoffice';
import { Page } from '../../components/Page';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';
import { can } from '../../app/permissions';
import { useSessionStore } from '../../stores/session';
import { InventoryHealthTab } from './InventoryHealthTab';

const TABS = ['existencias', 'movimientos', 'insumos', 'salud'] as const;
type Tab = (typeof TABS)[number];

export function StockPage() {
  const horaNegocio = useHoraDelNegocio();
  const levels = useQuery({ queryKey: ['stock', 'levels'], queryFn: backofficeApi.stockLevels });
  const moves = useQuery({ queryKey: ['stock', 'moves'], queryFn: backofficeApi.stockMovements });
  const ingredients = useQuery({ queryKey: ['ingredients', 'stock'], queryFn: () => backofficeApi.ingredients() });
  const user = useSessionStore((s) => s.user);
  const canHealth = can('inventory.health_view', user);
  // La dirección decide la pestaña y el insumo abierto: «Salud del inventario» manda aquí con
  // ?tab=insumos&open=N. Un valor desconocido cae a la primera pestaña, nunca a una que no existe.
  const [params, setParams] = useSearchParams();
  const asked = params.get('tab') as Tab | null;
  const tab: Tab = asked && TABS.includes(asked) && (asked !== 'salud' || canHealth) ? asked : 'existencias';
  const openId = Number(params.get('open'));
  const linked = (ingredients.data?.items ?? []).find((i) => i.id === openId);
  const [composing, setComposing] = useState<{ id: number; name: string } | null>(null);
  const sheet = composing ?? (tab === 'insumos' && linked ? { id: linked.id, name: linked.name } : null);
  const closeSheet = () => {
    setComposing(null);
    if (params.has('open')) { const p = new URLSearchParams(params); p.delete('open'); setParams(p, { replace: true }); }
  };

  if (levels.isLoading) return <Center h="60vh"><Spinner size="xl" /></Center>;

  return (
    <Page fill>
      <Heading size="lg" mb={4}>Almacén</Heading>
      <Tabs.Root value={tab} onValueChange={(e) => setParams({ tab: e.value }, { replace: true })}
        display="flex" flexDirection="column" flex="1" minH={0}>
        <Tabs.List>
          <Tabs.Trigger value="existencias" minH="44px">Existencias</Tabs.Trigger>
          <Tabs.Trigger value="movimientos" minH="44px">Movimientos</Tabs.Trigger>
          <Tabs.Trigger value="insumos" minH="44px">Insumos</Tabs.Trigger>
          {canHealth && <Tabs.Trigger value="salud" minH="44px">Salud</Tabs.Trigger>}
        </Tabs.List>
        <Tabs.Content value="existencias" px={0} flex="1" minH={0} overflowY="auto">
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
        <Tabs.Content value="movimientos" px={0} flex="1" minH={0} overflowY="auto">
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
        <Tabs.Content value="insumos" px={0} flex="1" minH={0} overflowY="auto">
          <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" overflowX="auto">
            <Table.Root size="sm">
              <Table.Header><Table.Row><Table.ColumnHeader>Insumo</Table.ColumnHeader><Table.ColumnHeader textAlign="end">Existencia</Table.ColumnHeader><Table.ColumnHeader>Receta</Table.ColumnHeader></Table.Row></Table.Header>
              <Table.Body>
                {(ingredients.data?.items ?? []).map((i) => (
                  <Table.Row key={i.id}>
                    <Table.Cell>{i.name}</Table.Cell>
                    <Table.Cell textAlign="end">{Number(i.onHand)} {i.baseUnitCode}</Table.Cell>
                    <Table.Cell>
                      <Button aria-label={`Receta de ${i.name}`} size="sm" variant="ghost" minH="44px" px={2}
                        onClick={() => setComposing({ id: i.id, name: i.name })}>
                        {i.isPrep ? 'Es un preparado · ver receta' : 'Se compra hecho'}
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
        {canHealth && (
          <Tabs.Content value="salud" px={0} flex="1" minH={0} display="flex" flexDirection="column">
            {tab === 'salud' && <InventoryHealthTab />}
          </Tabs.Content>
        )}
      </Tabs.Root>
      {sheet && (
        <CompositionSheet kind="ingredient" id={sheet.id} name={sheet.name} open
          onClose={closeSheet} />
      )}
    </Page>
  );
}
