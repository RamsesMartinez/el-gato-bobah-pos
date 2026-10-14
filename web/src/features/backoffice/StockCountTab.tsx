import { useState } from 'react';
import { Box, Button, Center, HStack, Input, Progress, Spinner, Text, VStack } from '@chakra-ui/react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
} from '../../components/ui/drawer';
import { NumericKeypad } from '../../components/NumericKeypad';
import { pressKey } from '../../components/pressKey';
import { backofficeApi, type CountClose, type CountItem, type StockCount } from '../../api/backoffice';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';
import { normalize } from '../../utils/format';
import { formatBaseQty } from '../../shared/units';
import { countedWords, diffWords, entryTotal, isBigDiff, looseUnitOf } from './countEntry';

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
        <Button minH="44px" colorPalette="red" disabled={done === 0} onClick={() => setClosing(true)}>Cerrar conteo</Button>
      </HStack>

      {view === 'lista'
        ? <ListView count={count} onSaved={onSaved} />
        : <OneByOne count={count} onSaved={onSaved} />}

      {closing && (
        <CloseSheet countId={count.id} onClose={() => setClosing(false)}
          onClosed={(r) => {
            setClosing(false); setClosed(r);
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
  const firstPending = count.items.findIndex((i) => i.countedQty === null);
  const [at, setAt] = useState(firstPending >= 0 ? firstPending : 0);
  const item = count.items[Math.min(at, count.items.length - 1)];
  if (!item) return <Text color="fg.muted">No hay insumos que contar.</Text>;
  const sameplace = count.items.filter((i) => placeOf(i) === placeOf(item));
  return (
    <OneCard key={item.ingredientId} countId={count.id} item={item}
      where={`${placeOf(item)} · ${sameplace.indexOf(item) + 1} de ${sameplace.length}`}
      onPrev={at > 0 ? () => setAt(at - 1) : undefined}
      onNext={() => setAt(Math.min(at + 1, count.items.length - 1))}
      onSaved={onSaved} />
  );
}

function OneCard({ countId, item, where, onPrev, onNext, onSaved }: {
  countId: number; item: CountItem; where: string; onPrev?: () => void; onNext: () => void;
  onSaved: (id: number, qty: string | null) => void;
}) {
  const e = useEntry(item);
  const save = useCapture(countId, item, (id, qty) => { onSaved(id, qty); onNext(); });
  return (
    <HStack align="stretch" gap={3} flex="1" minH={0}>
      <VStack align="stretch" justify="center" gap={4} flex="1" p={6} borderWidth="1px" borderRadius="lg" bg="bg.panel">
        <Text fontSize="xs" color="fg.muted">{where}</Text>
        <Text fontSize="2xl" fontWeight="700">{item.name}</Text>
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
  const close = useMutation({ mutationFn: () => backofficeApi.closeCount(countId), onSuccess: onClosed });
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
