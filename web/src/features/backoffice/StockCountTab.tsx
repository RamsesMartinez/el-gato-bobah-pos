import { createContext, useContext, useEffect, useRef, useState } from 'react';
import { presentationLabel } from '../../shared/presentationLabel';
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
import { matchedAlias, matchesWords } from '../../utils/format';
import { AltMatchTag } from '../../components/AltMatchTag';
import { formatBaseQty } from '../../shared/units';
import { countedWords, diffWords, grossNet, isBigDiff, looseUnitsOf, packsTotal, restoreWeighed, withGross, closedPacksOf } from './countEntry';
import { InfoButton } from '../../components/InfoButton';
import { Picker } from '../../components/Picker';
import { parseNumero } from '../../domain/numeros';
import { ApiError } from '../../api/client';
import { toaster } from '../../components/ui/toaster';
import { CountLossReport } from './CountLossReport';
import { PlaceChooser } from './CountPlaceField';
import { NO_LOCATION } from './useLocations';
import { useNavigate, useSearchParams } from 'react-router';
import { can } from '../../app/permissions';
import { countLimitsApi, DEFAULT_COUNT_LIMITS, overMaxMessage } from '../../api/countLimits';

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

// ---- Teclado físico (spec 044) ----

// KeyTarget es la captura que recibe las teclas: la hoja o la tarjeta abierta se registra al montar.
// Un solo listener en StockCountTab, no uno por teclado: con dos montados cada tecla se escribiría dos veces.
interface KeyTarget { key: (k: string) => void; next?: () => void; save: () => void }
const KeyTargetContext = createContext<{ current: KeyTarget | null } | null>(null);

function useKeyTarget(target: KeyTarget) {
  const ref = useContext(KeyTargetContext);
  const latest = useRef(target);
  useEffect(() => { latest.current = target; });
  useEffect(() => {
    if (!ref) return;
    const proxy: KeyTarget = {
      key: (k) => latest.current.key(k),
      get next() { return latest.current.next; },
      save: () => latest.current.save(),
    };
    // La hoja que se abre encima de la tarjeta (pesar con empaque, spec 058) toma las teclas y al
    // cerrarse se las devuelve a la tarjeta.
    const before = ref.current;
    ref.current = proxy;
    return () => { if (ref.current === proxy) ref.current = before; };
  }, [ref]);
}

// ownsKeys decide si la tecla es del conteo. Las casillas del conteo son de solo lectura (las llena el
// teclado); en otro campo o en un botón la tecla es de ellos (Enter sobre un botón ya hace su clic).
// Otra hoja abierta (el aviso de cifra rara, el lugar) tampoco se la deja al conteo.
function ownsKeys(e: KeyboardEvent): boolean {
  if (e.ctrlKey || e.metaKey || e.altKey) return false;
  const el = e.target instanceof HTMLElement ? e.target : null;
  if (el) {
    if (el.isContentEditable || (el.tagName === 'BUTTON' && !el.closest('[data-keypad]')) || el.tagName === 'SELECT' || el.tagName === 'TEXTAREA') return false;
    if (el.tagName === 'INPUT' && !(el as HTMLInputElement).readOnly) return false;
  }
  const dialogs = [...document.querySelectorAll('[role="dialog"], [role="alertdialog"]')];
  return dialogs.every((d) => d.querySelector('[data-count-entry]') !== null);
}

function keyOf(e: KeyboardEvent): string | null {
  if (/^\d$/.test(e.key)) return e.key;
  if (e.key === '.' || e.key === ',') return '.';
  if (e.key === 'Backspace') return 'back';
  return null;
}

