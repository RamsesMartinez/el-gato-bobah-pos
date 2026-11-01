import { useState } from 'react';
import {
  Box, Button, HStack, VStack, Text, Input, Badge, Center, Spinner, Separator, IconButton, SimpleGrid,
} from '@chakra-ui/react';
import { LuBan, LuMinus, LuPlus, LuX } from 'react-icons/lu';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';

import { toaster } from '../../components/ui/toaster';
import {
  DialogRoot, DialogBackdrop, DialogContent, DialogBody, DialogHeader, DialogTitle, DialogCloseTrigger, DialogFooter,
} from '../../components/ui/dialog';
import { money } from '../../utils/format';
import {
  backofficeApi, type EditExpenseBody, type ExpenseDetail, type ExpenseItem, type StockLineBody, type Unit, type Presentation,
} from '../../api/backoffice';
import { ApiError } from '../../api/client';
import { parseNumero, round2 } from '../../domain/numeros';
import { formatBaseQty } from '../../shared/units';
import { posApi } from '../../api/pos';
import { medirAccion } from '../../api/uso';
import { mensajeDeError } from '../../api/mensajes';
import { can } from '../../app/permissions';
import { useSessionStore } from '../../stores/session';
import { Switch } from '../../components/ui/switch';
import { ConfirmSheet } from '../../components/ConfirmSheet';
import { Picker } from '../../components/Picker';
import { DatePickerButton } from '../../components/DatePickerButton';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';
import { NumericKeypad } from '../../components/NumericKeypad';
import { useKeypadField } from '../../components/useKeypadField';
import { INCLUDE_IN_COUNT_LABEL } from './cajaDelGasto';
import { AddLineSheet, type DraftLine } from './AddLineSheet';
import { DeliverySheet } from './DeliverySheet';
import { EditPreviewSheet } from './EditPreviewSheet';
import { PayDialog } from './ExpensesPage';
import { dia, missingOf, pendingGoods } from './expenseEdit';

// Detalle del gasto (spec 038) con su edición (spec 053, maqueta «Detalle del gasto», opción A: ver y
// cambiar son dos estados). En *Ver* se lee el gasto, sus entregas y sus pagos; «Editar gasto» abre
// el encabezado como campos, los ✕ por renglón y «Agregar renglón». Nada se escribe hasta la vista
// previa. «Corregir lo que llegó» sigue como estaba: el almacén se mueve por la diferencia.

type State = 'llego' | 'despues' | 'no_llega';
type Mode = 'view' | 'edit' | 'correct';

interface Edit {
  qty: string; // lo que llegó, en la unidad del renglón
  state: State;
}

interface HeaderDraft {
  supplierId: string; // '' = sin proveedor
  categoryId: string;
  description: string;
  expenseDate: string;
}

const num = (s: string | null | undefined) => {
  const n = parseNumero(s ?? '');
  return n.estado === 'valido' ? n.valor : 0;
};

function stateOf(i: ExpenseItem): State {
  if (num(i.notComing) > 0 && num(i.qtyReceived) === 0) return 'no_llega';
  if (i.qtyReceived === null || num(i.qtyReceived) === 0) return 'despues';
  return 'llego';
}

// Lo que trae una unidad del renglón en la unidad base del artículo: el contenido copiado del
// empaque, o el factor de la unidad.
function factorOf(i: ExpenseItem, units: Unit[]): number {
  if (i.packQtyInBase) return num(i.packQtyInBase);
  return num(units.find((u) => u.code === i.unitCode)?.toBase ?? '1');
}

const headerOf = (d: ExpenseDetail): HeaderDraft => ({
  supplierId: d.supplierId ? String(d.supplierId) : '', categoryId: d.categoryId ? String(d.categoryId) : '',
  description: d.description ?? '', expenseDate: d.expenseDate,
});

