import { useState } from 'react';
import { useNavigate } from 'react-router';
import { canAccess } from '../../app/roles';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  Box, SimpleGrid, Text, Badge, HStack, VStack, Center, Spinner, Flex, Button, IconButton,
} from '@chakra-ui/react';
import { MenuRoot, MenuTrigger, MenuContent, MenuItem, MenuSeparator } from '../../components/ui/menu';
import { LuStore, LuBike, LuEllipsisVertical, LuMinus, LuPlus, LuCheck, LuShoppingBag, LuTrash2 } from 'react-icons/lu';
import type { IconType } from 'react-icons';
import { toaster } from '../../components/ui/toaster';
import { mensajeDeError } from '../../api/mensajes';
import { posApi } from '../../api/pos';
import { medirAccion } from '../../api/uso';
import type { BoardLine, BoardOrder } from '../../types/pos';
import { resumenPorCobrar } from './porCobrar';
import { entregados, faltante, pendientes, renglonesDe } from './entrega';
import { money } from '../../utils/format';
import { buildKitchenHtml } from '../../utils/printKitchen';
import { printHtmlOffscreen } from '../../utils/printReceipt';
import { useOrderEvents } from '../../hooks/useOrderEvents';
import { ReprintTicket } from '../../shared/tickets/ReprintTicket';
import { CobrarSheet } from '../../shared/CobrarSheet';
import { DevolucionSheet } from './DevolucionSheet';
import { CancelarRenglonDialog } from '../../shared/CancelarRenglonDialog';
import { CancelPendingSheet } from './CancelPendingSheet';
import { useSessionStore } from '../../stores/session';
import { can } from '../../app/permissions';
import { round2 } from '../../domain/numeros';
import { quedaPorDevolver } from '../../domain/devolucion';
import { diaCortoYHora } from '../../utils/horaDelNegocio';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';
import { tituloDeEntregadas, vacioDeEntregadas } from './ventanaDeEntregadas';

// para_llevar ya no se ofrece al cobrar, pero hay pedidos históricos con ese tipo y sin su etiqueta
// la tarjeta los mostraría como "para_llevar", con guion bajo.
const SERVICE_META: Record<string, { label: string; icon: IconType }> = {
  mostrador: { label: 'Mostrador', icon: LuStore },
  para_llevar: { label: 'Llevar', icon: LuShoppingBag },
  domicilio: { label: 'Domicilio', icon: LuBike },
};

// Alto mínimo de todo lo que se toca. Por debajo el dedo falla, y aquí fallar significa dar por
// entregado o por cobrado lo que no fue.
const TAP = '44px';

// Cuántas entregadas se listan. El resto vive en Ventas, que es la pantalla del histórico; aquí
// estorbarían lo que falta por atender.
const ENTREGADAS_VISIBLES = 5;