export function StockCountTab() {
  const qc = useQueryClient();
  const hora = useHoraDelNegocio();
  const { data: count, isLoading } = useQuery({ queryKey: ['stock', 'count'], queryFn: backofficeApi.currentCount });
  // ?q= llega de la ficha del artículo (spec 045): la lista ya buscada. Uno por uno no tiene buscador,
  // así que con búsqueda se abre la lista sin cambiar la vista recordada.
  const [params] = useSearchParams();
  const initialQuery = params.get('q') ?? '';
  const [view, setView] = useState<View>(() => (initialQuery ? 'lista' : readView()));
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
  // Se vuelven a leer al abrir el conteo: un cambio de límites aplica a la siguiente captura.
  useQuery({ queryKey: ['count-limits'], queryFn: countLimitsApi.get, refetchOnMount: 'always', retry: false });
  const keyTarget = useRef<KeyTarget | null>(null);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = keyTarget.current;
      if (!t || !ownsKeys(e)) return;
      if (e.key === 'Tab') {
        if (e.shiftKey || !t.next) return;
        e.preventDefault();
        t.next();
        return;
      }
      if (e.key === 'Enter') { e.preventDefault(); t.save(); return; }
      const k = keyOf(e);
      if (k) { e.preventDefault(); t.key(k); }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

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
              <Button minH="44px" variant="outline" onClick={() => navigate('/negocio#lugares')}>Ordenar lugares en Negocio ›</Button>
            )}
          </HStack>
        )}
        {initialQuery && <Text fontWeight="600">Al empezar, la lista ya estará buscando «{initialQuery}».</Text>}
        <Button alignSelf="start" minH="44px" colorPalette="brand" loading={open.isPending} onClick={() => open.mutate()}>
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
    <KeyTargetContext.Provider value={keyTarget}>
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
              colorPalette={view === v ? 'brand' : 'gray'} onClick={() => pickView(v)}>
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
          <Button minH="44px" colorPalette="brand" disabled={done === 0} onClick={() => setClosing(true)}>Cerrar conteo</Button>
          {done === 0 && <Text fontSize="xs" color="fg.muted">Cuenta al menos uno para cerrar</Text>}
        </VStack>
      </HStack>

      {view === 'lista'
        ? <ListView count={count} onSaved={onSaved} initialQuery={initialQuery} />
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
    </KeyTargetContext.Provider>
  );
}

// ---- Captura de una cifra ----

// Entry son las casillas de un artículo (spec 042, maqueta «Teclado del conteo», opción A): un renglón
// por empaque con − y + para ajustar de uno en uno, y lo suelto con su unidad elegida. Tocar una
// casilla lleva el teclado en pantalla ahí (`inputMode="none"`: el del sistema no se abre).
// 'gross' y 'containers' son las casillas de «Pesar con empaque» (spec 058).
type Field = number | 'rest' | 'gross' | 'containers';

// GROSS_UNITS son las unidades de la báscula: siempre peso, aunque el artículo se guarde en ml.
const GROSS_UNITS = [{ label: 'g', factor: 1, code: 'g' as const }, { label: 'kg', factor: 1000, code: 'kg' as const }];

// taredPacks son los empaques con los que se puede pesar: los que dicen cuánto pesan vacíos.
const taredPacks = (item: CountItem) => item.presentations.filter((p) => p.id !== undefined && !!p.tareG);