export function ExpenseDetailDialog({ id, onClose, onChanged }: {
  id: number | null; onClose: () => void; onChanged: () => void;
}) {
  const qc = useQueryClient();
  const { data, isLoading, refetch } = useQuery({
    queryKey: ['expense', id],
    queryFn: () => backofficeApi.expenseDetail(id!),
    enabled: id !== null,
  });
  const { data: units } = useQuery({ queryKey: ['units'], queryFn: backofficeApi.units, enabled: id !== null });
  const { data: presentations } = useQuery({ queryKey: ['presentations'], queryFn: backofficeApi.presentations, enabled: id !== null });
  const { data: settings } = useQuery({ queryKey: ['business-settings'], queryFn: posApi.businessSettings, enabled: id !== null });

  const [mode, setMode] = useState<Mode>('view');
  const [edits, setEdits] = useState<Record<number, Edit>>({});
  // «Tendrás X» se vuelve a pedir al servidor después de cada cambio (una venta pudo moverlo);
  // Guardar se habilita cuando la cifra que se ve llegó después del último cambio.
  const [editSeq, setEditSeq] = useState(0);
  const [freshSeq, setFreshSeq] = useState(0);
  const [confirmCancel, setConfirmCancel] = useState(false);
  const [conflict, setConflict] = useState(false);
  // Edición (spec 053): lo que se agrega, lo que se quita y el encabezado, sin guardar todavía.
  const [header, setHeader] = useState<HeaderDraft | null>(null);
  const [added, setAdded] = useState<DraftLine[]>([]);
  const [removed, setRemoved] = useState<Set<number>>(new Set());
  const [adding, setAdding] = useState(false);
  const [reviewing, setReviewing] = useState(false);
  const [receiving, setReceiving] = useState(false);
  const [history, setHistory] = useState<Record<number, boolean>>({});
  const [payDue, setPayDue] = useState<string | null>(null);
  // La casilla de «Corregir lo que llegó» que escribe el teclado en pantalla (revisión de tableta).
  const keypad = useKeypadField<string>();
  const keypadItem = (data?.items ?? []).find((g) => String(g.id) === keypad.field && g.itemType !== null);
  const keypadDecimal = !!keypadItem && !keypadItem.presentationId && keypadItem.unitCode !== 'pieza';

  const unitList = units?.items ?? [];
  const presList = presentations ?? [];
  const user = useSessionStore((s) => s.user);
  const canReceive = can('inventory.receive', user);
  const canEdit = can('expenses.edit', user);
  const hora = useHoraDelNegocio();
  const today = hora.diaDelNegocio(new Date());
  const editing = mode === 'edit';
  const correcting = mode === 'correct';
  const { data: categories } = useQuery({ queryKey: ['expense-categories'], queryFn: backofficeApi.expenseCategories, enabled: editing });
  const { data: suppliers } = useQuery({ queryKey: ['suppliers'], queryFn: backofficeApi.suppliers, enabled: editing });
  const { data: methods } = useQuery({ queryKey: ['payment-methods'], queryFn: posApi.paymentMethods, enabled: payDue !== null });

  const [excludeError, setExcludeError] = useState('');
  const [toExclude, setToExclude] = useState<ExpenseItem | null>(null);
  const goods = (data?.items ?? []).filter((i) => i.itemType !== null && !i.excluded);
  const others = (data?.items ?? []).filter((i) => i.itemType === null && !i.excluded);
  const excludedItems = (data?.items ?? []).filter((i) => i.excluded);
  const editOf = (i: ExpenseItem): Edit => edits[i.id] ?? { qty: i.qtyReceived && num(i.qtyReceived) > 0 ? i.qtyReceived : i.quantity, state: stateOf(i) };

  const desiredBase = (i: ExpenseItem) => {
    const e = editOf(i);
    return e.state === 'llego' ? round2(num(e.qty) * factorOf(i, unitList)) : 0;
  };
  const delta = (i: ExpenseItem) => round2(desiredBase(i) - num(i.applied));
  const changed = goods.filter((i) => !!edits[i.id] && (delta(i) !== 0 || editOf(i).state !== stateOf(i)));

  const setEdit = (i: ExpenseItem, patch: Partial<Edit>) => {
    setEdits((p) => ({ ...p, [i.id]: { ...editOf(i), ...patch } }));
    setConflict(false);
    const seq = editSeq + 1;
    setEditSeq(seq);
    void refetch().then(() => setFreshSeq((f) => Math.max(f, seq)));
  };

  const refresh = async () => {
    await refetch();
    void qc.invalidateQueries({ queryKey: ['stock'] });
    void qc.invalidateQueries({ queryKey: ['expenses'] });
    onChanged();
  };

  const save = useMutation({
    mutationFn: () => backofficeApi.saveStockLines(id!, goods.map((i): StockLineBody => {
      const e = editOf(i);
      const got = e.state === 'llego' ? num(e.qty) : 0;
      // Llegó de más: lo pedido sube con lo que llegó (el importe no cambia: el total pagado es fijo).
      const quantity = Math.max(num(i.quantity), got);
      return {
        id: i.id, seenApplied: i.applied, itemType: i.itemType!,
        ingredientId: i.ingredientId ?? undefined, productId: i.productId ?? undefined,
        description: i.description, quantity: String(quantity), received: String(got),
        notComing: e.state === 'no_llega' ? String(quantity) : '0',
        presentationId: i.presentationId ?? undefined,
        unitId: i.presentationId ? undefined : unitList.find((u) => u.code === i.unitCode)?.id,
        amount: i.amount, isGift: i.isGift || undefined,
      };
    })),
    onSuccess: (fresh) => {
      medirAccion('gastos', 'corregir-compra');
      qc.setQueryData(['expense', id], fresh);
      void qc.invalidateQueries({ queryKey: ['stock'] });
      setMode('view'); setEdits({});
      toaster.create({ title: 'Listo: el almacén quedó corregido', type: 'success' });
      onChanged();
    },
    onError: async (e) => {
      if (e instanceof ApiError && e.status === 409) {
        await refetch();
        setEdits({});
        setConflict(true);
        return;
      }
      toaster.create({ title: 'No se pudo corregir', description: mensajeDeError(e), type: 'error' });
    },
  });

  const toggleExcluded = useMutation({
    mutationFn: ({ itemId, excluded }: { itemId: number; excluded: boolean }) =>
      backofficeApi.setItemExcluded(id!, itemId, excluded),
    onSuccess: async () => { setExcludeError(''); await refresh(); },
    onError: (e) => {
      if (e instanceof ApiError && e.status === 409) {
        setExcludeError('Alguien más está moviendo estos artículos. Intenta de nuevo en un momento.');
        return;
      }
      setExcludeError(e instanceof ApiError && e.status === 400 ? e.message : `No se pudo cambiar: ${mensajeDeError(e)}`);
    },
  });

  const cancel = useMutation({
    mutationFn: () => backofficeApi.cancelExpense(id!, ''),
    onSuccess: async () => { setConfirmCancel(false); await refresh(); },
    onError: (e) => toaster.create({ title: 'No se pudo anular', description: mensajeDeError(e), type: 'error' }),
  });

  const canAdjustCount = can('cash.adjust_count', user);
  const setInCount = useMutation({
    mutationFn: ({ paymentId, include }: { paymentId: number; include: boolean }) =>
      backofficeApi.setPaymentInCount(id!, paymentId, include),
    onSuccess: async () => { await refetch(); void qc.invalidateQueries({ queryKey: ['cash'] }); onChanged(); },
    // Lo que el servidor rechaza (el corte se cerró en otra tableta) se dice y se recarga, para que
    // el interruptor muestre lo que de verdad quedó.
    onError: async (e) => {
      toaster.create({ title: 'No se pudo cambiar', description: mensajeDeError(e), type: 'error' });
      await refetch();
    },
  });

  const startEdit = () => {
    if (!data) return;
    setHeader(headerOf(data)); setAdded([]); setRemoved(new Set()); setMode('edit');
  };
  const stopEdit = () => { setMode('view'); setHeader(null); setAdded([]); setRemoved(new Set()); };

  // El cuerpo de la edición: solo lo que cambió del encabezado, lo agregado y lo quitado.
  const editBody = (): EditExpenseBody => {
    const b: EditExpenseBody = {};
    if (data && header) {
      const h: NonNullable<EditExpenseBody['header']> = {};
      const was = headerOf(data);
      if (header.supplierId !== was.supplierId) h.supplierId = header.supplierId ? Number(header.supplierId) : null;
      if (header.categoryId !== was.categoryId) h.categoryId = Number(header.categoryId);
      if (header.description !== was.description) h.description = header.description;
      if (header.expenseDate !== was.expenseDate) h.expenseDate = header.expenseDate;
      if (Object.keys(h).length > 0) b.header = h;
    }
    if (added.length > 0) b.add = added.map(({ label: _label, ...l }) => l);
    if (removed.size > 0) b.remove = [...removed];
    return b;
  };
  const pendingChanges = (() => {
    const b = editBody();
    return Object.keys(b.header ?? {}).length + (b.add?.length ?? 0) + (b.remove?.length ?? 0);
  })();

  const cancelled = data?.status === 'cancelada';
  const fresh = freshSeq >= editSeq;
  const due = data ? round2(num(data.amount) - num(data.paid)) : 0;
  const pendingToReceive = data ? pendingGoods(data) : [];
  // Lo que suman los renglones que quedarían (sin los quitados, con los nuevos): el importe no baja de
  // lo que tenía sin desglosar.
  const linesTotal = round2([...goods, ...others].filter((i) => !removed.has(i.id)).reduce((s, i) => s + num(i.amount), 0)
    + added.reduce((s, l) => s + num(l.amount), 0));
  const nameOf = (i: ExpenseItem) => i.itemName ?? i.description;

  return (
    <DialogRoot open={id !== null} onOpenChange={(e) => { if (!e.open) onClose(); }}
      placement="center" size="full" scrollBehavior="inside">
      <DialogBackdrop />
      {/* Pantalla completa: «cover» con scroll interno lo dejaba en 400 de los 600 px. */}
      <DialogContent h="100dvh" maxH="100dvh">
        <DialogHeader>
          <DialogTitle>{editing ? 'Editar gasto' : correcting ? 'Corregir lo que llegó' : `Gasto ${id}`}</DialogTitle>
        </DialogHeader>
        <DialogCloseTrigger />
        <DialogBody pb={4}>
          {isLoading || !data ? (
            <Center py={16}><Spinner size="xl" /></Center>
          ) : (
            <VStack align="stretch" gap={3} maxW="1000px" mx="auto">
              {editing && header ? (
                <VStack align="stretch" gap={2}>
                  <SimpleGrid columns={3} gap={3}>
                    <Field label="Proveedor">
                      <Picker value={header.supplierId} onChange={(v) => setHeader({ ...header, supplierId: v })}
                        placeholder="Sin proveedor" title="Proveedor" clearable clearLabel="Sin proveedor"
                        options={(suppliers?.items ?? []).map((s) => ({ value: String(s.id), label: s.name }))} />
                    </Field>
                    <Field label="Categoría">
                      <Picker value={header.categoryId} onChange={(v) => setHeader({ ...header, categoryId: v })}
                        placeholder="Categoría" title="Categoría"
                        options={(categories?.items ?? []).map((c) => ({ value: String(c.id), label: c.name }))} />
                    </Field>
                    <Field label="Fecha del gasto">
                      <DatePickerButton value={header.expenseDate} onChange={(v) => setHeader({ ...header, expenseDate: v })}
                        max={today} label="Fecha del gasto" w="100%" />
                    </Field>
                  </SimpleGrid>
                  <Field label="Comentario">
                    <Input aria-label="Comentario" minH="44px" value={header.description}
                      onChange={(e) => setHeader({ ...header, description: e.target.value })} />
                  </Field>
                  {/* Prueba con usuario nuevo: sin esto no se sabía si agregar renglones duplicaba el gasto. */}
                  <Text fontSize="sm" color="fg.muted" data-testid="importe-en-edicion">
                    Importe {money(data.amount, data.currency)} · pagado {money(data.paid, data.currency)} · los renglones suman {money(linesTotal, data.currency)}.
                    {' '}Lo que agregues ocupa primero lo que el importe no tiene en renglones.
                  </Text>
                </VStack>
              ) : (
                <>
                  <HStack flexWrap="wrap" gap={3}>
                    {cancelled
                      ? <Badge colorPalette="gray" size="lg">Anulada</Badge>
                      : <Badge colorPalette={data.status === 'pagada' ? 'green' : 'orange'}>{data.status}</Badge>}
                    {data.docKind && <Badge variant="outline">{data.docKind}{data.docFolio ? ` ${data.docFolio}` : ''}</Badge>}
                    {!cancelled && goods.length > 0 && (data.receivedAt
                      ? <Badge colorPalette="green">recibida {dia(data.receivedAt)}</Badge>
                      : <Badge colorPalette="orange">falta que llegue algo</Badge>)}
                  </HStack>
                  <Text fontSize="sm" color="fg.muted">
                    {data.category} · {data.supplier ?? 'sin proveedor'} · día {data.expenseDate}{data.documentDate ? ` · documento ${data.documentDate}` : ''}
                    {data.description ? ` · ${data.description}` : ''}
                  </Text>
                  <HStack gap={6} flexWrap="wrap">
                    <Box>
                      <Stat label="Importe" value={money(data.amount, data.currency)} />
                      {data.documentTotal && (
                        <Text fontSize="xs" color="fg.muted">Total del ticket {money(data.documentTotal, data.currency)}</Text>
                      )}
                    </Box>
                    <Stat label="Pagado" value={money(data.paid, data.currency)} tone={due <= 0 ? 'green' : 'orange'} />
                    {/* FR-012 y FR-013: lo que falta o lo que sobra, con palabras además del color. */}
                    {!cancelled && due > 0 && data.payments.length > 0 && <Stat label="⚠ Falta pagar" value={money(due, data.currency)} tone="orange" />}
                    {!cancelled && due < 0 && <Stat label="⚠ Pagado de más" value={money(-due, data.currency)} tone="orange" />}
                  </HStack>
                </>
              )}

              {(goods.length > 0 || others.length > 0 || added.length > 0 || editing) && <Separator />}
              {conflict && (
                <Text p={3} borderRadius="lg" bg="orange.subtle" fontSize="sm">
                  Alguien más corrigió esta compra mientras tanto. Ya ves lo que quedó: vuelve a hacer tu cambio.
                </Text>
              )}
              <VStack align="stretch" gap={2}>
                {goods.map((i) => (
                  <GoodsRow key={i.id} item={i} edit={editOf(i)} correcting={correcting} presentations={presList}
                    qtyRef={keypad.ref(String(i.id))} qtyActive={keypad.field === String(i.id)}
                    onQtyFocus={() => keypad.setField(String(i.id))}
                    desired={desiredBase(i)} onEdit={(patch) => {
                      // Un renglón que ya no «llegó» apaga su casilla: el teclado deja de escribir en ella.
                      if (patch.state && patch.state !== 'llego' && keypad.field === String(i.id)) keypad.setField(null);
                      else if (patch.qty !== undefined) keypad.keep();
                      setEdit(i, patch);
                    }}
                    showHistory={!!history[i.id]} onHistory={() => setHistory((h) => ({ ...h, [i.id]: !h[i.id] }))}
                    removal={editing ? { removed: removed.has(i.id), toggle: () => setRemoved((s) => toggled(s, i.id)) } : undefined}
                    onExclude={canReceive && !cancelled && mode === 'view' ? () => {
                      // Lo que ya entró al almacén sale con la exclusión: eso se confirma (spec 050).
                      if (num(i.applied) !== 0) setToExclude(i);
                      else toggleExcluded.mutate({ itemId: i.id, excluded: true });
                    } : undefined} />
                ))}
                {others.map((i) => (
                  <HStack key={i.id} borderWidth="1px" borderRadius="lg" p={3} gap={3} opacity={removed.has(i.id) ? 0.5 : 1}>
                    <Text flex="1" textDecoration={removed.has(i.id) ? 'line-through' : undefined}>{i.description}</Text>
                    <Text>{money(i.amount, data.currency)}</Text>
                    {editing && <RemoveButton removed={removed.has(i.id)} name={i.description} onToggle={() => setRemoved((s) => toggled(s, i.id))} />}
                  </HStack>
                ))}
                {added.map((l, idx) => (
                  <HStack key={`n-${idx}`} borderWidth="1px" borderRadius="lg" p={3} gap={3} bg="green.subtle" borderColor="green.muted" data-testid="renglon-nuevo">
                    <VStack align="start" gap={0} flex="1">
                      <Text fontWeight="600">{l.label}</Text>
                      <Text fontSize="xs" color="fg.muted">Nuevo · {l.arrivedOn ? `llegó el ${dia(l.arrivedOn)}` : l.itemType ? 'por recibir' : 'sin almacén'}</Text>
                    </VStack>
                    <Text fontWeight="600">{money(l.amount, data.currency)}</Text>
                    <Button minH="44px" variant="outline" onClick={() => setAdded((a) => a.filter((_, j) => j !== idx))}>Deshacer</Button>
                  </HStack>
                ))}
              </VStack>
              {editing && (
                <Button minH="44px" variant="outline" borderStyle="dashed" colorPalette="brand" onClick={() => setAdding(true)}>
                  + Agregar renglón
                </Button>
              )}
              {correcting && <Text fontSize="sm" color="fg.muted">El total pagado no cambia al corregir lo que llegó.</Text>}
              <ConfirmSheet isOpen={toExclude !== null}
                title={`Sacar del almacén lo que llegó de ${toExclude ? nameOf(toExclude) : ''}`}
                description="No es del negocio: se guarda en la compra, pero sale del almacén y del costo."
                cancelLabel="Volver" confirmLabel="Excluir y sacar del almacén" loading={toggleExcluded.isPending}
                onCancel={() => setToExclude(null)}
                onConfirm={() => { const i = toExclude; setToExclude(null); if (i) toggleExcluded.mutate({ itemId: i.id, excluded: true }); }} />

              {excludeError && (
                <Text p={3} borderRadius="lg" bg="red.subtle" fontSize="sm" role="alert">{excludeError}</Text>
              )}

              {excludedItems.length > 0 && mode === 'view' && (
                <VStack align="stretch" gap={1}>
                  {excludedItems.map((i) => (
                    <HStack key={i.id} gap={3} data-testid="renglon-excluido">
                      <Text flex="1" opacity={0.6}>{nameOf(i)} · no es del local</Text>
                      <Text opacity={0.6}>{money(i.amount, data.currency)}</Text>
                      {canReceive && !cancelled && (
                        <Button minH="44px" size="sm" variant="outline" loading={toggleExcluded.isPending}
                          onClick={() => toggleExcluded.mutate({ itemId: i.id, excluded: false })}>
                          Incluir
                        </Button>
                      )}
                    </HStack>
                  ))}
                </VStack>
              )}

              {data.payments.length > 0 && mode === 'view' && (
                <>
                  <Separator />
                  <Text fontWeight="700">Pagos</Text>
                  <VStack align="stretch" gap={1}>
                    {data.payments.map((p) => (
                      <HStack key={p.id} justify="space-between" py={1}>
                        <HStack gap={3}>
                          {/* A la izquierda y lejos del monto (R12): un toque que cambia el esperado del corte. */}
                          {canAdjustCount && p.affectsCashDrawer && p.includeInCount !== undefined && !cancelled && (
                            <Switch minH="44px" checked={p.includeInCount} disabled={p.sessionClosed || setInCount.isPending}
                              onCheckedChange={(e) => setInCount.mutate({ paymentId: p.id, include: e.checked })}>
                              {p.sessionClosed ? 'Corte cerrado' : INCLUDE_IN_COUNT_LABEL}
                            </Switch>
                          )}
                          <Text>{p.method}</Text>
                          <Text fontSize="xs" color="fg.subtle">{p.paidOn}</Text>
                          {!canAdjustCount && p.inCashCount && <Badge size="sm" colorPalette="blue">en arqueo</Badge>}
                        </HStack>
                        <Text fontWeight="600">
                          {money(p.amount, data.currency)}
                          {/* Del cajón salió también lo que no es del negocio (spec 050). */}
                          {Number(p.drawerExtra) > 0 && ` · salieron ${money(round2(num(p.amount) + num(p.drawerExtra)), data.currency)} de la caja`}
                        </Text>
                      </HStack>
                    ))}
                  </VStack>
                </>
              )}

              {confirmCancel && (
                <CancelConfirm data={data} pending={cancel.isPending}
                  onYes={() => cancel.mutate()} onNo={() => setConfirmCancel(false)} />
              )}
            </VStack>
          )}
        </DialogBody>

        {data && !cancelled && (
          <DialogFooter borderTopWidth="1px">
            {correcting && keypadItem ? (
              <HStack w="100%" gap={3} align="end">
                <Box flex="1" maxW="320px">
                  <NumericKeypad allowDecimal={keypadDecimal}
                    onKey={(k) => setEdit(keypadItem, { qty: keypad.apply(editOf(keypadItem).qty, k, keypadDecimal) })} />
                </Box>
                <Box flex="1" />
                <Button minH="56px" colorPalette="brand" onClick={() => keypad.setField(null)}>Listo</Button>
              </HStack>
            ) : correcting ? (
              <HStack w="100%" gap={3}>
                <Text fontSize="sm" flex="1" data-testid="resumen-correccion">
                  {changed.length === 0
                    ? 'Sin cambios todavía'
                    : changed.map((i) => {
                      const d = delta(i);
                      return `${nameOf(i)}: ${d > 0 ? '+' : d < 0 ? '−' : ''}${formatBaseQty(Math.abs(d), i.baseUnitCode ?? '')}`;
                    }).join(' · ')}
                </Text>
                <Button minH="44px" variant="outline" onClick={() => { setMode('view'); setEdits({}); }}>Cancelar</Button>
                <Button minH="44px" colorPalette="brand" disabled={changed.length === 0 || !fresh} loading={save.isPending}
                  onClick={() => save.mutate()}>
                  Guardar
                </Button>
              </HStack>
            ) : editing ? (
              <HStack w="100%" gap={3}>
                <Text fontSize="sm" flex="1" color="fg.muted">
                  {pendingChanges === 0 ? 'Sin cambios todavía' : `${pendingChanges} ${pendingChanges === 1 ? 'cambio' : 'cambios'} sin confirmar`}
                </Text>
                <Button minH="44px" variant="outline" onClick={stopEdit}>Cancelar</Button>
                <Button minH="44px" colorPalette="brand" disabled={pendingChanges === 0} onClick={() => setReviewing(true)}>
                  Revisar cambios
                </Button>
              </HStack>
            ) : (
              <HStack w="100%" gap={3}>
                {data.status === 'pendiente' && (
                  <Button minH="44px" variant="outline" colorPalette="red" onClick={() => setConfirmCancel(true)}>
                    Anular compra
                  </Button>
                )}
                <Box flex="1" />
                {canReceive && goods.length > 0 && (
                  <Button minH="44px" variant="outline" onClick={() => { setMode('correct'); void refetch(); }}>
                    Corregir lo que llegó
                  </Button>
                )}
                {canReceive && pendingToReceive.length > 0 && (
                  <Button minH="44px" variant="outline" onClick={() => setReceiving(true)}>Recibir</Button>
                )}
                {due > 0 && data.status === 'pendiente' && (
                  <Button minH="44px" variant="outline" onClick={() => setPayDue(String(due))}>Registrar pago</Button>
                )}
                {canEdit && (
                  <Button minH="44px" colorPalette="brand" onClick={startEdit}>Editar gasto</Button>
                )}
              </HStack>
            )}
          </DialogFooter>
        )}

        {adding && data && (
          <AddLineSheet expenseDate={header?.expenseDate ?? data.expenseDate} canReceive={canReceive}
            stockItemLabel={settings?.stockItemLabel || 'artículo'} onClose={() => setAdding(false)}
            onAdd={(l) => { setAdded((a) => [...a, l]); setAdding(false); }} />
        )}
        {reviewing && data && (
          <EditPreviewSheet
            request={(preview, decisions, fingerprint) => backofficeApi.editExpense(data.id, { ...editBody(), decisions, fingerprint }, preview)}
            onClose={() => setReviewing(false)}
            onDone={(r, decisions) => {
              medirAccion('gastos', 'editar-gasto');
              setReviewing(false); stopEdit();
              if (r.expense) qc.setQueryData(['expense', id], r.expense);
              void refresh();
              toaster.create({ title: 'Listo: el gasto quedó corregido', type: 'success' });
              if (decisions.money === 'pay') setPayDue(r.preview.money.due);
            }} />
        )}
        {receiving && data && (
          <DeliverySheet expense={data} onClose={() => setReceiving(false)}
            onDone={(r) => {
              setReceiving(false);
              if (r.expense) qc.setQueryData(['expense', id], r.expense);
              void refresh();
              toaster.create({ title: 'Listo: quedó registrado lo que llegó', type: 'success' });
            }} />
        )}
        {payDue !== null && data && (
          <PayDialog expense={data} methods={methods?.items ?? []} defaultAmount={payDue}
            onClose={() => setPayDue(null)} onPaid={() => { setPayDue(null); void refresh(); }} />
        )}
      </DialogContent>
    </DialogRoot>
  );
}

