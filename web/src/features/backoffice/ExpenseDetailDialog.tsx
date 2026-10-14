import { useState } from 'react';
import {
  Box, Button, HStack, VStack, Text, Input, Badge, Center, Spinner, Separator, IconButton,
} from '@chakra-ui/react';
import { LuMinus, LuPlus } from 'react-icons/lu';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';

import { toaster } from '../../components/ui/toaster';
import {
  DialogRoot, DialogBackdrop, DialogContent, DialogBody, DialogHeader, DialogTitle, DialogCloseTrigger, DialogFooter,
} from '../../components/ui/dialog';
import { money } from '../../utils/format';
import { backofficeApi, type ExpenseDetail, type ExpenseItem, type StockLineBody, type Unit, type Presentation } from '../../api/backoffice';
import { ApiError } from '../../api/client';
import { parseNumero, round2 } from '../../domain/numeros';
import { formatBaseQty } from '../../shared/units';
import { posApi } from '../../api/pos';
import { ArrivedItemSheet } from './ArrivedItemSheet';
import { medirAccion } from '../../api/uso';

// Detalle del gasto y «Corregir lo que llegó» (spec 038, maqueta B: editar todo y ver la
// diferencia). El almacén se mueve solo por la diferencia contra lo que cada renglón ya sumó, así
// que corregir dos veces lo mismo no duplica nada. El total pagado no cambia al corregir.

type State = 'llego' | 'despues' | 'no_llega';

interface Edit {
  qty: string; // lo que llegó, en la unidad del renglón
  state: State;
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

  const [editing, setEditing] = useState(false);
  const [edits, setEdits] = useState<Record<number, Edit>>({});
  // «Tendrás X» se vuelve a pedir al servidor después de cada cambio (una venta pudo moverlo);
  // Guardar se habilita cuando la cifra que se ve llegó después del último cambio.
  const [editSeq, setEditSeq] = useState(0);
  const [freshSeq, setFreshSeq] = useState(0);
  const [confirmCancel, setConfirmCancel] = useState(false);
  const [conflict, setConflict] = useState(false);
  const [addingMore, setAddingMore] = useState(false);
  const { data: settings } = useQuery({ queryKey: ['business-settings'], queryFn: posApi.businessSettings, enabled: id !== null });

