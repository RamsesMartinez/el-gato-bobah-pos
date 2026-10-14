import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { Badge, Box, Button, HStack, Text, VStack } from '@chakra-ui/react';
import { LuChevronDown, LuChevronRight } from 'react-icons/lu';
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from 'react-router';
import { ApiError } from '../../api/client';
import { backofficeApi, type HealthMalformed, type HealthQuery, type HealthUnsoldItem, type InventoryHealth } from '../../api/backoffice';
import { medirAccion } from '../../api/uso';
import { DatePickerButton } from '../../components/DatePickerButton';
import { ListRow } from '../../components/ListRow';
import { DrawerBackdrop, DrawerCloseTrigger, DrawerContent, DrawerRoot } from '../../components/ui/drawer';
import { money } from '../../utils/format';
import { formatDay } from '../../utils/calendar';

// Almacén › Salud (spec 037, maqueta A «Secciones que se abren»): lo que hace que las existencias
// no cuadren, en cinco secciones que se abren con un toque. Va DENTRO de la pestaña de Almacén: no
// trae su propio Page (anidarlo duplica el relleno y recorta el ancho dos veces).

const PAGE = 25;
type Preset = 'hoy' | '7d' | '30d' | 'mes' | 'fechas';
const PRESETS: { p: Preset; label: string }[] = [
  { p: 'hoy', label: 'Hoy' }, { p: '7d', label: 'Últimos 7 días' }, { p: '30d', label: 'Últimos 30 días' },
  { p: 'mes', label: 'Este mes' }, { p: 'fechas', label: 'Elegir fechas…' },
];
type SectionKey = 'unsold' | 'negative' | 'noPurchases' | 'malformed' | 'pending';

// Las fechas se cuentan desde el «hoy» que contestó el servidor (en la zona del negocio), no desde
// el reloj de la tableta: a las 19:00 de México el navegador en UTC ya vive en mañana.
function shift(day: string, days: number): string {
  const d = new Date(`${day}T00:00:00Z`);
  d.setUTCDate(d.getUTCDate() + days);
  return d.toISOString().slice(0, 10);
}

function rangeOf(p: Preset, today: string, from: string, to: string): Pick<HealthQuery, 'from' | 'to'> {
  switch (p) {
    case 'hoy': return { from: today, to: today };
    case '7d': return { from: shift(today, -6), to: today };
    case 'mes': return { from: `${today.slice(0, 8)}01`, to: today };
    case 'fechas': return { from: from || undefined, to: to || undefined };
    default: return {};
  }
}

const PROBLEM: Record<HealthMalformed['problem'], string> = {
  zero_quantity: 'Cantidad en cero',
  unit_mismatch: 'Unidad que no corresponde',
  archived_ingredient: 'Insumo archivado',
  duplicate_ingredient: 'Insumo repetido',
};

function statusLabel(it: HealthUnsoldItem): string | null {
  if (it.kind === 'extra') return 'Extra que no resta nada';
  if (it.status === 'undecided') return 'Falta decir qué resta';
  return null;
}

