import { DiaDelGasto } from './DiaDelGasto';
import { presentationLabel } from '../../shared/presentationLabel';
import { DatePickerButton } from '../../components/DatePickerButton';
import { useEffect, useRef, useState, type ReactNode } from 'react';
import {
  Box, Button, Grid, HStack, chakra, VStack, Text, Input, Badge, IconButton, Separator, Switch as ChakraSwitch,
} from '@chakra-ui/react';
import {
  LuPlus, LuTrash2, LuUpload, LuTriangleAlert, LuSparkles, LuMinus, LuScissors, LuGift, LuCheck, LuClock, LuUser,
  LuChevronRight, LuChevronDown, LuChevronUp, LuStore, LuWarehouse,
} from 'react-icons/lu';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';

import { Picker, type PickerOption } from '../../components/Picker';
import { toaster } from '../../components/ui/toaster';
import { existingByName, nextTempId } from '../../shared/createIngredient';
import {
  DialogRoot, DialogBackdrop, DialogContent, DialogBody, DialogHeader, DialogTitle, DialogCloseTrigger, DialogFooter,
} from '../../components/ui/dialog';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
} from '../../components/ui/drawer';
import { posApi } from '../../api/pos';
import { money } from '../../utils/format';
import { packToBase, lineAmount, purchaseMoney, blankLine, baseOfLine, crossConversion, ASK_FOR_DENSITY, type CrossConversion } from './expenseDraft';
import { DensitySheet } from './articles/DensitySheet';
import { ApiError } from '../../api/client';
import { can } from '../../app/permissions';
import { useSessionStore } from '../../stores/session';
import { useUiStore } from '../../stores/ui';
import {
  backofficeApi, type ExpenseCategory, type Supplier, type ArticleSuggestion, type Article, type Unit,
  type Presentation, type Warehouse, type ExpenseItemBody, type ExpensePaymentBody, type ParsedDoc, type DocLine, type CashRegister,
} from '../../api/backoffice';
import {
  activeRegisters, registerLabel, outsideAskOf, countsInDrawer, withCountChoice, drawerExtraOf, type CountChoice, type OutsideAsk,
} from '../../shared/cajaDelGasto';
import { DrawerOutField, IncludeInCountCheckbox, OutsideCountSheet } from './ExpenseCountControls';
import { montoTecleado, parseNumero, round2 } from '../../domain/numeros';
import { crossBuyUnits, formatBaseQty, purchaseUnitsFor, unitLabel } from '../../shared/units';
import { PresentationsSheet } from './PresentationsSheet';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';
import { medirAccion } from '../../api/uso';
import { Segmented } from '../../components/Segmented';
import { InfoButton } from '../../components/InfoButton';
import { useLastWarehouse } from './expense/useLastWarehouse';

// Alta de una compra (spec 038, maqueta «Captura de una compra», opción C: primero el ticket).
//
// El camino normal es volver de la tienda con el ticket: se sube la foto, los renglones llegan
// llenos y el operador confirma. Capturar a mano queda a un toque, y sirve también para un gasto
// sin mercancía (luz, gas, renta), que nunca toca el almacén.
//
// Nada se guarda sin que el operador confirme: una extracción equivocada cuesta una corrección,
// no un inventario mal.

// Línea en edición. Se guarda el texto original del documento (rawName/rawCode) aparte de la
// descripción porque es la LLAVE con la que el backend aprende el mapeo del proveedor.
interface DraftItem {
  key: string;
  rawCode: string;
  rawName: string;
  description: string;
  itemType: '' | 'ingrediente' | 'producto';
  itemId: number | null;
  itemName: string;
  quantity: string;
  unitId: number | null;
  presentationId: number | null;
  amount: string;
  packQtyInBase: string;
  // arrived: «Llegó» (todo o una parte) o «Llega después». Lo que no llegó queda en «Por recibir».
  arrived: boolean;
  // partQty: con «Llegó una parte», cuántos llegaron (FR-007e); null = llegó todo o llega después.
  partQty: string | null;
  isGift: boolean;
  // doubtful: el ticket sugiere un artículo que nadie ha confirmado. No suma hasta que se diga.
  doubtful: boolean;
  suggestion: ArticleSuggestion | null;
  suggestions: ArticleSuggestion[];
  docStatus: DocLine['status'];
  // excluded: venía en el ticket pero no es del local (el shampoo de la casa). Conserva su artículo,
  // no suma al gasto ni toca el almacén; se manda para que el proveedor lo recuerde (spec 049).
  excluded: boolean;
  // warehouseId: a qué almacén entra este renglón cuando no es el del ticket; null = el del ticket
  // («¿Dónde se guarda?» de arriba). Solo se pregunta con dos o más almacenes (spec 094).
  warehouseId: number | null;
}

interface DraftPayment {
  key: string;
  methodId: string;
  amount: string;
  paidOn: string;
  registerId: string;
  reference: string;
  includeInCount: boolean;
  // drawerOut: «Salió de la caja» (spec 050); null = no lo tocaron, salió lo mismo que el importe.
  drawerOut: string | null;
}

let seq = 0;
const nextKey = () => `k${++seq}`;

const emptyItem = (): DraftItem => ({
  key: nextKey(), rawCode: '', rawName: '', description: '', itemType: '', itemId: null,
  itemName: '', quantity: '1', unitId: null, presentationId: null, amount: '', packQtyInBase: '',
  arrived: true, partQty: null, isGift: false, doubtful: false, suggestion: null, suggestions: [], docStatus: '', excluded: false,
  warehouseId: null,
});

const emptyPayment = (paidOn: string): DraftPayment => ({
  key: nextKey(), methodId: '', amount: '', paidOn, registerId: '', reference: '', includeInCount: true, drawerOut: null,
});

// En qué se mide un insumo nuevo, y la unidad base que le toca (regla 4: se elige al crearlo).
const KINDS: { kind: Unit['kind']; label: string; base: string }[] = [
  { kind: 'masa', label: 'Peso (g, kg)', base: 'g' },
  { kind: 'volumen', label: 'Líquido (ml, L)', base: 'ml' },
  { kind: 'longitud', label: 'Longitud (cm, m)', base: 'cm' },
  { kind: 'pieza', label: 'Piezas', base: 'pieza' },
];

// plural de la palabra configurable («artículo» → «artículos», «pan» → «panes»).
const plural = (w: string) => (/[aeiouáéíóú]$/i.test(w) ? `${w}s` : `${w}es`);

type Step = 'start' | 'ask' | 'capture';