function useEntry(item: CountItem, opts: { startWeighing?: boolean; restore?: boolean } = {}) {
  const packs = item.presentations;
  const units = looseUnitsOf(item.baseUnitCode, !!item.weighable);
  const tared = item.grossWeighable ? taredPacks(item) : [];
  // Lo contado pesando vuelve con su desglose (#204), solo en la hoja: crecer la tarjeta de «Uno por
  // uno» sacaría sus botones de la pantalla.
  const [restored] = useState(() => (opts.restore && item.countedQty !== null && tared.length > 0 ? restoreWeighed(item.weighed, packs) : null));
  // Los ids de g y kg, para mandar un peso: solo se piden si el artículo se puede pesar.
  const unitList = useQuery({ queryKey: ['units'], queryFn: backofficeApi.units, enabled: !!item.weighable || tared.length > 0 });
  const weightUnitId = (code: 'g' | 'kg') => unitList.data?.items.find((u) => u.code === code)?.id;
  const prev = item.countedQty !== null ? Number(item.countedQty) : null;
  const [counts, setCounts] = useState<string[]>(() => restored?.counts ?? packs.map(() => ''));
  // Sin pesada, lo ya contado vuelve en lo suelto, en la unidad base: no se sabe cómo se repartió.
  const [rest, setRest] = useState(() => ((opts.startWeighing && tared.length > 0) || restored ? '' : prev !== null ? String(prev) : ''));
  const [unitIdx, setUnitIdx] = useState(0);
  // Pesar con empaque y teclear lo suelto son excluyentes: lo abierto se pesa o se teclea.
  const [grossOpen, setGrossOpen] = useState(!!opts.startWeighing || !!restored);
  const [grossWeight, setGrossWeight] = useState(restored?.grossWeight ?? '');
  const [grossUnitIdx, setGrossUnitIdx] = useState(0);
  const [grossPackId, setGrossPackId] = useState<number | null>(restored?.packId ?? null);
  const [containers, setContainers] = useState(restored?.containers ?? '1');
  const weighing = grossOpen && tared.length > 0;
  const [field, setField] = useState<Field>(weighing ? 'gross' : packs.length ? 0 : 'rest');
  const unit = units[unitIdx];
  const restNum = parseNumero(rest);
  const fromPacks = packsTotal(counts, packs.map((p) => p.qtyInBase));
  // El empaque se sigue por id: si al recargar ya no dice cuánto pesa vacío, se toma el primero que sí.
  const grossPack = tared.find((p) => p.id === grossPackId) ?? tared[0] ?? null;
  const grossUnit = GROSS_UNITS[grossUnitIdx];
  const gross = weighing && grossPack ? grossNet({
    weight: grossWeight, weightFactor: grossUnit.factor, tareG: grossPack.tareG!, containers: Number(containers),
    baseCode: item.baseUnitCode, density: item.densityGPerMl,
  }) : null;
  // Con la báscula escrita, la cuenta tiene que salir; vacía, la pesada no cuenta.
  const grossTyped = weighing && grossWeight.trim() !== '';
  const grossOk = gross !== null && gross.ok;
  const captured = fromPacks !== null || (!weighing && restNum.estado === 'valido') || grossOk;
  const invalid = (!weighing && restNum.estado === 'invalido') || (grossTyped && !grossOk);
  // Sin peso, el total se sabe aquí. Con peso lo calcula el servidor (no se adivina el redondeo).
  // Con empaque, la pantalla repite la regla del servidor para mostrar la cuenta; se guarda la suya.
  const total = !captured || invalid || (!weighing && unit.weightCode) ? null
    : grossOk ? withGross(fromPacks, gross.net)
      : Number(((fromPacks ?? 0) + (!weighing && restNum.estado === 'valido' ? restNum.valor * unit.factor : 0)).toFixed(4));
  // Si no se pudieron leer, los de siempre; el servidor decide con los de la empresa.
  const { data: limits = DEFAULT_COUNT_LIMITS } = useQuery({ queryKey: ['count-limits'], queryFn: countLimitsApi.get, retry: false });
  // Con peso el total lo calcula el servidor: ahí el máximo lo dice su 400.
  const overMax = total !== null ? overMaxMessage(limits, item.baseUnitCode, total) : null;
  const fields: Field[] = [...packs.map((_, i) => i), ...(weighing ? ['gross', 'containers'] as const : ['rest'] as const)];
  const key = (k: string) => {
    if (field === 'rest') setRest((v) => pressKey(v, k, unit.decimals));
    else if (field === 'gross') setGrossWeight((v) => pressKey(v, k, true));
    else if (field === 'containers') setContainers((v) => pressKey(v, k, false));
    else setCounts((cs) => cs.map((c, i) => (i === field ? pressKey(c, k, false) : c)));
  };
  const bump = (i: number, d: 1 | -1) => setCounts((cs) => cs.map((c, j) => (j === i ? String(Math.max(0, (Number(c) || 0) + d)) : c)));
  const bumpContainers = (d: 1 | -1) => setContainers((c) => String(Math.max(1, (Number(c) || 0) + d)));
  const next = () => setField((f) => fields[(fields.indexOf(f) + 1) % fields.length]);
  const openWeighing = () => { setGrossOpen(true); setRest(''); setField('gross'); };
  const closeWeighing = () => { setGrossOpen(false); setGrossWeight(''); setContainers('1'); setField('rest'); };
  const body = (): CountCaptureBody | null => {
    if (!captured || invalid || overMax) return null;
    if (grossOk && grossPack) {
      const wu = weightUnitId(grossUnit.code);
      return wu ? {
        countedQty: String(fromPacks ?? 0), confirmable: true,
        gross: { weight: grossWeight.trim(), weightUnitId: wu, presentationId: grossPack.id!, containers: Number(containers),
          seenTareG: grossPack.tareG!, seenDensity: item.densityGPerMl ?? null, closedPacks: closedPacksOf(counts, packs) },
      } : null;
    }
    if (!weighing && unit.weightCode && restNum.estado === 'valido') {
      const wu = weightUnitId(unit.weightCode);
      return wu ? { countedQty: String(fromPacks ?? 0), weight: String(restNum.valor), weightUnitId: wu, confirmable: true } : null;
    }
    return { countedQty: String(total), confirmable: true };
  };
  return { packs, units, unit, unitIdx, setUnitIdx, counts, rest, field, setField, total, captured, key, bump, next, body,
    hasNext: fields.length > 1,
    weighed: !weighing && !!unit.weightCode && restNum.estado === 'valido', fromPacks, overMax,
    allowDecimal: (field === 'rest' && unit.decimals) || field === 'gross',
    tared, weighing, openWeighing, closeWeighing, grossWeight, grossUnitIdx, setGrossUnitIdx, grossPack,
    setGrossPackId, containers, bumpContainers, gross };
}