export function OrdersBoardPage() {
  const horaNegocio = useHoraDelNegocio();
  const live = useOrderEvents();
  const [ticketOrderID, setTicketOrderID] = useState<number | null>(null);
  const [cobrando, setCobrando] = useState<BoardOrder | null>(null);
  const [devolviendo, setDevolviendo] = useState<{ pedido: BoardOrder; cancelando: boolean } | null>(null);
  // En qué terminal se devuelve lo cobrado con tarjeta, y si pedirá su folio (spec 032).
  const { data: infoDevolucion } = useQuery({
    queryKey: ['refund-info', devolviendo?.pedido.id],
    queryFn: () => posApi.refundInfo(devolviendo!.pedido.id),
    enabled: devolviendo !== null,
  });
  const [quitando, setQuitando] = useState<{ orderId: number; linea: BoardLine } | null>(null);
  const [quitandoFaltante, setQuitandoFaltante] = useState<BoardOrder | null>(null);
  const qc = useQueryClient();
  // Reembolsar = salida de dinero → solo admin/gerente ven las entregadas y la acción. El backend
  // igual aplica el 403; esto es UX (no mostrar lo que no pueden usar).
  const user = useSessionStore((s) => s.user);
  const role = user?.role;
  const canRefund = role === 'admin' || role === 'gerente';

  // COBRAR ES DE LA CUENTA, NO DEL TABLERO (spec 030, caso 25). El tablero tenía su propia puerta
  // de cobro y con ella eran tres; ahora cada tarjeta lleva a la cuenta en Vender —«Abrir cuenta»—,
  // donde se agrega o se cobra con los tres modos.
  //
  // Quien NO puede entrar a Vender —una cocina con su propio rol— solo ve cuánto falta. El ajuste
  // «el tablero puede cobrar» se quitó el 2026-10-09 (decisión del dueño): no tenía efecto en la
  // operación de hoy y era una cuarta puerta de cobro.
  const { data: settings } = useQuery({ queryKey: ['business-settings'], queryFn: posApi.businessSettings });
  const navigate = useNavigate();
  const puedeAbrirCuenta = canAccess(role, '/pos');

  const { data, isLoading } = useQuery({
    queryKey: ['orders', 'active'],
    queryFn: posApi.activeOrders,
    refetchInterval: 10_000, // respaldo si SSE se cae
  });
  const { data: deliveredData } = useQuery({
    queryKey: ['orders', 'delivered'],
    queryFn: posApi.deliveredOrders,
    enabled: canRefund,
    refetchInterval: 15_000, // SSE solo invalida 'active'; refrescamos entregadas aparte
  });

  // El prefijo entero: `active`, `delivered` y también `open`, que es la barra del POS. Cobrar
  // desde aquí dejaba a esa barra contando el dinero ya cobrado hasta su siguiente refresco.
  const invalidateAll = () => qc.invalidateQueries({ queryKey: ['orders'] });
  // El MENSAJE del servidor, no el objeto de error crudo. `String(e)` pintaba
  // "TypeError: Failed to fetch" al caerse la red y "Error: ..." delante de cada rechazo — el mismo
  // defecto que la hoja de cobro documenta como corregido, vivo todavía en entregar, cancelar y
  // reembolsar. La constitución lo prohíbe: en pantalla no van internals.
  const conError = (titulo: string) => (e: unknown) =>
    toaster.create({ title: titulo, description: mensajeDeError(e), type: 'error' });

  const entregarLinea = useMutation({
    mutationFn: ({ id, lineId, qty }: { id: number; lineId: number; qty: number }) =>
      posApi.deliverLine(id, lineId, qty),
    onSuccess: invalidateAll,
    onError: conError('No se pudo entregar'),
  });
  const entregarTodo = useMutation({
    mutationFn: (id: number) => posApi.deliverOrder(id),
    onSuccess: invalidateAll,
    onError: conError('No se pudo entregar'),
  });
  const cancelMut = useMutation({
    mutationFn: ({ id, reason, devolver, cardFolio }: { id: number; reason: string; devolver: boolean; cardFolio?: string }) =>
      posApi.cancelOrder(id, reason, devolver, cardFolio),
    onSuccess: invalidateAll,
    // Un pedido del que ya salió comida no se cancela: reponer el stock de lo que el cliente se
    // llevó le inventaría existencias al almacén. El servidor lo rechaza y aquí se dice por qué.
    onError: conError('No se pudo cancelar'),
  });
  // Cancelar UN renglón. El servidor responde si repuso el inventario, y eso se le dice al operador:
  // lo que ya salió a cocina baja de la cuenta pero no devuelve el ingrediente.
  const cancelarRenglonMut = useMutation({
    mutationFn: ({ id, lineId, reason, qty }: { id: number; lineId: number; reason: string; qty: number }) =>
      posApi.cancelOrderLine(id, lineId, reason, qty),
    onSuccess: (r) => {
      invalidateAll();
      toaster.create({
        title: 'Producto quitado',
        description: r.repusoInventario
          ? 'El ingrediente volvió al almacén.'
          : 'Ya se estaba preparando: el ingrediente no vuelve al almacén.',
        type: 'success',
      });
    },
    onError: conError('No se pudo quitar'),
  });

  // Quitar lo que falta. La hoja espera la respuesta para cerrarse, así que aquí no se atrapa el
  // error: lo pinta la hoja, que sigue abierta.
  const quitarFaltante = (o: BoardOrder, reason: string) =>
    posApi.cancelPendingLines(o.id, reason).then((r) => {
      invalidateAll();
      medirAccion('pedidos', 'cancel-pending');
      toaster.create({
        title: r.removed === 1 ? 'Producto quitado' : `${r.removed} productos quitados`,
        type: 'success',
      });
    });
  // Cerrar un pedido que ya no tiene productos ni pagos. Sin motivo: lo pone el servidor.
  const cerrarVacio = useMutation({
    mutationFn: (id: number) => posApi.cancelPendingLines(id),
    onSuccess: () => { invalidateAll(); medirAccion('pedidos', 'close-order'); },
    onError: conError('No se pudo cerrar'),
  });

  const refundMut = useMutation({
    mutationFn: ({ id, reason, amount, cardFolio }: { id: number; reason: string; amount?: number; cardFolio?: string }) =>
      posApi.refundOrder(id, reason, amount, undefined, cardFolio),
    onSuccess: () => { invalidateAll(); toaster.create({ title: 'Devolución registrada', type: 'success' }); },
    onError: conError('No se pudo devolver'),
  });

  if (isLoading) return <Center h="60vh"><Spinner size="xl" /></Center>;
  const orders = data?.items ?? [];
  const preparando = orders.filter((o) => o.status === 'abierta');
  const listos = orders.filter((o) => o.status === 'lista');
  const entregadas = deliveredData?.items ?? [];
  // Cuenta las entregadas también: es donde un pendiente deja de tener remedio, porque el cliente
  // ya se fue con la comida.
  const pendiente = resumenPorCobrar([...orders, ...entregadas]);

  // La hoja reemplaza al `window.prompt` que pedía el motivo. Ese diálogo lo pinta el sistema
  // operativo: caja de texto por debajo de 44 px, los motivos listados entre paréntesis sin poder
  // tocarlos, y —lo peor— tras varios Chrome ofrece suprimirlos, y a partir de ahí `prompt` devuelve
  // null y la acción deja de hacer nada, en silencio y sin aviso.
  //
  // La misma hoja para las dos: con cobros pide cuánto devolver, sin ellos solo el motivo.
  //
  // Con algo ya entregado, cancelar el pedido entero lo rechaza el servidor —reponer lo que el
  // cliente se llevó le inventaría existencias al almacén—, así que «Cancelar pedido» lleva a lo que
  // sí se puede hacer: quitar lo que falta.
  const cancel = (o: BoardOrder) => {
    const algoEntregado = renglonesDe(o).some((l) => Number(l.delivered) > 0);
    if (algoEntregado && pendientes(o).length > 0) setQuitandoFaltante(o);
    else setDevolviendo({ pedido: o, cancelando: true });
  };
  const refund = (o: BoardOrder) => setDevolviendo({ pedido: o, cancelando: false });

  // La comanda COMPLETA, como acción explícita. La que sale sola al agregar lleva solo lo nuevo
  // —cocina ya está preparando lo anterior—, así que cuando un papel se pierde o la impresora falla
  // hace falta poder pedir el pedido entero. Es el camino de recuperación, y el que nadie ejercita
  // a diario: por eso tiene su test.
  const reimprimirComanda = async (o: BoardOrder) => {
    const detalle = await posApi.order(o.id);
    const salio = await printHtmlOffscreen(buildKitchenHtml(detalle as never, undefined, horaNegocio.zona));
    if (!salio) {
      toaster.create({
        title: 'No salió la comanda',
        description: 'Revisa la impresora e inténtalo otra vez.',
        type: 'warning',
      });
    }
  };

  const acciones: Acciones = {
    puedeAbrirCuenta,
    abrirCuenta: (o) => navigate(`/pos?pedido=${o.id}`),
    entregarLinea: (id, lineId, qty) => entregarLinea.mutate({ id, lineId, qty }),
    quitarRenglon: (id, linea) => setQuitando({ orderId: id, linea }),
    entregarTodo: (o) => entregarTodo.mutate(o.id),
    // La misma petición que «Entregar todo», pero se cuenta aparte: cerrar es la salida que el
    // incidente no tenía, y medir cada entrega normal como cierre la ahogaría.
    cerrarEntregado: (o) => entregarTodo.mutate(o.id, {
      onSuccess: () => medirAccion('pedidos', 'close-order'),
    }),
    cobrar: setCobrando,
    ticket: (o) => setTicketOrderID(o.id),
    comanda: reimprimirComanda,
    cancelar: cancel,
    quitarFaltante: setQuitandoFaltante,
    cerrarVacio: (o) => cerrarVacio.mutate(o.id),
    // Por pedido, no global: la petición de una tarjeta no tiene por qué congelar las demás.
    cerrando: (o) =>
      (entregarTodo.isPending && entregarTodo.variables === o.id) ||
      (cerrarVacio.isPending && cerrarVacio.variables === o.id),
    puedeDevolverPagos: can('payments.void', user),
    puedeCancelar: can('orders.cancel', user),
    puedeQuitarFaltante: can('orders.cancel_pending', user),
    zona: horaNegocio.zona,
  };

  return (
    // p={3} y no p={4}: cada píxel de margen es un píxel menos de comida a la vista, y esta
    // pantalla vive en 600 px de alto.
    <Box p={3} h="100%" overflowY="auto">
      {/* Encabezado de un solo renglón. Antes ocupaba dos con el título en xl. */}
      <HStack mb={3} gap={2} flexWrap="wrap">
        <Text fontSize="lg" fontWeight="800">Pedidos</Text>
        <Badge colorPalette={live ? 'green' : 'gray'}>{live ? 'En vivo' : 'Sin conexión'}</Badge>
        {pendiente.cuantos > 0 && (
          <Badge colorPalette="orange" px={2} py={1}>
            {pendiente.cuantos} por cobrar · {money(String(pendiente.monto))}
          </Badge>
        )}
      </HStack>

      {/* Dos columnas en pantalla ancha, una sola abajo de 900 px: en una tableta de 7" dos
          columnas dejan tarjetas donde el nombre del producto ya no cabe. */}
      <SimpleGrid columns={{ base: 1, md: 2 }} gap={3} alignItems="start">
        <Columna titulo="En preparación" orders={preparando} acciones={acciones} />
        <Columna titulo="A entregar" orders={listos} acciones={acciones} />
      </SimpleGrid>

      {canRefund && (
        <Entregadas orders={entregadas} corteDeVista={settings?.corteDeVista} zona={horaNegocio.zona} onRefund={refund} onTicket={acciones.ticket}
          onAbrirCuenta={puedeAbrirCuenta ? acciones.abrirCuenta : undefined} />
      )}

      <ReprintTicket orderId={ticketOrderID} onClose={() => setTicketOrderID(null)} />
      {/* `key` por pedido: la hoja lleva estado de cobro y con otro pedido nada de eso aplica. */}
      {quitando && (
        <CancelarRenglonDialog
          nombre={quitando.linea.name}
          pendientes={faltante(quitando.linea)}
          yaSalioACocina={quitando.linea.enviadoACocina === true}
          enviando={cancelarRenglonMut.isPending}
          onCerrar={() => setQuitando(null)}
          onConfirmar={(motivo, qty) => {
            cancelarRenglonMut.mutate({ id: quitando.orderId, lineId: quitando.linea.id, reason: motivo, qty });
            setQuitando(null);
          }}
        />
      )}

      {quitandoFaltante && (
        <CancelPendingSheet
          key={quitandoFaltante.id}
          order={quitandoFaltante}
          onClose={() => setQuitandoFaltante(null)}
          onConfirm={(reason) => quitarFaltante(quitandoFaltante, reason)}
        />
      )}

      {devolviendo && (
        <DevolucionSheet
          key={devolviendo.pedido.id}
          pedido={devolviendo.pedido}
          cancelando={devolviendo.cancelando}
          enviando={cancelMut.isPending || refundMut.isPending}
          onCerrar={() => setDevolviendo(null)}
          tarjeta={infoDevolucion ? { terminales: infoDevolucion.cardTerminals, pideFolio: infoDevolucion.needsFolio } : undefined}
          onConfirmar={(monto, motivo, folio) => {
            const { pedido, cancelando } = devolviendo;
            const cardFolio = folio || undefined;
            if (cancelando) {
              cancelMut.mutate({ id: pedido.id, reason: motivo, devolver: monto > 0, cardFolio });
            } else {
              refundMut.mutate({ id: pedido.id, reason: motivo, amount: monto, cardFolio });
            }
            setDevolviendo(null);
          }}
        />
      )}

      <CobrarSheet key={cobrando?.id} order={cobrando} pantalla="pedidos"
        onClose={() => setCobrando(null)} onCobrado={() => invalidateAll()} />
    </Box>
  );
}