export function ExpenseDialog({ open, onClose, onSaved }: {
  open: boolean; onClose: () => void; onSaved: () => void;
}) {
  const qc = useQueryClient();
  const palette = useUiStore((s) => s.palette);
  const { data: cats } = useQuery({ queryKey: ['expense-cats'], queryFn: backofficeApi.expenseCategories, enabled: open });
  const { data: sups } = useQuery({ queryKey: ['suppliers'], queryFn: backofficeApi.suppliers, enabled: open });
  const { data: methods } = useQuery({ queryKey: ['payment-methods'], queryFn: posApi.paymentMethods, enabled: open });
  const { data: registers } = useQuery({ queryKey: ['cash', 'registers'], queryFn: backofficeApi.cashRegisters, enabled: open });
  const { data: units } = useQuery({ queryKey: ['units'], queryFn: backofficeApi.units, enabled: open });
  const { data: settings } = useQuery({ queryKey: ['business-settings'], queryFn: posApi.businessSettings, enabled: open });
  const { data: presentations } = useQuery({ queryKey: ['presentations'], queryFn: backofficeApi.presentations, enabled: open });
  // El catálogo completo, una sola vez: el picker filtra en local (buscar no debe costar un
  // round-trip por tecla en un tablet).
  const { data: articles } = useQuery({ queryKey: ['articles'], queryFn: () => backofficeApi.searchArticles(''), enabled: open });
  const { data: lastCount } = useQuery({ queryKey: ['stock', 'count', 'last-closed'], queryFn: backofficeApi.lastClosedCount, enabled: open });
  const hora = useHoraDelNegocio();
  const canDensity = can('inventory.configure', useSessionStore((s) => s.user));
  // La hoja de densidad de un artículo, encima del gasto (dos capas como máximo, spec 058).
  const [densityFor, setDensityFor] = useState<{ id: number; name: string } | null>(null);
  // El artículo que el servidor rechazó por falta de densidad al guardar (UNIT_KIND_MISMATCH).
  const [mismatch, setMismatch] = useState<{ id: number; name: string } | null>(null);
  // El día del negocio, no el de UTC: a las 11:30 pm en México `toISOString` ya dice mañana.
  const today = () => hora.diaDelNegocio(new Date());

  const [step, setStep] = useState<Step>('start');
  const [withGoods, setWithGoods] = useState(true);

  // ---- Encabezado ----
  const [expenseDate, setExpenseDate] = useState(today());
  const [expenseDay, setExpenseDay] = useState(today());
  const { data: caja } = useQuery({ queryKey: ['cash-status'], queryFn: posApi.cashStatus, enabled: open });
  const turno = caja?.open ? caja.businessDate ?? null : null;
  const [categoryId, setCategoryId] = useState('');
  const [supplierId, setSupplierId] = useState('');
  const [amount, setAmount] = useState('');
  const [description, setDescription] = useState('');
  const [docKind, setDocKind] = useState('');
  const [docFolio, setDocFolio] = useState('');
  const [docRaw, setDocRaw] = useState<unknown>(undefined);

  const [items, setItems] = useState<DraftItem[]>([]);
  const [payments, setPayments] = useState<DraftPayment[]>([]);
  // '' sin contestar, 'later' «Todavía no se paga», o el id del método.
  const [payWith, setPayWith] = useState('');
  const [payRegister, setPayRegister] = useState('');
  const [parsed, setParsed] = useState<ParsedDoc | null>(null);
  const [created, setCreated] = useState<Article[]>([]);
  // Los artículos nuevos de este gasto, por su id provisional (negativo): viajan en el guardado y solo
  // existen si el gasto se guarda (spec 042, FR-003).
  const [pendingNew, setPendingNew] = useState<Record<number, { name: string; baseUnitId: number }>>({});
  const [packsOf, setPacksOf] = useState<Article | null>(null);
  const kindAsk = useRef<((k: Unit['kind'] | null) => void) | null>(null);
  const [askingKind, setAskingKind] = useState(false);
  // Almacenes (spec 094): «¿Dónde se guarda?» solo aparece con dos o más. El del ticket arranca en el
  // último que se eligió en este equipo (T045).
  const whQuery = useQuery({ queryKey: ['warehouses'], queryFn: () => backofficeApi.warehouses(), enabled: open });
  const whList = whQuery.data?.items ?? [];
  const multiWh = whList.length > 1;
  const [ticketWh, setTicketWh] = useLastWarehouse(whList);
  // Un renglón cuyo almacén se archivó a media captura sigue al del ticket: mandar uno que ya no
  // existe haría que el servidor rechace la compra entera.
  const whOf = (i: DraftItem) => (i.warehouseId !== null && whList.some((w) => w.id === i.warehouseId) ? i.warehouseId : ticketWh);
  // El renglón elegido: su detalle va al lado de la lista; en una pantalla angosta, en una hoja.
  const [selKey, setSelKey] = useState<string | null>(null);
  const [sheetOpen, setSheetOpen] = useState(false);
  const narrow = useNarrow();
  const [moreOpen, setMoreOpen] = useState(false);

  const label = settings?.stockItemLabel || 'artículo';
  const activeCats = (cats?.items ?? []).filter((c) => c.isActive);
  const activeSups = (sups?.items ?? []).filter((s) => s.isActive);
  const cashRegisters = activeRegisters(registers?.items);
  // Si el servidor dice que el pago no entra a ningún corte, se pregunta antes de reenviar.
  const [outsideAsk, setOutsideAsk] = useState<OutsideAsk | null>(null);
  const unitList = units?.items ?? [];
  const methodList = methods?.items ?? [];
  const presList = presentations ?? [];
  const articleList = [...(articles?.items ?? []), ...created];
  const articleOptions = articleList.map((a) => ({ value: `${a.itemType}:${a.id}`, label: a.name, hint: a.unitCode, aliases: a.altNames }));

  const reset = () => {
    setStep('start'); setWithGoods(true);
    setExpenseDate(today()); setCategoryId(''); setSupplierId(''); setAmount('');
    setDescription(''); setDocKind(''); setDocFolio(''); setDocRaw(undefined);
    setItems([]); setPayments([]); setParsed(null); setPayWith(''); setPayRegister('');
    setSelKey(null); setSheetOpen(false); setMoreOpen(false);
  };

  // ---- Subir documento ----
  const fileRef = useRef<HTMLInputElement>(null);
  const parse = useMutation({
    mutationFn: (f: File) => backofficeApi.parseDoc(f),
    onSuccess: async (res) => {
      setParsed(res);
      setStep('capture');
      setWithGoods(true);
      const d = res.doc;
      if (d.issuedOn) setExpenseDate(d.issuedOn);
      if (d.total) setAmount(d.total);
      if (d.kind) setDocKind(d.kind);
      if (d.folio) setDocFolio(d.folio);
      setDocRaw(res.raw);
      if (!description && d.supplier) setDescription(`Compra ${d.supplier}`);

      // Las líneas entran como borrador y se piden sugerencias por renglón. El backend decide:
      // exacta aprendida (autollena) o parecidos (esperan confirmación: no suman hasta que se diga).
      const sid = supplierId ? Number(supplierId) : undefined;
      const drafts = await Promise.all(d.lines.map(async (l): Promise<DraftItem> => {
        const base: DraftItem = {
          ...emptyItem(),
          rawCode: l.rawCode, rawName: l.rawName,
          description: l.rawName,
          quantity: l.qty || '1',
          amount: lineAmount(l.amount, l.unitPrice, l.qty),
          packQtyInBase: packToBase(l.packQty, l.packUnit, unitList),
          docStatus: l.status,
          // Un renglón que el documento marca como no surtido llega después: no debe tocar el
          // almacén aunque esté impreso.
          arrived: l.status !== 'no_disponible',
        };
        if (!l.rawName) return base;
        try {
          const { items: s } = await backofficeApi.suggestArticles({
            supplierId: sid, rawCode: l.rawCode, rawName: l.rawName,
          });
          const best = s[0];
          if (best?.source === 'personal') {
            return { ...base, excluded: true, suggestion: best, suggestions: [] };
          }
          if (best?.source === 'aprendido') {
            return {
              ...base, itemType: best.itemType, itemId: best.itemId, itemName: best.itemName,
              packQtyInBase: best.packQtyInBase || base.packQtyInBase,
              unitId: best.unitId ?? base.unitId, suggestion: best, suggestions: s,
            };
          }
          return { ...base, suggestions: s, doubtful: s.length > 0 };
        } catch {
          return base; // una sugerencia que falla no debe tumbar la captura
        }
      }));
      setItems(drafts);
      toaster.create({ title: `Se llenaron ${d.lines.length} renglones del ticket.`, type: 'success' });
    },
    onError: (e) => toaster.create({ title: 'No se pudo leer el ticket', description: String(e), type: 'error' }),
  });

  // ---- Dinero ----
  const total = montoTecleado(amount) ?? 0;
  const counted = withGoods ? items.filter((i) => !blankLine(i)) : [];
  const hasArticle = (i: DraftItem) => !!i.itemId && !i.doubtful;
  const isGoods = (i: DraftItem) => hasArticle(i) && !i.excluded;
  // Lo excluido que, por el total escrito, termina como «otros»: el pie lo dice para que no pase sin verse.
  const excludedSum = items.filter((i) => i.excluded).reduce((s, i) => s + (Number(i.amount) || 0), 0);
  const receivedOf = (i: DraftItem) => (i.partQty !== null ? i.partQty : i.quantity || '1');
  // Una parte es más que cero y menos que lo pedido: «todo» y «nada» ya tienen su botón.
  const partBad = (i: DraftItem) => {
    if (!isGoods(i) || i.partQty === null) return false;
    const got = parseNumero(i.partQty);
    const asked = parseNumero(i.quantity);
    return got.estado !== 'valido' || asked.estado !== 'valido' || got.valor <= 0 || got.valor >= asked.valor;
  };
  const moneyCheck = purchaseMoney(counted.map((i) => ({
    amount: i.isGift ? '0' : i.amount, goods: isGoods(i), gift: i.isGift, excluded: i.excluded,
  })), total);
  // ¿Cómo se pagó? (FR-004d): sin pagos capturados abajo, se pregunta al guardar. Un método registra
  // un pago por el total; el efectivo sale de una caja abierta (si hay una sola, ésa).
  const quickMethod = payments.length === 0 && payWith && payWith !== 'later'
    ? methodList.find((m) => String(m.id) === payWith) : undefined;
  const quickRegister = quickMethod?.affectsCashDrawer
    ? (cashRegisters.length === 1 ? String(cashRegisters[0].id) : payRegister) : '';
  const quickPayment: ExpensePaymentBody | null = quickMethod ? {
    methodId: quickMethod.id, amount, paidOn: today(),
    registerId: quickRegister ? Number(quickRegister) : undefined,
    includeInCount: countsInDrawer(!!quickMethod.affectsCashDrawer, cashRegisters, Number(quickRegister)) || undefined,
  } : null;
  const paymentsSum = payments.reduce((a, p) => a + (montoTecleado(p.amount) ?? 0), 0) + (quickPayment ? total : 0);
  const status: 'pendiente' | 'pagada' = paymentsSum > 0 && paymentsSum >= total ? 'pagada' : 'pendiente';
  const payUnanswered = payments.length === 0 && payWith === '';
  const payNeedsRegister = !!quickMethod?.affectsCashDrawer && !quickRegister;
  const doubtfulCount = counted.filter((i) => i.doubtful).length;
  const ticketTotal = parsed?.doc.total ? montoTecleado(parsed.doc.total) : undefined;

  const factorOf = (i: DraftItem): { factor: string | null; base: string } => {
    const art = articleList.find((a) => a.itemType === i.itemType && a.id === i.itemId);
    const base = art?.unitCode ?? '';
    if (i.presentationId) {
      return { factor: presList.find((p) => p.id === i.presentationId)?.qtyInBase ?? null, base };
    }
    const u = unitList.find((x) => x.id === i.unitId);
    if (!u) return { factor: null, base };
    if (art && u.kind !== art.unitKind) {
      // Entre peso y líquido convierte la densidad del artículo, con la regla del servidor (spec 058).
      const cross = crossConversion('1', u, art.unitKind, art.unitCode, art.densityGPerMl);
      if (cross) return { factor: cross.factor === null ? null : String(cross.factor), base };
      return { factor: i.packQtyInBase || null, base };
    }
    return { factor: u.toBase, base };
  };
  // crossOf es la conversión entre peso y líquido de un renglón, o lo que le falta; null si no cruza.
  const crossOf = (i: DraftItem): CrossConversion | null => {
    if (!hasArticle(i) || i.presentationId) return null;
    const art = articleList.find((a) => a.itemType === i.itemType && a.id === i.itemId);
    const u = unitList.find((x) => x.id === i.unitId);
    return art && u ? crossConversion(i.quantity || '1', u, art.unitKind, art.unitCode, art.densityGPerMl) : null;
  };
  const sumsLine = (i: DraftItem) => {
    if (!isGoods(i) || !i.arrived) return null;
    const { factor, base } = factorOf(i);
    const n = baseOfLine(receivedOf(i), factor);
    return n === null ? null : `${i.itemName} +${formatBaseQty(n, base)}`;
  };

  // ---- Renglones: cuál se ve en el detalle ----
  const visibleItems = items.filter((i) => !i.doubtful);
  const selected = visibleItems.find((i) => i.key === selKey) ?? visibleItems[0] ?? null;
  const addLine = () => {
    const it = emptyItem();
    setItems((p) => [...p, it]);
    setSelKey(it.key);
    if (narrow) setSheetOpen(true);
  };
  // packOf es cómo se lee en la lista lo que llegó: «3 × Caja · 12 L», «2 kg».
  const packOf = (i: DraftItem): string => {
    if (!hasArticle(i)) return i.description ? '' : '';
    const pres = presList.find((p) => p.id === i.presentationId);
    if (pres) return `${i.quantity || '1'} × ${presentationLabel(pres)}`;
    const u = unitList.find((x) => x.id === i.unitId);
    return `${i.quantity || '1'} ${u ? unitLabel(u) : ''}`.trim();
  };
  const detailOf = (it: DraftItem) => (
    <ItemRow key={it.key} item={it} units={unitList} articles={articleOptions} articleList={articleList}
      presentations={presList} sums={sumsLine(it)} howItArrives={howItArrives} cross={crossOf(it)}
      onDensity={canDensity && densityShortcutRow.get(it.itemId ?? 0) === it.key
        ? () => setDensityFor({ id: it.itemId!, name: it.itemName }) : undefined}
      askForDensity={!canDensity || !((it.itemId ?? 0) > 0)}
      warehouses={multiWh ? whList : []} warehouseId={whOf(it)}
      onWarehouse={(id) => setItem(it.key, { warehouseId: id === ticketWh ? null : id })}
      canRemove={visibleItems.length > 1}
      onChange={(patch) => setItem(it.key, patch)}
      onSplit={() => {
        // La mitad nueva no lleva el texto del ticket: aprenderlo dos veces pisaría el primero.
        const copy: DraftItem = { ...emptyItem(), description: it.description, docStatus: it.docStatus, warehouseId: it.warehouseId };
        setItems((p) => {
          const at = p.findIndex((x) => x.key === it.key);
          return [...p.slice(0, at + 1), copy, ...p.slice(at + 1)];
        });
        setSelKey(copy.key);
      }}
      onPacks={(a) => setPacksOf(a)}
      onCreate={createArticle}
      onRemove={() => {
        const at = visibleItems.findIndex((x) => x.key === it.key);
        setItems((p) => p.filter((x) => x.key !== it.key));
        setSelKey(visibleItems[at + 1]?.key ?? visibleItems[at - 1]?.key ?? null);
        setSheetOpen(false);
      }} />
  );

  const create = useMutation({
    mutationFn: (choice?: CountChoice) => backofficeApi.createExpense({
      expenseDate,
      expenseDay: turno ? undefined : expenseDay,
      receivedAt: withGoods && counted.length > 0 ? today() : undefined,
      categoryId: Number(categoryId),
      supplierId: supplierId ? Number(supplierId) : undefined,
      amount,
      description: description || undefined,
      status,
      docKind: docKind || undefined,
      docFolio: docFolio || undefined,
      docRaw,
      documentTotal: parsed?.doc.total || undefined,
      items: counted.map((i): ExpenseItemBody => {
        // Un renglón dudoso se guarda como gasto sin tocar el almacén y sin su texto del ticket:
        // mandarlo con el texto haría que el proveedor «aprenda» que no es del almacén.
        const goods = hasArticle(i);
        return {
          itemType: goods ? i.itemType : '',
          ingredientId: goods && i.itemType === 'ingrediente' && i.itemId! > 0 ? i.itemId! : undefined,
          newArticle: goods && i.itemType === 'ingrediente' && i.itemId! < 0 ? pendingNew[i.itemId!] : undefined,
          productId: goods && i.itemType === 'producto' ? i.itemId! : undefined,
          description: i.description || i.itemName || i.rawName || 'Sin descripción',
          quantity: i.quantity || '1',
          unitId: goods && !i.presentationId ? i.unitId ?? undefined : undefined,
          presentationId: goods ? i.presentationId ?? undefined : undefined,
          qtyReceived: goods ? (i.arrived && !i.excluded ? receivedOf(i) : '0') : undefined,
          amount: i.isGift ? '0' : i.amount || '0',
          isGift: goods && i.isGift ? true : undefined,
          packQtyInBase: goods && !i.presentationId ? i.packQtyInBase || undefined : undefined,
          rawCode: i.doubtful ? undefined : i.rawCode || undefined,
          rawName: i.doubtful ? undefined : i.rawName || undefined,
          excluded: i.excluded || undefined,
          warehouseId: goods && multiWh ? whOf(i) ?? undefined : undefined,
        };
      }),
      payments: (quickPayment ? [quickPayment] : payments.map((p): ExpensePaymentBody => ({
        methodId: Number(p.methodId),
        amount: p.amount,
        paidOn: p.paidOn || undefined,
        registerId: p.registerId ? Number(p.registerId) : undefined,
        reference: p.reference || undefined,
        includeInCount: countsInDrawer(needsRegister(p, methodList), cashRegisters, Number(p.registerId)) ? p.includeInCount : undefined,
        drawerExtra: drawerOf(p).extra,
      }))).map((p) => withCountChoice(p, choice)),
    }),
    onSuccess: () => {
      if (withGoods && counted.length > 0) medirAccion('gastos', 'registrar-compra');
      void qc.invalidateQueries({ queryKey: ['stock'] });
      void qc.invalidateQueries({ queryKey: ['ingredients'] });
      reset();
      onSaved();
    },
    onError: (e) => {
      if (e instanceof ApiError && e.code === 'UNIT_KIND_MISMATCH' && e.details?.ingredientId) {
        const id = e.details.ingredientId;
        setMismatch({ id, name: articleList.find((a) => a.itemType === 'ingrediente' && a.id === id)?.name ?? '' });
        return;
      }
      const all = quickPayment ? [quickPayment] : payments.map((p) => ({ registerId: p.registerId ? Number(p.registerId) : undefined }));
      const ask = outsideAskOf(e, cashRegisters, all.map((p) => p.registerId));
      if (ask) { setOutsideAsk(ask); return; }
      toaster.create({ title: 'No se pudo registrar', description: String(e), type: 'error' });
    },
  });

  const missingUnit = counted.some((i) => isGoods(i) && !i.presentationId && !i.unitId);
  // «Salió de la caja» solo en efectivo que entra al arqueo y con algo excluido en el ticket (spec 050).
  const drawerOutShown = (p: DraftPayment) =>
    excludedSum > 0 && p.includeInCount && countsInDrawer(needsRegister(p, methodList), cashRegisters, Number(p.registerId));
  const drawerOf = (p: DraftPayment) => (drawerOutShown(p) ? drawerExtraOf(p.amount, p.drawerOut, excludedSum) : { bad: false });
  const badPayment = payments.find((p) => !p.methodId || !((montoTecleado(p.amount) ?? 0) > 0) || (needsRegister(p, methodList) && !p.registerId));
  const badDrawerOut = payments.some((p) => drawerOf(p).bad);
  const anyPartBad = counted.some(partBad);
  const missingDensity = counted.some((i) => isGoods(i) && !!crossOf(i)?.missing);
  // El atajo va una vez por artículo aunque tenga varios renglones: en el primero que lo necesita. Un
  // artículo nuevo (id provisional) todavía no existe y no puede llevar densidad.
  const densityShortcutRow = new Map<number, string>();
  for (const i of counted) {
    if (isGoods(i) && i.itemId! > 0 && crossOf(i)?.missing && !densityShortcutRow.has(i.itemId!)) densityShortcutRow.set(i.itemId!, i.key);
  }
  // Sin la lista de almacenes, guardar mandaría la mercancía al de venta sin preguntar.
  const whPending = items.some((i) => !!i.itemId && !i.excluded) && (whQuery.isLoading || whQuery.isError);
  const canSave = !whPending && !!categoryId && total > 0 && moneyCheck.missingAmount === 0 && !moneyCheck.over && !missingUnit && !badPayment
    && !payUnanswered && !payNeedsRegister && !anyPartBad && !badDrawerOut && !missingDensity;

  const setItem = (key: string, patch: Partial<DraftItem>) =>
    setItems((prev) => prev.map((i) => (i.key === key ? { ...i, ...patch } : i)));

  // howItArrives: el primer empaque de la empresa, o la unidad base del artículo. El empaque gana
  // sobre el tamaño aprendido del proveedor (regla 11).
  const howItArrives = (type: string, id: number, unitId: number | null) => {
    const pack = type === 'ingrediente' ? presList.find((p) => p.ingredientId === id && p.active) : undefined;
    const art = articleList.find((a) => a.itemType === type && a.id === id);
    return {
      presentationId: pack?.id ?? null,
      // Sin empaque llega elegida la unidad grande de su tipo (kg, L, m): nadie compra mililitros, y
      // con la base elegida «3» se guardaba como 3 ml.
      unitId: pack ? null : unitId ?? (art ? purchaseUnitsFor(art.unitKind, unitList)[0]?.id : undefined)
        ?? unitList.find((u) => u.code === art?.unitCode)?.id ?? null,
    };
  };

  // createArticle prepara un insumo nuevo preguntando antes en qué se mide: la unidad base no
  // cambia después sin un cambio de unidad (regla 4). No llama al servidor: el insumo viaja con el
  // gasto y solo existe si el gasto se guarda (spec 042, FR-003). Si ya hay uno con ese nombre, se
  // elige ése.
  const createArticle = async (name: string): Promise<PickerOption | null> => {
    const known = articleList.filter((a) => a.itemType === 'ingrediente');
    const found = existingByName(name, known);
    if (found) {
      toaster.create({ title: `Ya hay un insumo que se llama así: se eligió «${found.name}»`, type: 'info' });
      return { value: `ingrediente:${found.id}`, label: found.name };
    }
    setAskingKind(true);
    const kind = await new Promise<Unit['kind'] | null>((resolve) => { kindAsk.current = resolve; });
    setAskingKind(false);
    if (!kind) return null;
    const base = unitList.find((u) => u.code === KINDS.find((k) => k.kind === kind)!.base);
    if (!base) return null;
    const id = nextTempId();
    const clean = name.trim().replace(/\s+/g, ' ');
    setPendingNew((p) => ({ ...p, [id]: { name: clean, baseUnitId: base.id } }));
    setCreated((p) => [...p, { itemType: 'ingrediente', id, name: clean, unitCode: base.code, unitKind: base.kind }]);
    return { value: `ingrediente:${id}`, label: `${clean} (nuevo)` };
  };

  const goodsLines = counted.filter((i) => isGoods(i));
  const summary = goodsLines.map(sumsLine).filter(Boolean) as string[];

  return (
    <DialogRoot open={open} onOpenChange={(e) => { if (!e.open) { onClose(); reset(); } }}
      placement="center" size="full" scrollBehavior="inside">
      <DialogBackdrop />
      {/* Pantalla completa: «cover» con scroll interno lo dejaba en 400 de los 600 px. */}
      {/* El diálogo se pinta fuera del contenedor que lleva la paleta del negocio (Apariencia): lo
          elegido y lo activo la toman aquí (spec 094, color P1). */}
      <DialogContent h="100dvh" maxH="100dvh" colorPalette={palette}>
        <DialogHeader>
          <DialogTitle>{step === 'capture' && parsed ? 'Nuevo gasto · desde el ticket' : 'Nuevo gasto'}</DialogTitle>
        </DialogHeader>
        <DialogCloseTrigger />
        <input ref={fileRef} type="file" accept="application/pdf,image/*" hidden data-testid="ticket-file"
          onChange={(e) => { const f = e.target.files?.[0]; if (f) parse.mutate(f); e.target.value = ''; }} />

        {step === 'start' && (
          <DialogBody>
            <VStack gap={4} maxW="560px" mx="auto" py={6}>
              <Button size="xl" w="100%" minH="88px" loading={parse.isPending}
                onClick={() => fileRef.current?.click()}>
                <LuUpload /> Sube la foto del ticket
              </Button>
              <Button size="lg" w="100%" minH="56px" variant="outline" onClick={() => setStep('ask')}>
                Capturar sin ticket (también luz, gas, renta)
              </Button>
            </VStack>
          </DialogBody>
        )}

        {step === 'ask' && (
          <DialogBody>
            <VStack gap={4} maxW="560px" mx="auto" py={6}>
              <Text fontWeight="700" fontSize="lg">¿Trae mercancía para el almacén?</Text>
              <Button size="lg" w="100%" minH="56px"
                onClick={() => { setWithGoods(true); setItems([emptyItem()]); setStep('capture'); }}>
                Sí
              </Button>
              <Button size="lg" w="100%" minH="56px" variant="outline"
                onClick={() => { setWithGoods(false); setItems([]); setStep('capture'); }}>
                No, solo el gasto
              </Button>
            </VStack>
          </DialogBody>
        )}

        {step === 'capture' && (<>
          <DialogBody pb={4}>
            <VStack align="stretch" gap={4} maxW="1100px" mx="auto">
              {/* ---- Encabezado (spec 094, maqueta R3): lo obligatorio en una línea; comentario, folio y
                  día del gasto se pliegan en «Más datos». ---- */}
              <HStack flexWrap="wrap" gap={3} align="end">
                <Field label="Proveedor" flex="1" minW="180px">
                  <Picker value={supplierId} onChange={setSupplierId} placeholder="— Sin proveedor —" title="Proveedor"
                    clearable clearLabel="— Sin proveedor —"
                    options={activeSups.map((s) => ({ value: String(s.id), label: s.name }))}
                    onCreate={async (name) => {
                      const s = await backofficeApi.createSupplier({ name });
                      qc.setQueryData(['suppliers'], (old?: { items: Supplier[] }) => ({ items: [...(old?.items ?? []), s] }));
                      return { value: String(s.id), label: s.name };
                    }} />
                </Field>
                <Field label="Categoría" flex="1" minW="160px">
                  <Picker value={categoryId} onChange={setCategoryId} placeholder="Elegir categoría" title="Categoría"
                    options={activeCats.map((c) => ({ value: String(c.id), label: c.name }))}
                    onCreate={async (name) => {
                      const c = await backofficeApi.createExpenseCategory({ name, financialGroup: 'operacional' });
                      qc.setQueryData(['expense-cats'], (old?: { items: ExpenseCategory[] }) => ({ items: [...(old?.items ?? []), c] }));
                      return { value: String(c.id), label: c.name };
                    }} />
                </Field>
                <Field label="Fecha del ticket" w="150px">
                  <DatePickerButton label="Fecha del ticket" value={expenseDate} onChange={setExpenseDate} />
                </Field>
                <Field label="Total de la compra" w="140px">
                  <Input aria-label="Total de la compra" inputMode="decimal" placeholder="0.00" value={amount} minH="44px"
                    textAlign="end" fontWeight="700" onChange={(e) => setAmount(e.target.value)} />
                </Field>
                <Button minH="44px" variant="ghost" aria-expanded={moreOpen} onClick={() => setMoreOpen((m) => !m)}>
                  {moreOpen ? <>Menos datos <LuChevronUp /></> : <>Más datos <LuChevronDown /></>}
                </Button>
              </HStack>
              {moreOpen ? (
                <HStack flexWrap="wrap" gap={3} align="end">
                  <DiaDelGasto turno={turno} value={expenseDay} onChange={setExpenseDay} />
                  <Field label="Comentario" flex="1" minW="200px">
                    <Input placeholder="Concepto" value={description} minH="44px" onChange={(e) => setDescription(e.target.value)} />
                  </Field>
                  <Field label="Folio" w="150px">
                    <Input placeholder="—" value={docFolio} minH="44px" onChange={(e) => setDocFolio(e.target.value)} />
                  </Field>
                </HStack>
              ) : (
                <Text fontSize="sm" color="fg.muted" mt={-2}>
                  {[description, docFolio && `Folio ${docFolio}`, expenseDay !== today() && `Gasto del ${hora.soloFecha(`${expenseDay}T12:00:00`)}`]
                    .filter(Boolean).join(' · ')}
                </Text>
              )}

              {/* Los avisos van debajo del encabezado: no lo tapan. */}
              {ticketTotal !== undefined && total > 0 && Math.abs(ticketTotal - total) >= 0.01 && (
                <HStack gap={2} flexWrap="wrap" p={3} borderRadius="lg" bg="orange.subtle">
                  <Text fontSize="sm" flex="1">El ticket dice {money(ticketTotal.toFixed(2))} y capturaste {money(total.toFixed(2))}. ¿Cuál se pagó?</Text>
                  <Button minH="44px" variant="outline" onClick={() => setAmount(ticketTotal.toFixed(2))}>
                    {money(ticketTotal.toFixed(2))}, el del ticket
                  </Button>
                  <Button minH="44px" variant="outline" onClick={() => setParsed((p) => p && { ...p, doc: { ...p.doc, total: amount } })}>
                    {money(total.toFixed(2))}, el que capturé
                  </Button>
                </HStack>
              )}
              {parsed && <DocSummary parsed={parsed} />}
              {/* Lo que llegó antes del último conteo ya pudo contarse: entra igual (Assumptions del
                  spec 038), pero quien registra tarde tiene que saberlo. */}
              {withGoods && lastCount?.closedAt && expenseDate < hora.diaDelNegocio(lastCount.closedAt) && (
                <Warn>
                  Esta compra es de antes del último conteo ({hora.soloFecha(lastCount.closedAt)}). Si ya estaba en el
                  anaquel cuando se contó, marca sus renglones como «Llega después» o se sumará dos veces.
                </Warn>
              )}

              {/* ---- Renglones (spec 094, maqueta R3): lista de una línea y el detalle del elegido al
                  lado; en una pantalla angosta el detalle se abre en una hoja. ---- */}
              {withGoods && (
                <VStack align="stretch" gap={2}>
                  <HStack justify="space-between" flexWrap="wrap" gap={3} borderTopWidth="1px" pt={3}>
                    <Text fontWeight="700">{visibleItems.length} {visibleItems.length === 1 ? 'renglón' : 'renglones'}</Text>
                    {multiWh && ticketWh !== null && (
                      <HStack gap={2}>
                        <Text fontSize="sm" fontWeight="600">¿Dónde se guarda el ticket?</Text>
                        <Segmented label="¿Dónde se guarda el ticket?" value={String(ticketWh)}
                          options={whList.map((w) => ({ value: String(w.id), label: w.name, icon: w.isSales ? <LuStore /> : <LuWarehouse /> }))}
                          onChange={(v) => {
                            setTicketWh(Number(v));
                            // Todo el ticket: el renglón que ya iba a ese almacén vuelve a seguir al ticket; el
                            // que se mandó a mano a OTRO se queda donde lo pusieron.
                            setItems((prev) => prev.map((i) => (i.warehouseId === Number(v) ? { ...i, warehouseId: null } : i)));
                          }} />
                      </HStack>
                    )}
                    <Button minH="44px" variant="outline" onClick={addLine}>
                      <LuPlus /> {parsed ? `Agregar un ${label} que no venía` : `Agregar ${label}`}
                    </Button>
                  </HStack>
                  {/* Los dudosos arriba: son los que piden una decisión (FR-005a). */}
                  {items.filter((i) => i.doubtful).map((it) => (
                    <DoubtfulRow key={it.key} item={it} presentations={presList}
                      onYes={() => {
                        const s = it.suggestions[0];
                        setItem(it.key, {
                          doubtful: false, itemType: s.itemType, itemId: s.itemId, itemName: s.itemName,
                          packQtyInBase: it.packQtyInBase || s.packQtyInBase || '',
                          ...howItArrives(s.itemType, s.itemId, it.unitId ?? s.unitId ?? null),
                        });
                      }}
                      onOther={() => setItem(it.key, { doubtful: false })}
                      onNotArrived={() => setItem(it.key, { doubtful: false, arrived: false, docStatus: 'no_disponible' })} />
                  ))}
                  <Grid templateColumns={narrow || !selected ? '1fr' : 'minmax(0, 1.15fr) minmax(0, 1fr)'} gap={4} alignItems="start">
                    <VStack align="stretch" gap={2}>
                      {visibleItems.map((it, n) => (
                        <LineSummary key={it.key} n={n + 1} item={it} selected={!narrow && it.key === selected?.key}
                          pack={packOf(it)} label={label}
                          otherWarehouse={multiWh && whOf(it) !== ticketWh && hasArticle(it) && !it.excluded
                            ? whList.find((w) => w.id === whOf(it)) ?? null : null}
                          needsDensity={isGoods(it) && !!crossOf(it)?.missing}
                          onOpen={() => { setSelKey(it.key); if (narrow) setSheetOpen(true); }} />
                      ))}
                    </VStack>
                    {selected && !narrow && (
                      <Box borderWidth="1px" borderRadius="lg" p={4} bg="bg.panel" position="sticky" top={0}
                        maxH="calc(100dvh - 220px)" overflowY="auto">
                        {detailOf(selected)}
                      </Box>
                    )}
                  </Grid>
                  {selected && narrow && (
                    <DrawerRoot open={sheetOpen} placement="bottom" onOpenChange={(e) => { if (!e.open) setSheetOpen(false); }}>
                      <DrawerBackdrop />
                      <DrawerContent borderTopRadius="2xl" maxH="90dvh" colorPalette={palette}>
                        <DrawerHeader><Text fontWeight="700" fontSize="lg">Renglón {visibleItems.indexOf(selected) + 1}</Text></DrawerHeader>
                        <DrawerBody pb={4} overflowY="auto">{detailOf(selected)}</DrawerBody>
                        <DrawerFooter><Button minH="52px" onClick={() => setSheetOpen(false)}>Listo</Button></DrawerFooter>
                      </DrawerContent>
                    </DrawerRoot>
                  )}
                </VStack>
              )}

              {/* ---- Pagos ---- */}
              <Section title="Pagos" action={
                <Button minH="44px" variant="outline" onClick={() => setPayments((p) => [...p, emptyPayment(today())])}>
                  <LuPlus /> Agregar pago
                </Button>
              }>
                {payments.length === 0 ? (
                  <Text color="fg.muted" fontSize="sm">Sin pagos: el gasto queda pendiente (cuenta por pagar).</Text>
                ) : (
                  <VStack align="stretch" gap={3}>
                    {payments.map((p) => (
                      <PaymentRow key={p.key} payment={p} methods={methodList} registers={cashRegisters}
                        drawerOut={drawerOutShown(p) ? drawerOf(p) : null}
                        onChange={(patch) => setPayments((prev) => prev.map((x) => (x.key === p.key ? { ...x, ...patch } : x)))}
                        onRemove={() => setPayments((prev) => prev.filter((x) => x.key !== p.key))} />
                    ))}
                  </VStack>
                )}
                {payments.length > 0 && (
                  <Text fontSize="sm" mt={2} textAlign="end" color={paymentsSum >= total ? 'fg.success' : 'fg.muted'}>
                    Pagado {money(paymentsSum.toFixed(2))} de {money(total.toFixed(2))}
                    {status === 'pagada' ? ' · queda pagada' : ' · queda pendiente'}
                  </Text>
                )}
                {badPayment && <Warn>Cada pago necesita método e importe; en efectivo además la caja de la que sale.</Warn>}
                {badDrawerOut && <Warn>«Salió de la caja» va del importe del pago al importe más lo excluido ({money(excludedSum.toFixed(2))}).</Warn>}
              </Section>
            </VStack>
          </DialogBody>

          <DialogFooter borderTopWidth="1px">
            <VStack w="100%" gap={2} align="stretch">
            {payments.length === 0 && (
              <HStack gap={2} flexWrap="wrap" role="group" aria-label="¿Cómo se pagó?">
                <Text fontSize="sm" fontWeight="600">¿Cómo se pagó?</Text>
                {/* Los métodos de las plataformas (Uber, Didi, Rappi) cobran ventas, no pagan compras. */}
                {methodList.filter((m) => m.isActive !== false && m.kind !== 'plataforma'
                  && (!m.affectsCashDrawer || cashRegisters.length > 0)).map((m) => (
                  <Button key={m.id} minH="44px" size="sm" aria-pressed={payWith === String(m.id)}
                    variant={payWith === String(m.id) ? 'solid' : 'outline'} colorPalette={payWith === String(m.id) ? undefined : 'gray'}
                    onClick={() => setPayWith(String(m.id))}>
                    {m.name}
                  </Button>
                ))}
                <Button minH="44px" size="sm" aria-pressed={payWith === 'later'}
                  variant={payWith === 'later' ? 'solid' : 'outline'} colorPalette={payWith === 'later' ? undefined : 'gray'}
                  onClick={() => setPayWith('later')}>
                  Todavía no se paga
                </Button>
                {quickMethod?.affectsCashDrawer && cashRegisters.length > 1 && (
                  <Box minW="200px">
                    <Picker value={payRegister} onChange={setPayRegister} placeholder="¿De qué caja sale?" title="¿De qué caja sale?"
                      options={cashRegisters.map((r) => ({ value: String(r.id), label: registerLabel(r) }))} />
                  </Box>
                )}
              </HStack>
            )}
            {mismatch && (
              <HStack gap={2} flexWrap="wrap" p={2} borderRadius="lg" bg="orange.subtle">
                <Text fontSize="sm" flex="1">
                  Falta la densidad de {mismatch.name || `este ${label}`} para convertir su renglón.{!canDensity && ` ${ASK_FOR_DENSITY}`}
                </Text>
                {canDensity && <Button minH="44px" variant="outline" bg="bg" onClick={() => setDensityFor(mismatch)}>Capturar la densidad</Button>}
              </HStack>
            )}
            <HStack w="100%" gap={3} align="center">
              <Text fontSize="sm" flex="1" minW={0} lineClamp={1} data-testid="pie-compra">
                {doubtfulCount > 0 && (
                  <Text as="span" fontWeight="700" color="orange.fg">
                    {doubtfulCount} {doubtfulCount === 1 ? 'renglón sin confirmar: no suma' : 'renglones sin confirmar: no suman'} ·{' '}
                  </Text>
                )}
                {withGoods && (
                  <>{goodsLines.length} {goodsLines.length === 1 ? label : plural(label)}
                    {/* Tres y «y N más»: con un ticket largo, el pie se partía en varias líneas y le
                        quitaba alto a la lista de renglones. */}
                    {summary.length > 0 && ` · suma: ${summary.slice(0, 3).join(' · ')}${summary.length > 3 ? ` y ${summary.length - 3} más` : ''}`}</>
                )}
                {!categoryId && <Text as="span" color="orange.fg"> · falta la categoría</Text>}
                {!(total > 0) && <Text as="span" color="orange.fg"> · falta el total</Text>}
                {moneyCheck.missingAmount > 0 && (
                  <Text as="span" fontWeight="700" color="orange.fg">
                    {' · '}falta el importe de {moneyCheck.missingAmount === 1 ? 'un renglón' : `${moneyCheck.missingAmount} renglones`}
                  </Text>
                )}
                {missingUnit && <Text as="span" fontWeight="700" color="orange.fg"> · elige cómo llegó cada {label}</Text>}
                {anyPartBad && <Text as="span" fontWeight="700" color="orange.fg"> · di cuántos llegaron (menos de lo pedido)</Text>}
                {missingDensity && <Text as="span" fontWeight="700" color="orange.fg"> · falta la densidad para convertir un renglón</Text>}
                {payUnanswered && <Text as="span" fontWeight="700" color="orange.fg"> · elige cómo se pagó</Text>}
                {payNeedsRegister && <Text as="span" fontWeight="700" color="orange.fg"> · elige de qué caja sale</Text>}
                {whQuery.isError && <Text as="span" fontWeight="700" color="fg.error"> · No se pudieron leer los almacenes: vuelve a abrir la compra</Text>}
                {moneyCheck.over && (
                  <Text as="span" fontWeight="700" color="fg.error">
                    {' · '}los renglones suman {money(moneyCheck.linesSum.toFixed(2))} y el total es {money(total.toFixed(2))}
                  </Text>
                )}
                {!moneyCheck.over && moneyCheck.others > 0 && counted.length > 0 && total > 0 && (
                  <Text as="span" color="fg.muted">
                    {' · '}renglones {money(moneyCheck.linesSum.toFixed(2))} de {money(total.toFixed(2))}: el resto queda como «otros» del gasto
                    {excludedSum > 0 && ` (incluye ${money(Math.min(excludedSum, moneyCheck.others).toFixed(2))} de lo excluido)`}
                  </Text>
                )}
              </Text>
              <Button minH="44px" variant="outline" onClick={() => { onClose(); reset(); }}>Cancelar</Button>
              <Button minH="44px" disabled={!canSave} loading={create.isPending} onClick={() => create.mutate(undefined)}>
                Guardar
              </Button>
            </HStack>
            </VStack>
          </DialogFooter>
        </>)}

        {packsOf && (
          <PresentationsSheet isOpen
            ingredient={{ id: packsOf.id, name: packsOf.name, baseUnitCode: packsOf.unitCode, baseUnitKind: packsOf.unitKind }}
            presentations={presList.filter((p) => p.ingredientId === packsOf.id)}
            units={unitList} onClose={() => setPacksOf(null)} />
        )}

        <DrawerRoot open={askingKind} placement="bottom"
          onOpenChange={(e) => { if (!e.open) { kindAsk.current?.(null); kindAsk.current = null; setAskingKind(false); } }}>
          <DrawerBackdrop />
          <DrawerContent borderTopRadius="2xl">
            <DrawerHeader><Text fontWeight="700" fontSize="lg">¿En qué lo mides?</Text></DrawerHeader>
            <DrawerBody pb={6}>
              <VStack align="stretch" gap={2}>
                {KINDS.map((k) => (
                  <Button key={k.kind} minH="56px" variant="outline" justifyContent="start"
                    onClick={() => { const r = kindAsk.current; kindAsk.current = null; r?.(k.kind); }}>
                    {k.label}
                  </Button>
                ))}
              </VStack>
            </DrawerBody>
          </DrawerContent>
        </DrawerRoot>
        {densityFor && (
          <DensitySheet article={{ ...densityFor, density: null }} onClose={() => setDensityFor(null)}
            onDone={() => { setDensityFor(null); setMismatch(null); }} />
        )}
        <OutsideCountSheet ask={outsideAsk} loading={create.isPending} onCancel={() => setOutsideAsk(null)}
          onChoose={(c) => { setOutsideAsk(null); create.mutate(c); }} />
      </DialogContent>
    </DialogRoot>
  );
}