type Entry = ReturnType<typeof useEntry>;

const entryBox = (active: boolean) => ({
  w: '96px', minH: '56px', fontSize: '2xl', fontWeight: '800', textAlign: 'center' as const, inputMode: 'none' as const, readOnly: true,
  placeholder: '—', borderWidth: '2px', borderColor: active ? 'brand.solid' : undefined, bg: active ? 'brand.subtle' : undefined,
  'data-active': active ? '' : undefined,
});

// WeighRow es el renglón plegado «Pesar con empaque» (spec 058, maqueta Conteo A): una forma
// avanzada y opcional, así que plegado no le estorba a quien no pesa. onOpen decide dónde se abre:
// en su lugar en la hoja, o la hoja completa desde «Uno por uno».
function WeighRow({ open, onOpen, onClose, toMl }: { open: boolean; onOpen: () => void; onClose: () => void; toMl: boolean }) {
  const [info, setInfo] = useState(false);
  return (
    <VStack align="stretch" gap={1}>
      <HStack gap={1}>
        <Button minH="44px" variant="ghost" colorPalette="gray" px={1} aria-expanded={open} onClick={open ? onClose : onOpen}>
          <Text as="span" fontWeight="700">{open ? '▾' : '▸'} Pesar con empaque</Text>
          <Text as="span" fontSize="sm" color="fg.muted" fontWeight="400">· Opcional</Text>
        </Button>
        <InfoButton label="Para qué sirve pesar con empaque" open={info} onToggle={() => setInfo((v) => !v)} />
      </HStack>
      {info && (
        <Text fontSize="sm" color="fg.muted">
          Pon en la báscula los empaques abiertos con lo que traen. Se resta lo que pesan vacíos{toMl ? ' y se convierte a ml' : ''}. No lo sumes también como suelto.
        </Text>
      )}
    </VStack>
  );
}