export function InventoryHealthTab() {
  const navigate = useNavigate();
  const qc = useQueryClient();
  const [preset, setPreset] = useState<Preset>('30d');
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');
  // El rango que se veía antes del último cambio: si el nuevo se rechaza, se sigue mostrando ese.
  const [prevRange, setPrevRange] = useState<Pick<HealthQuery, 'from' | 'to'>>({});
  const [periodOpen, setPeriodOpen] = useState(false);
  const [open, setOpen] = useState<Record<SectionKey, boolean>>({ unsold: true, negative: false, noPurchases: false, malformed: false, pending: false });
  const [showNegatives, setShowNegatives] = useState(false);
  const [showNoPurchases, setShowNoPurchases] = useState(false);
  const [since, setSince] = useState('');
  const [more, setMore] = useState<{ unsold: number; negative: number; malformed: number }>({ unsold: 0, negative: 0, malformed: 0 });

  useEffect(() => { medirAccion('inventario', 'ver-salud'); }, []);

  // «Hoy» es el fin del periodo por omisión que contestó el servidor, en la zona del negocio.
  const today = qc.getQueryData<InventoryHealth>(['stock', 'health', {}])?.to ?? '';
  const range = rangeOf(preset, today, from, to);
  const report = useQuery({
    queryKey: ['stock', 'health', range],
    queryFn: () => backofficeApi.inventoryHealth(range),
    placeholderData: keepPreviousData,
    enabled: preset !== 'fechas' || (from !== '' && to !== ''),
  });
  const moreUnsold = useExtraPage(range, 'unsold_deduction', more.unsold);
  const moreNegative = useExtraPage(range, 'negative_stock', more.negative);
  const moreMalformed = useExtraPage(range, 'malformed_recipe', more.malformed);
  const noPurchaseList = useQuery({
    queryKey: ['stock', 'health', 'no_purchases', since],
    queryFn: () => backofficeApi.inventoryHealth({ section: 'no_purchases', limit: 100, noPurchasesSince: since || undefined }),
    enabled: showNoPurchases,
  });

  // Si el periodo nuevo se rechaza, la pantalla dice por qué y sigue mostrando lo de antes, en vez de
  // vaciarse.
  const data: InventoryHealth | undefined = report.data ?? qc.getQueryData<InventoryHealth>(['stock', 'health', prevRange]);

  const unsold = useMemo(() => [...(data?.unsoldDeduction.items ?? []), ...(moreUnsold.data?.unsoldDeduction.items ?? [])], [data, moreUnsold.data]);
  const negatives = useMemo(() => [...(data?.negativeStock.items ?? []), ...(moreNegative.data?.negativeStock.items ?? [])], [data, moreNegative.data]);
  const malformed = useMemo(() => [...(data?.malformedRecipes.items ?? []), ...(moreMalformed.data?.malformedRecipes.items ?? [])], [data, moreMalformed.data]);

  // Exporta lo que se ve: el mismo periodo, calculado por el servidor igual que la pantalla.
  const exporter = useMutation({
    mutationFn: () => backofficeApi.exportInventoryHealth(range),
    onSuccess: ({ blob, filename }) => {
      medirAccion('inventario', 'exportar-salud');
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = filename;
      a.click();
      URL.revokeObjectURL(url);
    },
  });
  const go = (fixAt: string, name?: string) => { if (fixAt) navigate(fixAt, { state: { name } }); };
  const toggle = (k: SectionKey) => setOpen((o) => ({ ...o, [k]: !o[k] }));
  const pickPreset = (p: Preset) => { setPrevRange(range); setPreset(p); setPeriodOpen(false); setMore({ unsold: 0, negative: 0, malformed: 0 }); };
  const pickFrom = (v: string) => { setPrevRange(range); setFrom(v); };
  const pickTo = (v: string) => { setPrevRange(range); setTo(v); };

  // Lo que se ve es de otro periodo: el que se pidió todavía no llega.
  const updating = report.isFetching && (report.isPlaceholderData || !report.data);
  const error = report.error instanceof ApiError ? report.error.message : report.error ? 'No hay conexión con el servidor. Intenta de nuevo.' : '';
  const presetLabel = PRESETS.find((x) => x.p === preset)?.label ?? '';

  if (!data) {
    return <Text color="fg.muted" py={6}>{error || 'Cargando…'}</Text>;
  }

  const pending = data.pendingRecipes;
  const pendingTotal = pending.undecided.products + pending.undecided.extras + pending.review.products + pending.review.extras;
  const unsoldTotal = data.unsoldDeduction.total + (data.comboChoices.items ?? []).length;
  const healthy = data.salesInPeriod > 0 && unsoldTotal === 0 && data.negativeStock.total === 0 && data.noPurchases.total === 0
    && data.malformedRecipes.total === 0 && pendingTotal === 0;

  return (
    <VStack align="stretch" gap={2} flex="1" minH={0}>
      <HStack gap={2} wrap="wrap">
        <Button aria-label={`Periodo: ${presetLabel}`} variant="outline" colorPalette="gray" minH="44px" onClick={() => setPeriodOpen(true)}>
          {presetLabel} <LuChevronDown />
        </Button>
        {preset === 'fechas' && (
          <>
            <DatePickerButton label="Desde" value={from} onChange={pickFrom} max={to || today || undefined} />
            <DatePickerButton label="Hasta" value={to} onChange={pickTo} max={today || undefined} />
          </>
        )}
        <Text fontSize="sm" color="fg.muted" flex="1" minW="200px">
          {/* Mientras llega el periodo nuevo se dice: los números de abajo todavía son del anterior. */}
          {updating ? 'Actualizando…'
            : `Del ${formatDay(data.from)} al ${formatDay(data.to)} · ${data.salesInPeriod} ${data.salesInPeriod === 1 ? 'venta' : 'ventas'}`}
        </Text>
        <Button minH="44px" variant="outline" loading={exporter.isPending} disabled={exporter.isPending} onClick={() => exporter.mutate()}>Exportar</Button>
      </HStack>
      {exporter.error && (
        <Text fontSize="sm" color="red.700">{exporter.error instanceof ApiError ? exporter.error.message : 'No se pudo exportar. Intenta de nuevo.'}</Text>
      )}
      {error && (
        <Text fontSize="sm" color="red.700" bg="red.50" borderWidth="1px" borderColor="red.200" borderRadius="md" px={3} py={2}>{error}</Text>
      )}

      <DrawerRoot open={periodOpen} placement="bottom" onOpenChange={(e) => { if (!e.open) setPeriodOpen(false); }} size="md">
        <DrawerBackdrop />
        <DrawerContent borderTopRadius="2xl" maxH="80dvh" aria-label="Periodo">
          <DrawerCloseTrigger />
          <VStack align="stretch" gap={1} px={3} pt={4} pb={4}>
            <Text fontWeight="700" fontSize="lg" px={1} pb={2}>Periodo</Text>
            {PRESETS.map((x) => <ListRow key={x.p} label={x.label} selected={x.p === preset} onClick={() => pickPreset(x.p)} />)}
          </VStack>
        </DrawerContent>
      </DrawerRoot>

      <Box flex="1" minH={0} overflowY="auto" borderWidth="1px" borderRadius="xl" data-testid="lista-salud"
        aria-busy={updating} opacity={updating ? 0.45 : 1} transition="opacity 0.15s">
        {healthy ? (
          <VStack py={12} gap={1}><Text fontWeight="700" fontSize="lg">El inventario está sano</Text><Text color="fg.muted">Todo lo que se vendió restó de lo guardado.</Text></VStack>
        ) : (
          <>
            <Section title="Se vende sin restar" color="orange.500" count={unsoldTotal} open={open.unsold} onToggle={() => toggle('unsold')}>
              {data.salesInPeriod === 0 && <Note>No hubo ventas en este periodo</Note>}
              {unsold.map((it) => (
                <Row key={`${it.kind}-${it.id}`} onClick={() => go(it.fixAt, it.name)}
                  title={it.name} subtitle={it.group} pill={statusLabel(it)}
                  right={<VStack gap={0} align="end"><Text fontWeight="600">{money(it.estimatedCost)}</Text><Text fontSize="xs" color="fg.muted">costo estimado</Text></VStack>}>
                  {(it.units ?? []).map((u) => <Badge key={u.channel} variant="subtle" colorPalette="gray">{u.channel} {Number(u.units)}</Badge>)}
                </Row>
              ))}
              {(data.comboChoices.items ?? []).map((c) => (
                <Row key={`combo-${c.productId}-${c.slot}`} onClick={() => go(c.fixAt, c.name)}
                  title={c.name} subtitle={`${c.slot}: ${c.options} opciones`} pill="Siempre resta el de fábrica" />
              ))}
              {data.unsoldDeduction.total > unsold.length && (
                <MoreButton left={data.unsoldDeduction.total - unsold.length} loading={moreUnsold.isFetching}
                  onClick={() => setMore((m) => ({ ...m, unsold: m.unsold + 1 }))} />
              )}
              {data.unsoldDeduction.unpairedPlatformLines > 0 && (
                <Note>{data.unsoldDeduction.unpairedPlatformLines} platillos de plataforma sin emparejar</Note>
              )}
              {unsoldTotal > 0 && <Note>Las ventas anteriores no se recalculan</Note>}
            </Section>

            <Section title="Lo guardado en negativo" color="orange.500" count={data.negativeStock.total} open={open.negative} onToggle={() => toggle('negative')}>
              {data.negativeStock.collapsed && !showNegatives ? (
                <Row onClick={() => setShowNegatives(true)} title="Todavía no se registran compras: lo guardado solo baja."
                  subtitle={`Ver los ${data.negativeStock.total}`} />
              ) : (
                <>
                  {negatives.map((n) => (
                    <Row key={`${n.ingredientId}-${n.branch ?? ''}`} onClick={() => go(n.fixAt, n.name)} title={n.name}
                      subtitle={`Lo usan ${n.usedBy} ${n.usedBy === 1 ? 'receta' : 'recetas'}${n.branch ? ` · ${n.branch}` : ''}`}
                      right={<Text fontWeight="600" color="red.700">{Number(n.onHand)} {n.unit}</Text>} />
                  ))}
                  {data.negativeStock.total > negatives.length && (
                    <MoreButton left={data.negativeStock.total - negatives.length} loading={moreNegative.isFetching}
                      onClick={() => setMore((m) => ({ ...m, negative: m.negative + 1 }))} />
                  )}
                </>
              )}
            </Section>

            <Section title="Insumos sin compras registradas" color="orange.500" count={data.noPurchases.total} open={open.noPurchases} onToggle={() => toggle('noPurchases')}>
              <HStack px={4} py={2} gap={2}>
                <Text fontSize="sm" color="fg.muted">Sin compras desde</Text>
                <DatePickerButton label="Sin compras desde" value={since} onChange={setSince} clearable max={today || undefined} />
              </HStack>
              {!showNoPurchases ? (
                <Row onClick={() => setShowNoPurchases(true)} title={`${data.noPurchases.total} de ${data.noPurchases.of} insumos sin compras registradas`} />
              ) : (
                (noPurchaseList.data?.noPurchases.items ?? []).map((n) => (
                  <Row key={n.ingredientId} onClick={() => go(n.fixAt, n.name)} title={n.name} subtitle={n.unit} />
                ))
              )}
            </Section>

            <Section title="Recetas con un error" color="orange.500" count={data.malformedRecipes.total} open={open.malformed} onToggle={() => toggle('malformed')}>
              {malformed.map((m) => (
                <Row key={`${m.ownerKind}-${m.ownerId}-${m.ingredient}-${m.problem}`} onClick={() => go(m.fixAt, m.ownerName)}
                  title={m.ownerName} subtitle={`${PROBLEM[m.problem]}: ${m.ingredient}`} />
              ))}
              {data.malformedRecipes.total > malformed.length && (
                <MoreButton left={data.malformedRecipes.total - malformed.length} loading={moreMalformed.isFetching}
                  onClick={() => setMore((m) => ({ ...m, malformed: m.malformed + 1 }))} />
              )}
            </Section>

            <Section title="Recetas pendientes" color="orange.500" count={pendingTotal} open={open.pending} onToggle={() => toggle('pending')}>
              <Row onClick={() => go(pending.undecided.fixAt)} title="Falta decir qué resta"
                subtitle={`${pending.undecided.products} productos · ${pending.undecided.extras} extras`} right={<Text color="fg.muted">Ir a Recetas</Text>} />
              <Row onClick={() => go(pending.review.fixAt)} title="Las cargó el sistema y nadie las ha confirmado"
                subtitle={`${pending.review.products} productos · ${pending.review.extras} extras`} right={<Text color="fg.muted">Ir a Recetas</Text>} />
            </Section>
          </>
        )}
      </Box>
    </VStack>
  );
}