// needsRegister: para un método que mueve el cajón, la caja es obligatoria — el backend lo
// exige porque efectivo que sale sin movimiento de caja descuadra el corte.
function needsRegister(p: DraftPayment, methods: { id: number; affectsCashDrawer?: boolean }[]) {
  const m = methods.find((x) => String(x.id) === p.methodId);
  return !!m?.affectsCashDrawer;
}

// ---- Renglón dudoso ----

function DoubtfulRow({ item, presentations, onYes, onOther, onNotArrived }: {
  item: DraftItem; presentations: Presentation[];
  onYes: () => void; onOther: () => void; onNotArrived: () => void;
}) {
  const s = item.suggestions[0];
  const pack = presentations.find((p) => p.ingredientId === s.itemId && p.active);
  return (
    <HStack gap={3} p={3} borderWidth="2px" borderColor="orange.fg" borderRadius="lg" bg="orange.subtle" flexWrap="wrap"
      data-testid="renglon-dudoso">
      <VStack align="start" gap={0} flex="1" minW="220px">
        <Text fontWeight="700">Ticket: {item.rawName}</Text>
        <Text fontSize="sm">¿Es {s.itemName}{pack ? ` · ${pack.name} · ${formatBaseQty(pack.qtyInBase, pack.baseUnitCode)}` : ''}? No suma hasta que digas.</Text>
      </VStack>
      <Button minH="44px" variant="outline" onClick={onYes}>Sí, es ése</Button>
      <Button minH="44px" variant="outline" onClick={onOther}>Es otro</Button>
      <Button minH="44px" variant="outline" onClick={onNotArrived}>No llegó</Button>
    </HStack>
  );
}

