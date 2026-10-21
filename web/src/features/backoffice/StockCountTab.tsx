import { useEffect, useState } from 'react';
import { medirAccion } from '../../api/uso';
import { Box, Button, Center, HStack, Input, Progress, Spinner, Text, VStack } from '@chakra-ui/react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
} from '../../components/ui/drawer';
import { NumericKeypad } from '../../components/NumericKeypad';
import { pressKey } from '../../components/pressKey';
import { backofficeApi, type CountCaptureBody, type CountClose, type CountItem, type StockCount } from '../../api/backoffice';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';
import { normalize } from '../../utils/format';
import { formatBaseQty } from '../../shared/units';
import { countedWords, diffWords, isBigDiff, looseUnitsOf, packsTotal } from './countEntry';
import { parseNumero } from '../../domain/numeros';
import { ApiError } from '../../api/client';
import { toaster } from '../../components/ui/toaster';
import { CountLossReport } from './CountLossReport';
import { PlaceChooser } from './CountPlaceField';
import { NO_LOCATION } from './useLocations';
import { useNavigate } from 'react-router';
import { can } from '../../app/permissions';

// Conteo de existencias (spec 038, maqueta «Almacén › Conteo»: las dos vistas). A ciegas —no se ve
// lo que el sistema tiene—, cada cifra se guarda al capturarla y el cierre dice qué no se contó y
// qué diferencias son grandes antes de ajustar.

type View = 'lista' | 'uno';
const VIEW_KEY = 'egb:conteo:vista';

function readView(): View {
  try { return localStorage.getItem(VIEW_KEY) === 'uno' ? 'uno' : 'lista'; } catch { return 'lista'; }
}
function saveView(v: View) {
  try { localStorage.setItem(VIEW_KEY, v); } catch { /* la tableta sin almacenamiento cuenta igual */ }
}

const placeOf = (i: CountItem) => i.place ?? NO_LOCATION;