function toggled(s: Set<number>, id: number): Set<number> {
  const n = new Set(s);
  if (n.has(id)) n.delete(id); else n.add(id);
  return n;
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <VStack align="stretch" gap={1}>
      <Text fontSize="xs" color="fg.muted">{label}</Text>
      {children}
    </VStack>
  );
}

function RemoveButton({ removed, name, onToggle }: { removed: boolean; name: string; onToggle: () => void }) {
  return removed
    ? <Button minH="44px" variant="outline" onClick={onToggle}>Deshacer</Button>
    : <Button minH="44px" variant="outline" colorPalette="red" aria-label={`Quitar ${name}`} onClick={onToggle}><LuX /> Quitar</Button>;
}

// howItCame dice en palabras cómo llegó el renglón: «3 cajas (12 L c/u)» o «2 kg».
function howItCame(i: ExpenseItem, qty: string, presentations: Presentation[]): string {
  const p = presentations.find((x) => x.id === i.presentationId);
  if (p && i.packQtyInBase) {
    const n = num(qty);
    const name = p.name.toLowerCase();
    return `${qty} ${n === 1 ? name : `${name}s`} (${formatBaseQty(i.packQtyInBase, i.baseUnitCode ?? '')} c/u)`;
  }
  if (i.unitCode === 'pieza') return `${qty} ${num(qty) === 1 ? 'pieza' : 'piezas'}`;
  return `${qty} ${i.unitCode ?? ''}`.trim();
}