// ---- Renglón de mercancía ----

function ItemRow({ item, units, articles, articleList, presentations, sums, cross, onDensity, askForDensity, howItArrives,
  warehouses, warehouseId, onWarehouse, canRemove, onChange, onSplit, onPacks, onCreate, onRemove }: {
  item: DraftItem;
  // Spec 058: la conversión entre peso y líquido del renglón, o lo que le falta, y el atajo a la densidad.
  cross: CrossConversion | null;
  onDensity?: () => void;
  // askForDensity: no hay atajo posible (sin permiso, o el artículo aún no existe) y el aviso dice qué hacer.
  askForDensity?: boolean;
  howItArrives: (type: string, id: number, unitId: number | null) => { presentationId: number | null; unitId: number | null };
  units: Unit[];
  articles: PickerOption[];
  articleList: Article[];
  presentations: Presentation[];
  sums: string | null;
  // Spec 094: los almacenes entre los que se elige (vacío con uno solo) y a cuál va este renglón.
  warehouses: Warehouse[];
  warehouseId: number | null;
  onWarehouse: (id: number) => void;
  // El único renglón no se quita: vacío no se manda (R12).
  canRemove: boolean;
  onChange: (patch: Partial<DraftItem>) => void;
  onSplit: () => void;
  onPacks: (a: Article) => void;
  onCreate: (name: string) => Promise<PickerOption | null>;
  onRemove: () => void;
}) {
  const [confirmRemove, setConfirmRemove] = useState(false);
  const [help, setHelp] = useState(false);
  const art = articleList.find((a) => a.itemType === item.itemType && a.id === item.itemId);
  const packs = art && art.itemType === 'ingrediente'
    ? presentations.filter((p) => p.ingredientId === art.id && p.active) : [];
  const buyUnits = art ? [...purchaseUnitsFor(art.unitKind, units), ...(art.itemType === 'ingrediente' ? crossBuyUnits(art.unitKind, units) : [])] : [];

  const pick = (v: string) => {
    // Sin artículo no hay regalo: el servidor solo lo acepta en un renglón del almacén.
    if (!v) { onChange({ itemType: '', itemId: null, itemName: '', presentationId: null, isGift: false }); return; }
    const [type, id] = v.split(':');
    const a = articleList.find((x) => x.itemType === type && x.id === Number(id));
    onChange({
      itemType: type as 'ingrediente' | 'producto', itemId: Number(id), itemName: a?.name ?? '',
      suggestion: null, ...howItArrives(type, Number(id), item.unitId),
    });
  };

  const goods = !!item.itemId && !item.excluded;
  const state = !item.arrived ? 'despues' : item.partQty !== null ? 'parte' : 'llego';

  return (
    <VStack align="stretch" gap={3} data-testid="renglon-detalle" data-excluded={item.excluded || undefined}>
      {item.rawName && (
        <HStack gap={2} flexWrap="wrap">
          <Text fontSize="xs" color="fg.subtle">{item.rawCode && `${item.rawCode} · `}{item.rawName}</Text>
          {item.suggestion && (
            <HStack gap={1}><LuSparkles size={12} /><Text fontSize="xs" color="fg.success">reconocido de «{item.suggestion.matchedVia}»</Text></HStack>
          )}
        </HStack>
      )}
      <Box opacity={item.excluded ? 0.6 : 1}>
        <Picker value={item.itemId ? `${item.itemType}:${item.itemId}` : ''}
          onChange={pick} placeholder="Buscar o crear" title="¿Qué llegó?"
          clearable clearLabel="— No es del almacén (solo gasto) —"
          options={articles} searchThreshold={0} onCreate={onCreate} />
      </Box>
      <HStack gap={2} align="end" flexWrap="wrap">
        {goods ? (
          <HStack gap={0} borderWidth="1px" borderRadius="md">
            <IconButton aria-label="Menos" variant="ghost" minH="44px" minW="44px"
              onClick={() => { const n = parseNumero(item.quantity); if (n.estado === 'valido' && n.valor > 1) onChange({ quantity: String(round2(n.valor - 1)) }); }}>
              <LuMinus />
            </IconButton>
            <Input aria-label="Cantidad" w="64px" minH="44px" textAlign="center" border="none" inputMode="decimal" value={item.quantity}
              onChange={(e) => onChange({ quantity: e.target.value })} />
            <IconButton aria-label="Más" variant="ghost" minH="44px" minW="44px"
              onClick={() => { const n = parseNumero(item.quantity); onChange({ quantity: String(round2((n.estado === 'valido' ? n.valor : 0) + 1)) }); }}>
              <LuPlus />
            </IconButton>
          </HStack>
        ) : (
          <Input aria-label="Descripción" placeholder="Qué es (gas, envío…)" flex="1" minW="180px" minH="44px" value={item.description}
            onChange={(e) => onChange({ description: e.target.value })} />
        )}
        <Field label={item.excluded ? 'Importe (no suma al gasto)' : 'Importe'} w="130px" opacity={item.excluded ? 0.6 : 1}>
          <Input aria-label="Importe" inputMode="decimal" textAlign="end" minH="44px" placeholder="0.00"
            value={item.isGift ? '0' : item.amount} disabled={item.isGift}
            onChange={(e) => onChange({ amount: e.target.value })} />
        </Field>
      </HStack>

      {goods && art && (
        <HStack gap={2} flexWrap="wrap" align="center">
          {packs.map((p) => (
            <Button key={p.id} minH="44px" size="sm" variant={item.presentationId === p.id ? 'subtle' : 'outline'}
              colorPalette={item.presentationId === p.id ? undefined : 'gray'} aria-pressed={item.presentationId === p.id}
              onClick={() => onChange({ presentationId: p.id, unitId: null })}>
              {presentationLabel(p)} · {p.approximate ? '≈ ' : ''}{formatBaseQty(p.qtyInBase, p.baseUnitCode)}
            </Button>
          ))}
          {buyUnits.map((u) => (
            <Button key={u.id} minH="44px" size="sm" aria-pressed={!item.presentationId && item.unitId === u.id}
              variant={!item.presentationId && item.unitId === u.id ? 'subtle' : 'outline'}
              colorPalette={!item.presentationId && item.unitId === u.id ? undefined : 'gray'}
              onClick={() => onChange({ unitId: u.id, presentationId: null })}>
              {unitLabel(u)}
            </Button>
          ))}
          {art.itemType === 'ingrediente' && (
            <Button minH="44px" size="sm" variant="outline" borderStyle="dashed" onClick={() => onPacks(art)}>
              + Otro empaque
            </Button>
          )}
          {cross?.words ? <Text fontSize="sm" fontWeight="700" color="fg.success">{cross.words}</Text>
            : sums && <Text fontSize="sm" fontWeight="700" color="fg.success">= {sums.split(' +').pop()}</Text>}
        </HStack>
      )}
      {goods && art && (
        <VStack align="stretch" gap={1}>
          <Text fontSize="sm" fontWeight="600">¿Llegó?</Text>
          <HStack gap={2} flexWrap="wrap">
            <Segmented label="Estado" value={state}
              options={[{ value: 'llego', label: 'Llegó' }, { value: 'parte', label: 'Llegó una parte' }, { value: 'despues', label: 'Llega después' }]}
              onChange={(v) => onChange(v === 'llego' ? { arrived: true, partQty: null }
                : v === 'parte' ? { arrived: true, partQty: item.partQty ?? '' } : { arrived: false, partQty: null })} />
            {item.arrived && item.partQty !== null && (
              <Input aria-label="¿Cuántos llegaron?" placeholder="¿Cuántos?" w="96px" minH="44px" inputMode="decimal"
                value={item.partQty} onChange={(e) => onChange({ partQty: e.target.value })} />
            )}
          </HStack>
        </VStack>
      )}
      {goods && warehouses.length > 1 && warehouseId !== null && (
        <VStack align="stretch" gap={1}>
          <Text fontSize="sm" fontWeight="600">¿Dónde se guarda este renglón?</Text>
          <Segmented label="¿Dónde se guarda este renglón?" value={String(warehouseId)}
            options={warehouses.map((w) => ({ value: String(w.id), label: w.name, icon: w.isSales ? <LuStore /> : <LuWarehouse /> }))}
            onChange={(v) => onWarehouse(Number(v))} />
        </VStack>
      )}
      {/* Una línea propia: dentro de la fila de unidades y estado la hacía crecer una línea entera. */}
      {goods && art && cross?.missing && (
        <HStack gap={2} justify="space-between" data-testid="sin-densidad">
          <Text fontSize="sm"><Text as="span" fontWeight="700" color="orange.fg">{cross.missing}</Text>{askForDensity && `. ${ASK_FOR_DENSITY}`}</Text>
          {onDensity && <Button minH="44px" size="sm" variant="outline" flexShrink={0} onClick={onDensity}>Capturar la densidad</Button>}
        </HStack>
      )}
      {!goods && !item.excluded && item.suggestions.length > 0 && !item.itemId && (
        <Suggestions list={item.suggestions} onPick={(s) => onChange({
          itemType: s.itemType, itemId: s.itemId, itemName: s.itemName,
          packQtyInBase: item.packQtyInBase || s.packQtyInBase || '',
          ...howItArrives(s.itemType, s.itemId, item.unitId ?? s.unitId ?? null),
        })} />
      )}

      {/* S1 (spec 094): quien lo vio no supo qué era «Fue regalo» ni «No es del negocio»; cada uno dice
          qué pasa al encenderlo, y el (i) da un ejemplo. Son excluyentes. */}
      <VStack align="stretch" gap={2}>
        <HStack justify="space-between">
          <Text fontSize="sm" fontWeight="600">¿Este renglón es especial?</Text>
          <InfoButton label="Qué es esto" open={help} onToggle={() => setHelp((h) => !h)} />
        </HStack>
        {help && (
          <VStack align="stretch" gap={1} p={3} borderRadius="md" bg="bg.muted" fontSize="sm">
            <Text><b>Regalo del proveedor:</b> el ticket lo trae en $0 o te lo dieron sin cobrar. Se suma a lo que tienes guardado sin subir el costo.</Text>
            <Text><b>No es del negocio:</b> en la misma compra pagaste algo para ti (el jabón de la casa, un refresco). Se guarda en el ticket para que el total cuadre, pero no es gasto del negocio ni mercancía.</Text>
          </VStack>
        )}
        <SpecialToggle icon={<LuGift />} title="Regalo del proveedor"
          text={item.itemId ? 'No te lo cobraron. Entra al almacén con costo $0.' : 'Elige primero el artículo'}
          checked={item.isGift} disabled={!item.itemId}
          onChange={(on) => onChange({ isGift: on, excluded: on ? false : item.excluded })} />
        <SpecialToggle icon={<LuUser />} title="No es del negocio"
          text="Se pagó en este ticket pero es personal. No cuenta como gasto ni entra al almacén."
          checked={item.excluded} onChange={(on) => onChange({ excluded: on, isGift: on ? false : item.isGift })} />
      </VStack>

      {/* Lejos de «Listo» y de los interruptores: quitar no se deshace y el dedo es gordo. */}
      <HStack gap={2} justify="space-between" pt={8} pb={8}>
        {item.rawName ? (
          <Button minH="44px" variant="outline" onClick={onSplit}><LuScissors /> Partir</Button>
        ) : <span />}
        {/* Separado del resto: quitar no se deshace y el dedo es gordo. */}
        {confirmRemove ? (
          <HStack gap={1}>
            <Button minH="44px" colorPalette="red" onClick={onRemove}>Quitar</Button>
            <Button minH="44px" variant="ghost" onClick={() => setConfirmRemove(false)}>No</Button>
          </HStack>
        ) : (
          <Button minH="44px" variant="outline" colorPalette="red" disabled={!canRemove} onClick={() => setConfirmRemove(true)}>
            <LuTrash2 /> Quitar renglón
          </Button>
        )}
      </HStack>
    </VStack>
  );
}

