import { useState } from 'react';
import { useSearchParams } from 'react-router';
import {
  Box, Button, Heading, HStack, Table, Center, Spinner, Badge, Tabs, Text,
} from '@chakra-ui/react';
import { useQuery } from '@tanstack/react-query';
import { CompositionSheet } from '../../shared/CompositionSheet';
import { backofficeApi } from '../../api/backoffice';
import { Page } from '../../components/Page';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';
import { can } from '../../app/permissions';
import { useSessionStore } from '../../stores/session';
import { InventoryHealthTab } from './InventoryHealthTab';
import { PresentationsSheet, type PresentationsIngredient } from './PresentationsSheet';
import { formatBaseQty } from '../../shared/units';
import { PendingReceiptsTab } from './PendingReceiptsTab';
import { StockCountTab } from './StockCountTab';
import { WasteTab } from './WasteTab';
import { posApi } from '../../api/pos';

// El orden lo fijó el plan del 038: Existencias · Movimientos · Insumos · Por recibir · Conteo · Salud.
const TABS = ['existencias', 'movimientos', 'insumos', 'por-recibir', 'conteo', 'merma', 'salud'] as const;
type Tab = (typeof TABS)[number];

export function StockPage() {
  const horaNegocio = useHoraDelNegocio();
  const levels = useQuery({ queryKey: ['stock', 'levels'], queryFn: backofficeApi.stockLevels });
  const moves = useQuery({ queryKey: ['stock', 'moves'], queryFn: backofficeApi.stockMovements });
  const ingredients = useQuery({ queryKey: ['ingredients', 'stock'], queryFn: () => backofficeApi.ingredients() });
  const user = useSessionStore((s) => s.user);
  const canHealth = can('inventory.health_view', user);
  // Cómo llega cada insumo (spec 038): lo ve quien registra compras y lo cambia quien configura.
  const canReceive = can('inventory.receive', user);
  const canConfigure = can('inventory.configure', user);
  const canCount = can('inventory.count', user);
  const presentations = useQuery({ queryKey: ['presentations'], queryFn: backofficeApi.presentations, enabled: canReceive });
  const units = useQuery({ queryKey: ['units'], queryFn: backofficeApi.units, enabled: canReceive });
  const [packaging, setPackaging] = useState<PresentationsIngredient | null>(null);
  const presOf = (id: number) => (presentations.data ?? []).filter((p) => p.ingredientId === id);
  const usedNames = [...new Set((presentations.data ?? []).map((p) => p.name))];
  // La dirección decide la pestaña y el insumo abierto: «Salud del inventario» manda aquí con
  // ?tab=insumos&open=N. Un valor desconocido cae a la primera pestaña, nunca a una que no existe.
  const [params, setParams] = useSearchParams();
  const asked = params.get('tab') as Tab | null;
  const allowed = (t: Tab) => ((t !== 'salud' && t !== 'merma') || canHealth) && (t !== 'por-recibir' || canReceive) && (t !== 'conteo' || canCount);
  const tab: Tab = asked && TABS.includes(asked) && allowed(asked) ? asked : 'existencias';
  const settings = useQuery({ queryKey: ['business-settings'], queryFn: posApi.businessSettings });
  const pending = useQuery({ queryKey: ['stock', 'pending-receipts'], queryFn: backofficeApi.pendingReceipts, enabled: canReceive });
  // La palabra de la empresa para lo que se guarda (spec 038), en plural y con mayúscula.
  const word = settings.data?.stockItemLabel || 'insumo';
  const plural = /[aeiouáéíóú]$/i.test(word) ? `${word}s` : `${word}es`;
  const itemsTab = plural.charAt(0).toUpperCase() + plural.slice(1);
  const itemCol = word.charAt(0).toUpperCase() + word.slice(1);
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
        {/* Con «Se echó a perder» son siete: la fila se desplaza a lo ancho antes que salirse de 1024 px. */}
        <Tabs.List overflowX="auto" flexShrink={0}>
          <Tabs.Trigger value="existencias" minH="44px">Existencias</Tabs.Trigger>
          <Tabs.Trigger value="movimientos" minH="44px">Movimientos</Tabs.Trigger>
          <Tabs.Trigger value="insumos" minH="44px">{itemsTab}</Tabs.Trigger>
          {canReceive && (
            <Tabs.Trigger value="por-recibir" minH="44px">
              Por recibir{pending.data && pending.data.length > 0 ? ` · ${pending.data.length}` : ''}
            </Tabs.Trigger>
          )}
          {canCount && <Tabs.Trigger value="conteo" minH="44px">Conteo</Tabs.Trigger>}
          {canHealth && <Tabs.Trigger value="merma" minH="44px" flexShrink={0}>Se echó a perder</Tabs.Trigger>}
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
              <Table.Header><Table.Row><Table.ColumnHeader>{itemCol}</Table.ColumnHeader><Table.ColumnHeader textAlign="end">Existencia</Table.ColumnHeader>{canReceive && <Table.ColumnHeader>Cómo llega</Table.ColumnHeader>}<Table.ColumnHeader>Receta</Table.ColumnHeader></Table.Row></Table.Header>
              <Table.Body>
                {(ingredients.data?.items ?? []).map((i) => (
                  <Table.Row key={i.id}>
                    <Table.Cell>{i.name}</Table.Cell>
                    <Table.Cell textAlign="end">{formatBaseQty(i.onHand, i.baseUnitCode)}</Table.Cell>
                    {canReceive && (() => {
                      const all = presOf(i.id);
                      const on = all.filter((p) => p.active);
                      const off = all.filter((p) => !p.active);
                      const label = (p: (typeof all)[number]) => `${p.name} ${p.approximate ? '≈' : '·'} ${formatBaseQty(p.qtyInBase, p.baseUnitCode)}`;
                      return (
                        <Table.Cell>
                          <HStack gap={2} justify="space-between">
                            <Box>
                              <Text fontSize="sm">{on.length ? on.slice(0, 2).map(label).join(' · ') + (on.length > 2 ? ` · +${on.length - 2}` : '') : ''}</Text>
                              {on.length === 0 && <Text fontSize="sm" color="fg.muted">Todavía no dices cómo llega</Text>}
                              {off.length > 0 && <Text fontSize="xs" color="fg.muted">Ya no llega en: {off.map((p) => p.name).join(', ')}</Text>}
                              {/* El lugar a la vista: quien no lo conocía no lo encontró en otra parte (040, T022). */}
                              <Text fontSize="xs" color="fg.muted">{i.countPlace ? `Lugar: ${i.countPlace}` : 'Sin lugar'}</Text>
                            </Box>
                            {canConfigure && (
                              <Button aria-label={`Cómo llega ${i.name}`} size="sm" variant="outline" minH="44px" flexShrink={0}
                                onClick={() => setPackaging({ id: i.id, name: i.name, baseUnitCode: i.baseUnitCode, baseUnitKind: i.baseUnitKind, countPlaceId: i.countPlaceId ?? null })}>
                                {on.length ? 'Cambiar o poner lugar ›' : '+ Empaque o lugar'}
                              </Button>
                            )}
                          </HStack>
                        </Table.Cell>
                      );
                    })()}
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
        {canReceive && (
          <Tabs.Content value="por-recibir" px={0} flex="1" minH={0} overflowY="auto">
            {tab === 'por-recibir' && <PendingReceiptsTab />}
          </Tabs.Content>
        )}
        {canCount && (
          <Tabs.Content value="conteo" px={0} flex="1" minH={0} display="flex" flexDirection="column">
            {tab === 'conteo' && <StockCountTab />}
          </Tabs.Content>
        )}
        {canHealth && (
          <Tabs.Content value="merma" px={0} flex="1" minH={0} display="flex" flexDirection="column">
            {tab === 'merma' && <WasteTab />}
          </Tabs.Content>
        )}
        {canHealth && (
          <Tabs.Content value="salud" px={0} flex="1" minH={0} display="flex" flexDirection="column">
            {tab === 'salud' && <InventoryHealthTab />}
          </Tabs.Content>
        )}
      </Tabs.Root>
      {packaging && (
        <PresentationsSheet isOpen ingredient={packaging} presentations={presOf(packaging.id)} usedNames={usedNames}
          units={units.data?.items ?? []} onClose={() => setPackaging(null)} />
      )}
      {sheet && (
        <CompositionSheet kind="ingredient" id={sheet.id} name={sheet.name} open
          onClose={closeSheet} />
      )}
    </Page>
  );
}