interface Acciones {
  // Lleva a la cuenta en Vender. Sin acceso a Vender, la tarjeta solo dice cuánto falta.
  puedeAbrirCuenta: boolean;
  abrirCuenta: (o: BoardOrder) => void;
  entregarLinea: (id: number, lineId: number, qty: number) => void;
  quitarRenglon: (id: number, linea: BoardLine) => void;
  entregarTodo: (o: BoardOrder) => void;
  cerrarEntregado: (o: BoardOrder) => void;
  cobrar: (o: BoardOrder) => void;
  ticket: (o: BoardOrder) => void;
  comanda: (o: BoardOrder) => void;
  cancelar: (o: BoardOrder) => void;
  quitarFaltante: (o: BoardOrder) => void;
  cerrarVacio: (o: BoardOrder) => void;
  cerrando: (o: BoardOrder) => boolean;
  // Se pregunta por permiso, nunca por nombre de rol (app/permissions.ts).
  puedeCancelar: boolean;
  puedeDevolverPagos: boolean;
  puedeQuitarFaltante: boolean;
  // La zona del negocio: la hora del pedido se lee en la del local, no en la de la tableta.
  zona: string;
}

// Lo ya devuelto de un pedido, si hubo algo.
const devueltoDe = (o: BoardOrder) => round2(Number(o.refund ?? 0));