// SpecialToggle es un renglón entero que se toca para encender o apagar: el interruptor solo es chico
// para el dedo, y la explicación al lado dice qué pasa (S1, spec 094).
function SpecialToggle({ icon, title, text, checked, disabled, onChange }: {
  icon: ReactNode; title: string; text: string; checked: boolean; disabled?: boolean; onChange: (on: boolean) => void;
}) {
  return (
    <ChakraSwitch.Root checked={checked} disabled={disabled} onCheckedChange={(e) => onChange(e.checked)}
      display="flex" alignItems="center" gap={3} p={3} minH="64px" borderWidth="1px" borderRadius="md" cursor={disabled ? 'not-allowed' : 'pointer'}
      opacity={disabled ? 0.6 : 1}>
      <ChakraSwitch.HiddenInput aria-label={title} />
      <Box boxSize="40px" borderRadius="md" bg="colorPalette.subtle" color="colorPalette.fg" display="flex" alignItems="center"
        justifyContent="center" fontSize="20px" flexShrink={0} aria-hidden>{icon}</Box>
      <VStack align="start" gap={0} flex="1" minW={0}>
        <Text fontWeight="600">{title}</Text>
        <Text fontSize="sm" color="fg.muted">{text}</Text>
      </VStack>
      <ChakraSwitch.Control><ChakraSwitch.Thumb /></ChakraSwitch.Control>
    </ChakraSwitch.Root>
  );
}

