import { useEffect, useMemo, useState } from 'react';
import { can } from '../app/permissions';
import { useSessionStore } from '../stores/session';
import { useHoraDelNegocio } from '../hooks/useHoraDelNegocio';
import { ModePicker, type SplitMode } from './cobro/ModePicker';
import { ByProducts } from './cobro/ByProducts';
import { PaymentChips } from './cobro/PaymentChips';
import { EvenSplit } from './cobro/EvenSplit';
import { nextPart } from './cobro/split';
import { ByAmount } from './cobro/ByAmount';
import { PaymentDetail } from './cobro/PaymentDetail';
import { MoveToOrder, NEW_ORDER, type MoveTarget } from './cobro/MoveToOrder';
import { listOrder, type ListRowState } from './cobro/listOrder';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
} from '../components/ui/drawer';
import { Box, Button, HStack, VStack, Text, Input, SimpleGrid, Flex } from '@chakra-ui/react';
import { LuArrowRightLeft, LuReceipt, LuSplit, LuTag } from 'react-icons/lu';
import { toaster } from '../components/ui/toaster';
import { medirAccion } from '../api/uso';
import { posApi } from '../api/pos';
import { descuentoDeLaCuenta, type ModoDeDescuento } from '../domain/descuento';
import { ApiError } from '../api/client';
import { VerTicket } from './tickets/ReprintTicket';
import { TicketPreview } from './tickets/TicketPreview';
import type { ChargeShape, CobroHecho, Currency, OrderView, PedidoParaCobrar, ReceiptOrder, SelectedPieces } from '../types/pos';
import { money } from '../utils/format';
import { TAP_LG, TAP_XL } from '../theme/ui';
import { useUiStore } from '../stores/ui';
import { uuid } from '../utils/uuid';
import { esEfectivo, metodosDeLaLista } from '../domain/metodosDePago';
import { billetesUtiles, presetsDePropina, validarCobro, round2 } from '../domain/cobro';
import type { MotivoInvalido } from '../domain/cobro';

// CuentaParaCobrar es lo que la hoja necesita para pintarse y cobrar.
//
// `id` y `number` son NULOS mientras el pedido no exista. Ese es el caso nuevo: tocar COBRAR ya no
// crea el pedido —no manda nada a cocina—, así que la hoja se abre sobre la cuenta y el pedido nace
// cuando se toca el botón final. Un `PedidoParaCobrar` satisface este tipo sin conversión.
export interface CuentaParaCobrar {
  id: number | null;
  number: number | null;
  folioName: string;
  total: string;
  outstanding: string;
  currency: Currency;
  deliveryPlatformId: number | null;
}

interface Props {
  order: CuentaParaCobrar | null;
  // Crea el pedido. Solo se llama si `order.id` es nulo, y SOLO al cobrar: es lo que hace que un
  // toque accidental en COBRAR no mande comida a cocina.
  //
  // Obligatoria cuando el id puede ser nulo. TypeScript no puede atar las dos cosas, así que el
  // camino de cobro lo comprueba y falla ruidoso en vez de cobrar contra un pedido inexistente.
  crearPedido?: () => Promise<PedidoParaCobrar>;
  // Se llama en cuanto el pedido EXISTE, antes de que el cobro entre. Quien la recibe necesita
  // saberlo para no decirle al operador que no pasó nada si el cobro falla después.
  onPedidoCreado?: (pedido: PedidoParaCobrar) => void;
  // El papel que se imprime cuando el pedido TODAVÍA NO existe. Lo arma quien tiene la cuenta —el
  // POS—, no esta hoja: la lista del botón naranja también la usa y ahí no hay carrito ninguno.
  preCuenta?: ReceiptOrder | null;
  onClose: () => void;
  // Se llama tras CADA cobro que entra, con lo que quedó del pedido. Quien la recibe decide qué
  // hacer: la barra solo refresca; el POS, cuando el pedido queda saldado, lo relee para imprimir
  // el ticket con el PAGADO que dice el servidor.
  onCobrado: (res: CobroHecho, orderId: number) => void;
  // Desde qué pantalla se está cobrando, para la medición de uso (spec 017).
  //
  // Obligatoria y sin default: esta hoja se monta desde el POS y desde el tablero de pedidos, y con
  // un literal fijo aquí todos los cobros del tablero se contarían como del POS — `pedidos/cobrar`
  // quedaría en cero permanente, que se lee como «nadie cobra desde ahí» en vez de «está mal
  // medido». Un default la habría dejado igual de rota y en silencio.
  pantalla: 'pos' | 'pedidos';
}