// MarcaDeDevolucion: que se vea que ya se devolvió, y cuánto, antes de que alguien lo intente
// otra vez con el cliente enfrente.
function MarcaDeDevolucion({ o }: { o: BoardOrder }) {
  const devuelto = devueltoDe(o);
  if (devuelto <= 0) return null;
  return (
    <Badge colorPalette="red" variant="subtle" flexShrink={0}>Devuelto {money(devuelto, o.currency)}</Badge>
  );
}

// Con más pendientes que esto, la lista de la tarjeta se recorta y hace scroll propio.
const PENDIENTES_A_LA_VISTA = 5;

function Columna({ titulo, orders, acciones }: { titulo: string; orders: BoardOrder[]; acciones: Acciones }) {
  return (
    <Box>
      <HStack mb={2} gap={2}>
        <Text fontWeight="700">{titulo}</Text>
        <Badge borderRadius="full" px={2}>{orders.length}</Badge>
      </HStack>
      <VStack align="stretch" gap={2}>
        {orders.length === 0 && <Text color="fg.subtle" fontSize="sm">Sin pedidos</Text>}
        {orders.map((o) => <Tarjeta key={o.id} o={o} acciones={acciones} />)}
      </VStack>
    </Box>
  );
}

// La tarjeta de un pedido, con sus productos SIEMPRE a la vista.
//
// Antes venían plegados detrás de un tap. Lo que falta por entregar es justo lo que el operador
// vino a leer: esconderlo le cobraba un tap por pedido y dejaba la tarjeta llena de encabezado.
function Tarjeta({ o, acciones }: { o: BoardOrder; acciones: Acciones }) {
  const Svc = SERVICE_META[o.serviceType]?.icon;
  const faltan = pendientes(o);
  const sinProductos = renglonesDe(o).length === 0;
  const listo = faltan.length === 0;
  const debe = Number(o.outstanding) > 0;
  // El tablero no trae los pagos; lo cobrado sale de lo que el pedido ya no debe, menos lo devuelto.
  // Basta para un pedido sin productos, cuyo total es a lo más el envío: si algo queda cobrado, hay
  // que devolverlo antes de cerrarlo, y si la cifra se equivoca el servidor lo rechaza con su texto.
  const tienePagos = quedaPorDevolver(o) > 0;
  const algoEntregado = renglonesDe(o).some((l) => Number(l.delivered) > 0);
  // Con todo entregado, cancelar el pedido lo rechaza el servidor y la salida es «Cerrar pedido».
  const ofreceCancelar = acciones.puedeCancelar && !(listo && algoEntregado);
  const ofreceQuitarFaltante = acciones.puedeQuitarFaltante && faltan.length > 0;

  return (
    <Box data-order-card bg="bg.panel" borderWidth="1px" borderColor={debe ? 'orange.300' : 'border'} borderRadius="lg" p={2.5}>
      <Flex justify="space-between" align="start" gap={2} mb={2}>
        <Box minW={0}>
          {/* El nombre manda: es con lo que se canta el pedido. El número queda en el renglón de
              abajo, junto a lo demás que solo se consulta. */}
          <Text fontWeight="800" fontSize="lg" lineHeight="1.2" lineClamp={1}>
            {o.folioName || `#${o.number}`}
          </Text>
          <HStack fontSize="xs" color="fg.muted" gap={1}>
            {Svc && <Svc size={12} />}
            <Text as="span" lineClamp={1}>
              #{o.number} · {SERVICE_META[o.serviceType]?.label ?? o.serviceType}
              {o.customerName ? ` · ${o.customerName}` : ''}
              {renglonesDe(o).length > 1 ? ` · ${entregados(o)}/${renglonesDe(o).length}` : ''}
              {` · ${diaCortoYHora(o.openedAt, acciones.zona)}`}
            </Text>
          </HStack>
        </Box>
        <VStack align="end" gap={0} flexShrink={0}>
          <Text fontWeight="700" lineHeight="1.2">{money(o.total, o.currency)}</Text>
          {debe && (
            <Text fontSize="xs" fontWeight="700" color="orange.600">
              debe {money(o.outstanding, o.currency)}
            </Text>
          )}
          <MarcaDeDevolucion o={o} />
        </VStack>
      </Flex>

      {/* Con muchos pendientes la lista se recorta y hace scroll propio: una tarjeta de once
          productos medía ~620 px y en una tableta de 600 los botones quedaban fuera, sin salida a
          la vista. El alto en dvh porque lo que se reparte es el alto de la tableta. */}
      <VStack role="list" aria-label="Falta por entregar" align="stretch" gap={1} mb={2}
        {...(faltan.length > PENDIENTES_A_LA_VISTA ? { maxH: '40dvh', overflowY: 'auto' } : {})}>
        {faltan.map((l) => (
          // La clave lleva lo ya entregado a propósito: el contador del renglón es estado local, y
          // sin esto seguiría en 3 después de entregar 3 de 5 — el botón mandaría al servidor una
          // cantidad mayor a la que falta y el operador vería un error por haber acertado.
          <Renglon key={`${l.id}-${l.delivered}`} l={l}
            onEntregar={(qty) => acciones.entregarLinea(o.id, l.id, qty)}
            onQuitar={() => acciones.quitarRenglon(o.id, l)} />
        ))}
        {listo && !sinProductos && (
          <HStack color="green.600" py={1} gap={1}>
            <LuCheck size={16} />
            <Text fontSize="sm" fontWeight="600">Todo entregado</Text>
          </HStack>
        )}
        {sinProductos && <Text fontSize="sm" color="fg.muted" py={1}>Sin productos</Text>}
      </VStack>

      {/* Toda combinación ofrece algo que cierra el pedido o dice dónde se cierra. El incidente fue
          una tarjeta con todo entregado, sin deuda y sin un solo botón: la única salida era
          cancelar, y el servidor la rechaza porque ya salió comida. */}
      <HStack gap={2}>
        {/* Entregar todo desaparece cuando ya no falta nada: un botón que no hace nada enseña a
            ignorar el que sí hace. */}
        {!listo && (
          <Button flex="1" minH={TAP} colorPalette="green" onClick={() => acciones.entregarTodo(o)}>
            Entregar todo
          </Button>
        )}
        {/* Lo vivo entregado y sin deuda: entregar el pedido lo cierra. */}
        {listo && !sinProductos && !debe && (
          <Button flex="1" minH={TAP} colorPalette="green" loading={acciones.cerrando(o)}
            disabled={acciones.cerrando(o)} onClick={() => acciones.cerrarEntregado(o)}>
            Cerrar pedido
          </Button>
        )}
        {listo && !sinProductos && debe && (
          <Text flex="1" fontSize="sm" fontWeight="700" color="orange.600">
            Falta cobrar {money(o.outstanding, o.currency)}
          </Text>
        )}
        {!sinProductos && acciones.puedeAbrirCuenta && (
          <Button flex="1" minH={TAP} colorPalette="orange" variant={debe ? 'solid' : 'outline'}
            onClick={() => acciones.abrirCuenta(o)}>
            Abrir cuenta
          </Button>
        )}
        {sinProductos && !tienePagos && (
          <Button flex="1" minH={TAP} colorPalette="gray" loading={acciones.cerrando(o)}
            disabled={acciones.cerrando(o)} onClick={() => acciones.cerrarVacio(o)}>
            Cerrar pedido
          </Button>
        )}
        {sinProductos && tienePagos && (
          <Text flex="1" fontSize="sm" fontWeight="700" color="orange.600">
            {acciones.puedeDevolverPagos ? 'Tiene pagos por devolver' : 'Tiene pagos por devolver: avisa a quien encargue la caja'}
          </Text>
        )}
        {/* El camino para devolverlos: la hoja de cobro con las fichas de sus pagos. Solo con el
            permiso; sin él, el texto de arriba dice a quién acudir. */}
        {sinProductos && tienePagos && acciones.puedeDevolverPagos && (
          <Button minH={TAP} variant="outline" colorPalette="gray" onClick={() => acciones.cobrar(o)}>
            Devolver pagos
          </Button>
        )}
        <MenuRoot>
          <MenuTrigger asChild>
            <IconButton aria-label="Más" variant="outline" minH={TAP} minW={TAP}>
              <LuEllipsisVertical />
            </IconButton>
          </MenuTrigger>
          {/* Los renglones del menú eran los únicos controles tappables de esta pantalla sin el
              piso de 44 px: la receta `md` de Chakra da 12 px de padding sobre 20 px de texto, o sea
              32. El tercero cancela el pedido y repone inventario, y estaba pegado al segundo.
              Va con separador y con aire: la constitución pide separar las acciones destructivas, y
              aquí un dedo que erra por 6 px repone stock que no se repuso. */}
          <MenuContent minW="220px">
            <MenuItem value="ticket" minH={TAP} px={3} onClick={() => acciones.ticket(o)}>Ver ticket</MenuItem>
            <MenuItem value="comanda" minH={TAP} px={3} onClick={() => acciones.comanda(o)}>Reimprimir comanda</MenuItem>
            {(ofreceQuitarFaltante || ofreceCancelar) && <MenuSeparator />}
            {ofreceQuitarFaltante && (
              <MenuItem value="cancel-pending" minH={TAP} px={3} mt={1}
                onClick={() => acciones.quitarFaltante(o)}>Quitar lo que falta</MenuItem>
            )}
            {ofreceQuitarFaltante && ofreceCancelar && <MenuSeparator />}
            {ofreceCancelar && (
              <MenuItem value="cancel" minH={TAP} px={3} mt={1} color="red.500"
                onClick={() => acciones.cancelar(o)}>Cancelar pedido</MenuItem>
            )}
          </MenuContent>
        </MenuRoot>
      </HStack>
    </Box>
  );
}

