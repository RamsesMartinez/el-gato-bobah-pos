import { useState } from 'react';
import { useSearchParams } from 'react-router';
import { formatBaseQty } from '../../shared/units';
import {
  Box, Heading, Table, Center, Spinner, Badge, Tabs, Text,
} from '@chakra-ui/react';
import { useQuery } from '@tanstack/react-query';
import { backofficeApi } from '../../api/backoffice';
import { Page } from '../../components/Page';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';
import { can } from '../../app/permissions';
import { useSessionStore } from '../../stores/session';
import { InventoryHealthTab } from './InventoryHealthTab';
import { PendingReceiptsTab } from './PendingReceiptsTab';
import { StockCountTab } from './StockCountTab';
import { WasteTab } from './WasteTab';
import { ArticlesTab } from './articles/ArticlesTab';
import { ArticleSheet } from './articles/ArticleSheet';
import { posApi } from '../../api/pos';

// El orden lo fijó el plan del 038: Existencias · Movimientos · Insumos · Por recibir · Conteo · Salud.
const TABS = ['existencias', 'movimientos', 'insumos', 'por-recibir', 'conteo', 'merma', 'salud'] as const;
type Tab = (typeof TABS)[number];

export function StockPage() {
  const horaNegocio = useHoraDelNegocio();
  const levels = useQuery({ queryKey: ['stock', 'levels'], queryFn: backofficeApi.stockLevels });
  const moves = useQuery({ queryKey: ['stock', 'moves'], queryFn: backofficeApi.stockMovements });
  const user = useSessionStore((s) => s.user);
  const canHealth = can('inventory.health_view', user);
  // Cómo llega cada insumo (spec 038): lo ve quien registra compras y lo cambia quien configura.
  const canReceive = can('inventory.receive', user);
  const canCount = can('inventory.count', user);
  // La dirección decide la pestaña y el insumo abierto: «Salud del inventario» manda aquí con
  // ?tab=insumos&open=N. Un valor desconocido cae a la primera pestaña, nunca a una que no existe.
  const [params, setParams] = useSearchParams();
  const asked = params.get('tab') as Tab | null;
  const canView = can('inventory.view', user);
  const allowed = (t: Tab) => (t !== 'insumos' || canView) && ((t !== 'salud' && t !== 'merma') || canHealth) && (t !== 'por-recibir' || canReceive) && (t !== 'conteo' || canCount);
  const tab: Tab = asked && TABS.includes(asked) && allowed(asked) ? asked : 'existencias';
  const settings = useQuery({ queryKey: ['business-settings'], queryFn: posApi.businessSettings });
  const pending = useQuery({ queryKey: ['stock', 'pending-receipts'], queryFn: backofficeApi.pendingReceipts, enabled: canReceive });
  // La palabra de la empresa para lo que se guarda (spec 038), en plural y con mayúscula.
  const word = settings.data?.stockItemLabel || 'insumo';
  const plural = /[aeiouáéíóú]$/i.test(word) ? `${word}s` : `${word}es`;
  const itemsTab = plural.charAt(0).toUpperCase() + plural.slice(1);
  // La dirección abre la ficha que pide Salud (?tab=insumos&open=N).
  const openId = Number(params.get('open'));
  const [opened, setOpened] = useState<{ id: number; justCreated: boolean } | null>(null);
  const article = opened ?? (tab === 'insumos' && openId > 0 ? { id: openId, justCreated: false } : null);
  const openArticle = (id: number, justCreated = false) => setOpened({ id, justCreated });
  const closeArticle = () => {
    setOpened(null);
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
          {canView && <Tabs.Trigger value="insumos" minH="44px">{itemsTab}</Tabs.Trigger>}
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
                    <Table.Row key={i}>
                      <Table.Cell>{s.item_name}</Table.Cell>
                      <Table.Cell>{s.item_type}</Table.Cell>
                      <Table.Cell textAlign="end" color={Number(s.on_hand) < 0 ? 'fg.error' : undefined}>
                        {formatBaseQty(s.on_hand, s.unit_code)}{low && <Badge ml={2} colorPalette="orange">bajo</Badge>}
                        {/* Igual que en Artículos: la cifra en rojo y la etiqueta, no el renglón entero. */}
                        {Number(s.on_hand) < 0 && (
                          <Text as="span" ml={2} px={2} py="1px" borderRadius="md" fontWeight="700" color="fg.error" bg="bg.error">Revisar</Text>
                        )}
                      </Table.Cell>
                      <Table.Cell textAlign="end">{s.min_stock == null ? '—' : formatBaseQty(s.min_stock, s.unit_code)}</Table.Cell>
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
                    <Table.Cell textAlign="end" color={Number(m.quantity) < 0 ? 'fg.error' : 'fg.success'}>{m.quantity}</Table.Cell>
                    <Table.Cell>{m.reason ?? '—'}</Table.Cell>
                  </Table.Row>
                ))}
              </Table.Body>
            </Table.Root>
          </Box>
        </Tabs.Content>
        {/* Artículos (spec 042): buscador, filtros, «+ Nuevo» y la ficha de cada uno. */}
        <Tabs.Content value="insumos" px={0} flex="1" minH={0} display="flex" flexDirection="column">
          {tab === 'insumos' && <ArticlesTab word={word} onOpen={(id) => openArticle(id)} />}
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
      {article && <ArticleSheet id={article.id} justCreated={article.justCreated} onClose={closeArticle}
        onCount={canCount ? (name) => { setOpened(null); setParams({ tab: 'conteo', q: name }); } : undefined} />}
    </Page>
  );
}