function WeighFields({ item, e }: { item: CountItem; e: Entry }) {
  const pack = e.grossPack;
  if (!pack) return null;
  const packName = presentationLabel(pack);
  return (
    <VStack align="stretch" gap={2} data-testid="pesar-con-empaque">
      <HStack gap={3}>
        <Text minW="110px">Báscula</Text>
        <Input aria-label={`Peso en la báscula de ${item.name}`} value={e.grossWeight} {...entryBox(e.field === 'gross')} w="140px"
          onFocus={() => e.setField('gross')} onClick={() => e.setField('gross')} />
        <HStack gap={0} borderWidth="1px" borderRadius="lg" overflow="hidden" role="group" aria-label="Unidad de la báscula">
          {GROSS_UNITS.map((u, i) => (
            <Button key={u.label} minH="48px" minW="64px" borderRadius="0" fontSize="lg" fontWeight="700"
              variant={e.grossUnitIdx === i ? 'solid' : 'ghost'} colorPalette="gray" aria-pressed={e.grossUnitIdx === i}
              onClick={() => e.setGrossUnitIdx(i)}>{u.label}</Button>
          ))}
        </HStack>
      </HStack>
      <HStack gap={3}>
        <Text minW="110px">¿Cuál empaque?</Text>
        {e.tared.length > 1 ? (
          <Box flex="1" maxW="320px">
            <Picker title="¿Qué empaque pesaste?" value={String(pack.id)} onChange={(v) => e.setGrossPackId(Number(v))}
              options={e.tared.map((p) => ({ value: String(p.id), label: presentationLabel(p), hint: `pesa vacío ${formatBaseQty(p.tareG!, 'g')}` }))} />
          </Box>
        ) : (
          <Text fontWeight="600">{packName} · pesa vacío {formatBaseQty(pack.tareG!, 'g')}</Text>
        )}
      </HStack>
      <HStack gap={3}>
        <Text minW="110px">Cuántos pesaste</Text>
        <Button aria-label="Quitar un empaque pesado" minH="52px" minW="52px" variant="outline" fontSize="2xl" onClick={() => e.bumpContainers(-1)}>−</Button>
        <Input aria-label={`Empaques pesados de ${item.name}`} value={e.containers} {...entryBox(e.field === 'containers')}
          onFocus={() => e.setField('containers')} onClick={() => e.setField('containers')} />
        <Button aria-label="Agregar un empaque pesado" minH="52px" minW="52px" variant="outline" fontSize="2xl" onClick={() => e.bumpContainers(1)}>+</Button>
      </HStack>
      {e.gross && (e.gross.ok
        ? <Text fontSize="lg" fontWeight="600" data-testid="cuenta-pesada">{e.gross.words}</Text>
        : <Text color="fg.error" fontWeight="700" role="alert">{e.gross.error}</Text>)}
    </VStack>
  );
}