// Un producto que todavía debe salir.
//
// El botón verde entrega TODO lo que falta con un tap, que es el caso de siempre. El contador solo
// aparece cuando falta más de uno: es la excepción —salen 3 de 5 alitas y las otras 2 siguen en la
// freidora— y cobrarle un tap al caso común para servir a la excepción está al revés.
function Renglon({ l, onEntregar, onQuitar }: {
  l: BoardLine; onEntregar: (qty: number) => void; onQuitar: () => void;
}) {
  const falta = faltante(l);
  const [cantidad, setCantidad] = useState(falta);
  const parcial = falta > 1;
  // Con piezas pagadas el servidor rechaza quitarlo: primero se devuelve el pago. El bote se queda
  // en su lugar, apagado y diciendo por qué, para que la fila no cambie de forma.
  const pagado = Number(l.paidQty ?? 0) > 0;
  const extras = [...(l.modifiers ?? []), ...(l.notes ? [l.notes] : [])];

  return (
    <HStack role="listitem" gap={1.5} align="center" borderWidth="1px" borderColor="border" borderRadius="md" px={1.5} py={1}>
      <Text fontWeight="800" fontSize="sm" minW="1.75rem" textAlign="center" flexShrink={0}>
        {Number(l.qty)}
      </Text>
      <Box flex="1" minW={0}>
        <Text fontWeight="600" fontSize="sm" lineHeight="1.25" lineClamp={1}>{l.name}</Text>
        {/* En una cocina "Alitas" y "Alitas BBQ sin cebolla" son platillos distintos. Sin esto la
            tarjeta no alcanza a reemplazar la libreta. */}
        {extras.length > 0 && (
          <Text fontSize="2xs" color="fg.muted" lineHeight="1.25" lineClamp={1}>{extras.join(' · ')}</Text>
        )}
        {Number(l.delivered) > 0 && (
          <Text fontSize="2xs" color="orange.600" lineHeight="1.25">
            salieron {Number(l.delivered)} de {Number(l.qty)}
          </Text>
        )}
      </Box>
      {parcial && (
        <HStack gap={0.5} flexShrink={0}>
          <IconButton aria-label="Uno menos" size="sm" variant="ghost" minH={TAP} minW={TAP}
            disabled={cantidad <= 1} onClick={() => setCantidad((c) => Math.max(1, c - 1))}>
            <LuMinus />
          </IconButton>
          <Text minW="1.25rem" textAlign="center" fontWeight="700" fontSize="sm">{cantidad}</Text>
          <IconButton aria-label="Uno más" size="sm" variant="ghost" minH={TAP} minW={TAP}
            disabled={cantidad >= falta} onClick={() => setCantidad((c) => Math.min(falta, c + 1))}>
            <LuPlus />
          </IconButton>
        </HStack>
      )}
      <Button size="sm" minH={TAP} px={3} colorPalette="green" flexShrink={0}
        onClick={() => onEntregar(parcial ? cantidad : falta)}>
        <LuCheck />
      </Button>
      {/* Quitar el renglón. Va DESPUÉS de entregar y separado, porque es la acción destructiva de la
          fila y la constitución pide separarla. En una fila tan apretada no hay distancia que dé
          seguridad de verdad, así que la barrera real es el diálogo: no borra al tocar, pregunta —
          y de paso dice qué pasa con el ingrediente. */}
      {pagado ? (
        <Button aria-label={`Quitar ${l.name}`} size="sm" variant="ghost" colorPalette="gray"
          minH={TAP} minW={TAP} ml={1} px={1} flexShrink={0} disabled flexDirection="column" gap={0}>
          <LuTrash2 />
          <Text as="span" fontSize="2xs" lineHeight="1">Pagado</Text>
        </Button>
      ) : (
        <IconButton aria-label={`Quitar ${l.name}`} size="sm" variant="ghost" colorPalette="red"
          minH={TAP} minW={TAP} ml={1} flexShrink={0} onClick={onQuitar}>
          <LuTrash2 />
        </IconButton>
      )}
    </HStack>
  );
}