// LineSummary es un renglón de la lista (maqueta R3, color P1): el estado como ícono al inicio y solo
// lo excepcional escrito; a la derecha el importe. Tocarlo abre su detalle.
function LineSummary({ n, item, selected, pack, label, otherWarehouse, needsDensity, onOpen }: {
  n: number; item: DraftItem; selected: boolean; pack: string; label: string;
  otherWarehouse: Warehouse | null; needsDensity: boolean; onOpen: () => void;
}) {
  const name = item.itemName || item.description || item.rawName || `Renglón nuevo (${label} o gasto)`;
  const [icon, status] = item.excluded ? [<LuUser key="u" />, 'No es del negocio']
    : !item.arrived ? [<LuClock key="c" />, 'Llega después']
      : item.partQty !== null ? [<LuClock key="c" />, 'Llegó una parte']
        : [<LuCheck key="k" />, ''];
  const amountText = item.isGift ? 'Regalo' : item.amount ? money(item.amount) : 'sin importe';
  return (
    <chakra.button type="button" onClick={onOpen} aria-label={`Renglón ${n}`} aria-current={selected || undefined}
      data-testid="renglon" data-excluded={item.excluded || undefined}
      display="flex" alignItems="center" gap={3} w="100%" minH="60px" px={3} py={2} textAlign="start"
      borderWidth={selected ? '2px' : '1px'} borderStyle={item.excluded ? 'dashed' : 'solid'}
      borderColor={selected ? 'colorPalette.solid' : 'border'} borderRadius="lg"
      bg={selected ? 'colorPalette.subtle' : 'bg.panel'} opacity={item.excluded ? 0.75 : 1} cursor="pointer">
      <Box boxSize="32px" borderRadius="full" display="flex" alignItems="center" justifyContent="center" flexShrink={0}
        bg={status ? 'bg.muted' : 'colorPalette.subtle'} color={item.excluded ? 'fg.subtle' : status ? 'orange.fg' : 'colorPalette.fg'} aria-hidden>
        {icon}
      </Box>
      <VStack align="start" gap={0} flex="1" minW={0}>
        <Text fontWeight="700" truncate maxW="100%">{name}</Text>
        {pack && <Text fontSize="sm" color="fg.muted" truncate maxW="100%">{pack}</Text>}
      </VStack>
      <HStack gap={2} flexShrink={0}>
        {needsDensity && <Box color="orange.fg" aria-label="Falta la densidad"><LuTriangleAlert /></Box>}
        {otherWarehouse && <Badge variant="subtle">{otherWarehouse.isSales ? <LuStore /> : <LuWarehouse />} {otherWarehouse.name}</Badge>}
        {status && <Text fontSize="sm" color={item.excluded ? 'fg.muted' : 'orange.fg'}>{status}</Text>}
        <Text fontWeight="700" w="96px" textAlign="end" textDecoration={item.excluded ? 'line-through' : undefined}
          color={item.amount || item.isGift ? undefined : 'fg.muted'}>{amountText}</Text>
        <LuChevronRight aria-hidden />
      </HStack>
    </chakra.button>
  );
}