export function StockCountTab() {
  const qc = useQueryClient();
  const hora = useHoraDelNegocio();
  const { data: count, isLoading } = useQuery({ queryKey: ['stock', 'count'], queryFn: backofficeApi.currentCount });
  const [view, setView] = useState<View>(readView);
  const [closed, setClosed] = useState<CountClose | null>(null);
  const [savedAt, setSavedAt] = useState<Date | null>(null);
  const [confirmDiscard, setConfirmDiscard] = useState(false);
  const [closing, setClosing] = useState(false);
  // El conteo que se acaba de cerrar o el último cerrado, para ver lo que se perdió (spec 039).
  const [closedId, setClosedId] = useState<number | null>(null);
  const [showLoss, setShowLoss] = useState(false);
  const canSeeLoss = can('inventory.health_view');
  const canOrder = can('inventory.configure');
  const navigate = useNavigate();
  // El orden del recorrido a la vista antes de empezar, y el camino a cambiarlo: la segunda prueba sin
  // contexto (040, T022) no encontró dónde se ordenan los lugares.
  const { data: places = [] } = useQuery({ queryKey: ['count-places'], queryFn: backofficeApi.countPlaces, enabled: !count });
  const { data: lastClosed } = useQuery({
    queryKey: ['stock', 'count', 'last-closed'], queryFn: backofficeApi.lastClosedCount, enabled: canSeeLoss && !count,
  });
  const lossCountId = closedId ?? lastClosed?.id ?? null;
  useEffect(() => { medirAccion('inventario', 'ver-conteo'); }, []);

  const open = useMutation({
    mutationFn: backofficeApi.openCount,
    onSuccess: (c) => { setClosed(null); qc.setQueryData(['stock', 'count'], c); },
    // Otra tableta lo abrió primero: se toma ése.
    onError: () => void qc.invalidateQueries({ queryKey: ['stock', 'count'] }),
  });
  const discard = useMutation({
    mutationFn: (id: number) => backofficeApi.discardCount(id),
    onSuccess: () => { setConfirmDiscard(false); qc.setQueryData(['stock', 'count'], null); },
  });

  if (isLoading) return <Center py={16}><Spinner size="xl" /></Center>;

  if (!count) {
    return (
      <VStack align="stretch" gap={3} maxW="640px" py={4}>
        {closed && <ClosedSummary result={closed} />}
        <Text>{closed ? 'El conteo quedó cerrado.' : 'No hay un conteo abierto.'}</Text>
        {canSeeLoss && lossCountId !== null && (showLoss
          ? <CountLossReport countId={lossCountId} />
          : (
            <Button alignSelf="start" minH="44px" variant="outline"
              onClick={() => { setShowLoss(true); medirAccion('inventario', 'ver-faltante'); }}>
              {closed ? 'Ver lo que se perdió desde el conteo anterior' : 'Ver lo que se perdió en el último conteo'}
            </Button>
          ))}
        <Text fontSize="sm" color="fg.muted">
          Cuenta lo que hay en el anaquel; al cerrar, lo guardado se corrige con lo que contaste. Lo que
          no cuentes se queda como está.
        </Text>
        {places.length > 0 && (
          <HStack gap={3} flexWrap="wrap">
            <Text fontSize="sm">Se recorre en este orden: {places.map((p) => p.name).join(' → ')}</Text>
            {canOrder && (
              <Button minH="44px" variant="outline" onClick={() => navigate('/negocio#lugares')}>Cambiar el orden ›</Button>
            )}
          </HStack>
        )}
        <Button alignSelf="start" minH="44px" colorPalette="red" loading={open.isPending} onClick={() => open.mutate()}>
          {closed ? 'Empezar otro conteo' : 'Empezar conteo'}
        </Button>
      </VStack>
    );
  }

  const done = count.items.filter((i) => i.countedQty !== null).length;
  const onSaved = (ingredientId: number, qty: string | null) => {
    setSavedAt(new Date());
    qc.setQueryData(['stock', 'count'], (old: StockCount | null | undefined) => old && {
      ...old, items: old.items.map((i) => (i.ingredientId === ingredientId ? { ...i, countedQty: qty } : i)),
    });
  };
  const pickView = (v: View) => { setView(v); saveView(v); };

  return (
    <VStack align="stretch" gap={3} h="100%" minH={0}>
      <HStack gap={3} flexWrap="wrap">
        <Box flex="1" minW="260px">
          <Text fontSize="sm">
            <Text as="span" fontWeight="700" data-testid="avance">{done} de {count.items.length} contados</Text>
            <Text as="span" color="fg.muted"> · empezado {hora.soloHora(count.openedAt)}{savedAt ? ` · ✓ guardado ${hora.soloHora(savedAt)}` : ' · se guarda solo'}</Text>
          </Text>
          <Progress.Root value={count.items.length ? (done / count.items.length) * 100 : 0} size="sm" colorPalette="green" mt={1}>
            <Progress.Track><Progress.Range /></Progress.Track>
          </Progress.Root>
        </Box>
        <HStack gap={0} borderWidth="1px" borderRadius="md" role="group" aria-label="Vista">
          {(['lista', 'uno'] as const).map((v) => (
            <Button key={v} minH="44px" aria-pressed={view === v} variant={view === v ? 'solid' : 'ghost'}
              colorPalette={view === v ? 'red' : 'gray'} onClick={() => pickView(v)}>
              {v === 'lista' ? 'Lista' : 'Uno por uno'}
            </Button>
          ))}
        </HStack>
        {confirmDiscard ? (
          <HStack gap={2}>
            <Text fontSize="sm">¿Descartar? Lo guardado no cambia.</Text>
            <Button minH="44px" colorPalette="red" loading={discard.isPending} onClick={() => discard.mutate(count.id)}>Sí, descartar</Button>
            <Button minH="44px" variant="ghost" onClick={() => setConfirmDiscard(false)}>No</Button>
          </HStack>
        ) : (
          <Button minH="44px" variant="outline" onClick={() => setConfirmDiscard(true)}>Descartar conteo</Button>
        )}
        <VStack gap={0} align="end">
          <Button minH="44px" colorPalette="red" disabled={done === 0} onClick={() => setClosing(true)}>Cerrar conteo</Button>
          {done === 0 && <Text fontSize="xs" color="fg.muted">Cuenta al menos uno para cerrar</Text>}
        </VStack>
      </HStack>

      {view === 'lista'
        ? <ListView count={count} onSaved={onSaved} />
        : <OneByOne count={count} onSaved={onSaved} />}

      {closing && (
        <CloseSheet countId={count.id} onClose={() => setClosing(false)}
          onClosed={(r) => {
            setClosing(false); setClosed(r); setClosedId(count.id); setShowLoss(false);
            qc.setQueryData(['stock', 'count'], null);
            void qc.invalidateQueries({ queryKey: ['stock', 'levels'] });
          }} />
      )}
    </VStack>
  );
}

