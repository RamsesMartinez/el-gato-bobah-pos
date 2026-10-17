import { useMemo, useState } from 'react';
import { Box, Button, Flex, Grid, HStack, Input, Text, VStack } from '@chakra-ui/react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
} from '../../components/ui/drawer';
import { NumericKeypad } from '../../components/NumericKeypad';
import { Picker } from '../../components/Picker';
import { pressKey } from '../../components/pressKey';
import { ApiError } from '../../api/client';
import { medirAccion } from '../../api/uso';
import { moneyExact } from '../../utils/format';
import { WASTE_STATUS_WORDS as STATUS_WORDS, wasteApi, type WasteItems, type WasteReason, type WasteRow } from '../../api/waste';

// Lo que se echó a perder (spec 039, maqueta D: «Una pantalla» y «Paso a paso» con interruptor).
// Llega «Una pantalla» (decisión del dueño); cada tableta recuerda la vista que eligió, como el
// conteo. Lo que resta lo decide el servidor: si pasa el tope de la empresa, queda por aprobar.

type View = 'una' | 'pasos';
const VIEW_KEY = 'egb:merma:vista';
function readView(): View {
  try { return localStorage.getItem(VIEW_KEY) === 'pasos' ? 'pasos' : 'una'; } catch { return 'una'; }
}
function saveView(v: View) {
  try { localStorage.setItem(VIEW_KEY, v); } catch { /* sin almacenamiento, la vista dura la sesión */ }
}

type Item = { kind: 'i' | 'p'; id: number };
// En qué se captura: la unidad base (sin nada que mandar), otra unidad del mismo tipo o un empaque.
type Measure = { kind: 'base' } | { kind: 'unit'; id: number } | { kind: 'pack'; id: number };


