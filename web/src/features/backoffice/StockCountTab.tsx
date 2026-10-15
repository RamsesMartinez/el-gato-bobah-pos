import { useEffect, useState } from 'react';
import { medirAccion } from '../../api/uso';
import { Box, Button, Center, HStack, Input, Progress, Spinner, Text, VStack } from '@chakra-ui/react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
} from '../../components/ui/drawer';
import { NumericKeypad } from '../../components/NumericKeypad';
import { Picker } from '../../components/Picker';
import { pressKey } from '../../components/pressKey';
import { backofficeApi, type CountClose, type CountItem, type StockCount } from '../../api/backoffice';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';
import { normalize } from '../../utils/format';
import { formatBaseQty } from '../../shared/units';
import { countedWords, diffWords, entryTotal, isBigDiff, looseUnitOf } from './countEntry';
import { CountLossReport } from './CountLossReport';
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

const placeOf = (i: CountItem) => i.place ?? 'Sin lugar';

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

// Entry son las casillas de un insumo: su primer empaque y lo suelto o abierto. Usan el teclado en
// pantalla (`inputMode="none"`: el del sistema no se abre).
function useEntry(item: CountItem) {
  const pack = item.presentations[0] ?? null;
  const loose = looseUnitOf(item.baseUnitCode);
  const prev = item.countedQty !== null ? Number(item.countedQty) : null;
  // Lo ya contado vuelve en la casilla de suelto: no se sabe cómo se repartió.
  const [packs, setPacks] = useState('');
  const [rest, setRest] = useState(prev !== null ? String(Number((prev / loose.factor).toFixed(4))) : '');
  const [field, setField] = useState<'packs' | 'rest'>(pack ? 'packs' : 'rest');
  const total = entryTotal(packs, pack?.qtyInBase ?? null, rest, loose.factor);
  const key = (k: string) => {
    if (field === 'packs') setPacks((v) => pressKey(v, k, false));
    else setRest((v) => pressKey(v, k, loose.decimals));
  };
  const next = () => setField((f) => (f === 'packs' ? 'rest' : pack ? 'packs' : 'rest'));
  return { pack, loose, packs, rest, field, setField, total, key, next, allowDecimal: field === 'rest' && loose.decimals };
}

function EntryFields({ item, e }: { item: CountItem; e: ReturnType<typeof useEntry> }) {
  const box = (active: boolean) => ({
    w: '96px', minH: '56px', fontSize: 'xl', textAlign: 'center' as const, inputMode: 'none' as const, readOnly: true,
    placeholder: '—', borderColor: active ? 'red.500' : undefined, bg: active ? 'red.subtle' : undefined,
  });
  return (
    <VStack align="start" gap={2}>
      <HStack gap={3} flexWrap="wrap">
        {e.pack && (
          <>
            <Input aria-label={`${e.pack.name} de ${item.name}`} value={e.packs} {...box(e.field === 'packs')}
              onFocus={() => e.setField('packs')} onClick={() => e.setField('packs')} />
            <Text>{e.pack.name.toLowerCase()} <Text as="span" fontSize="xs" color="fg.muted">({formatBaseQty(e.pack.qtyInBase, item.baseUnitCode)})</Text></Text>
            <Text fontWeight="700">+</Text>
          </>
        )}
        <Input aria-label={`${e.loose.label} de ${item.name}`} value={e.rest} {...box(e.field === 'rest')}
          onFocus={() => e.setField('rest')} onClick={() => e.setField('rest')} />
        <Text>{e.loose.label}{e.pack ? ' abiertos o sueltos' : ''}</Text>
      </HStack>
      {e.total !== null && <Text fontWeight="700" color="green.700">= {countedWords(e.total, item.baseUnitCode)}</Text>}
    </VStack>
  );
}

function useCapture(countId: number, item: CountItem, onSaved: (id: number, qty: string | null) => void) {
  return useMutation({
    mutationFn: (total: number | null) => (total === null
      ? backofficeApi.uncaptureCount(countId, item.ingredientId)
      : backofficeApi.captureCount(countId, item.ingredientId, String(total))),
    onSuccess: (_, total) => onSaved(item.ingredientId, total === null ? null : String(total)),
  });
}