// ---- Captura de una cifra ----

// Entry son las casillas de un artículo (spec 042, maqueta «Teclado del conteo», opción A): un renglón
// por empaque con − y + para ajustar de uno en uno, y lo suelto con su unidad elegida. Tocar una
// casilla lleva el teclado en pantalla ahí (`inputMode="none"`: el del sistema no se abre).
type Field = number | 'rest';

function useEntry(item: CountItem) {
  const packs = item.presentations;
  const units = looseUnitsOf(item.baseUnitCode, !!item.weighable);
  // Los ids de g y kg, para mandar un peso: solo se piden si el artículo se puede pesar.
  const unitList = useQuery({ queryKey: ['units'], queryFn: backofficeApi.units, enabled: !!item.weighable });
  const weightUnitId = (code: 'g' | 'kg') => unitList.data?.items.find((u) => u.code === code)?.id;
  const prev = item.countedQty !== null ? Number(item.countedQty) : null;
  const [counts, setCounts] = useState<string[]>(() => packs.map(() => ''));
  // Lo ya contado vuelve en lo suelto, en la unidad base: no se sabe cómo se repartió.
  const [rest, setRest] = useState(prev !== null ? String(prev) : '');
  const [unitIdx, setUnitIdx] = useState(0);
  const [field, setField] = useState<Field>(packs.length ? 0 : 'rest');
  const unit = units[unitIdx];
  const restNum = parseNumero(rest);
  const fromPacks = packsTotal(counts, packs.map((p) => p.qtyInBase));
  const captured = fromPacks !== null || restNum.estado === 'valido';
  const invalid = restNum.estado === 'invalido';
  // Sin peso, el total se sabe aquí. Con peso lo calcula el servidor (no se adivina el redondeo).
  const total = !captured || invalid || unit.weightCode ? null
    : Number(((fromPacks ?? 0) + (restNum.estado === 'valido' ? restNum.valor * unit.factor : 0)).toFixed(4));
  const key = (k: string) => {
    if (field === 'rest') setRest((v) => pressKey(v, k, unit.decimals));
    else setCounts((cs) => cs.map((c, i) => (i === field ? pressKey(c, k, false) : c)));
  };
  const bump = (i: number, d: 1 | -1) => setCounts((cs) => cs.map((c, j) => (j === i ? String(Math.max(0, (Number(c) || 0) + d)) : c)));
  const next = () => setField((f) => (f === 'rest' ? (packs.length ? 0 : 'rest') : f + 1 < packs.length ? f + 1 : 'rest'));
  const body = (): CountCaptureBody | null => {
    if (!captured || invalid) return null;
    if (unit.weightCode && restNum.estado === 'valido') {
      const wu = weightUnitId(unit.weightCode);
      return wu ? { countedQty: String(fromPacks ?? 0), weight: String(restNum.valor), weightUnitId: wu, confirmable: true } : null;
    }
    return { countedQty: String(total), confirmable: true };
  };
  return { packs, units, unit, unitIdx, setUnitIdx, counts, rest, field, setField, total, captured, key, bump, next, body,
    weighed: !!unit.weightCode && restNum.estado === 'valido', fromPacks, allowDecimal: field === 'rest' && unit.decimals };
}

