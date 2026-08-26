import { useRef, useState, type ReactNode } from 'react';
import {
  Box, Button, HStack, VStack, Text, Input, Badge, IconButton, Separator,
} from '@chakra-ui/react';
import { LuPlus, LuTrash2, LuUpload, LuTriangleAlert, LuSparkles } from 'react-icons/lu';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';

import { Picker, type PickerOption } from '../../components/Picker';
import { Switch } from '../../components/ui/switch';
import { toaster } from '../../components/ui/toaster';
import {
  DialogRoot, DialogBackdrop, DialogContent, DialogBody, DialogHeader, DialogTitle, DialogCloseTrigger,
} from '../../components/ui/dialog';
import { posApi } from '../../api/pos';
import { money } from '../../utils/format';
import { packToBase, lineAmount, splitTotals } from './expenseDraft';
import {
  backofficeApi, type ExpenseCategory, type Supplier, type ArticleSuggestion,
  type ExpenseItemBody, type ExpensePaymentBody, type ParsedDoc, type DocLine,
} from '../../api/backoffice';

// Alta de gasto con mercancía y pagos.
//
// Tres secciones porque son tres cosas que se mueven en tiempos distintos: el DOCUMENTO (qué
// se compró y cuándo se emitió), la MERCANCÍA (que toca el almacén al recibirse, no al
// capturarse) y los PAGOS (que pueden ser varios y en fechas distintas).
//
// El documento se puede subir para prellenar las líneas, pero nada se guarda sin que el
// operador confirme: una extracción equivocada cuesta una corrección, no un inventario mal.

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
  amount: string;
  packQtyInBase: string;
  qtyReceived: string;
  // suggestion: de dónde salió el artículo asignado, para que el operador sepa si revisar.
  suggestion: ArticleSuggestion | null;
  suggestions: ArticleSuggestion[];
  docStatus: DocLine['status'];
  // personal: venía en el ticket pero no es del local (el shampoo de la casa). No suma al gasto
  // ni toca el almacén; se manda igual para que el proveedor lo recuerde la próxima vez.
  personal: boolean;
}

interface DraftPayment {
  key: string;
  methodId: string;
  amount: string;
  paidOn: string;
  registerId: string;
  reference: string;
}

const today = () => new Date().toISOString().slice(0, 10);
let seq = 0;
const nextKey = () => `k${++seq}`;


const emptyItem = (): DraftItem => ({
  key: nextKey(), rawCode: '', rawName: '', description: '', itemType: '', itemId: null,
  itemName: '', quantity: '1', unitId: null, amount: '', packQtyInBase: '', qtyReceived: '',
  suggestion: null, suggestions: [], docStatus: '', personal: false,
});

const emptyPayment = (): DraftPayment => ({
  key: nextKey(), methodId: '', amount: '', paidOn: today(), registerId: '', reference: '',
});

