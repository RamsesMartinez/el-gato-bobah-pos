import { useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router';
import { ListaDeCompra } from './ListaDeCompra';
import { Destinatarios } from './Destinatarios';
import { formatBaseQty } from '../../shared/units';
import {
  Box, Button, Heading, HStack, Table, Center, Spinner, Badge, Tabs, Text,
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
import { Segmented } from '../../components/Segmented';
import { TransferTab } from './warehouses/TransferTab';
import { BringSheet } from './warehouses/BringSheet';
import { richestSource } from './warehouses/transferUnits';
import type { StockLevel, StockMovement } from '../../api/backoffice';
import { InfoButton } from '../../components/InfoButton';
import { money } from '../../utils/format';
import { ExpiriesTab } from './expiries/ExpiriesTab';
import { ExpiryNotice } from './expiries/ExpiryNotice';

// El orden lo fijó el plan del 038: Existencias · Movimientos · Insumos · Por recibir · Conteo · Salud.
const TABS = ['existencias', 'traspaso', 'caducidades', 'movimientos', 'insumos', 'compra', 'por-recibir', 'conteo', 'merma', 'salud', 'avisos'] as const;
type Tab = (typeof TABS)[number];

export function StockPage() {
  const horaNegocio = useHoraDelNegocio();
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  // El almacén que se ve en Existencias (spec 094): por omisión el de venta. Con un solo almacén no
  // hay selector y la pantalla queda como antes.
  const [picked, setWarehouse] = useState<number | 'all' | undefined>(undefined);
  const warehouses = useQuery({ queryKey: ['warehouses'], queryFn: () => backofficeApi.warehouses() });
  const whList = warehouses.data?.items ?? [];
  // Si el almacén elegido se archivó (en otra tableta), se vuelve al de venta: si no, ningún botón
  // queda marcado y la lista falla sin salida.
  const warehouse = typeof picked === 'number' && warehouses.data && !whList.some((w) => w.id === picked) ? undefined : picked;
  // El mínimo lo manda el servidor por almacén (el propio; en el de venta, el del artículo si no tiene
  // propio; en «Todos», ninguno). «Traer de…» solo tiene sentido en el de venta.
  const atSales = warehouse === undefined;
  const [bring, setBring] = useState<StockLevel | null>(null);
  const levels = useQuery({ queryKey: ['stock', 'levels', warehouse ?? 'venta'], queryFn: () => backofficeApi.stockLevels(warehouse),
    // Cambiar de almacén no borra la pantalla mientras llega la otra lista.
    placeholderData: (prev) => prev });
  // Movimientos sigue al mismo almacén que Existencias: cambiar de pestaña no cambia de almacén.
  const moves = useQuery({ queryKey: ['stock', 'moves', warehouse ?? 'venta'], queryFn: () => backofficeApi.stockMovements(warehouse),
    placeholderData: (prev) => prev });
  const [valueHelp, setValueHelp] = useState(false);
  const canViewStock = can('inventory.view', useSessionStore((st) => st.user));
  const warehouseSegmented = whList.length > 1 && (
      <Segmented label="Almacén"
        value={String(warehouse ?? whList.find((w) => w.isSales)?.id ?? '')}
        options={[...whList.map((w) => ({ value: String(w.id), label: w.isSales ? `${w.name} · de venta` : w.name })),
          { value: 'all', label: 'Todos' }]}
        onChange={(v) => setWarehouse(v === 'all' ? 'all' : whList.find((w) => String(w.id) === v)?.isSales ? undefined : Number(v))} />
  );
  const warehousePicker = warehouseSegmented && <Box mb={3}>{warehouseSegmented}</Box>;
  // En Existencias, el aviso de caducidad va a la derecha del selector cuando cabe: así no le quita un
  // renglón a la lista (revisión de tableta, spec 098).
  const stockHeader = (
    <HStack mb={warehouseSegmented ? 3 : 0} gap={3} flexWrap="wrap" align="center">
      {warehouseSegmented}
      {canViewStock && <Box flex="1" minW="280px" mb={warehouseSegmented ? 0 : 3} _empty={{ display: 'none' }}><ExpiryNotice warehouse={warehouse} onOpen={() => setParams({ tab: 'caducidades' }, { replace: true })} /></Box>}
    </HStack>
  );
  // El valor solo se pinta si el servidor lo manda: la API anterior no lo trae durante una subida.
  const withValue = levels.data?.totalValue !== undefined;
  const levelCols = withValue ? 5 : 4;
  // El total va arriba, junto al selector: al pie de cientos de renglones nadie lo encontraba. En
  // «Todos», el valor de cada almacén al lado.
  const valueSummary = withValue && !levels.isError && (
    <Box mb={3} data-testid="valor-total">
      <HStack gap={2} flexWrap="wrap">
        <Text fontWeight="700">Valor total</Text>
        <Text fontWeight="700">{money(levels.data!.totalValue!)}</Text>
        {(levels.data!.valueByWarehouse ?? []).length > 1 && (
          <Text color="fg.muted">({levels.data!.valueByWarehouse!.map((v) => `${v.warehouseName} ${money(v.value)}`).join(' · ')})</Text>
        )}
        <InfoButton label="¿Qué suma el valor total?" open={valueHelp} onToggle={() => setValueHelp((o) => !o)} />
      </HStack>
      {valueHelp && (
        <Text color="fg.muted" fontSize="sm">
          Suma lo que hay a su costo de hoy, almacén por almacén. Lo que está en negativo en un almacén no resta: cuéntalo para corregirlo.
        </Text>
      )}
    </Box>
  );
  const user = useSessionStore((s) => s.user);
  const canHealth = can('inventory.health_view', user);
  // Cómo llega cada insumo (spec 038): lo ve quien registra compras y lo cambia quien configura.
  const canReceive = can('inventory.receive', user);
  const canCount = can('inventory.count', user);
  // La dirección decide la pestaña y el insumo abierto: «Salud del inventario» manda aquí con
  // ?tab=insumos&open=N. Un valor desconocido cae a la primera pestaña, nunca a una que no existe.
  const asked = params.get('tab') as Tab | null;
  const canView = can('inventory.view', user);
  const canConfigureStock = can('inventory.configure', user);
  // Traspaso (spec 094): solo con el permiso y con dos o más almacenes; con uno no hay a dónde llevar.
  const canTransfer = can('inventory.transfer', user) && whList.length > 1;
  const allowed = (t: Tab) => (t !== 'traspaso' || canTransfer) && (t !== 'avisos' || canConfigureStock) && ((t !== 'insumos' && t !== 'compra' && t !== 'caducidades') || canView) && ((t !== 'salud' && t !== 'merma') || canHealth) && (t !== 'por-recibir' || canReceive) && (t !== 'conteo' || canCount);
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
  // Destinatarios se queda montado una vez abierto: lo que se capturó ahí no se pierde por ir a otra
  // pestaña a revisar algo y volver. Se ajusta al pintar (estado derivado), no en un efecto.
  const [avisosAbierto, setAvisosAbierto] = useState(false);
  if (tab === 'avisos' && !avisosAbierto) setAvisosAbierto(true);
  const article = opened ?? (tab === 'insumos' && openId > 0 ? { id: openId, justCreated: false } : null);
  const openArticle = (id: number, justCreated = false) => setOpened({ id, justCreated });
  const closeArticle = () => {
    setOpened(null);
    if (params.has('open')) { const p = new URLSearchParams(params); p.delete('open'); setParams(p, { replace: true }); }
  };

  if (levels.isLoading) return <Center h="60vh"><Spinner size="xl" /></Center>;

  return (
    <Page fill>
      <HStack justify="space-between" mb={4}>
        <Heading size="lg">Almacén</Heading>
        {/* La persona nueva buscaba aquí sus almacenes (prueba con usuario nuevo, 094): viven en Negocio,
            junto a los lugares para contar, y este atajo la lleva. */}
        {can('inventory.warehouse_manage', user) && (
          <Button minH="44px" variant="ghost" onClick={() => navigate('/negocio?seccion=inventario#almacenes')}>Almacenes ›</Button>
        )}
      </HStack>
      <Tabs.Root value={tab} onValueChange={(e) => setParams({ tab: e.value }, { replace: true })}
        display="flex" flexDirection="column" flex="1" minH={0}>
        {/* Con «Se echó a perder» son siete: la fila se desplaza a lo ancho antes que salirse de 1024 px. */}
        <Tabs.List overflowX="auto" flexShrink={0}>
          <Tabs.Trigger value="existencias" minH="44px">Existencias</Tabs.Trigger>
          {canTransfer && <Tabs.Trigger value="traspaso" minH="44px">Traspaso</Tabs.Trigger>}
          {canView && <Tabs.Trigger value="caducidades" minH="44px">Caducidades</Tabs.Trigger>}
          <Tabs.Trigger value="movimientos" minH="44px">Movimientos</Tabs.Trigger>
          {canView && <Tabs.Trigger value="insumos" minH="44px">{itemsTab}</Tabs.Trigger>}
          {canView && <Tabs.Trigger value="compra" minH="44px" flexShrink={0}>Lista de compra</Tabs.Trigger>}
          {canReceive && (
            <Tabs.Trigger value="por-recibir" minH="44px" flexShrink={0} whiteSpace="nowrap">
              Por recibir{pending.data && pending.data.length > 0 ? ` · ${pending.data.length}` : ''}
            </Tabs.Trigger>
          )}
          {canCount && <Tabs.Trigger value="conteo" minH="44px">Conteo</Tabs.Trigger>}
          {canHealth && <Tabs.Trigger value="merma" minH="44px" flexShrink={0}>Se echó a perder</Tabs.Trigger>}
          {canHealth && <Tabs.Trigger value="salud" minH="44px">Salud</Tabs.Trigger>}
          {canConfigureStock && <Tabs.Trigger value="avisos" minH="44px">Avisos</Tabs.Trigger>}
        </Tabs.List>
        <Tabs.Content value="existencias" px={0} flex="1" minH={0} overflowY="auto">
          {stockHeader}
          {valueSummary}
          {/* Mientras llega la lista del almacén elegido, la anterior se atenúa: no se leen cifras de
              otro almacén como si fueran de éste. */}
          <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" overflowX="auto" opacity={levels.isPlaceholderData ? 0.4 : 1}
            aria-busy={levels.isPlaceholderData || undefined}>
            <Table.Root size="sm">
              <Table.Header><Table.Row><Table.ColumnHeader>Artículo</Table.ColumnHeader><Table.ColumnHeader>Tipo</Table.ColumnHeader><Table.ColumnHeader textAlign="end">Existencia</Table.ColumnHeader><Table.ColumnHeader textAlign="end">Mínimo</Table.ColumnHeader>{withValue && <Table.ColumnHeader textAlign="end">Valor</Table.ColumnHeader>}</Table.Row></Table.Header>
              <Table.Body>
                {levels.isError && (
                  <Table.Row><Table.Cell colSpan={levelCols}><Text color="fg.error">No se pudieron leer las existencias de este almacén</Text></Table.Cell></Table.Row>
                )}
                {!levels.isError && (levels.data?.items ?? []).length === 0 && (
                  <Table.Row><Table.Cell colSpan={levelCols}><Text color="fg.subtle">
                    {typeof warehouse === 'number' ? 'Este almacén todavía no tiene mercancía' : 'Sin existencias que mostrar'}
                  </Text></Table.Cell></Table.Row>
                )}
                {!levels.isError && (levels.data?.items ?? []).map((s, i) => {
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
                        {/* Un negativo en el de venta con mercancía en otro almacén se trae de ahí (spec 094). */}
                        {atSales && canTransfer && Number(s.on_hand) < 0 && (s.transfer_from?.length ?? 0) > 0 && (s.ingredient_id ?? s.product_id) && (
                          <Button ml={2} size="sm" minH="44px" variant="outline" onClick={() => setBring(s)}>
                            Traer de {richestSource(s.transfer_from!).name}
                          </Button>
                        )}
                      </Table.Cell>
                      <Table.Cell textAlign="end">{s.min_stock == null ? '—' : formatBaseQty(s.min_stock, s.unit_code)}</Table.Cell>
                      {withValue && <Table.Cell textAlign="end">{s.value == null ? '—' : money(s.value)}</Table.Cell>}
                    </Table.Row>
                  );
                })}
              </Table.Body>
            </Table.Root>
          </Box>
        </Tabs.Content>
        {canView && (
          <Tabs.Content value="caducidades" px={0} flex="1" minH={0} overflowY="auto">
            <ExpiriesTab onGoToCount={canCount ? () => setParams({ tab: 'conteo' }, { replace: true }) : undefined} />
          </Tabs.Content>
        )}
        {canTransfer && (
          <Tabs.Content value="traspaso" px={0} flex="1" minH={0}>
            <TransferTab warehouses={whList} />
          </Tabs.Content>
        )}
        <Tabs.Content value="movimientos" px={0} flex="1" minH={0} overflowY="auto">
          {warehousePicker}
          <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" overflowX="auto" opacity={moves.isPlaceholderData ? 0.4 : 1}
            aria-busy={moves.isPlaceholderData || undefined}>
            <Table.Root size="sm">
              <Table.Header><Table.Row><Table.ColumnHeader>Fecha</Table.ColumnHeader>{warehouse === 'all' && <Table.ColumnHeader>Almacén</Table.ColumnHeader>}<Table.ColumnHeader>Artículo</Table.ColumnHeader><Table.ColumnHeader>Tipo</Table.ColumnHeader><Table.ColumnHeader textAlign="end">Cantidad</Table.ColumnHeader><Table.ColumnHeader>Motivo</Table.ColumnHeader></Table.Row></Table.Header>
              <Table.Body>
                {(moves.data?.items ?? []).map((m) => (
                  <Table.Row key={m.id}>
                    <Table.Cell whiteSpace="nowrap">{horaNegocio.fechaYHora(m.created_at)}</Table.Cell>
                    {warehouse === 'all' && <Table.Cell>{m.warehouse_name ?? '—'}</Table.Cell>}
                    <Table.Cell>{m.item_name}</Table.Cell>
                    <Table.Cell>{m.movement_type}{transferSide(m)}</Table.Cell>
                    <Table.Cell textAlign="end" color={Number(m.quantity) < 0 ? 'fg.error' : 'fg.success'}>
                      {m.unit_code ? formatBaseQty(m.quantity, m.unit_code) : m.quantity}
                    </Table.Cell>
                    {/* Un motivo largo partía la fecha en cuatro renglones: se recorta, y el texto completo
                        sale al dejar el dedo encima. */}
                    <Table.Cell maxW="240px"><Text lineClamp={2} title={m.reason ?? undefined}>{m.reason ?? '—'}</Text></Table.Cell>
                  </Table.Row>
                ))}
              </Table.Body>
            </Table.Root>
          </Box>
        </Tabs.Content>
        {/* Artículos (spec 042): buscador, filtros, «+ Nuevo» y la ficha de cada uno. */}
        <Tabs.Content value="insumos" px={0} flex="1" minH={0} display="flex" flexDirection="column">
          {tab === 'insumos' && (
            <ArticlesTab word={word} onOpen={(id) => openArticle(id)}
              notice={params.get('nuevo') !== null ? { name: params.get('nuevo') ?? '', id: Number(params.get('aviso')) || 0 } : undefined}
              onNoticeDone={() => setParams({ tab: 'insumos' }, { replace: true })} />
          )}
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
        {canView && (
          <Tabs.Content value="compra" px={0} flex="1" minH={0} display="flex" flexDirection="column">
            {tab === 'compra' && <ListaDeCompra />}
          </Tabs.Content>
        )}
        {canConfigureStock && (
          <Tabs.Content value="avisos" px={0} flex="1" minH={0} display="flex" flexDirection="column">
            {(tab === 'avisos' || avisosAbierto) && <Destinatarios />}
          </Tabs.Content>
        )}
      </Tabs.Root>
      {article && <ArticleSheet key={article.id} id={article.id} justCreated={article.justCreated} onClose={closeArticle}
        onOpenArticle={(id) => openArticle(id)}
        onCount={canCount ? (name) => { setOpened(null); setParams({ tab: 'conteo', q: name }); } : undefined} />}
      {bring && bring.transfer_from?.[0] && whList.find((w) => w.isSales) && (
        <BringSheet onClose={() => setBring(null)}
          item={{ itemType: bring.item_type, id: (bring.ingredient_id ?? bring.product_id)!, name: bring.item_name, unit: bring.unit_code, onHand: bring.on_hand }}
          sources={bring.transfer_from}
          to={{ id: whList.find((w) => w.isSales)!.id, name: whList.find((w) => w.isSales)!.name }} />
      )}
    </Page>
  );
}

// transferSide dice de qué lado del traspaso está el renglón: lo que sale va «→ a dónde», lo que
// entra «← de dónde».
function transferSide(m: StockMovement): string {
  if (!m.counterpart_warehouse_name) return '';
  const name = m.counterpart_warehouse_archived ? `${m.counterpart_warehouse_name} (archivado)` : m.counterpart_warehouse_name;
  return Number(m.quantity) < 0 ? ` → ${name}` : ` ← ${name}`;
}