  const unitList = units?.items ?? [];
  const presList = presentations ?? [];
  const goods = (data?.items ?? []).filter((i) => i.itemType !== null);
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
      setEditing(false); setEdits({});
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
      toaster.create({ title: 'No se pudo corregir', description: String(e), type: 'error' });
    },
  });

  const receiveAll = useMutation({
    mutationFn: () => backofficeApi.receiveExpense(id!, {}),
    onSuccess: async () => { await refetch(); void qc.invalidateQueries({ queryKey: ['stock'] }); onChanged(); },
    onError: (e) => toaster.create({ title: 'No se pudo recibir', description: String(e), type: 'error' }),
  });

  const cancel = useMutation({
    mutationFn: () => backofficeApi.cancelExpense(id!, ''),
    onSuccess: async () => {
      setConfirmCancel(false);
      await refetch();
      void qc.invalidateQueries({ queryKey: ['stock'] });
      onChanged();
    },
    onError: (e) => toaster.create({ title: 'No se pudo anular', description: String(e), type: 'error' }),
  });

  const cancelled = data?.status === 'cancelada';
  const fresh = freshSeq >= editSeq;

  return (
    <DialogRoot open={id !== null} onOpenChange={(e) => { if (!e.open) onClose(); }}
      placement="center" size="cover" scrollBehavior="inside">
      <DialogBackdrop />
      <DialogContent>
        <DialogHeader><DialogTitle>{editing ? 'Corregir lo que llegó' : `Gasto ${id}`}</DialogTitle></DialogHeader>
        <DialogCloseTrigger />
        <DialogBody pb={4}>
          {isLoading || !data ? (
            <Center py={16}><Spinner size="xl" /></Center>
          ) : (
            <VStack align="stretch" gap={4} maxW="1000px" mx="auto">
              <HStack flexWrap="wrap" gap={3}>
                {cancelled
                  ? <Badge colorPalette="gray" size="lg">Anulada</Badge>
                  : <Badge colorPalette={data.status === 'pagada' ? 'green' : 'orange'}>{data.status}</Badge>}
                {data.docKind && <Badge variant="outline">{data.docKind}{data.docFolio ? ` ${data.docFolio}` : ''}</Badge>}
                {!cancelled && goods.length > 0 && (data.receivedAt
                  ? <Badge colorPalette="green">recibida {data.receivedAt}</Badge>
                  : <Badge colorPalette="orange">falta que llegue algo</Badge>)}
              </HStack>
              <Text fontSize="sm" color="fg.muted">
                {data.category} · {data.supplier ?? 'sin proveedor'} · día {data.expenseDate}{data.documentDate ? ` · documento ${data.documentDate}` : ''}
                {data.description ? ` · ${data.description}` : ''}
              </Text>
              <HStack gap={6} flexWrap="wrap">
                <Stat label="Total pagado" value={money(data.amount, data.currency)} />
                <Stat label="Pagado" value={money(data.paid, data.currency)}
                  tone={Number(data.paid) >= Number(data.amount) ? 'green' : 'orange'} />
              </HStack>

              {goods.length > 0 && (
                <>
                  <Separator />
                  <Text fontWeight="700">Lo que llegó{data.receivedAt ? ` · recibido el ${data.receivedAt}` : ''}</Text>
                  {conflict && (
                    <Text p={3} borderRadius="lg" bg="orange.subtle" fontSize="sm">
                      Alguien más corrigió esta compra mientras tanto. Ya ves lo que quedó: vuelve a hacer tu cambio.
                    </Text>
                  )}
                  <VStack align="stretch" gap={2}>
                    {goods.map((i) => (
                      <GoodsRow key={i.id} item={i} edit={editOf(i)} editing={editing} presentations={presList}
                        desired={desiredBase(i)} onEdit={(patch) => setEdit(i, patch)} />
                    ))}
                  </VStack>
                  {editing && (
                    <Button minH="44px" variant="outline" borderStyle="dashed" colorPalette="red" onClick={() => setAddingMore(true)}>
                      + Agregar un {settings?.stockItemLabel || 'artículo'} que faltó
                    </Button>
                  )}
                  {editing && <Text fontSize="sm" color="fg.muted">El total pagado no cambia al corregir lo que llegó.</Text>}
                </>
              )}

              {data.items.some((i) => i.itemType === null) && !editing && (
                <>
                  <Separator />
                  <Text fontWeight="700">Otros del gasto</Text>
                  {data.items.filter((i) => i.itemType === null).map((i) => (
                    <HStack key={i.id} justify="space-between">
                      <Text>{i.description}</Text>
                      <Text>{money(i.amount, data.currency)}</Text>
                    </HStack>
                  ))}
                </>
              )}

              {data.payments.length > 0 && !editing && (
                <>
                  <Separator />
                  <Text fontWeight="700">Pagos</Text>
                  <VStack align="stretch" gap={1}>
                    {data.payments.map((p) => (
                      <HStack key={p.id} justify="space-between" py={1}>
                        <HStack gap={2}>
                          <Text>{p.method}</Text>
                          <Text fontSize="xs" color="fg.subtle">{p.paidOn}</Text>
                          {p.inCashCount && <Badge size="sm" colorPalette="blue">en arqueo</Badge>}
                        </HStack>
                        <Text fontWeight="600">{money(p.amount, data.currency)}</Text>
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

        {addingMore && data && (
          <ArrivedItemSheet expenseId={data.id} supplier={data.supplier} onClose={() => setAddingMore(false)}
            onDone={() => { setAddingMore(false); void refetch(); void qc.invalidateQueries({ queryKey: ['stock'] }); onChanged(); }} />
        )}

        {data && !cancelled && goods.length > 0 && (
          <DialogFooter borderTopWidth="1px">
            {editing ? (
              <HStack w="100%" gap={3}>
                <Text fontSize="sm" flex="1" data-testid="resumen-correccion">
                  {changed.length === 0
                    ? 'Sin cambios todavía'
                    : changed.map((i) => {
                      const d = delta(i);
                      return `${i.itemName ?? i.description}: ${d > 0 ? '+' : d < 0 ? '−' : ''}${formatBaseQty(Math.abs(d), i.baseUnitCode ?? '')}`;
                    }).join(' · ')}
                </Text>
                <Button minH="44px" variant="outline" onClick={() => { setEditing(false); setEdits({}); }}>Cancelar</Button>
                <Button minH="44px" colorPalette="red" disabled={changed.length === 0 || !fresh} loading={save.isPending}
                  onClick={() => save.mutate()}>
                  Guardar
                </Button>
              </HStack>
            ) : (
              <HStack w="100%" gap={3}>
                <Button minH="44px" variant="outline" colorPalette="red" onClick={() => setConfirmCancel(true)}>
                  Anular compra
                </Button>
                <Box flex="1" />
                {!data.receivedAt && (
                  <Button minH="44px" variant="outline" loading={receiveAll.isPending} onClick={() => receiveAll.mutate()}>
                    Llegó todo
                  </Button>
                )}
                <Button minH="44px" colorPalette="red" onClick={() => { setEditing(true); void refetch(); }}>
                  Corregir lo que llegó
                </Button>
              </HStack>
            )}
          </DialogFooter>
        )}
      </DialogContent>
    </DialogRoot>
  );
}

// howItCame dice en palabras cómo llegó el renglón: «3 cajas (12 L c/u)» o «2 kg».
function howItCame(i: ExpenseItem, qty: string, presentations: Presentation[]): string {
  const p = presentations.find((x) => x.id === i.presentationId);
  if (p && i.packQtyInBase) {
    const n = num(qty);
    const name = p.name.toLowerCase();
    return `${qty} ${n === 1 ? name : `${name}s`} (${formatBaseQty(i.packQtyInBase, i.baseUnitCode ?? '')} c/u)`;
  }
  return `${qty} ${i.unitCode ?? ''}`.trim();
}

function GoodsRow({ item, edit, editing, presentations, desired, onEdit }: {
  item: ExpenseItem; edit: Edit; editing: boolean; presentations: Presentation[];
  desired: number; onEdit: (patch: Partial<Edit>) => void;
}) {
  const base = item.baseUnitCode ?? '';
  const onHand = num(item.onHand);
  const willHave = round2(onHand + desired - num(item.applied));
  const states: { v: State; label: string }[] = [
    { v: 'llego', label: 'Llegó' }, { v: 'despues', label: 'Llega después' }, { v: 'no_llega', label: 'No va a llegar' },
  ];
  return (
    <HStack borderWidth="1px" borderRadius="lg" p={3} gap={3} flexWrap="wrap" data-testid="renglon-recibido">
      <VStack align="start" gap={0} flex="1" minW="200px">
        <Text fontWeight="600">{item.itemName ?? item.description}{item.isGift ? ' · regalo' : ''}</Text>
        <Text fontSize="xs" color="fg.muted">
          {stateOf(item) === 'llego' ? `Llegaron ${howItCame(item, item.qtyReceived ?? item.quantity, presentations)}`
            : stateOf(item) === 'no_llega' ? 'No va a llegar' : `Se pidieron ${howItCame(item, item.quantity, presentations)}; no ha llegado`}
        </Text>
        {editing && (
          <Text fontSize="sm" color="fg" data-testid="tendras">
            Tendrás {formatBaseQty(willHave, base)} guardados (hoy {formatBaseQty(onHand, base)})
          </Text>
        )}
      </VStack>
      {editing && (
        <>
          <HStack gap={0} borderWidth="1px" borderRadius="md" opacity={edit.state === 'llego' ? 1 : 0.4}>
            <IconButton aria-label="Menos" variant="ghost" minH="44px" minW="44px" disabled={edit.state !== 'llego'}
              onClick={() => { const n = num(edit.qty); if (n > 0) onEdit({ qty: String(round2(Math.max(0, n - 1))) }); }}>
              <LuMinus />
            </IconButton>
            <Input aria-label="Cuántos llegaron" w="64px" textAlign="center" border="none" inputMode="decimal" value={edit.qty}
              disabled={edit.state !== 'llego'} onChange={(e) => onEdit({ qty: e.target.value })} />
            <IconButton aria-label="Más" variant="ghost" minH="44px" minW="44px" disabled={edit.state !== 'llego'}
              onClick={() => onEdit({ qty: String(round2(num(edit.qty) + 1)) })}>
              <LuPlus />
            </IconButton>
          </HStack>
          <HStack gap={0} borderWidth="1px" borderRadius="md" role="group" aria-label="Estado">
            {states.map((s) => (
              <Button key={s.v} minH="44px" size="sm" aria-pressed={edit.state === s.v}
                variant={edit.state === s.v ? 'solid' : 'ghost'} colorPalette={edit.state === s.v ? 'red' : 'gray'}
                onClick={() => onEdit({ state: s.v })}>
                {s.label}
              </Button>
            ))}
          </HStack>
        </>
      )}
      {!editing && (
        <Text fontSize="sm" color="fg.muted">{formatBaseQty(item.applied, base)} en el almacén</Text>
      )}
    </HStack>
  );
}

// CancelConfirm dice cuánto quedará guardado de cada artículo antes de anular: anular revierte lo
// que la compra sumó.
function CancelConfirm({ data, pending, onYes, onNo }: {
  data: ExpenseDetail; pending: boolean; onYes: () => void; onNo: () => void;
}) {
  const goods = data.items.filter((i) => i.itemType !== null && num(i.applied) !== 0);
  return (
    <VStack align="stretch" gap={2} p={4} borderWidth="2px" borderColor="red.500" borderRadius="lg" data-testid="confirmar-anular">
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