// PlacePicker pone el lugar del insumo desde el conteo (FR-015f, decisión del dueño del 14-oct):
// nadie lo encontraba en la hoja «Cómo llega». Ofrece los lugares que la empresa ya usa y uno nuevo
// escribiéndolo; al guardarlo el conteo se vuelve a ordenar por lugar.
function PlacePicker({ item, places }: { item: CountItem; places: string[] }) {
  const qc = useQueryClient();
  const save = useMutation({
    mutationFn: (place: string | null) => backofficeApi.setCountPlace(item.ingredientId, place),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['stock', 'count'] }),
  });
  return (
    <Box w="160px" flexShrink={0}>
      <Picker value={item.place ?? ''} placeholder="Lugar" title={`¿Dónde está ${item.name}?`}
        clearable clearLabel="— Sin lugar —"
        options={places.map((p) => ({ value: p, label: p }))}
        onChange={(v) => save.mutate(v || null)}
        onCreate={async (name) => ({ value: name, label: name })} />
    </Box>
  );
}

const placesOf = (count: StockCount) => [...new Set(count.items.map((i) => i.place).filter((p): p is string => !!p))].sort();

// ---- Lista ----

function ListView({ count, onSaved }: { count: StockCount; onSaved: (id: number, qty: string | null) => void }) {
  const [q, setQ] = useState('');
  const [editing, setEditing] = useState<CountItem | null>(null);
  const needle = normalize(q.trim());
  const shown = needle ? count.items.filter((i) => normalize(i.name).includes(needle)) : count.items;
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
                <PlacePicker item={i} places={placesOf(count)} />
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
  const save = useCapture(countId, item, onSaved);
  return (
    <DrawerRoot open placement="bottom" size="full" onOpenChange={(x) => { if (!x.open) onClose(); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="2xl">
        <DrawerHeader><Text fontWeight="700" fontSize="lg">¿Cuánto hay de {item.name}?</Text></DrawerHeader>
        <DrawerBody>
          <HStack align="start" gap={6}>
            <Box flex="1"><EntryFields item={item} e={e} /></Box>
            <Box w="240px"><NumericKeypad allowDecimal={e.allowDecimal} onKey={e.key} onNext={e.pack ? e.next : undefined} /></Box>
          </HStack>
        </DrawerBody>
        <DrawerFooter>
          {item.countedQty !== null && (
            <Button minH="44px" variant="ghost" mr="auto" onClick={() => save.mutate(null)}>Quitar lo contado</Button>
          )}
          <Button minH="44px" variant="outline" onClick={onClose}>Cancelar</Button>
          <Button minH="44px" colorPalette="red" disabled={e.total === null} loading={save.isPending} onClick={() => save.mutate(e.total)}>
            Guardar
          </Button>
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
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
    <OneCard key={item.ingredientId} countId={count.id} item={item} places={placesOf(count)}
      where={`${placeOf(item)} · ${sameplace.indexOf(item) + 1} de ${sameplace.length}`}
      onPrev={at > 0 ? () => go(at - 1) : undefined}
      onNext={() => go(at + 1)}
      onSaved={onSaved} />
  );
}

function OneCard({ countId, item, places, where, onPrev, onNext, onSaved }: {
  countId: number; item: CountItem; places: string[]; where: string; onPrev?: () => void; onNext: () => void;
  onSaved: (id: number, qty: string | null) => void;
}) {
  const e = useEntry(item);
  const save = useCapture(countId, item, (id, qty) => { onSaved(id, qty); onNext(); });
  return (
    <HStack align="stretch" gap={3} flex="1" minH={0}>
      <VStack align="stretch" justify="center" gap={4} flex="1" p={6} borderWidth="1px" borderRadius="lg" bg="bg.panel">
        <Text fontSize="xs" color="fg.muted">{where}</Text>
        <HStack gap={3} flexWrap="wrap">
          <Text fontSize="2xl" fontWeight="700" flex="1">{item.name}</Text>
          <PlacePicker item={item} places={places} />
        </HStack>
        <Text>¿Cuánto hay?</Text>
        <EntryFields item={item} e={e} />
        <HStack gap={3}>
          <Button minH="44px" variant="outline" disabled={!onPrev} onClick={onPrev}>‹ Anterior</Button>
          <Box flex="1" />
          <Button minH="44px" variant="outline" onClick={onNext}>Saltar (no contar hoy)</Button>
          <Button minH="44px" colorPalette="red" disabled={e.total === null} loading={save.isPending} onClick={() => save.mutate(e.total)}>
            Guardar y seguir ›
          </Button>
        </HStack>
      </VStack>
      <Box w="232px"><NumericKeypad allowDecimal={e.allowDecimal} onKey={e.key} onNext={e.pack ? e.next : undefined} /></Box>
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