// Traduce el error del servidor a lo que necesita leer quien está cobrando.
//
// Antes salía `String(e)` — el objeto de error crudo — justo en los momentos en que el operador
// tiene el dinero del cliente en la mano y necesita saber qué hacer a continuación.
function loQueLee(e: unknown): { titulo: string; detalle?: string; recargar: boolean } {
  const msg = e instanceof ApiError ? e.message : String(e);
  const codigo = e instanceof ApiError ? e.code : undefined;
  if (/caja/i.test(msg) || codigo === 'NO_OPEN_REGISTER') {
    return { titulo: 'No hay caja abierta', detalle: 'Ábrela y vuelve a cobrar.', recargar: false };
  }
  if (/ya está cobrado|ya está pagado|no puedes cobrar más/i.test(msg)) {
    return { titulo: 'Otra caja acaba de cobrar este pedido', recargar: true };
  }
  if (/ya se registró/i.test(msg)) {
    return {
      titulo: 'Ese cobro ya entró con otro método o monto',
      detalle: 'Revisa el pedido antes de volver a cobrar.',
      recargar: true,
    };
  }
  if (/ya no está activo/i.test(msg)) {
    return {
      titulo: 'Ese método de pago ya no está activo',
      detalle: 'Elige otro, o vuelve a activarlo en Ajustes.',
      recargar: false,
    };
  }
  // Los rechazos de dividir la cuenta ya vienen escritos para quien opera (spec 027). Los que dicen
  // que algo cambió entretanto —otra tableta cobró esa pieza, ese pago ya se devolvió— refrescan el
  // pedido para que la lista deje de ofrecerlo.
  if (/ya se pagó|ya se cobró|ya se devolvió|Vuelve a cobrar|otros productos|Ya es su propio|ya no recibe|otro turno/i.test(msg)) {
    return { titulo: msg, recargar: true };
  }
  if (/no se divide|no se puede pasar|Quita el descuento|tiene envío|devolución|pedido viejo|Ya se cobró más|sin elegir productos|Elige/i.test(msg)) {
    return { titulo: msg, recargar: false };
  }
  if (/plataforma/i.test(msg)) {
    return { titulo: 'Con ese método no se puede cobrar este pedido', recargar: false };
  }
  return { titulo: 'No se pudo cobrar', detalle: msg, recargar: true };
}

// shapeKey es la huella de una forma de cobro, para la llave de la cotización.
function shapeKey(shape: ChargeShape | null): string {
  if (shape === null) return '';
  if ('lines' in shape) return shape.lines.map((l) => `${l.lineId}:${l.qty}`).join(',');
  if ('split' in shape) return `split:${shape.split.part}/${shape.split.of}`;
  return 'all';
}

// useDebounced devuelve `value` cuando lleva `ms` sin cambiar: la cotización se pide una vez por
// ráfaga de toques, no una por toque.
function useDebounced<T>(value: T, ms: number): T {
  const [settled, setSettled] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setSettled(value), ms);
    return () => clearTimeout(t);
  }, [value, ms]);
  return settled;
}