// Entregadas del día: solo admin/gerente, para reembolsar y para cobrar lo que quedó pendiente.
// TOPADA para no competir con el flujo operativo de arriba: en una jornada llena son decenas.
function Entregadas({ orders, corteDeVista, zona, onRefund, onTicket, onAbrirCuenta }: {
  orders: BoardOrder[];
  zona: string;
  // El negocio elige cuándo se vacía esta lista; el rótulo tiene que decir esa misma ventana.
  corteDeVista?: string;
  onRefund: (o: BoardOrder) => void;
  onTicket: (o: BoardOrder) => void;
  // Una entregada que debe se cobra en su cuenta, en Vender (caso 25).
  onAbrirCuenta?: (o: BoardOrder) => void;
}) {
  return (
    <Box mt={4}>
      <HStack mb={2} gap={2}>
        <Text fontWeight="700">{tituloDeEntregadas(corteDeVista)}</Text>
        <Badge borderRadius="full" px={2}>{orders.length}</Badge>
        {orders.length > ENTREGADAS_VISIBLES && (
          <Text fontSize="xs" color="fg.muted">últimas {ENTREGADAS_VISIBLES}</Text>
        )}
      </HStack>
      {orders.length === 0 ? (
        <Text color="fg.subtle" fontSize="sm">{vacioDeEntregadas(corteDeVista)}</Text>
      ) : (
        <VStack align="stretch" gap={1.5}>
          {orders.slice(0, ENTREGADAS_VISIBLES).map((o) => {
            const debe = Number(o.outstanding) > 0;
            return (
              <Flex key={o.id} data-delivered-row bg="bg.panel" borderWidth="1px"
                borderColor={debe ? 'orange.300' : 'border'} borderRadius="lg"
                px={3} py={1.5} justify="space-between" align="center" gap={2}>
                <Box minW={0}>
                  <Text fontWeight="700" lineHeight="1.2" lineClamp={1}>{o.folioName || `#${o.number}`}</Text>
                  <Text fontSize="xs" color="fg.muted" lineClamp={1}>
                    #{o.number} · {SERVICE_META[o.serviceType]?.label ?? o.serviceType}
                    {o.customerName ? ` · ${o.customerName}` : ''}
                    {` · ${diaCortoYHora(o.openedAt, zona)}`}
                  </Text>
                </Box>
                <HStack gap={2} flexShrink={0}>
                  <MarcaDeDevolucion o={o} />
                  <Text fontWeight="700">{money(o.total, o.currency)}</Text>
                  {/* Aquí es donde el pendiente deja de tener remedio: el cliente ya se fue con la
                      comida. Por eso el camino a su cuenta vive junto al aviso. */}
                  {debe && onAbrirCuenta && (
                    <Button size="sm" minH={TAP} colorPalette="orange" onClick={() => onAbrirCuenta(o)}>
                      Abrir cuenta · debe {money(o.outstanding, o.currency)}
                    </Button>
                  )}
                  <Button size="sm" minH={TAP} variant="outline" onClick={() => onTicket(o)}>Ticket</Button>
                  {/* Devolver solo lo que SE COBRÓ. El botón vivía al lado de "Cobrar $220" en la
                      misma tarjeta, y tocarlo anotaba $220 de pérdida por un ingreso que nunca
                      ocurrió — mientras la cuenta por cobrar desaparecía del contador sin haberse
                      cobrado. Ofrecer una acción que el servidor va a rechazar es peor que no
                      ofrecerla: el operador la toca con el cliente enfrente. Y lo cobrado se resta
                      de lo ya devuelto: devuelto todo, el botón solo podía rebotar. */}
                  {quedaPorDevolver(o) > 0 && (
                    <Button size="sm" minH={TAP} variant="outline" colorPalette="red"
                      onClick={() => onRefund(o)}>Devolver</Button>
                  )}
                </HStack>
              </Flex>
            );
          })}
        </VStack>
      )}
    </Box>
  );
}