// useNarrow dice si la pantalla es angosta (menos de 1100 px): ahí el detalle del renglón no cabe al
// lado de la lista y se abre en una hoja.
function useNarrow() {
  const query = '(max-width: 1099px)';
  const [narrow, setNarrow] = useState(() => typeof window !== 'undefined' && !!window.matchMedia?.(query).matches);
  useEffect(() => {
    const mq = window.matchMedia?.(query);
    if (!mq) return;
    const on = () => setNarrow(mq.matches);
    mq.addEventListener?.('change', on);
    return () => mq.removeEventListener?.('change', on);
  }, []);
  return narrow;
}

// Suggestions muestra los candidatos como chips: un toque para aceptar. Nunca se aplica solo.
function Suggestions({ list, onPick }: { list: ArticleSuggestion[]; onPick: (s: ArticleSuggestion) => void }) {
  return (
    <HStack gap={1} flexWrap="wrap">
      <Text fontSize="xs" color="fg.subtle">¿Es…?</Text>
      {list.slice(0, 3).map((s) => (
        <Button key={`${s.itemType}:${s.itemId}`} minH="44px" size="sm" variant="outline"
          onClick={() => onPick(s)}>
          {s.itemName}
        </Button>
      ))}
    </HStack>
  );
}

// ---- Renglón de pago ----