export function WastePage() {
  const qc = useQueryClient();
  const [view, setView] = useState<View>(readView);
  const [tab, setTab] = useState<'registrar' | 'hoy'>('registrar');
  const [reasonId, setReasonId] = useState<number | null>(null);
  const [item, setItem] = useState<Item | null>(null);
  const [measure, setMeasure] = useState<Measure>({ kind: 'base' });
  const [qty, setQty] = useState('');
  const [step, setStep] = useState(1);
  const [message, setMessage] = useState<{ text: string; tone: 'ok' | 'warn' | 'error' } | null>(null);
  const [newReason, setNewReason] = useState(false);

  const { data: fetchedReasons = [] } = useQuery({ queryKey: ['merma', 'motivos'], queryFn: wasteApi.reasons });
  // El orden por uso se toma al entrar y no se mueve a media captura: quien toca de memoria le
  // atinaba a otro motivo en cuanto guardaba. Un motivo nuevo va al final.
  const [order, setOrder] = useState<number[] | null>(null);
  if (order === null && fetchedReasons.length > 0) setOrder(fetchedReasons.map((r) => r.id));
  const rank = (id: number) => { const i = order?.indexOf(id) ?? -1; return i < 0 ? Number.MAX_SAFE_INTEGER : i; };
  const reasons = [...fetchedReasons].sort((a, b) => rank(a.id) - rank(b.id));
  const { data: items } = useQuery({ queryKey: ['merma', 'articulos'], queryFn: wasteApi.items });
  const { data: today = [] } = useQuery({ queryKey: ['merma', 'hoy'], queryFn: wasteApi.today });

  const ingredient = item?.kind === 'i' ? items?.ingredients.find((i) => i.id === item.id) : undefined;
  const product = item?.kind === 'p' ? items?.products.find((p) => p.id === item.id) : undefined;
  const allowDecimal = item?.kind === 'i' && measure.kind !== 'pack';
  const ready = reasonId !== null && item !== null && Number(qty) > 0;

  const chooseView = (v: View) => { setView(v); saveView(v); setStep(1); };
  const chooseItem = (it: Item) => { setItem(it); setMeasure({ kind: 'base' }); setQty(''); setMessage(null); if (view === 'pasos') setStep(3); };
  const chooseReason = (id: number) => { setReasonId(id); setMessage(null); if (view === 'pasos') setStep(2); };

  const save = useMutation({
    mutationFn: () => wasteApi.record({
      reasonId: reasonId!, qty,
      ...(item!.kind === 'i' ? { ingredientId: item!.id } : { productId: item!.id }),
      ...(measure.kind === 'unit' ? { unitId: measure.id } : {}),
      ...(measure.kind === 'pack' ? { presentationId: measure.id } : {}),
    }),
    onSuccess: (res) => {
      medirAccion('merma', 'registrar');
      setMessage(res.status === 'pending'
        ? { text: 'Quedó por aprobar: espera aprobación de quien revisa y todavía no se resta.', tone: 'warn' }
        : { text: 'Guardado: ya se restó de lo que tienes.', tone: 'ok' });
      setItem(null); setQty(''); setReasonId(null); setStep(1); setMeasure({ kind: 'base' });
      void qc.invalidateQueries({ queryKey: ['merma'] });
    },
    onError: (e) => setMessage({ text: e instanceof ApiError ? e.message : 'No se pudo guardar; vuelve a intentar.', tone: 'error' }),
  });

  const header = (
    <Flex h="52px" align="center" justify="space-between" px={3} borderBottomWidth="1px" flexShrink={0} gap={2}>
      <HStack gap={2}>
        {view === 'pasos' && tab === 'registrar' && step > 1 && (
          <Button minH="44px" variant="outline" onClick={() => setStep(step - 1)}>Atrás</Button>
        )}
        <Text fontWeight="700" fontSize="lg">Se echó a perder</Text>
      </HStack>
      <HStack gap={1}>
        {(['registrar', 'hoy'] as const).map((t) => (
          <Button key={t} minH="44px" aria-pressed={tab === t} variant={tab === t ? 'solid' : 'outline'} onClick={() => setTab(t)}>
            {t === 'registrar' ? 'Registrar' : `Hoy${today.length ? ` · ${today.length}` : ''}`}
          </Button>
        ))}
        <Box w="12px" />
        {(['una', 'pasos'] as const).map((v) => (
          <Button key={v} minH="44px" aria-pressed={view === v} variant={view === v ? 'subtle' : 'ghost'} onClick={() => chooseView(v)}>
            {v === 'una' ? 'Una pantalla' : 'Paso a paso'}
          </Button>
        ))}
      </HStack>
    </Flex>
  );

  const reasonButtons = (
    <ReasonButtons reasons={reasons} selected={reasonId} onChoose={chooseReason} onOther={() => setNewReason(true)} />
  );
  const itemChooser = <ItemChooser items={items} item={item} onChoose={chooseItem} />;
  const amount = (
    <VStack align="stretch" gap={2} minH={0}>
      <Text fontSize="xs" fontWeight="700" color="fg.muted" textTransform="uppercase" letterSpacing="0.05em">¿Cuánto?</Text>
      <Input aria-label="Cantidad" readOnly inputMode="none" value={qty} placeholder="—" minH="52px" fontSize="2xl" fontWeight="700" />
      {ingredient && (
        <Flex gap={2} wrap="wrap">
          {ingredient.units.map((u) => {
            const active = (u.code === ingredient.unitCode && measure.kind === 'base') || (measure.kind === 'unit' && measure.id === u.id);
            return (
              <Button key={`u${u.id}`} minH="44px" aria-pressed={active} variant={active ? 'solid' : 'outline'}
                onClick={() => setMeasure(u.code === ingredient.unitCode ? { kind: 'base' } : { kind: 'unit', id: u.id })}>
                {u.code}
              </Button>
            );
          })}
          {ingredient.packs.map((p) => {
            const active = measure.kind === 'pack' && measure.id === p.id;
            return (
              <Button key={`p${p.id}`} minH="44px" aria-pressed={active} variant={active ? 'solid' : 'outline'}
                onClick={() => { setMeasure({ kind: 'pack', id: p.id }); setQty((q) => q.replace(/\..*$/, '')); }}>
                {p.name}
              </Button>
            );
          })}
        </Flex>
      )}
      {product && <Text color="fg.muted">En piezas</Text>}
      <NumericKeypad allowDecimal={allowDecimal} onKey={(k) => setQty((q) => pressKey(q, k, allowDecimal))} />
      <Button minH="56px" position="sticky" bottom={0} zIndex={1} colorPalette="red" disabled={!ready || save.isPending} onClick={() => save.mutate()}>Guardar</Button>
    </VStack>
  );

  return (
    <Flex direction="column" h="100%" minH={0}>
      {header}
      {message && (
        <Text role="status" px={3} py={2} fontWeight="600"
          color={message.tone === 'error' ? 'red.600' : message.tone === 'warn' ? 'orange.600' : 'green.700'}>
          {message.text}
        </Text>
      )}
      {tab === 'hoy' ? (
        <TodayList rows={today} />
      ) : view === 'una' ? (
        <Grid templateColumns="minmax(0, 260px) minmax(0, 1fr) minmax(0, 280px)" gap={4} p={2} flex="1" minH={0}>
          <Box minH={0} overflowY="auto">{reasonButtons}</Box>
          <Box minH={0} overflowY="auto">{itemChooser}</Box>
          <Box minH={0} overflowY="auto">{amount}</Box>
        </Grid>
      ) : (
        <Box p={3} flex="1" minH={0} overflowY="auto">
          <Text color="fg.muted" mb={2}>Paso {step} de 3 · {step === 1 ? 'Motivo' : step === 2 ? 'Qué' : 'Cuánto'}</Text>
          {step === 1 && reasonButtons}
          {step === 2 && itemChooser}
          {step === 3 && <Box maxW="360px">{amount}</Box>}
        </Box>
      )}
      <NewReasonSheet open={newReason} onClose={() => setNewReason(false)}
        onChosen={(id) => { setNewReason(false); void qc.invalidateQueries({ queryKey: ['merma', 'motivos'] }); chooseReason(id); }} />
    </Flex>
  );
}