// Cobra un pedido que se mandó a cocina sin cobrar, entero o por pedazos.
//
// UN PEDAZO A LA VEZ, y de ahí sale toda la forma de la pantalla. Capturar tres pagos y mandarlos de
// un golpe registra dinero que todavía no se recibió: si la terminal declina la tarjeta del segundo
// comensal DESPUÉS de que el servidor acusó, el sistema ya lo dio por cobrado. Cobrando de a uno, el
// registro coincide con el instante en que el dinero está en la mano.
//
// «Dividir» abre tres formas (spec 027): por productos —cada quien paga lo suyo—, entre personas y
// por monto. En las dos primeras el monto lo calcula el SERVIDOR (quote) con la misma regla con la
// que cobra; aquí no hay una segunda regla de dinero. Lo pagado vive en el servidor: tras recargar
// la tableta, las fichas y lo gris siguen ahí.
//
// Detalle de un pago y «Pasar a otro pedido» son VISTAS de la misma hoja, no hojas apiladas.
export function CobrarSheet({ order, crearPedido, onPedidoCreado, preCuenta, onClose, onCobrado, pantalla }: Props) {
  const qc = useQueryClient();
  const palette = useUiStore((s) => s.palette);
  const user = useSessionStore((s) => s.user);
  const { zona } = useHoraDelNegocio();
  const [metodo, setMetodo] = useState<number | null>(null);
  const [recibido, setRecibido] = useState('');
  // El pedido, una vez que existe. Nulo mientras la hoja se abrió sobre una cuenta sin confirmar.
  //
  // Se guarda aquí y no se recalcula: al dividir la cuenta, el primer cobro crea el pedido y los
  // siguientes tienen que ir CONTRA ESE MISMO. También es a donde salta la hoja tras pasar productos
  // a un pedido nuevo y tocar «Cobrar #N».
  const [pedidoCreado, setPedidoCreado] = useState<PedidoParaCobrar | null>(null);
  const [viendoPapel, setViendoPapel] = useState<false | { paymentId?: number }>(false);
  const [mode, setMode] = useState<SplitMode>('none');
  const [view, setView] = useState<'charge' | 'detail' | 'move'>('charge');
  const [detailId, setDetailId] = useState<number | null>(null);
  // Cuántas piezas de cada renglón paga esta persona.
  const [selection, setSelection] = useState<Record<number, number>>({});
  // «Todo lo que falta» se manda como tal y no como lista: la lista que la hoja tiene puede estar
  // vieja, y el servidor sabe qué falta.
  const [allRemaining, setAllRemaining] = useState(false);
  const [people, setPeople] = useState(2);
  const [typedAmount, setTypedAmount] = useState('');
  const [propina, setPropina] = useState('');
  // El último rebote del servidor, EN LA HOJA y no solo en un toast: el toast se va solo, y esto se
  // lee justo cuando el operador tiene el dinero del cliente en la mano.
  const [rebote, setRebote] = useState<{ titulo: string; detalle?: string } | null>(null);
  // La llave de ESTE pedazo, estable mientras el pedazo siga sin cobrarse. Rota SOLO al cobrar con
  // éxito: generarla en cada envío volvería a cobrar el pago cuya respuesta se perdió.
  const [llave, setLlave] = useState(uuid);
  // Tras pasar productos a un pedido nuevo, la hoja ofrece cobrarlo sin cerrar una y abrir otra.
  const [movedTo, setMovedTo] = useState<PedidoParaCobrar | null>(null);

  // La hoja monta CERRADA y se abre en el render siguiente. Quien la abre normalmente cierra otra en
  // la MISMA actualización, y sin una transición de cerrado a abierto Chakra 3.37 no la monta (ver
  // AGENTS.md §3). React no da otra forma de provocar una transición al montar.
  const [visible, setVisible] = useState(false);
  // eslint-disable-next-line react-hooks/set-state-in-effect
  useEffect(() => { setVisible(true); }, []);

  const idPedido = pedidoCreado?.id ?? order?.id ?? null;
  const numero = pedidoCreado?.number ?? order?.number ?? null;
  const folio = pedidoCreado?.folioName ?? order?.folioName ?? '';
  const plataforma = pedidoCreado?.deliveryPlatformId ?? order?.deliveryPlatformId ?? null;

  // El pedido VIVO, no la foto que traía el prop: un pedido que otra caja cobró entretanto dejaría de
  // decir la verdad. Cobrar emite su evento SSE, que invalida esta query.
  const { data: vivo } = useQuery({
    queryKey: ['orders', idPedido],
    queryFn: () => posApi.order(idPedido as number),
    enabled: idPedido !== null,
  });

  const { data: metodos } = useQuery({
    queryKey: ['payment-methods'],
    queryFn: posApi.paymentMethods,
    enabled: order !== null,
    // Una tableta encendida lleva horas con el catálogo en caché y puede estar ofreciendo un método
    // que el negocio ya desactivó. Al abrir la hoja se vuelve a preguntar.
    refetchOnMount: 'always',
  });

  const falta = vivo ? Number(vivo.outstanding) : Number(pedidoCreado?.outstanding ?? order?.outstanding ?? 0);
  const totalDelPedido = vivo ? Number(vivo.total) : Number(pedidoCreado?.total ?? order?.total ?? 0);
  const payments = useMemo(() => vivo?.payments ?? [], [vivo]);
  const livePayments = payments.filter((p) => !p.voided);
  const lines = useMemo(() => vivo?.lines ?? [], [vivo]);
  const rows = useMemo(() => listOrder(lines, payments), [lines, payments]);

  // QUÉ FORMAS DE DIVIDIR ADMITE ESTE PEDIDO. Uno de plataforma lo cobró la plataforma entero: solo
  // se reparte por monto. Uno de un turno cerrado tampoco se divide por productos ni partes. Por
  // productos necesita el pedido ya creado: sobre una cuenta sin confirmar no hay renglones aún.
  const splittable = plataforma === null && (vivo ? vivo.canSplit !== false : idPedido === null);
  const available: Array<Exclude<SplitMode, 'none'>> = [
    ...(splittable && idPedido !== null && rows.pending.length > 0 ? ['products' as const] : []),
    ...(splittable ? ['people' as const] : []),
    'amount' as const,
  ];

  // CORREGIR EL DESCUENTO DE UN PEDIDO QUE YA EXISTE. Con un pago hecho ya no se ofrece (D-17): el
  // servidor lo rechaza, porque reescribiría el monto de lo pendiente.
  const [editandoDescuento, setEditandoDescuento] = useState(false);
  const [descuentoTecleado, setDescuentoTecleado] = useState('');
  const [modoDescuento, setModoDescuento] = useState<ModoDeDescuento>('monto');
  const descuentoVigente = Number(vivo?.discount ?? 0);
  const baseDelDescuento = totalDelPedido + descuentoVigente;
  const descuentoTeclado = descuentoDeLaCuenta(descuentoTecleado, modoDescuento, baseDelDescuento);
  // QUITARLO ES DEJAR EL CAMPO VACÍO, y nada más: `{}` le dice al servidor «quita el descuento», y
  // un campo ilegible no puede borrar uno de $50 sin aviso.
  const descuentoNoSePuedeGuardar = descuentoTeclado.malEscrito || descuentoTeclado.excede;
  const puedeDescontar = idPedido !== null && livePayments.length === 0;
  const guardarDescuento = useMutation({
    mutationFn: () => posApi.setOrderDiscount(idPedido as number, descuentoTeclado.paraElServidor ?? {}),
    onSuccess: (pedido) => {
      // Se repinta con lo que DEVOLVIÓ el servidor, nunca con una resta hecha aquí.
      qc.setQueryData(['orders', idPedido], pedido);
      qc.invalidateQueries({ queryKey: ['orders'] });
      setEditandoDescuento(false);
      setRebote(null);
    },
    onError: (e: unknown) => {
      const { titulo, detalle } = loQueLee(e);
      setRebote({ titulo, detalle });
    },
  });

  // LA FORMA DEL COBRO. Nula = monto tecleado o todo lo que falta, que es el cobro de siempre.
  const selected: SelectedPieces[] = Object.entries(selection)
    .filter(([, q]) => q > 0).map(([id, q]) => ({ lineId: Number(id), qty: String(q) }));
  const part = mode === 'people' ? nextPart(people, payments) : null;
  let shape: ChargeShape | null = null;
  if (mode === 'products') {
    if (allRemaining) shape = { allRemaining: true };
    else if (selected.length > 0) shape = { lines: selected };
  } else if (mode === 'people' && part !== null) {
    shape = { split: { part, of: people } };
  }
  const key = shapeKey(shape);
  const settledKey = useDebounced(key, 250);
  const { data: quote, isFetching: quoting, error: quoteError } = useQuery({
    queryKey: ['quote', idPedido, settledKey, vivo?.outstanding],
    queryFn: () => posApi.quoteOrder(idPedido as number, shape as ChargeShape),
    enabled: shape !== null && idPedido !== null && settledKey === key,
    retry: false,
  });
  const quoteFresh = quote !== undefined && settledKey === key && !quoting;

  // El monto que se cobra. Con una forma, el de la cotización; tecleado, lo tecleado; sin dividir,
  // todo lo que falta. Una parte de una cuenta sin confirmar no tiene cotización todavía: el monto
  // lo dice el servidor al cobrarla.
  const firstPartUnconfirmed = mode === 'people' && idPedido === null;
  let monto: string;
  if (mode === 'amount') monto = typedAmount;
  else if (shape !== null) monto = quoteFresh ? quote.amount : '';
  else if (mode === 'products') monto = '';
  else monto = String(falta);

  const elegibles = useMemo(
    // Espejo de domain.MetodoCorrespondeALaPlataforma: ofrecer un método que va a rebotar manda al
    // operador a adivinar cuál sirve, con el cliente enfrente.
    () => metodosDeLaLista(metodos?.items ?? [], plataforma),
    [metodos, plataforma],
  );
  const elegido = elegibles.find((m) => m.id === metodo);
  const efectivo = esEfectivo(elegido);

  const v = validarCobro({
    monto: firstPartUnconfirmed ? String(falta) : monto, metodoId: metodo, propina, recibido,
    esEfectivo: efectivo && !firstPartUnconfirmed, falta, totalDelPedido,
  });

  // Rota todo lo de ESTE pedazo para el siguiente.
  const nextPiece = () => {
    setLlave(uuid());
    setSelection({});
    setAllRemaining(false);
    setTypedAmount('');
    // El método NO se hereda: cada persona paga con lo suyo.
    setMetodo(null);
    setRecibido('');
    setPropina('');
  };

  const cobrar = useMutation({
    mutationFn: async () => {
      // EL PEDIDO NACE AQUÍ, no al abrir la hoja: un toque por equivocación en COBRAR no manda
      // comida a cocina. La creación es idempotente por el id de la cuenta.
      let id = pedidoCreado?.id ?? order?.id ?? null;
      if (id === null) {
        if (!crearPedido) {
          throw new Error('la cuenta no está confirmada y no hay cómo confirmarla');
        }
        const creado = await crearPedido();
        setPedidoCreado(creado);
        onPedidoCreado?.(creado);
        id = creado.id;
      }
      const tip = v.propina > 0 ? { tip: v.propina } : {};
      const splitShape: ChargeShape | null = firstPartUnconfirmed ? { split: { part: 1, of: people } } : shape;
      if (splitShape !== null) {
        return posApi.chargeOrderShape(id, { methodId: metodo!, clientUuid: llave, ...tip, ...splitShape });
      }
      return posApi.chargeOrder(id, { methodId: metodo!, amount: v.monto, ...tip, clientUuid: llave });
    },
    onSuccess: (res) => {
      // Se mide DESPUÉS de que el servidor cobró, nunca antes: medir primero convertiría un cobro en
      // algo que espera a la medición (spec 017).
      medirAccion(pantalla, 'cobrar');
      if (mode === 'products') medirAccion(pantalla, 'split-by-products');
      setRebote(null);
      if (res.yaEstaba) {
        toaster.create({ title: 'Ese cobro ya estaba registrado', type: 'info' });
      }
      // La respuesta entra al MISMO caché del que la hoja lee: es la cifra que acaba de calcular el
      // servidor, y guardarla aparte daría dos lugares con lo que falta.
      const idCobrado = pedidoCreado?.id ?? order?.id;
      qc.setQueryData(['orders', idCobrado], (prev: OrderView | undefined) =>
        (prev ? { ...prev, outstanding: res.outstanding, paid: res.paid } : prev));
      qc.invalidateQueries({ queryKey: ['orders'] });
      qc.invalidateQueries({ queryKey: ['quote'] });
      if (idCobrado !== undefined && idCobrado !== null) onCobrado(res, idCobrado);
      if (res.paid || Number(res.outstanding) <= 0) {
        toaster.create({ title: 'Cobrado', type: 'success' });
        onClose();
        return;
      }
      // Queda saldo: la hoja NO se cierra. Se prepara para la siguiente persona.
      nextPiece();
    },
    onError: (e) => {
      const { titulo, detalle, recargar } = loQueLee(e);
      setRebote({ titulo, detalle });
      toaster.create({ title: titulo, description: detalle, type: 'error' });
      // Tras un error la cifra y lo pagado pueden estar viejos: otra tableta cobró esa pieza.
      if (recargar) {
        qc.invalidateQueries({ queryKey: ['orders'] });
        qc.invalidateQueries({ queryKey: ['quote'] });
        setSelection({});
        setAllRemaining(false);
      }
    },
  });

  const devolver = useMutation({
    mutationFn: ({ paymentId, reason }: { paymentId: number; reason: string }) =>
      posApi.voidPayment(idPedido as number, paymentId, reason),
    onSuccess: () => {
      medirAccion(pantalla, 'void-payment');
      toaster.create({ title: 'Pago devuelto', type: 'success' });
      qc.invalidateQueries({ queryKey: ['orders'] });
      qc.invalidateQueries({ queryKey: ['quote'] });
      setView('charge');
      setDetailId(null);
      setRebote(null);
    },
    onError: (e) => {
      const { titulo, detalle } = loQueLee(e);
      setRebote({ titulo, detalle });
      toaster.create({ title: titulo, description: detalle, type: 'error' });
    },
  });

  // Lo elegido para pasar: la selección, o todo lo pendiente con «Todo lo que falta».
  const toMove: SelectedPieces[] = allRemaining
    ? rows.pending.map((r) => ({ lineId: r.line.id, qty: String(r.free) }))
    : selected;
  const movingEverything = rows.paid.length === 0 && rows.pending.length > 0
    && rows.pending.every((r) => toMove.some((m) => m.lineId === r.line.id && Number(m.qty) >= Number(r.line.quantity)));
  const pasar = useMutation({
    mutationFn: ({ target }: { target: MoveTarget; label: string }) => posApi.moveLines(idPedido as number, {
      clientUuid: llave, toOrderId: target === NEW_ORDER ? null : target, lines: toMove,
    }),
    onSuccess: (res, { target, label }) => {
      medirAccion(pantalla, 'move-lines');
      qc.invalidateQueries({ queryKey: ['orders'] });
      qc.invalidateQueries({ queryKey: ['quote'] });
      setRebote(null);
      if (res.from.mergedIntoOrderId) {
        toaster.create({ title: `Se juntó con ${label}`, type: 'success' });
        onClose();
        return;
      }
      toaster.create({ title: `Pasado a ${target === NEW_ORDER ? `#${res.to.number}` : label}`, type: 'success' });
      nextPiece();
      setView('charge');
      if (target === NEW_ORDER) {
        setMovedTo({
          id: res.to.id, number: res.to.number, folioName: res.to.folioName, total: res.to.total,
          outstanding: res.to.outstanding, currency: res.to.currency, deliveryPlatformId: res.to.deliveryPlatformId,
        });
      }
    },
    onError: (e) => {
      const { titulo, detalle } = loQueLee(e);
      setRebote({ titulo, detalle });
      toaster.create({ title: titulo, description: detalle, type: 'error' });
    },
  });

  if (!order) return null;

  const moneda = order.currency;
  const sending = cobrar.isPending || pasar.isPending;
  const aCubrirEnEfectivo = round2(v.monto + v.propina);
  const billetes = billetesUtiles(aCubrirEnEfectivo);
  // El cambio que sobra se puede dejar como propina de un toque, en vez de que el operador teclee la
  // resta: sin este gesto, «quédese con el cambio» rebotaba con ErrCobroExcede.
  const cambioComoPropina = efectivo && v.cambio > 0 && v.propina === 0
    && round2(v.propina + v.cambio) <= totalDelPedido ? v.cambio : 0;
  const saldado = falta <= 0;

  // `Record<MotivoInvalido, string>` EXHAUSTIVO: un motivo de rechazo nuevo en `domain/cobro` sin
  // escribir aquí qué lee el operador NO COMPILA.
  const textos: Record<MotivoInvalido, string> = {
    'sin-monto': mode === 'products' ? 'Elige qué productos paga.' : 'Escribe cuánto vas a cobrar.',
    'monto-invalido': 'Escribe el monto solo con números y punto, sin comas.',
    'sin-metodo': 'Falta con qué paga.',
    excede: `Es más de lo que falta (${money(String(falta), moneda)}).`,
    'propina-excede': 'La propina no puede ser mayor que la cuenta.',
    'falta-efectivo': `Faltan ${money(String(v.faltaEfectivo), moneda)}.`,
  };
  const waitingQuote = shape !== null && idPedido !== null && !quoteFresh && !quoteError;
  const aviso = saldado ? 'Este pedido ya está cobrado.'
    : waitingQuote ? 'Calculando…'
      : quoteError ? loQueLee(quoteError).titulo
        : textos[v.motivo ?? 'sin-monto'];
  const canCharge = (v.ok || (firstPartUnconfirmed && metodo !== null)) && !waitingQuote && !quoteError;

  const chosenCount = allRemaining ? rows.pending.reduce((n, r) => n + r.free, 0) : selected.reduce((n, s) => n + Number(s.qty), 0);
  const chosenNames = (allRemaining ? rows.pending.map((r) => r.line) : selected.map((s) => lines.find((l) => l.id === s.lineId)))
    .filter((l) => l !== undefined).map((l) => l!.productName);
  // Lo que se sabe de antemano que «Pasar» rechazaría deshabilita el botón con su motivo, antes de
  // abrir la lista: descubrirlo al confirmar es hacer el recorrido dos veces.
  const moveBlocked = descuentoVigente > 0 ? 'Quita el descuento antes de pasar productos'
    : toMove.some((m) => {
      const l = lines.find((x) => x.id === m.lineId);
      if (!l) return false;
      const pending = Number(l.quantity) - Number(l.delivered);
      const k = Number(m.qty);
      return Number(l.delivered) > 0 && pending > 0 && k > pending && k < Number(l.quantity);
    }) ? 'Ese producto tiene piezas entregadas y otras sin entregar. Pásalas todas juntas' : null;
  const canMove = mode === 'products' && toMove.length > 0 && can('orders.move_lines', user);

  const toggle = (r: ListRowState) => {
    setAllRemaining(false);
    setSelection((s) => {
      const next = { ...s };
      if (next[r.line.id]) delete next[r.line.id];
      else next[r.line.id] = 1;
      return next;
    });
  };

  const detail = detailId === null ? null : payments.find((p) => p.id === detailId && (p.voided || true)) ?? null;
  const startSplit = () => {
    setMode(available[0]);
    setRebote(null);
  };

  return (
    <DrawerRoot open={visible} placement="bottom" onOpenChange={(e) => { if (!e.open) onClose(); }} size="md">
      <DrawerBackdrop />
      {/* La paleta del negocio, como el resto del POS. El verde se queda solo en las dos acciones
          que meten dinero —COBRAR y "el cambio es propina"—. La hoja usa el alto de la tableta
          (dvh) y lo que crece va dentro de la zona con scroll: el pie con el botón nunca sale de
          la pantalla, tampoco con el teclado del sistema abierto. */}
      <DrawerContent colorPalette={palette} borderTopRadius="2xl" maxH="100dvh"
        maxW={{ base: '100%', lg: '960px' }} mx="auto">
        <DrawerHeader borderBottomWidth="1px" py={3}>
          <HStack justify="space-between" align="start">
            <Box minW={0}>
              <Text fontWeight="800" fontSize="lg" lineClamp={1}>
                {folio || (numero !== null ? `#${numero}` : 'Cuenta')}
              </Text>
              {/* El folio lo asigna el SERVIDOR al confirmar: enseñar uno inventado es peor que no
                  enseñar ninguno. */}
              <Text fontSize="sm" color="fg.muted">
                {numero !== null ? `#${numero}` : 'Sin confirmar'}
              </Text>
            </Box>
            {(idPedido !== null || preCuenta) && (
              <Button size="sm" minH="44px" variant="outline" colorPalette="gray" flexShrink={0}
                onClick={() => setViendoPapel({})}>
                <LuReceipt /> Cuenta
              </Button>
            )}
            {puedeDescontar && descuentoVigente === 0 && !editandoDescuento && view === 'charge' && (
              <Button size="sm" minH="44px" variant="outline" colorPalette="gray" flexShrink={0}
                onClick={() => { setDescuentoTecleado(''); setModoDescuento('monto'); setEditandoDescuento(true); }}>
                <LuTag /> Descuento
              </Button>
            )}
            {falta > 0 && mode === 'none' && view === 'charge' && (
              <Button size="sm" minH="44px" variant="outline" colorPalette="gray" flexShrink={0} onClick={startSplit}>
                <LuSplit /> Dividir
              </Button>
            )}
            {/* Las DOS cifras: pintando solo el faltante, un pedido de $500 con $300 abonados se
                veía idéntico a uno de $200. */}
            <Box textAlign="right" flexShrink={0}>
              <Text fontSize="xs" color="fg.muted">Total {money(String(totalDelPedido), moneda)}</Text>
              <Text fontWeight="800" fontSize="2xl" lineHeight="1.1">
                Falta {money(String(falta), moneda)}
              </Text>
            </Box>
          </HStack>
        </DrawerHeader>

        <DrawerBody py={3}>
          <Box hidden={view !== 'detail'}>
            {detail && (
              <PaymentDetail payment={detail} lines={lines} currency={moneda} zona={zona}
                canVoid={can('payments.void', user)} voiding={devolver.isPending}
                onBack={() => { setView('charge'); setDetailId(null); }}
                onReprint={() => setViendoPapel({ paymentId: detail.id })}
                onVoid={(reason) => devolver.mutate({ paymentId: detail.id, reason })} />
            )}
          </Box>
          <Box hidden={view !== 'move'}>
            {view === 'move' && idPedido !== null && (
              <MoveToOrder fromOrderId={idPedido} count={chosenCount} names={chosenNames} currency={moneda}
                everything={movingEverything} moving={pasar.isPending}
                onBack={() => setView('charge')}
                onConfirm={(target, label) => pasar.mutate({ target, label })} />
            )}
          </Box>
          <VStack align="stretch" gap={3} hidden={view !== 'charge'}>
            {movedTo && (
              <HStack justify="space-between" px={3} py={2} borderRadius="md" bg="bg.muted">
                <Text fontSize="sm">Se abrió {movedTo.folioName || `#${movedTo.number}`} con lo que se pasó.</Text>
                <Button size="sm" minH="44px" colorPalette="brand"
                  onClick={() => { setPedidoCreado(movedTo); setMovedTo(null); setMode('none'); nextPiece(); }}>
                  Cobrar #{movedTo.number}
                </Button>
              </HStack>
            )}
            {idPedido !== null && descuentoVigente > 0 && !editandoDescuento && (
              <HStack justify="space-between" gap={2}>
                <Text fontSize="sm" color="fg.muted">
                  Descuento −{money(String(descuentoVigente), moneda)}
                </Text>
                {puedeDescontar && (
                  <Button size="sm" minH="44px" px={3} variant="outline" colorPalette="gray"
                    onClick={() => {
                      setDescuentoTecleado(String(descuentoVigente));
                      setModoDescuento('monto');
                      setEditandoDescuento(true);
                    }}>
                    Cambiar el descuento
                  </Button>
                )}
              </HStack>
            )}
            {editandoDescuento && (
              <HStack gap={2}>
                <HStack gap={1} flexShrink={0}>
                  <Button size="sm" minH="44px" minW="44px" px={3} aria-label="Descuento en pesos"
                    aria-pressed={modoDescuento === 'monto'}
                    variant={modoDescuento === 'monto' ? 'solid' : 'outline'}
                    colorPalette={modoDescuento === 'monto' ? undefined : 'gray'}
                    onClick={() => setModoDescuento('monto')}>$</Button>
                  <Button size="sm" minH="44px" minW="44px" px={3} aria-label="Descuento en porcentaje"
                    aria-pressed={modoDescuento === 'pct'}
                    variant={modoDescuento === 'pct' ? 'solid' : 'outline'}
                    colorPalette={modoDescuento === 'pct' ? undefined : 'gray'}
                    onClick={() => setModoDescuento('pct')}>%</Button>
                </HStack>
                <Input flex="1" minW={0} minH="44px" inputMode="decimal" autoFocus aria-label="Descuento"
                  value={descuentoTecleado} onChange={(e) => setDescuentoTecleado(e.target.value)} />
                {descuentoTeclado.malEscrito && (
                  <Text fontSize="xs" color="red.fg" flexShrink={0}>Solo números</Text>
                )}
                {descuentoTeclado.excede && (
                  <Text fontSize="xs" color="red.fg" flexShrink={0}>
                    Máx {modoDescuento === 'pct' ? '100 %' : money(String(baseDelDescuento), moneda)}
                  </Text>
                )}
                <Button size="sm" minH="44px" px={3} colorPalette="brand" flexShrink={0}
                  disabled={descuentoNoSePuedeGuardar}
                  loading={guardarDescuento.isPending} onClick={() => guardarDescuento.mutate()}>
                  Guardar
                </Button>
                <Button size="sm" minH="44px" px={3} variant="ghost" colorPalette="gray" flexShrink={0}
                  onClick={() => setEditandoDescuento(false)}>Cancelar</Button>
              </HStack>
            )}
            {mode !== 'none' && (
              <ModePicker mode={mode} available={available} disabled={sending}
                onChange={(m) => { setMode(m); setSelection({}); setAllRemaining(false); setTypedAmount(''); }}
                onStop={() => { setMode('none'); setSelection({}); setAllRemaining(false); setTypedAmount(''); }} />
            )}
            <PaymentChips payments={payments} currency={moneda}
              onOpen={(p) => { setDetailId(p.id); setView('detail'); }} />

            {mode === 'products' && (
              <ByProducts rows={rows} selection={allRemaining
                ? Object.fromEntries(rows.pending.map((r) => [r.line.id, r.free])) : selection}
                payments={payments} currency={moneda} disabled={sending}
                onToggle={toggle}
                onQty={(lineId, qty) => { setAllRemaining(false); setSelection((s) => ({ ...s, [lineId]: qty })); }} />
            )}
            {mode === 'people' && (
              <EvenSplit of={people} payments={payments} outstanding={String(falta)} currency={moneda}
                currentAmount={quoteFresh ? quote.amount : null} disabled={sending} onChange={setPeople} />
            )}
            {mode === 'amount' && (
              <ByAmount value={typedAmount} outstanding={falta} currency={moneda} disabled={sending} onChange={setTypedAmount} />
            )}

            <Box>
              <Text fontSize="sm" fontWeight="600" mb={2}>¿Con qué paga?</Text>
              {/* Botones y no un desplegable. NINGUNO viene preseleccionado, a propósito: un dedo que
                  va directo a Cobrar registraría con tarjeta dinero que entró en efectivo. */}
              {elegibles.length === 0 ? (
                <Text fontSize="sm" color="fg.muted">
                  Este pedido no tiene métodos de pago configurados. Agrégalos en Ajustes para poder
                  cobrarlo.
                </Text>
              ) : (
                <SimpleGrid columns={{ base: 2, sm: 4 }} gap={2}>
                  {elegibles.map((m) => (
                    <Button key={m.id} minH={TAP_LG} variant={metodo === m.id ? 'solid' : 'outline'}
                      colorPalette={metodo === m.id ? undefined : 'gray'} disabled={sending}
                      onClick={() => { setMetodo(m.id); setRecibido(''); }}>
                      {m.name}
                    </Button>
                  ))}
                </SimpleGrid>
              )}
            </Box>

            {/* Propina. El porcentaje es de lo que se cobra AHORA, no del total del pedido. */}
            {metodo !== null && v.monto > 0 && (
              <Box>
                <Text fontSize="sm" fontWeight="600" mb={2}>Propina</Text>
                <HStack gap={2} flexWrap="wrap">
                  <Button minH={TAP_LG} variant={propina === '' ? 'solid' : 'outline'}
                    colorPalette={propina === '' ? undefined : 'gray'} onClick={() => setPropina('')}>
                    Sin
                  </Button>
                  {presetsDePropina(v.monto).map((p) => {
                    const on = propina === String(p.monto);
                    return (
                      <Button key={p.etiqueta} minH={TAP_LG}
                        variant={on ? 'solid' : 'outline'} colorPalette={on ? undefined : 'gray'}
                        onClick={() => setPropina(String(p.monto))}>
                        <VStack gap={0}>
                          <Text fontSize="2xs" opacity={0.8}>{p.etiqueta}</Text>
                          <Text fontWeight="700">{money(String(p.monto), moneda)}</Text>
                        </VStack>
                      </Button>
                    );
                  })}
                  <Input w="7rem" minH={TAP_LG} inputMode="decimal" placeholder="Otra"
                    aria-label="Otra propina"
                    value={propina} onChange={(e) => setPropina(e.target.value)} />
                </HStack>
              </Box>
            )}

            {/* Con qué billete paga, solo para efectivo: es lo único que produce cambio. */}
            {efectivo && !firstPartUnconfirmed && (
              <Box>
                <Text fontSize="sm" fontWeight="600" mb={2}>¿Con cuánto paga?</Text>
                <HStack gap={2} flexWrap="wrap">
                  <Button minH={TAP_LG} variant={recibido === '' ? 'solid' : 'outline'}
                    colorPalette={recibido === '' ? undefined : 'gray'}
                    onClick={() => setRecibido('')}>
                    Exacto
                  </Button>
                  {billetes.map((b) => (
                    <Button key={b} minH={TAP_LG} variant={recibido === String(b) ? 'solid' : 'outline'}
                      colorPalette={recibido === String(b) ? undefined : 'gray'}
                      onClick={() => setRecibido(String(b))}>
                      {money(String(b), moneda)}
                    </Button>
                  ))}
                  <Input w="7rem" minH={TAP_LG} inputMode="decimal" placeholder="Otro"
                    aria-label="Con cuánto paga"
                    value={recibido} onChange={(e) => setRecibido(e.target.value)} />
                </HStack>
                {v.cambio > 0 && (
                  <Flex mt={2} align="center" justify="space-between" gap={2}>
                    <Text fontWeight="700" fontSize="lg" color="orange.600">
                      Cambio {money(String(v.cambio), moneda)}
                    </Text>
                    {cambioComoPropina > 0 && (
                      <Button minH="44px" size="sm" variant="outline" colorPalette="green"
                        onClick={() => setPropina(String(cambioComoPropina))}>
                        El cambio es propina
                      </Button>
                    )}
                  </Flex>
                )}
              </Box>
            )}
          </VStack>
        </DrawerBody>

        {view === 'charge' && (
          <DrawerFooter borderTopWidth="1px" flexDirection="column" gap={2} alignItems="stretch" maxH="45dvh">
            {rebote && (
              <Box borderWidth="1px" borderColor="red.emphasized" bg="red.subtle" borderRadius="md" px={3} py={2}>
                <Text fontWeight="700" color="red.fg">{rebote.titulo}</Text>
                {rebote.detalle && <Text fontSize="sm" color="fg.muted">{rebote.detalle}</Text>}
              </Box>
            )}
            {mode === 'products' && (
              <HStack justify="space-between" gap={2} flexWrap="wrap">
                <Text fontSize="sm" fontWeight="600" lineClamp={1} flex="1" minW={0}>
                  Esta persona: {chosenCount} producto{chosenCount === 1 ? '' : 's'}
                  {chosenNames.length > 0 ? ` · ${chosenNames.join(' · ')}` : ''}
                </Text>
                <HStack gap={2} flexShrink={0}>
                  <Button minH="44px" size="sm" variant={allRemaining ? 'solid' : 'outline'} colorPalette="gray"
                    disabled={sending || rows.pending.length === 0} aria-pressed={allRemaining}
                    onClick={() => { setAllRemaining((a) => !a); setSelection({}); }}>
                    Todo lo que falta
                  </Button>
                  {can('orders.move_lines', user) && (
                    <Button minH="44px" size="sm" variant="outline" colorPalette="gray"
                      disabled={sending || !canMove || moveBlocked !== null}
                      onClick={() => setView('move')}>
                      <LuArrowRightLeft /> Pasar a otro pedido
                    </Button>
                  )}
                </HStack>
              </HStack>
            )}
            {mode === 'products' && toMove.length > 0 && moveBlocked && (
              <Text fontSize="xs" color="fg.muted" textAlign="right">{moveBlocked}</Text>
            )}
            {!canCharge && aviso && (
              <Text fontSize="sm" color="fg.muted" textAlign="center">{aviso}</Text>
            )}
            {/* Se cobra el MONTO, no lo que entregó el cliente: el excedente es cambio, no ingreso. */}
            {saldado ? (
              <Button w="100%" size="lg" minH={TAP_XL} variant="outline" colorPalette="gray" onClick={onClose}>
                Cerrar
              </Button>
            ) : (
              <Button w="100%" size="lg" minH={TAP_XL} colorPalette="green"
                disabled={!canCharge} loading={cobrar.isPending}
                onClick={() => cobrar.mutate()}>
                {firstPartUnconfirmed
                  ? `Cobrar parte 1 de ${people}`
                  : `Cobrar ${money(String(round2(v.monto + v.propina)), moneda)}`}
              </Button>
            )}
          </DrawerFooter>
        )}
      </DrawerContent>
      {/* El papel se monta dentro del mismo Drawer para que la hoja se quede abierta detrás. */}
      {idPedido !== null
        ? <VerTicket orderId={viendoPapel ? idPedido : null} paymentId={viendoPapel ? viendoPapel.paymentId : undefined}
            onClose={() => setViendoPapel(false)} />
        : <TicketPreview order={preCuenta ?? null} preCuenta
            isOpen={viendoPapel !== false} onClose={() => setViendoPapel(false)} />}
    </DrawerRoot>
  );
}