function EntryFields({ item, e }: { item: CountItem; e: ReturnType<typeof useEntry> }) {
  const box = (active: boolean) => ({
    w: '96px', minH: '56px', fontSize: '2xl', fontWeight: '800', textAlign: 'center' as const, inputMode: 'none' as const, readOnly: true,
    placeholder: '—', borderWidth: '2px', borderColor: active ? 'red.500' : undefined, bg: active ? 'red.subtle' : undefined,
  });
  const many = e.packs.length > 3;
  const totalLine = (
    <HStack justify="space-between" align="baseline" data-testid="total-contado">
      <Text color="fg.muted">Total contado</Text>
      <Text fontSize="4xl" fontWeight="800" lineHeight="1">
        {e.total !== null ? countedWords(e.total, item.baseUnitCode)
          : e.weighed ? `${e.fromPacks !== null ? `${countedWords(e.fromPacks, item.baseUnitCode)} + ` : ''}${e.rest} ${e.unit.label} pesados`
            : '—'}
      </Text>
    </HStack>
  );
  return (
    <VStack align="stretch" gap={2} flex="1" minH={0}>
      {many && totalLine}
      {e.packs.length > 0 && (
        <VStack align="stretch" gap={0} overflowY={many ? 'auto' : undefined} flex={many ? '1' : undefined} minH={0}>
          <Text fontWeight="700">Empaques cerrados</Text>
          {e.packs.map((p, i) => (
            <HStack key={p.name} minH="56px" borderBottomWidth="1px" gap={3}>
              <Text flex="1" fontSize="lg"><b>{p.name}</b> · {formatBaseQty(p.qtyInBase, item.baseUnitCode)}</Text>
              <Button aria-label={`Quitar ${p.name}`} minH="52px" minW="52px" variant="outline" fontSize="2xl" onClick={() => e.bump(i, -1)}>−</Button>
              <Input aria-label={`${p.name} de ${item.name}`} value={e.counts[i]} {...box(e.field === i)}
                onFocus={() => e.setField(i)} onClick={() => e.setField(i)} />
              <Button aria-label={`Agregar ${p.name}`} minH="52px" minW="52px" variant="outline" fontSize="2xl" onClick={() => e.bump(i, 1)}>+</Button>
            </HStack>
          ))}
        </VStack>
      )}
      <Text fontWeight="700">{e.packs.length ? 'Abierto o suelto' : 'Cuánto hay'}</Text>
      <HStack gap={3}>
        <Input aria-label={`Suelto de ${item.name}`} value={e.rest} {...box(e.field === 'rest')} w="140px"
          onFocus={() => e.setField('rest')} onClick={() => e.setField('rest')} />
        {e.units.length > 1 ? (
          <HStack gap={0} borderWidth="1px" borderRadius="lg" overflow="hidden" role="group" aria-label="Unidad de lo suelto">
            {e.units.map((u, i) => (
              <Button key={u.label} minH="48px" minW="64px" borderRadius="0" fontSize="lg" fontWeight="700"
                variant={e.unitIdx === i ? 'solid' : 'ghost'} colorPalette="gray" aria-pressed={e.unitIdx === i}
                onClick={() => e.setUnitIdx(i)}>{u.label}</Button>
            ))}
          </HStack>
        ) : <Text fontSize="2xl" fontWeight="800">{e.unit.label}</Text>}
      </HStack>
      {!many && <Box pt={2} borderTopWidth="1px">{totalLine}</Box>}
    </VStack>
  );
}

// useCapture guarda pidiendo el aviso (confirmable). Si el servidor dice que la cifra es rara, se pide
// confirmar mostrándola en grande; el servidor no dice contra qué comparó.
function useCapture(countId: number, item: CountItem, onSaved: (id: number, qty: string | null) => void) {
  const [confirming, setConfirming] = useState<CountCaptureBody | null>(null);
  const save = useMutation({
    mutationFn: (b: CountCaptureBody | null) => (b === null
      ? backofficeApi.uncaptureCount(countId, item.ingredientId).then(() => null)
      : backofficeApi.captureCount(countId, item.ingredientId, b)),
    onSuccess: (r, b) => {
      setConfirming(null);
      const qty = b === null ? null : r?.countedQty ?? b.countedQty;
      if (qty !== null) toaster.create({ title: `Guardado: ${countedWords(qty, item.baseUnitCode)}`, type: 'success', duration: 1500 });
      onSaved(item.ingredientId, qty);
    },
    onError: (err, b) => {
      if (b && err instanceof ApiError && err.code === 'NEEDS_CONFIRM') { setConfirming(b); return; }
      toaster.create({ title: err instanceof ApiError && err.status === 400 ? err.message : 'No se pudo guardar', type: 'error' });
    },
  });
  return { save, confirming, setConfirming };
}

// NoHay registra el cero con una confirmación en el mismo lugar: con un toque, un dedazo junto a
// «Guardar» dejaba la existencia en 0 al cerrar el conteo, y contar 0 nunca avisa (revisión de la 042).
function NoHay({ item, onConfirm }: { item: CountItem; onConfirm: () => void }) {
  const [asking, setAsking] = useState(false);
  if (!asking) return <Button minH="44px" variant="outline" onClick={() => setAsking(true)}>No hay</Button>;
  return (
    <HStack gap={2}>
      <Text>¿No hay nada de {item.name}?</Text>
      <Button minH="44px" variant="ghost" onClick={() => setAsking(false)}>No</Button>
      <Button minH="44px" variant="outline" colorPalette="red" onClick={() => { setAsking(false); onConfirm(); }}>Sí, no hay</Button>
    </HStack>
  );
}