function ReasonButtons({ reasons, selected, onChoose, onOther }: {
  reasons: WasteReason[]; selected: number | null; onChoose: (id: number) => void; onOther: () => void;
}) {
  return (
    <VStack align="stretch" gap={2}>
      <Text fontSize="xs" fontWeight="700" color="fg.muted" textTransform="uppercase" letterSpacing="0.05em">¿Por qué?</Text>
      {reasons.map((r) => (
        <Button key={r.id} minH="52px" justifyContent="flex-start" aria-pressed={selected === r.id}
          variant={selected === r.id ? 'solid' : 'outline'} onClick={() => onChoose(r.id)}>
          {r.name}
        </Button>
      ))}
      <Button minH="52px" variant="ghost" justifyContent="flex-start" onClick={onOther}>Otro motivo…</Button>
    </VStack>
  );
}

function ItemChooser({ items, item, onChoose }: { items?: WasteItems; item: Item | null; onChoose: (it: Item) => void }) {
  const options = useMemo(() => [
    ...(items?.ingredients ?? []).map((i) => ({ value: `i:${i.id}`, label: i.name, hint: i.unitCode })),
    ...(items?.products ?? []).map((p) => ({ value: `p:${p.id}`, label: p.name, hint: 'producto' })),
  ], [items]);
  const nameOf = (kind: 'i' | 'p', id: number) =>
    (kind === 'i' ? items?.ingredients.find((x) => x.id === id)?.name : items?.products.find((x) => x.id === id)?.name) ?? '';
  const recent = (items?.recent ?? []).slice(0, 6);
  const value = item ? `${item.kind}:${item.id}` : '';
  return (
    <VStack align="stretch" gap={2}>
      <Text fontSize="xs" fontWeight="700" color="fg.muted" textTransform="uppercase" letterSpacing="0.05em">¿Qué se echó a perder?</Text>
      <Picker value={value} options={options} placeholder="Buscar artículo o producto" title="¿Qué se echó a perder?"
        onChange={(v) => { const [k, id] = v.split(':'); onChoose({ kind: k as 'i' | 'p', id: Number(id) }); }} />
      {recent.length > 0 && <Text fontSize="xs" color="fg.muted">Recientes</Text>}
      {recent.map((r) => {
        const kind = r.itemType === 'ingrediente' ? 'i' : 'p';
        const active = item?.kind === kind && item.id === r.id;
        return (
          <Button key={`${kind}${r.id}`} minH="48px" justifyContent="flex-start" aria-pressed={active}
            variant={active ? 'solid' : 'outline'} onClick={() => onChoose({ kind, id: r.id })}>
            {nameOf(kind, r.id)}
          </Button>
        );
      })}
    </VStack>
  );
}