function GoodsRow({ item, edit, correcting, presentations, desired, onEdit, onExclude, showHistory, onHistory, removal, qtyRef, qtyActive, onQtyFocus }: {
  item: ExpenseItem; edit: Edit; correcting: boolean; presentations: Presentation[];
  desired: number; onEdit: (patch: Partial<Edit>) => void; onExclude?: () => void;
  showHistory: boolean; onHistory: () => void;
  removal?: { removed: boolean; toggle: () => void };
  qtyRef?: (el: HTMLElement | null) => void; qtyActive?: boolean; onQtyFocus?: () => void;
}) {
  const base = item.baseUnitCode ?? '';
  const onHand = num(item.onHand);
  const willHave = round2(onHand + desired - num(item.applied));
  const deliveries = item.deliveries ?? [];
  const missing = missingOf(item);
  const states: { v: State; label: string }[] = [
    { v: 'llego', label: 'Llegó' }, { v: 'despues', label: 'Llega después' }, { v: 'no_llega', label: 'No va a llegar' },
  ];
  const name = item.itemName ?? item.description;
  const got = num(item.qtyReceived);
  const status = stateOf(item) === 'no_llega' ? 'No va a llegar'
    : got > 0 && missing > 0
      ? `Se pidieron ${howItCame(item, item.quantity, presentations)} · llegaron ${howItCame(item, item.qtyReceived ?? '0', presentations)}`
      : got > 0
        ? `${got === 1 ? 'Llegó' : 'Llegaron'} ${howItCame(item, item.qtyReceived ?? item.quantity, presentations)}`
        : `${num(item.quantity) === 1 ? 'Se pidió' : 'Se pidieron'} ${howItCame(item, item.quantity, presentations)}; no ha llegado`;
  return (
    <VStack align="stretch" gap={1} borderWidth="1px" borderRadius="lg" p={3} data-testid="renglon-recibido"
      opacity={removal?.removed ? 0.5 : 1}>
      <HStack gap={3} flexWrap="wrap">
        <VStack align="start" gap={0} flex="1" minW="200px">
          <Text fontWeight="600" textDecoration={removal?.removed ? 'line-through' : undefined}>{name}{item.isGift ? ' · regalo' : ''}</Text>
          <Text fontSize="xs" color="fg.muted">{status}</Text>
          {correcting && (
            <Text fontSize="sm" color="fg" data-testid="tendras">
              Tendrás {formatBaseQty(willHave, base)} (hoy {formatBaseQty(onHand, base)})
            </Text>
          )}
        </VStack>
        {!correcting && missing > 0 && stateOf(item) !== 'no_llega' && num(item.qtyReceived) > 0 && (
          <Badge colorPalette="orange">⚠ Faltan {missing}</Badge>
        )}
        {correcting && (
          <>
            <HStack gap={0} borderWidth="1px" borderRadius="md" opacity={edit.state === 'llego' ? 1 : 0.4}>
              <IconButton aria-label="Menos" variant="ghost" minH="44px" minW="44px" disabled={edit.state !== 'llego'}
                onClick={() => { const n = num(edit.qty); if (n > 0) onEdit({ qty: String(round2(Math.max(0, n - 1))) }); }}>
                <LuMinus />
              </IconButton>
              {/* Teclado en pantalla: el del sistema tapaba «Guardar» en la tableta (revisión de la 053). */}
              <Input ref={qtyRef} aria-label="Cuántos llegaron" w="64px" minH="44px" textAlign="center" inputMode="none" readOnly
                value={edit.qty} disabled={edit.state !== 'llego'} borderWidth="2px"
                borderColor={qtyActive ? 'brand.solid' : 'transparent'} onClick={onQtyFocus} onFocus={onQtyFocus} />
              <IconButton aria-label="Más" variant="ghost" minH="44px" minW="44px" disabled={edit.state !== 'llego'}
                onClick={() => onEdit({ qty: String(round2(num(edit.qty) + 1)) })}>
                <LuPlus />
              </IconButton>
            </HStack>
            <HStack gap={0} borderWidth="1px" borderRadius="md" role="group" aria-label="Estado">
              {states.map((s) => (
                <Button key={s.v} minH="44px" size="sm" aria-pressed={edit.state === s.v}
                  variant={edit.state === s.v ? 'solid' : 'ghost'} colorPalette={edit.state === s.v ? 'brand' : 'gray'}
                  onClick={() => onEdit({ state: s.v })}>
                  {s.label}
                </Button>
              ))}
            </HStack>
          </>
        )}
        {!correcting && !removal && (
          <Text fontSize="sm" color="fg.muted">Sumó {formatBaseQty(item.applied, base)} al almacén</Text>
        )}
        {!correcting && deliveries.length > 0 && (
          <Button minH="44px" variant="ghost" size="sm" onClick={onHistory}>
            {showHistory ? 'Ocultar entregas' : `Entregas (${deliveries.length})`}
          </Button>
        )}
        {!correcting && !removal && onExclude && (
          <Button title="Excluir: no es del local" variant="outline" minH="44px" onClick={onExclude}>
            <LuBan /> Excluir
          </Button>
        )}
        {removal && <RemoveButton removed={removal.removed} name={name} onToggle={removal.toggle} />}
      </HStack>
      {showHistory && !correcting && (
        <HStack gap={4} flexWrap="wrap" fontSize="sm" color="fg.muted" data-testid="historial-entregas">
          {deliveries.map((d) => (
            <Text key={d.on}>{dia(d.on)}: {num(d.qtyBase) < 0 ? '−' : ''}{formatBaseQty(Math.abs(num(d.qtyBase)), base)}</Text>
          ))}
        </HStack>
      )}
    </VStack>
  );
}