function PaymentRow({ payment, methods, registers, drawerOut, onChange, onRemove }: {
  payment: DraftPayment;
  drawerOut: { bad: boolean } | null;
  methods: { id: number; name: string; affectsCashDrawer?: boolean }[];
  registers: CashRegister[];
  onChange: (patch: Partial<DraftPayment>) => void;
  onRemove: () => void;
}) {
  const cash = needsRegister(payment, methods);
  return (
    <HStack flexWrap="wrap" gap={3} align="start" bg="bg.subtle" p={3} borderRadius="lg">
      <Field label="Método" flex="1" minW="160px">
        <Picker value={payment.methodId} onChange={(v) => onChange({ methodId: v })}
          placeholder="Método de pago" title="Método de pago"
          options={methods.map((m) => ({ value: String(m.id), label: m.name }))} />
      </Field>
      <Field label="Importe" w="120px">
        <Input inputMode="decimal" placeholder="0.00" value={payment.amount} minH="44px"
          onChange={(e) => onChange({ amount: e.target.value })} />
      </Field>
      {drawerOut && (
        <DrawerOutField amount={payment.amount} value={payment.drawerOut} bad={drawerOut.bad}
          onChange={(v) => onChange({ drawerOut: v })} />
      )}
      <Field label="Fecha del pago" w="160px">
        <DatePickerButton label="Fecha del pago" value={payment.paidOn} onChange={(v) => onChange({ paidOn: v })} />
      </Field>
      <Field label={cash ? 'Caja (obligatoria)' : 'Caja (arqueo)'} flex="1" minW="160px">
        {registers.length === 0 ? (
          <Text fontSize="sm" color={cash ? 'fg.error' : 'fg.muted'}>
            Sin cajas.
          </Text>
        ) : (
          <Picker value={payment.registerId} onChange={(v) => onChange({ registerId: v })}
            placeholder={cash ? 'Elegir caja' : '— Sin arqueo —'} title="Caja"
            clearable={!cash} clearLabel="— No entra al arqueo —"
            options={registers.map((r) => ({ value: String(r.id), label: registerLabel(r) }))} />
        )}
      </Field>
      <HStack mt={5} gap={2}>
        {countsInDrawer(cash, registers, Number(payment.registerId)) && (
          <IncludeInCountCheckbox checked={payment.includeInCount} onChange={(v) => onChange({ includeInCount: v })} />
        )}
        <IconButton minH="44px" minW="44px" variant="ghost" colorPalette="red" aria-label="Quitar pago" onClick={onRemove}>
          <LuTrash2 />
        </IconButton>
      </HStack>
    </HStack>
  );
}

// ---- Resumen del documento leído ----

// amt formatea un importe que viene del DOCUMENTO, no de nuestra aritmética: el extractor lo deja
// vacío cuando no lo pudo leer, y money('') pinta "$0" — un cargo de importe desconocido no es un
// cargo de cero.
function amt(v: string) {
  return v === '' ? 'importe ilegible' : money(v);
}

// DocSummary es el semáforo de confianza de la extracción: qué cuadró, qué no se pudo leer y
// qué advirtió el extractor.
function DocSummary({ parsed }: { parsed: ParsedDoc }) {
  const { doc, reconciliation: r } = parsed;
  const problems = [...r.unreadable, ...doc.warnings];
  if (r.balanced && problems.length === 0 && doc.charges.length === 0) return null;
  return (
    <VStack align="stretch" gap={2} p={3} borderRadius="lg" bg="bg.subtle" borderWidth="1px">
      <HStack gap={2} flexWrap="wrap">
        <Badge colorPalette={r.balanced ? 'green' : 'orange'}>
          {r.balanced ? 'El ticket cuadra' : `Diferencia ${money(r.diff)}`}
        </Badge>
        <Text fontSize="xs" color="fg.muted">
          renglones {money(r.linesSum)}
          {r.chargesSum !== '0' && ` · cargos ${money(r.chargesSum)}`}
          {' · total '}{money(r.total)}
        </Text>
      </HStack>
      {doc.charges.length > 0 && (
        <Text fontSize="xs" color="fg.muted">
          {doc.charges.map((c) => `${c.label} ${amt(c.amount)}${c.affectsTotal ? '' : ' (ya incluido)'}`).join(' · ')}
        </Text>
      )}
      {problems.length > 0 && (
        <VStack align="stretch" gap={0.5}>
          {problems.slice(0, 6).map((p, i) => (
            <HStack key={i} gap={1} align="start">
              <Box mt="2px" color="orange.fg"><LuTriangleAlert size={12} /></Box>
              <Text fontSize="xs" color="fg.muted">{p}</Text>
            </HStack>
          ))}
        </VStack>
      )}
    </VStack>
  );
}

// ---- Piezas de layout ----

function Section({ title, action, children }: { title: string; action?: ReactNode; children: ReactNode }) {
  return (
    <Box>
      <HStack justify="space-between" mb={2} flexWrap="wrap" gap={2}>
        <Text fontWeight="700">{title}</Text>
        <HStack gap={2}>{action}</HStack>
      </HStack>
      <Separator mb={3} />
      {children}
    </Box>
  );
}

function Field({ label, children, ...box }: { label: string; children: ReactNode } & Record<string, unknown>) {
  return (
    <Box {...box}>
      <Text fontSize="xs" color="fg.muted" mb={1}>{label}</Text>
      {children}
    </Box>
  );
}

function Warn({ children }: { children: ReactNode }) {
  return (
    <HStack gap={2} mt={2} color="orange.fg">
      <LuTriangleAlert size={14} />
      <Text fontSize="sm">{children}</Text>
    </HStack>
  );
}