// useExtraPage es el «Ver 25 más» de una sección: la misma consulta con su sección y desde donde
// termina la primera página.
function useExtraPage(range: Pick<HealthQuery, 'from' | 'to'>, section: 'unsold_deduction' | 'negative_stock' | 'malformed_recipe', n: number) {
  return useQuery({
    queryKey: ['stock', 'health', range, section, n],
    queryFn: () => backofficeApi.inventoryHealth({ ...range, section, limit: n * PAGE, offset: PAGE }),
    enabled: n > 0,
  });
}

function Section({ title, color, count, open, onToggle, children }: {
  title: string; color: string; count: number; open: boolean; onToggle: () => void; children: ReactNode;
}) {
  return (
    <Box borderBottomWidth="1px" borderColor="border.muted">
      <Button variant="ghost" w="100%" minH="52px" px={4} gap={3} borderRadius={0} justifyContent="start"
        aria-expanded={open} onClick={onToggle}>
        <Box w="10px" h="10px" borderRadius="full" bg={count > 0 ? color : 'gray.300'} flex="none" aria-hidden />
        <Text fontWeight="700" flex="1" textAlign="start">{title}</Text>
        <Text fontWeight="700" color={count > 0 ? 'fg' : 'fg.muted'}>{count}</Text>
        {open ? <LuChevronDown /> : <LuChevronRight />}
      </Button>
      {open && <Box bg="bg.subtle">{children}</Box>}
    </Box>
  );
}