// CancelConfirm dice cuánto quedará guardado de cada artículo antes de anular: anular revierte lo
// que la compra sumó.
function CancelConfirm({ data, pending, onYes, onNo }: {
  data: ExpenseDetail; pending: boolean; onYes: () => void; onNo: () => void;
}) {
  const goods = data.items.filter((i) => i.itemType !== null && num(i.applied) !== 0);
  return (
    <VStack align="stretch" gap={2} p={4} borderWidth="2px" borderColor="border.error" borderRadius="lg" data-testid="confirmar-anular">
      <Text fontWeight="700">¿Anular esta compra?</Text>
      {goods.length > 0 ? goods.map((i) => (
        <Text key={i.id} fontSize="sm">
          {i.itemName ?? i.description}: quedarán {formatBaseQty(round2(num(i.onHand) - num(i.applied)), i.baseUnitCode ?? '')} guardados
        </Text>
      )) : <Text fontSize="sm">No sumó nada al almacén.</Text>}
      <HStack gap={3}>
        <Button minH="44px" variant="outline" onClick={onNo}>No, dejarla</Button>
        <Button minH="44px" colorPalette="red" loading={pending} onClick={onYes}>Sí, anular</Button>
      </HStack>
    </VStack>
  );
}

function Stat({ label, value, tone }: { label: string; value: string; tone?: string }) {
  return (
    <Box>
      <Text fontSize="xs" color="fg.muted">{label}</Text>
      <Text fontSize="lg" fontWeight="700" color={tone ? `${tone}.600` : undefined}>{value}</Text>
    </Box>
  );
}