export function ExpenseDialog({ open, onClose, onSaved }: {
  open: boolean; onClose: () => void; onSaved: () => void;
}) {
  const qc = useQueryClient();
  const { data: cats } = useQuery({ queryKey: ['expense-cats'], queryFn: backofficeApi.expenseCategories });
  const { data: sups } = useQuery({ queryKey: ['suppliers'], queryFn: backofficeApi.suppliers });
  const { data: methods } = useQuery({ queryKey: ['payment-methods'], queryFn: posApi.paymentMethods });
  const { data: registers } = useQuery({ queryKey: ['cash', 'registers'], queryFn: backofficeApi.cashRegisters });
  const { data: units } = useQuery({ queryKey: ['units'], queryFn: backofficeApi.units });
  // El catálogo completo, una sola vez: el picker filtra en local (buscar no debe costar un
  // round-trip por tecla en un tablet).
  const { data: articles } = useQuery({ queryKey: ['articles'], queryFn: () => backofficeApi.searchArticles('') });

  // ---- Encabezado ----
  const [expenseDate, setExpenseDate] = useState(today());
  const [categoryId, setCategoryId] = useState('');
  const [supplierId, setSupplierId] = useState('');
  const [amount, setAmount] = useState('');
  const [description, setDescription] = useState('');
  const [docKind, setDocKind] = useState('');
  const [docFolio, setDocFolio] = useState('');
  const [docRaw, setDocRaw] = useState<unknown>(undefined);
  // markReceived separado de la fecha: lo normal es volver de la tienda con la mercancía y
  // capturar una sola vez, pero un pedido se registra hoy y llega el jueves.
  const [markReceived, setMarkReceived] = useState(true);
  const [receivedAt, setReceivedAt] = useState(today());

  const [items, setItems] = useState<DraftItem[]>([]);
  const [payments, setPayments] = useState<DraftPayment[]>([]);
  const [parsed, setParsed] = useState<ParsedDoc | null>(null);

  const activeCats = (cats?.items ?? []).filter((c) => c.isActive);
  const activeSups = (sups?.items ?? []).filter((s) => s.isActive);
  const openRegisters = (registers?.items ?? []).filter((r) => r.openSessionId !== null);
  const unitList = units?.items ?? [];
  const methodList = methods?.items ?? [];
  const articleOptions = (articles?.items ?? []).map((a) => ({
    value: `${a.itemType}:${a.id}`, label: a.name, hint: a.unitCode,
  }));

  const reset = () => {
    setExpenseDate(today()); setCategoryId(''); setSupplierId(''); setAmount('');
    setDescription(''); setDocKind(''); setDocFolio(''); setDocRaw(undefined);
    setMarkReceived(true); setReceivedAt(today());
    setItems([]); setPayments([]); setParsed(null);
  };

  // ---- Subir documento ----
  const fileRef = useRef<HTMLInputElement>(null);
  const parse = useMutation({
    mutationFn: (f: File) => backofficeApi.parseDoc(f),
    onSuccess: async (res) => {
      setParsed(res);
      const d = res.doc;
      if (d.issuedOn) setExpenseDate(d.issuedOn);
      if (d.total) setAmount(d.total);
      if (d.kind) setDocKind(d.kind);
      if (d.folio) setDocFolio(d.folio);
      setDocRaw(res.raw);
      if (!description && d.supplier) setDescription(`Compra ${d.supplier}`);

      // Las líneas entran como borrador y se piden sugerencias por renglón. El backend decide:
      // exacta aprendida (autollena) o parecidos (esperan confirmación).
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
          // Un renglón que el documento marca como no surtido entra con 0 recibido: no debe
          // tocar el almacén aunque esté impreso.
          qtyReceived: l.status === 'no_disponible' ? '0' : (l.qty || '1'),
        };
        if (!l.rawName) return base;
        try {
          const { items: s } = await backofficeApi.suggestArticles({
            supplierId: sid, rawCode: l.rawCode, rawName: l.rawName,
          });
          const best = s[0];
          // Aprendido como de la casa: se marca solo y no hay nada que sugerir.
          if (best?.source === 'personal') {
            return { ...base, personal: true, suggestion: best, suggestions: [] };
          }
          if (best?.source === 'aprendido') {
            return {
              ...base, itemType: best.itemType, itemId: best.itemId, itemName: best.itemName,
              // El formato aprendido gana sobre el leído: ya se confirmó una vez con este proveedor.
              packQtyInBase: best.packQtyInBase || base.packQtyInBase,
              unitId: best.unitId ?? base.unitId, suggestion: best, suggestions: s,
            };
          }
          return { ...base, suggestions: s };
        } catch {
          return base; // una sugerencia que falla no debe tumbar la captura
        }
      }));
      setItems(drafts);
      toaster.create({
        title: `${d.lines.length} renglones leídos`,
        description: res.reconciliation.balanced
          ? 'El documento cuadra consigo mismo.'
          : 'El documento NO cuadra: revisa los importes.',
        type: res.reconciliation.balanced ? 'success' : 'warning',
      });
    },
    onError: (e) => toaster.create({ title: 'No se pudo leer el documento', description: String(e), type: 'error' }),
  });

  // ---- Guardar ----
  // Las líneas de la casa se cuentan aparte: el importe del gasto es solo lo del local.
  const { local: itemsSum, personal: personalSum } = splitTotals(items);
  const paymentsSum = payments.reduce((a, p) => a + (parseFloat(p.amount) || 0), 0);
  const total = parseFloat(amount) || 0;
  const itemsDiff = total - itemsSum;
  const status: 'pendiente' | 'pagada' = paymentsSum > 0 && paymentsSum >= total ? 'pagada' : 'pendiente';

  const create = useMutation({
    mutationFn: () => backofficeApi.createExpense({
      expenseDate,
      receivedAt: markReceived ? receivedAt : undefined,
      categoryId: Number(categoryId),
      supplierId: supplierId ? Number(supplierId) : undefined,
      amount,
      description: description || undefined,
      status,
      docKind: docKind || undefined,
      docFolio: docFolio || undefined,
      docRaw,
      items: items.map((i): ExpenseItemBody => ({
        itemType: i.itemType,
        ingredientId: i.itemType === 'ingrediente' && i.itemId ? i.itemId : undefined,
        productId: i.itemType === 'producto' && i.itemId ? i.itemId : undefined,
        description: i.description || i.rawName || 'Sin descripción',
        quantity: i.quantity || '1',
        unitId: i.unitId ?? undefined,
        qtyReceived: markReceived ? (i.qtyReceived || i.quantity || '0') : undefined,
        amount: i.amount || '0',
        packQtyInBase: i.packQtyInBase || undefined,
        rawCode: i.rawCode || undefined,
        rawName: i.rawName || undefined,
        personal: i.personal || undefined,
      })),
      payments: payments.map((p): ExpensePaymentBody => ({
        methodId: Number(p.methodId),
        amount: p.amount,
        paidOn: p.paidOn || undefined,
        registerId: p.registerId ? Number(p.registerId) : undefined,
        reference: p.reference || undefined,
      })),
    }),
    onSuccess: () => { reset(); onSaved(); },
    onError: (e) => toaster.create({ title: 'No se pudo registrar', description: String(e), type: 'error' }),
  });

  // Una línea inventariable exige unidad (sin ella el backend no puede convertir a almacén);
  // se valida aquí para no gastar un round-trip en un 400 evitable.
  const badItem = items.find((i) => i.itemType !== '' && !i.unitId);
  const badPayment = payments.find((p) => !p.methodId || !(parseFloat(p.amount) > 0) || (needsRegister(p, methodList) && !p.registerId));
  const canSave = !!categoryId && total > 0 && !badItem && !badPayment;

  const setItem = (key: string, patch: Partial<DraftItem>) =>
    setItems((prev) => prev.map((i) => (i.key === key ? { ...i, ...patch } : i)));

  return (
    <DialogRoot open={open} onOpenChange={(e) => { if (!e.open) { onClose(); reset(); } }}
      placement="center" size="cover" scrollBehavior="inside">
      <DialogBackdrop />
      <DialogContent>
        <DialogHeader><DialogTitle>Nuevo gasto</DialogTitle></DialogHeader>
        <DialogCloseTrigger />
        <DialogBody pb={6}>
          <VStack align="stretch" gap={5} maxW="1100px" mx="auto">

            {/* ---- 1. Documento ---- */}
            <Section title="Documento" action={
              <>
                <input ref={fileRef} type="file" accept="application/pdf,image/*" hidden
                  onChange={(e) => { const f = e.target.files?.[0]; if (f) parse.mutate(f); e.target.value = ''; }} />
                <Button size="sm" variant="outline" loading={parse.isPending} onClick={() => fileRef.current?.click()}>
                  <LuUpload /> Subir ticket o factura
                </Button>
              </>
            }>
              <HStack flexWrap="wrap" gap={3} align="start">
                <Field label="Fecha del documento" w="170px">
                  <Input type="date" value={expenseDate} onChange={(e) => setExpenseDate(e.target.value)} />
                </Field>
                <Field label="Categoría" flex="1" minW="180px">
                  <Picker value={categoryId} onChange={setCategoryId} placeholder="Elegir categoría" title="Categoría"
                    options={activeCats.map((c) => ({ value: String(c.id), label: c.name }))}
                    onCreate={async (name) => {
                      const c = await backofficeApi.createExpenseCategory({ name, financialGroup: 'operacional' });
                      qc.setQueryData(['expense-cats'], (old?: { items: ExpenseCategory[] }) => ({ items: [...(old?.items ?? []), c] }));
                      return { value: String(c.id), label: c.name };
                    }} />
                </Field>
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
                <Field label="Importe total" w="140px">
                  <Input type="number" inputMode="decimal" placeholder="0.00" value={amount}
                    onChange={(e) => setAmount(e.target.value)} />
                </Field>
              </HStack>
              <HStack flexWrap="wrap" gap={3} align="start" mt={3}>
                <Field label="Comentario" flex="1" minW="240px">
                  <Input placeholder="Concepto" value={description} onChange={(e) => setDescription(e.target.value)} />
                </Field>
                <Field label="Folio" w="180px">
                  <Input placeholder="—" value={docFolio} onChange={(e) => setDocFolio(e.target.value)} />
                </Field>
              </HStack>
              {parsed && <DocSummary parsed={parsed} />}
            </Section>

            {/* ---- 2. Mercancía ---- */}
            <Section title="Mercancía" action={
              <Button size="sm" variant="outline" onClick={() => setItems((p) => [...p, emptyItem()])}>
                <LuPlus /> Agregar artículo
              </Button>
            }>
              {items.length === 0 ? (
                <Text color="fg.muted" fontSize="sm">
                  Sin detalle. Sube el documento o agrega los artículos a mano; sin líneas el gasto se
                  registra solo como importe (no toca el almacén).
                </Text>
              ) : (
                <VStack align="stretch" gap={2}>
                  {items.map((it) => (
                    <ItemRow key={it.key} item={it} units={unitList} articles={articleOptions}
                      showReceived={markReceived}
                      onChange={(patch) => setItem(it.key, patch)}
                      onRemove={() => setItems((p) => p.filter((x) => x.key !== it.key))} />
                  ))}
                </VStack>
              )}
              {items.length > 0 && (
                <HStack justify="space-between" mt={3} flexWrap="wrap" gap={2}>
                  <Switch checked={markReceived} onCheckedChange={(e) => setMarkReceived(e.checked)}>
                    La mercancía ya llegó
                  </Switch>
                  {markReceived && (
                    <Field label="Fecha de recepción" w="170px">
                      <Input type="date" value={receivedAt} onChange={(e) => setReceivedAt(e.target.value)} />
                    </Field>
                  )}
                  <VStack align="end" gap={0}>
                    <Text fontSize="sm" color={Math.abs(itemsDiff) < 0.01 ? 'fg.muted' : 'orange.600'}>
                      Líneas {money(itemsSum.toFixed(2))} de {money(total.toFixed(2))}
                      {Math.abs(itemsDiff) >= 0.01 && ` · diferencia ${money(itemsDiff.toFixed(2))}`}
                    </Text>
                    {personalSum > 0 && (
                      <Text fontSize="xs" color="purple.600">
                        de la casa {money(personalSum.toFixed(2))} (fuera del gasto)
                      </Text>
                    )}
                  </VStack>
                </HStack>
              )}
              {/* El total viene del documento e incluye lo de la casa. Restarlo solo cuando cuadra
                  exacto: así el botón es un hecho verificado, no una corrección a ciegas. */}
              {personalSum > 0 && Math.abs(itemsDiff - personalSum) < 0.01 && (
                <HStack gap={2} mt={2} flexWrap="wrap">
                  <Text fontSize="sm" color="fg.muted">
                    El total del documento incluye {money(personalSum.toFixed(2))} de la casa.
                  </Text>
                  <Button size="xs" variant="subtle" colorPalette="purple"
                    onClick={() => setAmount((total - personalSum).toFixed(2))}>
                    Dejar el gasto en {money((total - personalSum).toFixed(2))}
                  </Button>
                </HStack>
              )}
              {badItem && (
                <Warn>Un artículo de inventario necesita unidad de compra para poder descontarse del almacén.</Warn>
              )}
              {/* Una sola vez, no por renglón: sin proveedor no hay a quién atribuir el aprendizaje. */}
              {items.some((i) => i.rawName) && !supplierId && (
                <Warn>Elige un proveedor para que el sistema recuerde estos artículos en la próxima compra.</Warn>
              )}
            </Section>

            {/* ---- 3. Pagos ---- */}
            <Section title="Pagos" action={
              <Button size="sm" variant="outline" onClick={() => setPayments((p) => [...p, emptyPayment()])}>
                <LuPlus /> Agregar pago
              </Button>
            }>
              {payments.length === 0 ? (
                <Text color="fg.muted" fontSize="sm">Sin pagos: el gasto queda pendiente (cuenta por pagar).</Text>
              ) : (
                <VStack align="stretch" gap={3}>
                  {payments.map((p) => (
                    <PaymentRow key={p.key} payment={p} methods={methodList} registers={openRegisters}
                      onChange={(patch) => setPayments((prev) => prev.map((x) => (x.key === p.key ? { ...x, ...patch } : x)))}
                      onRemove={() => setPayments((prev) => prev.filter((x) => x.key !== p.key))} />
                  ))}
                </VStack>
              )}
              {payments.length > 0 && (
                <HStack justify="end" mt={3}>
                  <Text fontSize="sm" color={paymentsSum >= total ? 'green.600' : 'fg.muted'}>
                    Pagado {money(paymentsSum.toFixed(2))} de {money(total.toFixed(2))}
                    {status === 'pagada' ? ' · queda pagada' : ' · queda pendiente'}
                  </Text>
                </HStack>
              )}
              {badPayment && <Warn>Cada pago necesita método e importe; en efectivo además la caja de la que sale.</Warn>}
            </Section>

            <Button size="lg" colorPalette="green" disabled={!canSave} loading={create.isPending}
              onClick={() => create.mutate()}>
              {status === 'pagada' ? 'Registrar y pagar' : 'Registrar pendiente'}
              {markReceived && items.some((i) => i.itemType) ? ' · entra al almacén' : ''}
            </Button>
          </VStack>
        </DialogBody>
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

// ---- Renglón de mercancía ----

// La marca "es de la casa" es una opción MÁS del picker del artículo, no un control aparte: ese
// picker ya contesta "¿qué es este renglón?" y las tres respuestas posibles (un artículo, algo no
// inventariable, algo que no es del local) caben en la misma pregunta. Un toque, cero controles
// nuevos peleando por el ancho de la pantalla.
const CASA = 'casa';

function withCasa(options: PickerOption[]): PickerOption[] {
  return [{ value: CASA, label: '🏠 De la casa (no es del local)' }, ...options];
}

function ItemRow({ item, units, articles, showReceived, onChange, onRemove }: {
  item: DraftItem;
  units: { id: number; code: string; name: string; kind: string }[];
  articles: PickerOption[];
  showReceived: boolean;
  onChange: (patch: Partial<DraftItem>) => void;
  onRemove: () => void;
}) {
  // created: los artículos dados de alta inline en esta sesión, para que aparezcan en el picker
  // sin recargar el catálogo completo.
  const [created, setCreated] = useState<PickerOption[]>([]);
  const options = created.length ? [...articles, ...created] : articles;

  const pick = (v: string) => {
    if (v === CASA) {
      onChange({ personal: true, itemType: '', itemId: null, itemName: '' });
      return;
    }
    if (!v) {
      onChange({ personal: false, itemType: '', itemId: null, itemName: '' });
      return;
    }
    const [type, id] = v.split(':');
    const opt = options.find((o) => o.value === v);
    onChange({
      itemType: type as 'ingrediente' | 'producto',
      itemId: Number(id),
      itemName: opt?.label ?? '',
      suggestion: null,
      personal: false,
    });
  };

  const notReceived = item.docStatus === 'no_disponible';

  // Tarjeta, no renglón de tabla: en 7" seis columnas dejan los campos numéricos en 30px y el
  // operador no ve lo que teclea. Aquí el artículo ocupa el ancho y los números se acomodan solos.
  return (
    <Box borderWidth="1px" borderRadius="lg" p={3} bg="bg.panel"
      opacity={notReceived ? 0.55 : 1} borderStyle={item.personal ? 'dashed' : 'solid'}>
      <VStack align="stretch" gap={2}>
        <HStack gap={2} align="start">
          <VStack align="stretch" gap={1} flex="1" minW={0}>
            {item.rawName && (
              <HStack gap={2} flexWrap="wrap">
                <Text fontSize="xs" color="fg.subtle">{item.rawCode && `${item.rawCode} · `}{item.rawName}</Text>
                {notReceived && <Badge colorPalette="orange" size="sm">no surtido</Badge>}
                {item.docStatus === 'ajustado' && <Badge colorPalette="blue" size="sm">ajustado</Badge>}
                {item.personal && <Badge colorPalette="purple" size="sm">de la casa</Badge>}
              </HStack>
            )}
            <Picker size="sm" value={item.personal ? CASA : (item.itemId ? `${item.itemType}:${item.itemId}` : '')}
              onChange={pick} placeholder="— No inventariable —" title="¿Qué es este renglón?"
              clearable clearLabel="— No inventariable (no toca almacén) —"
              options={withCasa(options)} searchThreshold={0}
              onCreate={async (name) => {
                // Alta rápida: unidad = la de compra de la línea si ya se eligió, y si no pieza.
                const unit = units.find((u) => u.id === item.unitId) ?? units.find((u) => u.code === 'pieza');
                const ing = await backofficeApi.createIngredient({ name, baseUnitId: unit?.id ?? 1 });
                const value = `ingrediente:${ing.id}`;
                setCreated((p) => [...p, { value, label: ing.name }]);
                return { value, label: ing.name };
              }} />
            {item.suggestion && (
              <HStack gap={1}>
                <LuSparkles size={12} />
                <Text fontSize="xs" color="green.600">
                  reconocido de «{item.suggestion.matchedVia}»
                </Text>
              </HStack>
            )}
            {!item.itemId && !item.personal && item.suggestions.length > 0 && (
              <Suggestions list={item.suggestions} onPick={(s) => onChange({
                itemType: s.itemType, itemId: s.itemId, itemName: s.itemName,
                packQtyInBase: item.packQtyInBase || s.packQtyInBase || '',
                unitId: item.unitId ?? s.unitId ?? null,
                personal: false,
              })} />
            )}
          </VStack>
          {/* Separado del resto de los controles: borrar no se deshace y el dedo es gordo. */}
          <IconButton size="sm" variant="ghost" colorPalette="red" aria-label="Quitar" onClick={onRemove}>
            <LuTrash2 />
          </IconButton>
        </HStack>

        <HStack gap={2} flexWrap="wrap" align="start">
          {item.personal ? (
            <Field label="Importe (no suma al gasto)" w="180px">
              <Input size="sm" type="number" inputMode="decimal" textAlign="end" value={item.amount}
                onChange={(e) => onChange({ amount: e.target.value })} />
            </Field>
          ) : (<>
          <Field label="Cant" w="88px">
            <Input size="sm" type="number" inputMode="decimal" value={item.quantity}
              onChange={(e) => onChange({ quantity: e.target.value })} />
          </Field>
          <Field label="Unidad" w="104px">
            <Picker size="sm" value={item.unitId ? String(item.unitId) : ''}
              onChange={(v) => onChange({ unitId: v ? Number(v) : null })}
              placeholder="—" title="Unidad de compra" clearable clearLabel="—"
              options={units.map((u) => ({ value: String(u.id), label: u.code, hint: u.name }))} />
          </Field>
          {/* Contenido de UNA unidad en unidad base: es lo que permite descontar "4 piezas" de un
              ingrediente que se lleva en gramos. */}
          <Field label="Contenido" w="104px">
            <Input size="sm" type="number" inputMode="decimal" placeholder="—" value={item.packQtyInBase}
              onChange={(e) => onChange({ packQtyInBase: e.target.value })} />
          </Field>
          <Field label="Importe" w="104px">
            <Input size="sm" type="number" inputMode="decimal" textAlign="end" value={item.amount}
              onChange={(e) => onChange({ amount: e.target.value })} />
          </Field>
          {showReceived && (
            <Field label="Llegó" w="88px">
              <Input size="sm" type="number" inputMode="decimal" value={item.qtyReceived}
                onChange={(e) => onChange({ qtyReceived: e.target.value })} />
            </Field>
          )}
          </>)}
        </HStack>
      </VStack>
    </Box>
  );
}

// Suggestions muestra los candidatos como chips: un toque para aceptar. Nunca se aplica solo.
function Suggestions({ list, onPick }: { list: ArticleSuggestion[]; onPick: (s: ArticleSuggestion) => void }) {
  return (
    <HStack gap={1} flexWrap="wrap">
      <Text fontSize="xs" color="fg.subtle">¿Es…?</Text>
      {list.slice(0, 3).map((s) => (
        <Button key={`${s.itemType}:${s.itemId}`} size="xs" variant="outline" colorPalette="blue"
          onClick={() => onPick(s)}>
          {s.itemName}
          <Text as="span" fontSize="2xs" color="fg.subtle" ml={1}>{Math.round(s.score * 100)}%</Text>
        </Button>
      ))}
    </HStack>
  );
}

// ---- Renglón de pago ----

function PaymentRow({ payment, methods, registers, onChange, onRemove }: {
  payment: DraftPayment;
  methods: { id: number; name: string; affectsCashDrawer?: boolean }[];
  registers: { id: number; name: string }[];
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
        <Input type="number" inputMode="decimal" placeholder="0.00" value={payment.amount}
          onChange={(e) => onChange({ amount: e.target.value })} />
      </Field>
      <Field label="Fecha del pago" w="160px">
        <Input type="date" value={payment.paidOn} onChange={(e) => onChange({ paidOn: e.target.value })} />
      </Field>
      <Field label={cash ? 'Caja (obligatoria)' : 'Caja (arqueo)'} flex="1" minW="160px">
        {registers.length === 0 ? (
          <Text fontSize="sm" color={cash ? 'red.500' : 'fg.muted'}>
            {cash ? 'Abre una caja para pagar en efectivo.' : 'Sin cajas abiertas.'}
          </Text>
        ) : (
          <Picker value={payment.registerId} onChange={(v) => onChange({ registerId: v })}
            placeholder={cash ? 'Elegir caja' : '— Sin arqueo —'} title="Caja"
            clearable={!cash} clearLabel="— No entra al arqueo —"
            options={registers.map((r) => ({ value: String(r.id), label: r.name }))} />
        )}
      </Field>
      <IconButton mt={5} size="sm" variant="ghost" colorPalette="red" aria-label="Quitar pago" onClick={onRemove}>
        <LuTrash2 />
      </IconButton>
    </HStack>
  );
}

// ---- Resumen del documento leído ----

// DocSummary es el semáforo de confianza de la extracción: qué cuadró, qué no se pudo leer y
// qué advirtió el extractor. Se muestra siempre, porque un documento que no cuadra sigue
// siendo válido (un pedido con descuento a nivel documento) y el operador tiene que decidir.
// amt formatea un importe que viene del DOCUMENTO, no de nuestra aritmética: el extractor lo deja
// vacío cuando no lo pudo leer, y money('') pinta "$0" — un cargo de importe desconocido no es un
// cargo de cero. El motivo aparece en las advertencias de abajo.
function amt(v: string) {
  return v === '' ? 'importe ilegible' : money(v);
}

function DocSummary({ parsed }: { parsed: ParsedDoc }) {
  const { doc, reconciliation: r } = parsed;
  const problems = [...r.unreadable, ...doc.warnings];
  return (
    <VStack align="stretch" gap={2} mt={3} p={3} borderRadius="lg" bg="bg.subtle" borderWidth="1px">
      <HStack gap={2} flexWrap="wrap">
        <Badge colorPalette={r.balanced ? 'green' : 'orange'}>
          {r.balanced ? 'El documento cuadra' : `Diferencia ${money(r.diff)}`}
        </Badge>
        {r.hasSubtotal && !r.linesMatchSubtotal && (
          <Badge colorPalette="orange">Las líneas no explican el subtotal impreso</Badge>
        )}
        <Text fontSize="xs" color="fg.muted">
          líneas {money(r.linesSum)}
          {r.chargesSum !== '0' && ` · cargos ${money(r.chargesSum)}`}
          {r.breakdownSum !== '0' && ` · impuesto desglosado ${money(r.breakdownSum)}`}
          {' · total '}{money(r.total)}
        </Text>
      </HStack>
      {doc.charges.length > 0 && (
        <Text fontSize="xs" color="fg.muted">
          {doc.charges.map((c) => `${c.label} ${amt(c.amount)}${c.affectsTotal ? '' : ' (ya incluido)'}`).join(' · ')}
        </Text>
      )}
      {doc.payments.length > 0 && (
        <Text fontSize="xs" color="fg.muted">
          Pagos del documento: {doc.payments.map((p) => `${p.method} ${amt(p.amount)}`).join(' + ')}
        </Text>
      )}
      {problems.length > 0 && (
        <VStack align="stretch" gap={0.5}>
          {problems.slice(0, 6).map((p, i) => (
            <HStack key={i} gap={1} align="start">
              <Box mt="2px" color="orange.500"><LuTriangleAlert size={12} /></Box>
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
    <HStack gap={2} mt={2} color="orange.600">
      <LuTriangleAlert size={14} />
      <Text fontSize="sm">{children}</Text>
    </HStack>
  );
}