function EntryFields({ item, e, serverError, onWeigh }: {
  item: CountItem; e: Entry; serverError?: string | null;
  // onWeigh abre la pesada en otro lado (la hoja completa) en vez de crecer aquí.
  onWeigh?: () => void;
}) {
  const many = e.packs.length > 3;
  // La casilla activa se trae a la vista al cambiar: con varios empaques y la pesada abierta queda
  // debajo del área con scroll, y el teclado escribiría en una casilla que no se ve.
  const fieldsRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    fieldsRef.current?.querySelector('[data-active]')?.scrollIntoView?.({ block: 'nearest' });
  }, [e.field, e.weighing]);
  const totalLine = (
    <HStack justify="space-between" align="baseline" data-testid="total-contado" flexShrink={0}>
      <Text color="fg.muted">Total contado</Text>
      <Text fontSize="4xl" fontWeight="800" lineHeight="1">
        {e.total !== null ? countedWords(e.total, item.baseUnitCode)
          : e.weighed ? `${e.fromPacks !== null ? `${countedWords(e.fromPacks, item.baseUnitCode)} + ` : ''}${e.rest} ${e.unit.label} pesados`
            : '—'}
      </Text>
    </HStack>
  );
  return (
    <VStack align="stretch" gap={2} flex="1" minH={0} data-count-entry="">
      {many && totalLine}
      {/* Lo capturable se desplaza y el total se queda fijo: con la pesada abierta y varios empaques no
          cabe en 600 px, y «Guardar» no puede salirse de la pantalla. */}
      <VStack ref={fieldsRef} align="stretch" gap={2} flex="1" minH={0} overflowY="auto" data-testid="campos-conteo">
        {e.packs.length > 0 && (
          <VStack align="stretch" gap={0} flexShrink={0}>
            <Text fontWeight="700">Empaques cerrados</Text>
            {e.packs.map((p, i) => (
              <HStack key={p.id ?? `${p.name}-${i}`} minH="56px" borderBottomWidth="1px" gap={3}>
                <Text flex="1" fontSize="lg"><b>{presentationLabel(p)}</b> · {formatBaseQty(p.qtyInBase, item.baseUnitCode)}</Text>
                <Button aria-label={`Quitar ${presentationLabel(p)}`} minH="52px" minW="52px" variant="outline" fontSize="2xl" onClick={() => e.bump(i, -1)}>−</Button>
                <Input aria-label={`${presentationLabel(p)} de ${item.name}`} value={e.counts[i]} {...entryBox(e.field === i)}
                  onFocus={() => e.setField(i)} onClick={() => e.setField(i)} />
                <Button aria-label={`Agregar ${presentationLabel(p)}`} minH="52px" minW="52px" variant="outline" fontSize="2xl" onClick={() => e.bump(i, 1)}>+</Button>
              </HStack>
            ))}
          </VStack>
        )}
        {!e.weighing && (
          <>
            <Text fontWeight="700">{e.packs.length ? 'Abierto o suelto' : 'Cuánto hay'}</Text>
            <HStack gap={3}>
              <Input aria-label={`Suelto de ${item.name}`} value={e.rest} {...entryBox(e.field === 'rest')} w="140px"
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
          </>
        )}
        {e.tared.length > 0 && (
          <Box borderTopWidth="1px" pt={1} flexShrink={0}>
            <WeighRow open={e.weighing} onOpen={onWeigh ?? e.openWeighing} onClose={e.closeWeighing} toMl={item.baseUnitCode === 'ml'} />
            {e.weighing && <WeighFields item={item} e={e} />}
          </Box>
        )}
      </VStack>
      {!many && <Box pt={2} borderTopWidth="1px" flexShrink={0}>{totalLine}</Box>}
      {(e.overMax ?? serverError) && <Text color="fg.error" fontWeight="700" role="alert">{e.overMax ?? serverError}</Text>}
    </VStack>
  );
}

// useCapture guarda pidiendo el aviso (confirmable). Si el servidor dice que la cifra es rara, se pide
// confirmar mostrándola en grande; el servidor no dice contra qué comparó.
function useCapture(countId: number, item: CountItem, onSaved: (id: number, qty: string | null) => void) {
  const qc = useQueryClient();
  const [confirming, setConfirming] = useState<CountCaptureBody | null>(null);
  // El 400 del servidor (p. ej. el máximo) se muestra tal cual junto a la cifra.
  const [serverError, setServerError] = useState<string | null>(null);
  const save = useMutation({
    mutationFn: (b: CountCaptureBody | null) => (b === null
      ? backofficeApi.uncaptureCount(countId, item.ingredientId).then(() => null)
      : backofficeApi.captureCount(countId, item.ingredientId, b)),
    onMutate: () => setServerError(null),
    onSuccess: (r, b) => {
      setConfirming(null);
      if (b?.gross) medirAccion('inventario', 'pesar-con-empaque');
      // Lo contado que se muestra es lo que guardó el servidor: con una pesada, su cuenta manda.
      const qty = b === null ? null : r?.countedQty ?? b.countedQty;
      // El desglose que quedó guardado reemplaza al de antes (#204): sin esto, reabrir mostraba la
      // captura anterior y «Guardar» sin tocar nada deshacía la corrección.
      qc.setQueryData(['stock', 'count'], (old: StockCount | null | undefined) => old && {
        ...old, items: old.items.map((i) => (i.ingredientId === item.ingredientId ? { ...i, weighed: r?.weighed ?? null } : i)),
      });
      if (qty !== null) toaster.create({ title: `Guardado: ${countedWords(qty, item.baseUnitCode)}`, type: 'success', duration: 1500 });
      onSaved(item.ingredientId, qty);
    },
    onError: (err, b) => {
      if (b && err instanceof ApiError && err.code === 'NEEDS_CONFIRM') { setConfirming(b); return; }
      // Alguien cambió lo que pesa vacío el empaque o la densidad: se recarga y se dice, no se adivina.
      if (err instanceof ApiError && err.code === 'COUNT_INPUTS_CHANGED') {
        setConfirming(null);
        setServerError('Lo que pesa vacío el empaque o la densidad cambiaron. Ya se actualizó la cuenta: revísala y vuelve a guardar.');
        void qc.invalidateQueries({ queryKey: ['stock', 'count'] });
        return;
      }
      if (err instanceof ApiError && err.status === 400) { setConfirming(null); setServerError(err.message); return; }
      toaster.create({ title: 'No se pudo guardar', type: 'error' });
    },
  });
  return { save, confirming, setConfirming, serverError };
}