// ConfirmFigure es el aviso de una cifra rara: la cifra con su unidad en grande, «Corregir» y «Sí, guardar».
function ConfirmFigure({ item, e, c }: { item: CountItem; e: ReturnType<typeof useEntry>; c: ReturnType<typeof useCapture> }) {
  if (!c.confirming) return null;
  const figure = e.total !== null ? countedWords(e.total, item.baseUnitCode) : `${e.rest} ${e.unit.label}`;
  return (
    <DrawerRoot open placement="bottom" onOpenChange={(x) => { if (!x.open) c.setConfirming(null); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="2xl">
        <DrawerHeader><Text fontWeight="700" fontSize="lg">¿Seguro? Revisa la cifra y la unidad</Text></DrawerHeader>
        <DrawerBody><Text textAlign="center" fontSize="5xl" fontWeight="800" py={2}>{figure}</Text></DrawerBody>
        <DrawerFooter>
          <Button minH="52px" variant="outline" onClick={() => c.setConfirming(null)}>Corregir</Button>
          <Button minH="52px" colorPalette="red" loading={c.save.isPending}
            onClick={() => c.save.mutate({ ...c.confirming!, confirmed: true })}>Sí, guardar</Button>
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}

// PlacePicker pone el lugar del insumo desde el conteo (FR-015f, decisión del dueño del 14-oct):
// nadie lo encontraba en la hoja «Cómo llega». Desde la 040 elige de la lista de lugares de la
// empresa, igual que la hoja del artículo; al guardarlo el conteo se vuelve a ordenar.
function PlacePicker({ item }: { item: CountItem }) {
  const qc = useQueryClient();
  return (
    <Box w="160px" flexShrink={0}>
      <PlaceChooser ingredientId={item.ingredientId} placeId={item.placeId ?? null} title={`¿Dónde está ${item.name}?`}
        onSaved={() => void qc.invalidateQueries({ queryKey: ['stock', 'count'] })} />
    </Box>
  );
}


// ---- Lista ----

function ListView({ count, onSaved }: { count: StockCount; onSaved: (id: number, qty: string | null) => void }) {
  const [q, setQ] = useState('');
  const [editing, setEditing] = useState<CountItem | null>(null);
  const needle = normalize(q.trim());
  // Buscar también por nombre de empaque (spec 042): quien busca «bolsa» encuentra el hielo.
  const shown = needle
    ? count.items.filter((i) => normalize(i.name).includes(needle) || i.presentations.some((p) => normalize(p.name).includes(needle)))
    : count.items;
  const groups = new Map<string, CountItem[]>();
  for (const i of shown) groups.set(placeOf(i), [...(groups.get(placeOf(i)) ?? []), i]);
  return (
    <VStack align="stretch" gap={3} flex="1" minH={0}>
      <Input aria-label="Buscar" placeholder="Buscar…" minH="44px" value={q} onChange={(e) => setQ(e.target.value)} />
      <Box borderWidth="1px" borderRadius="lg" overflowY="auto" flex="1" minH={0} bg="bg.panel">
        {[...groups.entries()].map(([place, items]) => (
          <Box key={place}>
            <Text px={3} py={2} fontWeight="700" bg="bg.subtle">{place}</Text>
            {items.map((i) => (
              <HStack key={i.ingredientId} px={3} py={2} gap={3} borderTopWidth="1px" data-testid="renglon-conteo">
                <VStack align="start" gap={0} flex="1">
                  <Text fontWeight="600">{i.name}</Text>
                  {i.presentations.length > 0 && (
                    <Text fontSize="xs" color="fg.muted">
                      {i.presentations.map((p) => `${p.name} (${formatBaseQty(p.qtyInBase, i.baseUnitCode)})`).join(' · ')}
                    </Text>
                  )}
                </VStack>
                <PlacePicker item={i} />
                <Text fontSize="sm" color={i.countedQty !== null ? 'green.700' : 'fg.muted'} minW="120px">
                  {i.countedQty !== null ? `✓ ${countedWords(i.countedQty, i.baseUnitCode)}` : 'Sin contar'}
                </Text>
                <Button minH="44px" minW="120px" variant="outline" onClick={() => setEditing(i)}>
                  {i.countedQty !== null ? 'Cambiar' : 'Contar'}
                </Button>
              </HStack>
            ))}
          </Box>
        ))}
      </Box>
      {editing && <EntrySheet countId={count.id} item={editing} onClose={() => setEditing(null)}
        onSaved={(id, qty) => { onSaved(id, qty); setEditing(null); }} />}
    </VStack>
  );
}

function EntrySheet({ countId, item, onClose, onSaved }: {
  countId: number; item: CountItem; onClose: () => void; onSaved: (id: number, qty: string | null) => void;
}) {
  const e = useEntry(item);
  const c = useCapture(countId, item, onSaved);
  return (
    <>
    <DrawerRoot open placement="bottom" size="full" onOpenChange={(x) => { if (!x.open) onClose(); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="2xl">
        <DrawerHeader py={2}><Text fontWeight="700" fontSize="lg">¿Cuánto hay de {item.name}?</Text></DrawerHeader>
        <DrawerBody display="flex" minH={0}>
          <HStack align="stretch" gap={6} flex="1" minH={0}>
            <EntryFields item={item} e={e} />
            <Box w="240px" flexShrink={0}><NumericKeypad allowDecimal={e.allowDecimal} onKey={e.key} onNext={e.packs.length ? e.next : undefined} /></Box>
          </HStack>
        </DrawerBody>
        <DrawerFooter>
          {item.countedQty !== null && (
            <Button minH="44px" variant="ghost" mr="auto" onClick={() => c.save.mutate(null)}>Quitar lo contado</Button>
          )}
          <Button minH="44px" variant="outline" onClick={onClose}>Cancelar</Button>
          <NoHay item={item} onConfirm={() => c.save.mutate({ countedQty: '0', confirmable: true })} />
          <Button minH="56px" ml={8} colorPalette="red" disabled={e.body() === null} loading={c.save.isPending} onClick={() => c.save.mutate(e.body())}>
            Guardar
          </Button>
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
    {/* El aviso es su propia hoja, encima de la del teclado. */}
    <ConfirmFigure item={item} e={e} c={c} />
    </>
  );
}

// ---- Uno por uno ----

function OneByOne({ count, onSaved }: { count: StockCount; onSaved: (id: number, qty: string | null) => void }) {
  // Se sigue al INSUMO, no a la posición: poner un lugar reordena la lista, y por posición la tarjeta
  // saltaría a otro insumo a media captura.
  const [atId, setAtId] = useState<number | null>(() => (count.items.find((i) => i.countedQty === null) ?? count.items[0])?.ingredientId ?? null);
  const item = count.items.find((i) => i.ingredientId === atId) ?? count.items[0];
  if (!item) return <Text color="fg.muted">No hay insumos que contar.</Text>;
  const at = count.items.indexOf(item);
  const go = (i: number) => setAtId(count.items[Math.max(0, Math.min(i, count.items.length - 1))].ingredientId);
  const sameplace = count.items.filter((i) => placeOf(i) === placeOf(item));
  return (
    <OneCard key={item.ingredientId} countId={count.id} item={item}
      where={`${placeOf(item)} · ${sameplace.indexOf(item) + 1} de ${sameplace.length}`}
      onPrev={at > 0 ? () => go(at - 1) : undefined}
      onNext={() => go(at + 1)}
      onSaved={onSaved} />
  );
}

function OneCard({ countId, item, where, onPrev, onNext, onSaved }: {
  countId: number; item: CountItem; where: string; onPrev?: () => void; onNext: () => void;
  onSaved: (id: number, qty: string | null) => void;
}) {
  const e = useEntry(item);
  const c = useCapture(countId, item, (id, qty) => { onSaved(id, qty); onNext(); });
  return (
    <HStack align="stretch" gap={3} flex="1" minH={0}>
      <VStack align="stretch" gap={3} flex="1" minH={0} p={4} borderWidth="1px" borderRadius="lg" bg="bg.panel">
        <Text fontSize="xs" color="fg.muted">{where}</Text>
        <HStack gap={3} flexWrap="wrap">
          <Text fontSize="2xl" fontWeight="700" flex="1">{item.name}</Text>
          <PlacePicker item={item} />
        </HStack>
        <EntryFields item={item} e={e} />
        <HStack gap={3}>
          <Button minH="44px" variant="outline" disabled={!onPrev} onClick={onPrev}>‹ Anterior</Button>
          <Box flex="1" />
          <Button minH="44px" variant="outline" onClick={onNext}>Saltar (no contar hoy)</Button>
          <NoHay item={item} onConfirm={() => c.save.mutate({ countedQty: '0', confirmable: true })} />
          <Button minH="56px" ml={8} colorPalette="red" disabled={e.body() === null} loading={c.save.isPending} onClick={() => c.save.mutate(e.body())}>
            Guardar y seguir ›
          </Button>
        </HStack>
      </VStack>
      <Box w="232px"><NumericKeypad allowDecimal={e.allowDecimal} onKey={e.key} onNext={e.packs.length ? e.next : undefined} /></Box>
      <ConfirmFigure item={item} e={e} c={c} />
    </HStack>
  );
}

// ---- Cierre ----

function CloseSheet({ countId, onClose, onClosed }: { countId: number; onClose: () => void; onClosed: (r: CountClose) => void }) {
  const { data: preview, isLoading } = useQuery({ queryKey: ['stock', 'count', countId, 'preview'], queryFn: () => backofficeApi.previewCount(countId) });
  const close = useMutation({
    mutationFn: () => backofficeApi.closeCount(countId),
    onSuccess: (r) => { medirAccion('inventario', 'cerrar-conteo'); onClosed(r); },
  });
  return (
    <DrawerRoot open placement="bottom" onOpenChange={(x) => { if (!x.open) onClose(); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="2xl" maxH="85dvh">
        <DrawerHeader><Text fontWeight="700" fontSize="lg">¿Cerrar el conteo?</Text></DrawerHeader>
        <DrawerBody overflowY="auto">
          {isLoading || !preview ? <Spinner /> : <CloseDetail preview={preview} />}
        </DrawerBody>
        <DrawerFooter>
          <Button minH="44px" variant="outline" onClick={onClose}>Seguir contando</Button>
          <Button minH="44px" colorPalette="red" disabled={!preview} loading={close.isPending} onClick={() => close.mutate()}>
            Sí, cerrar y corregir lo guardado
          </Button>
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}

function CloseDetail({ preview }: { preview: CountClose }) {
  const big = preview.adjusted.filter((a) => isBigDiff(Number(a.delta), Number(a.counted), Number(a.system)));
  return (
    <VStack align="stretch" gap={3} data-testid="cierre">
      {preview.notCounted.length > 0 && (
        <Box p={3} borderRadius="lg" bg="orange.subtle">
          <Text fontWeight="600">Sin contar ({preview.notCounted.length}): se quedan como están</Text>
          <Text fontSize="sm">{preview.notCounted.map((n) => n.name).join(', ')}</Text>
        </Box>
      )}
      {big.length > 0 && <Text fontWeight="600">Revisa {big.length === 1 ? 'esta diferencia grande' : `estas ${big.length} diferencias grandes`} antes de cerrar:</Text>}
      {preview.adjusted.map((a) => {
        const isBig = big.includes(a);
        return (
          <HStack key={a.ingredientId} justify="space-between" p={2} borderRadius="md" bg={isBig ? 'red.subtle' : undefined}>
            <Text>{a.name}: contaste {countedWords(a.counted, a.baseUnitCode)}</Text>
            <Text fontWeight={isBig ? '700' : '400'}>{diffWords(Number(a.delta), a.baseUnitCode)}</Text>
          </HStack>
        );
      })}
      {preview.adjusted.length === 0 && <Text color="fg.muted">Todo lo contado cuadra con lo guardado.</Text>}
    </VStack>
  );
}

function ClosedSummary({ result }: { result: CountClose }) {
  return (
    <Box p={3} borderRadius="lg" bg="green.subtle" data-testid="conteo-cerrado">
      <Text fontWeight="700">Conteo cerrado: {result.adjusted.length} {result.adjusted.length === 1 ? 'insumo corregido' : 'insumos corregidos'}</Text>
      {result.notCounted.length > 0 && <Text fontSize="sm">{result.notCounted.length} sin contar se quedaron como estaban.</Text>}
    </Box>
  );
}
