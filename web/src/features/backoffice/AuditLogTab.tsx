import { useState } from 'react';
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { Box, Button, Flex, HStack, SimpleGrid, Spinner, Text, VStack } from '@chakra-ui/react';
import { auditApi, type AuditEvent, type AuditGroup } from '../../api/audit';
import { LoadError } from '../../shared/LoadError';
import { Picker } from '../../components/Picker';
import { RangoDeFechas } from '../../components/RangoDeFechas';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';
import { soloHora, soloFecha } from '../../utils/horaDelNegocio';
import { ACTION_LABELS, actionLabel, summarizeChange } from '../../shared/audit/labels';
import { OrderHistoryTimeline } from '../../shared/audit/OrderHistoryTimeline';

// `short` va en el contador (una línea de 44 px); `label` es lo que se anuncia completo.
const GROUPS: Array<{ id: AuditGroup; label: string; short: string; color: string }> = [
  { id: 'money', label: 'Pedidos y cobros', short: 'Pedidos', color: 'green.500' },
  { id: 'cancel', label: 'Cancelaciones y devoluciones', short: 'Cancelaciones', color: 'red.500' },
  { id: 'price', label: 'Precios', short: 'Precios', color: 'purple.500' },
  { id: 'cash', label: 'Caja', short: 'Caja', color: 'blue.500' },
  { id: 'attendance', label: 'Asistencia', short: 'Asistencia', color: 'orange.400' },
];
const COLOR: Record<string, string> = Object.fromEntries(GROUPS.map((g) => [g.id, g.color]));
// El grupo de cada acción, espejo de domain.AuditGroupOf: solo pinta el punto de color.
function groupOf(action: string): AuditGroup {
  if (['payment.voided', 'payment.refunded', 'order.cancelled', 'order.line_removed', 'stock.written_off'].includes(action)) return 'cancel';
  if (action.startsWith('price.') || action.startsWith('product.')) return 'price';
  if (action.startsWith('cash.') || action.startsWith('expense.') || action.startsWith('tip.')) return 'cash';
  if (action.startsWith('attendance.') || action.startsWith('user.')) return 'attendance';
  return 'money';
}

const PRESETS = [
  { id: 'hoy', label: 'Hoy' },
  { id: 'ayer', label: 'Ayer' },
  { id: '7d', label: '7 días' },
];

function subtractDays(iso: string, n: number): string {
  const d = new Date(`${iso}T12:00:00Z`);
  d.setUTCDate(d.getUTCDate() - n);
  return d.toISOString().slice(0, 10);
}

function deviceLabel(e: AuditEvent): string {
  const d = e.device ?? {};
  return [d.platform, d.browser, d.model, d.screen].filter(Boolean).join(' · ');
}