// useCountKeys conecta la captura abierta al teclado físico: Enter es «Guardar» solo si «Guardar» está
// encendido y no hay aviso abierto (el aviso se confirma tocando su botón).
function useCountKeys(e: Entry, c: ReturnType<typeof useCapture>) {
  useKeyTarget({
    key: e.key,
    next: e.hasNext ? e.next : undefined,
    save: () => {
      const b = e.body();
      if (b === null || c.confirming || c.save.isPending) return;
      c.save.mutate(b);
    },
  });
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
      <Button minH="44px" variant="outline" colorPalette="brand" onClick={() => { setAsking(false); onConfirm(); }}>Sí, no hay</Button>
    </HStack>
  );
}

// ConfirmFigure es el aviso de una cifra rara: la cifra con su unidad en grande, «Corregir» y «Sí, guardar».
function ConfirmFigure({ item, e, c }: { item: CountItem; e: Entry; c: ReturnType<typeof useCapture> }) {
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
          <Button minH="52px" colorPalette="brand" loading={c.save.isPending}
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

function ListView({ count, onSaved, initialQuery = '' }: { count: StockCount; onSaved: (id: number, qty: string | null) => void; initialQuery?: string }) {
  const [q, setQ] = useState(initialQuery);
  // Se sigue por id: si el conteo se recarga (p. ej. cambió lo que pesa vacío un empaque), la hoja
  // abierta recibe el artículo nuevo.
  const [editingId, setEditingId] = useState<number | null>(null);
  const editing = count.items.find((i) => i.ingredientId === editingId) ?? null;
  const needle = q.trim();
  // Buscar también por nombre de empaque (spec 042): quien busca «bolsa» encuentra el hielo.
  // Y por sus otros nombres (spec 063), con la misma regla; el que coincidió se señala en la fila.
  const altOf = (i: CountItem) => (needle && !matchesWords(i.name, needle) ? matchedAlias(i.altNames, needle, matchesWords) : undefined);
  const shown = needle
    ? count.items.filter((i) => matchesWords(i.name, needle) || i.presentations.some((p) => matchesWords(presentationLabel(p), needle)) || altOf(i))
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
                <VStack align="start" gap={0} flex="1" minW={0}>
                  <HStack gap={2} minW={0} maxW="100%">
                    <Text fontWeight="600" flexShrink={0}>{i.name}</Text>
                    {altOf(i) && <AltMatchTag name={altOf(i)!} />}
                  </HStack>
                  {i.presentations.length > 0 && (
                    <Text fontSize="xs" color="fg.muted">
                      {i.presentations.map((p) => `${presentationLabel(p)} (${formatBaseQty(p.qtyInBase, i.baseUnitCode)})`).join(' · ')}
                    </Text>
                  )}
                </VStack>
                <PlacePicker item={i} />
                <Text fontSize="sm" color={i.countedQty !== null ? 'fg.success' : 'fg.muted'} minW="120px">
                  {i.countedQty !== null ? `✓ ${countedWords(i.countedQty, i.baseUnitCode)}` : 'Sin contar'}
                </Text>
                <Button minH="44px" minW="120px" variant="outline" onClick={() => setEditingId(i.ingredientId)}>
                  {i.countedQty !== null ? 'Cambiar' : 'Contar'}
                </Button>
              </HStack>
            ))}
          </Box>
        ))}
      </Box>
      {editing && <EntrySheet countId={count.id} item={editing} onClose={() => setEditingId(null)}
        onSaved={(id, qty) => { onSaved(id, qty); setEditingId(null); }} />}
    </VStack>
  );
}