function NewReasonSheet({ open, onClose, onChosen }: { open: boolean; onClose: () => void; onChosen: (id: number) => void }) {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [error, setError] = useState<string | null>(null);
  const create = useMutation({
    mutationFn: () => wasteApi.createReason(name, description),
    onSuccess: (r) => { setName(''); setDescription(''); onChosen(r.id); },
    onError: (e) => {
      // Ya existe con ese nombre: se elige el que existe en vez de crear otro igual.
      if (e instanceof ApiError && e.details?.existingId) { setName(''); setDescription(''); onChosen(e.details.existingId); return; }
      setError(e instanceof ApiError ? e.message : 'No se pudo agregar; vuelve a intentar.');
    },
  });
  return (
    <DrawerRoot open={open} onOpenChange={(e) => { if (!e.open) onClose(); }} placement="bottom">
      <DrawerBackdrop />
      <DrawerContent>
        <DrawerHeader><Text fontWeight="700">Otro motivo</Text></DrawerHeader>
        <DrawerBody>
          <VStack align="stretch" gap={3}>
            <Input aria-label="Nombre del motivo" placeholder="Nombre del motivo" minH="44px" maxLength={40} value={name} onChange={(e) => setName(e.target.value)} />
            <Input aria-label="Descripción" placeholder="Descripción: cuándo se usa" minH="44px" maxLength={200} value={description} onChange={(e) => setDescription(e.target.value)} />
            {error && <Text color="red.600">{error}</Text>}
          </VStack>
        </DrawerBody>
        <DrawerFooter>
          <Button minH="44px" variant="ghost" onClick={onClose}>Cancelar</Button>
          <Button minH="44px" disabled={!name.trim() || !description.trim() || create.isPending} onClick={() => create.mutate()}>Agregar motivo</Button>
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}

function TodayList({ rows }: { rows: WasteRow[] }) {
  const qc = useQueryClient();
  const [confirm, setConfirm] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const undo = useMutation({
    mutationFn: (id: number) => wasteApi.undo(id),
    onMutate: () => setError(null),
    onSuccess: () => { medirAccion('merma', 'deshacer'); setConfirm(null); void qc.invalidateQueries({ queryKey: ['merma'] }); },
    onError: (e) => {
      setError(e instanceof ApiError ? e.message : 'No se pudo deshacer; vuelve a intentar.');
      setConfirm(null);
      void qc.invalidateQueries({ queryKey: ['merma'] });
    },
  });
  if (rows.length === 0) return <Text p={3} color="fg.muted">Hoy no se ha registrado nada.</Text>;
  return (
    <Box flex="1" minH={0} overflowY="auto" px={3}>
      {error && <Text role="alert" py={2} color="red.fg">{error}</Text>}
      {rows.map((r) => (
        <Flex key={r.id} data-testid="renglon-merma" minH="56px" align="center" justify="space-between" gap={3} borderBottomWidth="1px" py={1}>
          <Box minW={0}>
            <Text fontWeight="600">{r.itemName} · {r.quantity} {r.unitCode}</Text>
            <Text fontSize="sm" color="fg.muted">{r.reason} · {r.recordedBy} · {STATUS_WORDS[r.status] ?? r.status}{r.totalCost ? ` · ${moneyExact(r.totalCost)}` : ''}</Text>
          </Box>
          {r.canUndo && (confirm === r.id ? (
            <HStack gap={3} flexShrink={0}>
              <Button minH="44px" colorPalette="red" disabled={undo.isPending} onClick={() => undo.mutate(r.id)}>Sí, deshacer</Button>
              <Button minH="44px" variant="ghost" onClick={() => setConfirm(null)}>No</Button>
            </HStack>
          ) : (
            <Button minH="44px" variant="outline" flexShrink={0} onClick={() => setConfirm(r.id)}>Deshacer</Button>
          ))}
        </Flex>
      ))}
    </Box>
  );
}