// Row: el renglón completo es el botón (54 px), como en Recetas; «›» solo lo señala.
function Row({ title, subtitle, pill, right, children, onClick }: {
  title: string; subtitle?: string; pill?: string | null; right?: ReactNode; children?: ReactNode; onClick: () => void;
}) {
  return (
    <Button variant="ghost" w="100%" h="auto" minH="54px" px={4} py={2} gap={3} borderRadius={0} borderTopWidth="1px"
      borderColor="border.muted" justifyContent="start" fontWeight="normal" onClick={onClick}>
      <VStack align="start" gap={1} flex="1" minW={0}>
        <HStack gap={2} maxW="100%" minW={0}>
          <Text fontWeight="600" truncate>{title}</Text>
          {subtitle && <Text fontSize="sm" color="fg.muted" truncate>{subtitle}</Text>}
        </HStack>
        {(pill || children) && (
          <HStack gap={1} wrap="wrap">
            {pill && <Badge colorPalette="orange" variant="subtle">{pill}</Badge>}
            {children}
          </HStack>
        )}
      </VStack>
      {right}
      <LuChevronRight />
    </Button>
  );
}

function Note({ children }: { children: ReactNode }) {
  return <Text fontSize="sm" color="fg.muted" px={4} py={2} borderTopWidth="1px" borderColor="border.muted">{children}</Text>;
}

function MoreButton({ left, loading, onClick }: { left: number; loading: boolean; onClick: () => void }) {
  return (
    <HStack justify="center" py={3} borderTopWidth="1px" borderColor="border.muted">
      <Button minH="44px" variant="outline" loading={loading} onClick={onClick}>
        {left <= PAGE ? `Ver los ${left} que quedan` : `Ver ${PAGE} más · quedan ${left}`}
      </Button>
    </HStack>
  );
}