// AuditLogTab es la bitácora general (maqueta C de «Pantallas y maquetas»): contadores por tipo que
// filtran, una lista por día y el detalle de cada evento. Solo con `audit.view`; quien la monta ya
// preguntó.
export function AuditLogTab() {
  const { zona: zone, diaDelNegocio } = useHoraDelNegocio();
  const today = diaDelNegocio(new Date());
  const [preset, setPreset] = useState('hoy');
  const [fromDate, setFromDate] = useState('');
  const [toDate, setToDate] = useState('');
  const [personFilter, setPersonFilter] = useState('');
  const [groupFilter, setGroupFilter] = useState<AuditGroup | undefined>();
  const [actionFilter, setActionFilter] = useState('');
  const [deviceKeyFilter, setDeviceKeyFilter] = useState('');
  const [moreFilters, setMoreFilters] = useState(false);
  const [page, setPage] = useState(0);
  const [selectedEvent, setSelectedEvent] = useState<AuditEvent | null>(null);
  const [historyOrderId, setHistoryOrderId] = useState<number | null>(null);
  const [historyLabel, setHistoryLabel] = useState('');

  const range = preset === 'hoy' ? { from: today, to: today }
    : preset === 'ayer' ? { from: subtractDays(today, 1), to: subtractDays(today, 1) }
      : preset === '7d' ? { from: subtractDays(today, 6), to: today }
        : { from: fromDate, to: toDate };
  const ready = Boolean(range.from && range.to);
  const listQuery = { ...range, page, size: 25 as const, actorUserId: personFilter ? Number(personFilter) : undefined,
    group: groupFilter, action: actionFilter || undefined, deviceKey: deviceKeyFilter || undefined };

  const actorsQuery = useQuery({ queryKey: ['audit', 'actors'], queryFn: auditApi.actors });
  const { data: fetched, isLoading, isError, error, refetch, isFetching } = useQuery({
    queryKey: ['audit', 'list', listQuery],
    queryFn: () => auditApi.list(listQuery),
    enabled: ready,
    placeholderData: keepPreviousData,
    // Sin red, TanStack pausa y deja la página anterior bajo el filtro nuevo, sin aviso. Con `always`
    // falla, y la página vieja se descarta: no responde a lo que se pidió.
    networkMode: 'always', retry: 1,
  });
  const data = isError ? undefined : fetched;
  const resetPage = () => setPage(0);
  const pageCount = Math.max(1, Math.ceil((data?.total ?? 0) / 25));

  // Separador de día: la lista va por hora, y un rango de varios días se lee por bloques.
  const items = data?.items ?? [];
  const dayLabels = items.map((e) => soloFecha(e.occurredAt, zone));

  return (
    <Flex direction="column" gap={3} minH={0} h="100%">
      <Flex gap={2} align="flex-start" wrap="wrap">
        <Box flex="1" minW="320px">
          <RangoDeFechas presets={PRESETS} preset={preset} onPreset={(id) => { setPreset(id); resetPage(); }}
            desde={fromDate} hasta={toDate} onRango={(d, h) => { setFromDate(d); setToDate(h); resetPage(); }} hoy={today} />
        </Box>
        <Box w="220px">
          <Picker title="Persona" placeholder="Todas las personas" clearable clearLabel="Todas las personas"
            value={personFilter} onChange={(v) => { setPersonFilter(v); resetPage(); }}
            options={(actorsQuery.data ?? []).map((a) => ({ value: String(a.id), label: a.name }))} />
        </Box>
        <Button variant="ghost" minH="44px" onClick={() => setMoreFilters(true)}>
          Más filtros{actionFilter || deviceKeyFilter ? ' ·' : ''}
        </Button>
      </Flex>

      <SimpleGrid columns={5} gap={2}>
        {GROUPS.map((g) => (
          <Button key={g.id} variant="outline" minH="44px" px={3} justifyContent="flex-start" gap={2}
            aria-label={`${g.label}: ${data?.countsByGroup[g.id] ?? 0}`} title={g.label}
            borderColor={groupFilter === g.id ? 'colorPalette.solid' : undefined} aria-pressed={groupFilter === g.id}
            onClick={() => { setGroupFilter(groupFilter === g.id ? undefined : g.id); resetPage(); }}>
            <Box w="8px" h="8px" borderRadius="full" bg={g.color} flexShrink={0} />
            <Text fontWeight="800">{data?.countsByGroup[g.id] ?? 0}</Text>
            <Text fontSize="sm" color="fg.muted" lineClamp={1}>{g.short}</Text>
          </Button>
        ))}
      </SimpleGrid>

      <Box flex="1" minH={0} overflowY="auto" borderWidth="1px" borderRadius="lg">
        {isError && <LoadError error={error} onRetry={() => void refetch()} retrying={isFetching} />}
        {isLoading && <HStack p={4}><Spinner size="sm" /><Text color="fg.muted">Cargando…</Text></HStack>}
        {data && data.items.length === 0 && <Text p={4} color="fg.muted">No hay acciones con estos filtros.</Text>}
        {items.map((e, i) => {
          const dayOf = dayLabels[i];
          const showSeparator = i === 0 || dayOf !== dayLabels[i - 1];
          return (
            <Box key={e.id}>
              {showSeparator && (
                <Text px={4} py={1} fontSize="xs" fontWeight="700" textTransform="uppercase" color="fg.muted" bg="bg.muted">{dayOf}</Text>
              )}
              <HStack as="button" w="100%" textAlign="left" minH="52px" px={4} gap={3} borderBottomWidth="1px"
                _hover={{ bg: 'bg.subtle' }} onClick={() => setSelectedEvent(e)}>
                <Text w="72px" fontSize="sm" color="fg.muted" flexShrink={0}>{soloHora(e.occurredAt, zone)}</Text>
                <Text w="150px" fontWeight="700" lineClamp={1} flexShrink={0}>{e.actorName}</Text>
                <Box flex="1" minW={0}>
                  <HStack gap={2}><Box w="8px" h="8px" borderRadius="full" bg={COLOR[groupOf(e.action)]} /><Text fontWeight="600">{actionLabel(e.action)}</Text>
                    {e.orderLabel && <Text fontSize="sm" color="fg.muted" lineClamp={1}>{e.orderLabel}</Text>}</HStack>
                  <Text fontSize="sm" color="fg.muted" lineClamp={1}>{summarizeChange(e.before, e.after)}</Text>
                </Box>
              </HStack>
            </Box>
          );
        })}
      </Box>

      <HStack justify="space-between">
        <Button minH="44px" variant="outline" disabled={page === 0} onClick={() => setPage(page - 1)}>Anteriores</Button>
        <Text color="fg.muted" fontSize="sm">{data ? `${data.total} acciones · página ${page + 1} de ${pageCount}` : ''}</Text>
        <Button minH="44px" variant="outline" disabled={page + 1 >= pageCount} onClick={() => setPage(page + 1)}>Siguientes</Button>
      </HStack>

      {moreFilters && (
        <BottomSheet titulo="Más filtros" onCerrar={() => setMoreFilters(false)}>
          <VStack align="stretch" gap={3}>
            <Picker title="Acción" placeholder="Todas las acciones" clearable clearLabel="Todas las acciones" value={actionFilter}
              onChange={(v) => { setActionFilter(v); resetPage(); }}
              options={Object.entries(ACTION_LABELS).map(([value, label]) => ({ value, label }))} />
            <Picker title="Aparato" placeholder="Todos los aparatos" clearable clearLabel="Todos los aparatos" value={deviceKeyFilter}
              onChange={(v) => { setDeviceKeyFilter(v); resetPage(); }}
              options={(data?.deviceKeys ?? []).map((k) => ({ value: k, label: `Aparato ${k.slice(0, 4).toUpperCase()}` }))} />
          </VStack>
        </BottomSheet>
      )}

      {selectedEvent && (
        <BottomSheet titulo={actionLabel(selectedEvent.action)} onCerrar={() => setSelectedEvent(null)}>
          <VStack align="stretch" gap={1} fontSize="sm">
            <DetailRow k="Quién" v={selectedEvent.actorName} />
            <DetailRow k="Cuándo" v={`${soloFecha(selectedEvent.occurredAt, zone)} · ${soloHora(selectedEvent.occurredAt, zone)}`} />
            <DetailRow k="Red" v={selectedEvent.ip || '—'} />
            <DetailRow k="Aparato" v={[deviceLabel(selectedEvent), selectedEvent.deviceKey && selectedEvent.deviceKey.slice(0, 4).toUpperCase()].filter(Boolean).join(' · ') || '—'} />
            <DetailRow k="Qué cambió" v={summarizeChange(selectedEvent.before, selectedEvent.after) || '—'} />
            {selectedEvent.orderId && (
              <Button mt={2} minH="44px" variant="outline" alignSelf="flex-start"
                onClick={() => { setHistoryOrderId(selectedEvent.orderId!); setHistoryLabel(selectedEvent.orderLabel ?? ''); setSelectedEvent(null); }}>
                Ver historial del pedido
              </Button>
            )}
          </VStack>
        </BottomSheet>
      )}

      {historyOrderId !== null && (
        <BottomSheet titulo={historyLabel ? `Historial · ${historyLabel}` : 'Historial del pedido'} onCerrar={() => setHistoryOrderId(null)}>
          <OrderHistoryTimeline orderId={historyOrderId} />
        </BottomSheet>
      )}
    </Flex>
  );
}

function DetailRow({ k, v }: { k: string; v: string }) {
  return (
    <HStack justify="space-between" align="baseline" gap={4}>
      <Text color="fg.muted">{k}</Text>
      <Text fontWeight="600" textAlign="end">{v}</Text>
    </HStack>
  );
}

// Hoja inferior propia: el detalle se lee encima de la lista sin perderla.
function BottomSheet({ titulo: heading, onCerrar, children }: { titulo: string; onCerrar: () => void; children: React.ReactNode }) {
  return (
    <Flex position="fixed" inset={0} bg="blackAlpha.400" align="flex-end" zIndex={1400} onClick={(e) => { if (e.target === e.currentTarget) onCerrar(); }}>
      <Box role="dialog" aria-label={heading} bg="bg" w="100%" maxH="80dvh" overflowY="auto" borderTopRadius="xl"
        px={6} pt={4} pb={6}>
        <Flex justify="space-between" align="center" mb={3}>
          <Text fontSize="lg" fontWeight="800">{heading}</Text>
          <Button variant="ghost" minH="44px" onClick={onCerrar}>Cerrar</Button>
        </Flex>
        {children}
      </Box>
    </Flex>
  );
}