function EntrySheet({ countId, item, onClose, onSaved, startWeighing }: {
  countId: number; item: CountItem; onClose: () => void; onSaved: (id: number, qty: string | null) => void;
  // startWeighing abre con «Pesar con empaque» desplegado (desde «Uno por uno»).
  startWeighing?: boolean;
}) {
  const e = useEntry(item, { startWeighing, restore: true });
  const c = useCapture(countId, item, onSaved);
  useCountKeys(e, c);
  return (
    <>
    <DrawerRoot open placement="bottom" size="full" onOpenChange={(x) => { if (!x.open) onClose(); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="2xl">
        <DrawerHeader py={2}><Text fontWeight="700" fontSize="lg">¿Cuánto hay de {item.name}?</Text></DrawerHeader>
        <DrawerBody display="flex" minH={0}>
          <HStack align="stretch" gap={6} flex="1" minH={0}>
            <EntryFields item={item} e={e} serverError={c.serverError} />
            <Box w="240px" flexShrink={0}><NumericKeypad allowDecimal={e.allowDecimal} onKey={e.key} onNext={e.hasNext ? e.next : undefined} /></Box>
          </HStack>
        </DrawerBody>
        <DrawerFooter>
          {item.countedQty !== null && (
            <Button minH="44px" variant="ghost" mr="auto" onClick={() => c.save.mutate(null)}>Quitar lo contado</Button>
          )}
          <Button minH="44px" variant="outline" onClick={onClose}>Cancelar</Button>
          <NoHay item={item} onConfirm={() => c.save.mutate({ countedQty: '0', confirmable: true })} />
          <Button minH="56px" ml={8} colorPalette="brand" disabled={e.body() === null} loading={c.save.isPending} onClick={() => c.save.mutate(e.body())}>
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
  // Pesar con empaque abre la hoja completa del artículo: crecer la tarjeta sacaría los botones de la
  // pantalla (revisión de arquitectura de la 058).
  const [weighing, setWeighing] = useState(false);
  useCountKeys(e, c);
  return (
    <HStack align="stretch" gap={3} flex="1" minH={0}>
      <VStack align="stretch" gap={3} flex="1" minH={0} p={4} borderWidth="1px" borderRadius="lg" bg="bg.panel">
        <Text fontSize="xs" color="fg.muted">{where}</Text>
        <HStack gap={3} flexWrap="wrap">
          <Text fontSize="2xl" fontWeight="700" flex="1">{item.name}</Text>
          <PlacePicker item={item} />
        </HStack>
        <EntryFields item={item} e={e} serverError={c.serverError} onWeigh={() => setWeighing(true)} />
        <HStack gap={3}>
          <Button minH="44px" variant="outline" disabled={!onPrev} onClick={onPrev}>‹ Anterior</Button>
          <Box flex="1" />
          <Button minH="44px" variant="outline" onClick={onNext}>Saltar (no contar hoy)</Button>
          <NoHay item={item} onConfirm={() => c.save.mutate({ countedQty: '0', confirmable: true })} />
          <Button minH="56px" ml={8} colorPalette="brand" disabled={e.body() === null} loading={c.save.isPending} onClick={() => c.save.mutate(e.body())}>
            Guardar y seguir ›
          </Button>
        </HStack>
      </VStack>
      <Box w="232px"><NumericKeypad allowDecimal={e.allowDecimal} onKey={e.key} onNext={e.hasNext ? e.next : undefined} /></Box>
      <ConfirmFigure item={item} e={e} c={c} />
      {weighing && (
        <EntrySheet countId={countId} item={item} startWeighing onClose={() => setWeighing(false)}
          onSaved={(id, qty) => { setWeighing(false); onSaved(id, qty); onNext(); }} />
      )}
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
          <Button minH="44px" colorPalette="brand" disabled={!preview} loading={close.isPending} onClick={() => close.mutate()}>
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
