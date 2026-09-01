import { useMemo, useState } from 'react';
import {
  Button, VStack, HStack, SimpleGrid, Text, Input, Box, Flex, IconButton,
} from '@chakra-ui/react';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
  DrawerCloseTrigger, DrawerTitle, DrawerGrabber,
} from '../../components/ui/drawer';
import { useSwipeDownToClose } from '../../hooks/useSwipeDownToClose';
import {
  LuBanknote, LuCreditCard, LuLandmark, LuSmartphone, LuX, LuTriangleAlert,
  LuSplit, LuPlus, LuTrash2,
} from 'react-icons/lu';
import type { IconType } from 'react-icons';
import { Picker } from '../../components/Picker';
import { toaster } from '../../components/ui/toaster';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useMenu } from '../../hooks/useMenu';
import { useTicketStore, useActiveTicket, ticketTotal } from '../../stores/ticket';
import { useUiStore } from '../../stores/ui';
import { posApi, type CreateOrderBody } from '../../api/pos';
import type { OrderView } from '../../types/pos';
import { money } from '../../utils/format';
import { uuid } from '../../utils/uuid';
import { ApiError } from '../../api/client';
import { esEfectivo, metodoPorDefecto, metodosDeLaLista, primerMetodoLibre } from './metodosDePago';

// El icono se elige por la NATURALEZA del método, no por su id: desde que payment_methods es
// per-tenant cada empresa tiene los suyos y los ids ya no son estables entre negocios.
const ICONO_POR_TIPO: Record<string, IconType> = {
  efectivo: LuBanknote,
  tarjeta: LuCreditCard,
  transferencia: LuLandmark,
  plataforma: LuSmartphone,
};
const iconoDe = (kind: string): IconType => ICONO_POR_TIPO[kind] ?? LuCreditCard;
// Billetes MXN para pago rápido en efectivo (además de "Exacto").
const BILLS = [50, 100, 200, 500, 1000];

interface Props {
  isOpen: boolean;
  onClose: () => void;
  onDone: (order: OrderView) => void;
}

export function CheckoutSheet({ isOpen, onClose, onDone }: Props) {
  const swipe = useSwipeDownToClose(onClose);
  const { id: activeId, lines, serviceType, customerName, folioName, platformId: lista } = useActiveTicket();
  const removeLine = useTicketStore((s) => s.removeLine);
  const closeTab = useTicketStore((s) => s.closeTab);
  const qc = useQueryClient();

  // Un producto puede haberse inactivado (p.ej. tamaños de alitas → modificador) y quedar
  // en un ticket viejo. El backend lo rechaza; aquí avisamos y lo excluimos del cobro
  // (no bloqueamos: se cobra el resto). El menú solo trae productos activos.
  const { data: menu } = useMenu();
  const available = new Set((menu?.products ?? []).map((p) => p.id));
  const unavailable = menu ? lines.filter((l) => !available.has(l.productId)) : [];
  const chargeLines = menu ? lines.filter((l) => available.has(l.productId)) : lines;
  const total = ticketTotal(chargeLines);

  const palette = useUiStore((s) => s.palette);
  // Los métodos los manda el servidor: son los de ESTA empresa. Antes eran cuatro ids quemados y
  // cualquier negocio que no fuera el primero se quedaba sin poder cobrar.
  const { data: methodsData } = useQuery({ queryKey: ['payment-methods'], queryFn: posApi.paymentMethods });
  // Con una plataforma activa solo se ofrecen SUS métodos —en línea y efectivo—: el servidor
  // rechaza cualquier otro, y ofrecerlos sería dejar armar un cobro que va a fallar.
  const methods = useMemo(() => metodosDeLaLista(methodsData?.items ?? [], lista), [methodsData, lista]);
  // null hasta que llega el catálogo; en cuanto llega, el default sale de la regla probada.
  const [methodId, setMethodId] = useState<number | null>(null);
  // "Ya se lo llevó": el pedido nace entregado y no pasa por Pedidos. Apagado por default — el caso
  // común sí pasa por cocina, y encenderlo por inercia escondería pedidos que faltan por preparar.
  // A qué pedido en curso se agrega lo que está en la cuenta. Vacío = pedido nuevo, que es el caso
  // de siempre. Es la libreta que vuelve de la mesa con "la 3 pidió dos más".
  const [agregarA, setAgregarA] = useState('');
  const { data: enCurso } = useQuery({
    queryKey: ['orders', 'active'],
    queryFn: posApi.activeOrders,
    enabled: isOpen,
  });
  const pedidosAbiertos = (enCurso?.items ?? []).filter((o) => o.status === 'abierta' || o.status === 'lista');
  const metodoActivo = methodId ?? metodoPorDefecto(methods);
  const metodoElegido = methods.find((m) => m.id === metodoActivo);
  const [tendered, setTendered] = useState('');  // '' = Exacto (sin cambio)
  const [tip, setTip] = useState('');
  // Pago dividido: cada línea es {método, monto}. Se activa con "Dividir pago".
  const [splitMode, setSplitMode] = useState(false);
  const [splits, setSplits] = useState<Array<{ methodId: number; amount: string }>>([]);
  // Costo de envío: solo a domicilio. Se pre-llena con el ajuste de negocio ($20) pero el
  // operador puede editarlo o ponerlo en 0 (envío gratis) para este pedido. null = usar default.
  const { data: settings } = useQuery({ queryKey: ['business-settings'], queryFn: posApi.businessSettings });
  const [feeOverride, setFeeOverride] = useState<string | null>(null);
  const isDelivery = serviceType === 'domicilio';
  const defaultFee = settings ? settings.deliveryFee : '20';
  const feeInput = feeOverride ?? defaultFee;
  const deliveryFee = isDelivery ? Math.max(0, Math.round((parseFloat(feeInput) || 0) * 100) / 100) : 0;

  const tipAmount = Math.max(0, Math.round((parseFloat(tip) || 0) * 100) / 100);
  // orderTotal = lo que cubren los pagos (subtotal + envío); la propina va aparte, en la 1ª línea.
  const orderTotal = Math.round((total + deliveryFee) * 100) / 100;
  const grandTotal = Math.round((orderTotal + tipAmount) * 100) / 100;

  // "Exacto" es el default: campo vacío ⇒ el cliente pagó justo, sin cambio.
  const isExact = tendered === '';
  const received = isExact ? grandTotal : (parseFloat(tendered) || 0);
  const change = Math.max(0, received - grandTotal);
  const cashShort = !splitMode && esEfectivo(metodoElegido) && !isExact && received < grandTotal;

  // --- Pago dividido ---
  const round2 = (n: number) => Math.round(n * 100) / 100;
  const splitSum = round2(splits.reduce((a, s) => a + (parseFloat(s.amount) || 0), 0));
  const splitRemaining = round2(orderTotal - splitSum);
  // Válido cuando la suma cubre EXACTO el total del pedido (tolerancia de 1 centavo).
  const splitValid = splits.length > 0 && splitSum > 0 && Math.abs(splitRemaining) < 0.005;

  const enableSplit = () => {
    // arranca con una línea = método actual y el monto completo (editable); dividir = ir bajando.
    setSplits([{ methodId: metodoActivo ?? 0, amount: String(orderTotal) }]);
    setTendered('');
    setSplitMode(true);
  };
  const addSplit = () => setSplits((xs) => [
    ...xs,
    { methodId: primerMetodoLibre(methods, xs.map((x) => x.methodId)) ?? 0, amount: '' },
  ]);
  const removeSplit = (i: number) => setSplits((xs) => xs.filter((_, j) => j !== i));
  const setSplitMethod = (i: number, id: number) => setSplits((xs) => xs.map((s, j) => (j === i ? { ...s, methodId: id } : s)));
  const setSplitAmount = (i: number, v: string) => setSplits((xs) => xs.map((s, j) => (j === i ? { ...s, amount: v } : s)));
  // Rellena esta línea con lo que falta para cubrir el total (un tap para cuadrar el resto).
  const fillRest = (i: number) => setSplits((xs) => {
    const others = xs.reduce((a, s, j) => a + (j === i ? 0 : parseFloat(s.amount) || 0), 0);
    return xs.map((s, j) => (j === i ? { ...s, amount: String(Math.max(0, round2(orderTotal - others))) } : s));
  });

  // Líneas de pago: en modo dividido, una por método (montos que cuadran con orderTotal); en
  // modo simple, una sola por orderTotal. La propina va en la PRIMERA línea (amount = total del
  // pedido, la propina es aparte; si no incluyera el envío, un domicilio quedaría "no pagado").
  const buildPayments = (): NonNullable<CreateOrderBody['payments']> => {
    if (splitMode) {
      return splits
        .map((s) => ({ methodId: s.methodId, amount: round2(parseFloat(s.amount) || 0) }))
        .filter((p) => p.amount > 0)
        .map((p, i) => (i === 0 && tipAmount > 0 ? { ...p, tip: tipAmount } : p));
    }
    return [{ methodId: metodoActivo ?? 0, amount: orderTotal, ...(tipAmount > 0 ? { tip: tipAmount } : {}) }];
  };

  const build = (withPayment: boolean): CreateOrderBody => ({
    clientUuid: uuid(),
    // El animal que la cuenta lleva mostrando desde que se abrió. Se manda para que el ticket
    // salga con el mismo nombre que el operador ya le dijo al cliente; el servidor lo sanea y le
    // agrega la vuelta si otro pedido del día se le adelantó.
    folioName,
    // Un pedido de plataforma ES a domicilio: lo reparte la plataforma. El servidor lo exige por
    // el check de la tabla, así que la pantalla no puede mandar otra cosa.
    serviceType: lista !== null ? 'domicilio' : serviceType,
    customerName: customerName || undefined,
    // El envío del negocio no aplica: lo cobra la plataforma. El servidor también lo fuerza a 0,
    // pero mandarlo ya en 0 evita que la pantalla muestre un total que el servidor no va a cobrar.
    deliveryFee: lista !== null ? 0 : deliveryFee,
    deliveryPlatformId: lista ?? undefined,
    lines: chargeLines.map((l) => ({
      productId: l.productId,
      qty: l.qty,
      notes: l.notes,
      modifiers: l.modifiers.map((m) => ({ optionId: m.optionId, qty: m.qty })),
    })),
    payments: withPayment ? buildPayments() : undefined,
    // Solo al cobrar: entregar sin cobrar sería regalar comida sin dejar rastro, porque el pedido
    // nace terminado y no vuelve a aparecer en ninguna pantalla operativa. El servidor lo rechaza
    // igual; esto evita el viaje.
  });

  const mutation = useMutation({
    mutationFn: (withPayment: boolean) =>
      agregarA
        ? posApi.addOrderLines(Number(agregarA), build(false).lines)
        : posApi.createOrder(build(withPayment)),
    onSuccess: (order) => {
      closeTab(activeId); // la cuenta se envió/cobró: se cierra y queda la siguiente activa
      setTendered('');
      setTip('');
      setSplitMode(false); setSplits([]); // el siguiente pedido arranca en modo simple
      setFeeOverride(null); // siguiente pedido vuelve al costo de envío por defecto
      qc.invalidateQueries({ queryKey: ['orders', 'active'] });
      // el pedido nuevo alimenta las recomendaciones → refetch para verlas al instante
      qc.invalidateQueries({ queryKey: ['modifier-defaults'] });
      onDone(order);
    },
    onError: (e: unknown) => {
      // El backend dice QUÉ producto tumbó el cobro; aquí solo se pinta. El nombre va en el título
      // para que sea lo primero que se lee: con el carrito lleno, un mensaje que solo trae un id no
      // le dice al operador qué renglón quitar.
      const detalle = e instanceof ApiError ? e.details : undefined;
      toaster.create({
        title: detalle?.productName ? `No se pudo cobrar: ${detalle.productName}` : 'No se pudo crear el pedido',
        description: e instanceof ApiError ? e.message : String(e),
        type: 'error',
      });
    },
  });
  const charging = mutation.isPending && mutation.variables === true;
  const sending = mutation.isPending && mutation.variables === false;

  return (
    <DrawerRoot open={isOpen} placement="bottom" onOpenChange={(e) => { if (!e.open) onClose() }} size="full">
      <DrawerBackdrop />
      <DrawerContent
        colorPalette={palette}
        borderTopRadius={{ base: 0, md: '2xl' }}
        maxH={{ base: '100dvh', md: '94vh' }}
        maxW={{ base: '100%', md: '640px', lg: '920px' }}
        mx="auto"
        style={{
          transform: swipe.offset ? `translateY(${swipe.offset}px)` : undefined,
          transition: swipe.dragging ? 'none' : 'transform 0.2s ease',
        }}
      >
        <DrawerGrabber {...swipe.handlers} />
        <DrawerCloseTrigger />
        <DrawerHeader style={{ touchAction: 'none' }} {...swipe.handlers}><DrawerTitle>Cobrar · {money(total)}</DrawerTitle></DrawerHeader>
        <DrawerBody>
          <VStack align="stretch" gap={4}>
            {/* Advertencia: productos que ya no están en el menú activo */}
            {unavailable.length > 0 && (
              <Box colorPalette="orange" borderWidth="1px" borderColor="colorPalette.emphasized"
                bg="colorPalette.subtle" borderRadius="lg" p={3}>
                <HStack align="start" gap={2}>
                  <Box color="colorPalette.fg" mt="2px"><LuTriangleAlert size={18} /></Box>
                  <Box flex="1">
                    <Text fontWeight="700" color="colorPalette.fg">Productos no disponibles</Text>
                    <Text fontSize="sm" color="fg.muted" mb={2}>
                      Ya no están en el menú, así que no se incluirán en el cobro:
                    </Text>
                    <VStack align="stretch" gap={0.5}>
                      {unavailable.map((l) => (
                        <Text key={l.lineId} fontSize="sm">• {l.name}{l.qty > 1 ? ` ×${l.qty}` : ''}</Text>
                      ))}
                    </VStack>
                    <Button size="sm" minH="40px" mt={2} variant="outline" colorPalette="orange"
                      onClick={() => unavailable.forEach((l) => removeLine(l.lineId))}>
                      <LuX /> Quitar del pedido
                    </Button>
                  </Box>
                </HStack>
              </Box>
            )}

            {/* Dos columnas en pantalla ancha. En una sola, esta hoja mide más de lo que cabe en
                600 px de alto: el operador tiene que desplazarse para llegar al total, con el
                cliente enfrente, mientras el modal ocupa 640 px de 1024 y deja 384 sin usar. */}
            <SimpleGrid columns={{ base: 1, lg: 2 }} gap={{ base: 4, lg: 6 }} alignItems="start">
              {/* Columna del dinero: con qué paga y cuánto entrega. */}
              <VStack align="stretch" gap={4}>
                {/* Método de pago */}
                <Box>
                  <HStack justify="space-between" mb={2}>
                    <Text fontWeight="600">Método de pago</Text>
                    <Button size="xs" minH="36px" variant="ghost" colorPalette={splitMode ? undefined : 'gray'}
                      onClick={() => (splitMode ? setSplitMode(false) : enableSplit())}>
                      <LuSplit /> {splitMode ? 'Un solo método' : 'Dividir pago'}
                    </Button>
                  </HStack>

                  {!splitMode ? (
                    <SimpleGrid columns={2} gap={2}>
                      {methods.map((m) => {
                        const Icon = iconoDe(m.kind);
                        const on = metodoActivo === m.id;
                        return (
                          <Button key={m.id} h="56px" flexDir="column" gap={1} whiteSpace="normal" fontSize="sm"
                            variant={on ? 'solid' : 'outline'} colorPalette={on ? undefined : 'gray'}
                            onClick={() => setMethodId(m.id)}>
                            <Icon size={20} />
                            {m.name}
                          </Button>
                        );
                      })}
                    </SimpleGrid>
                  ) : (
                    <VStack align="stretch" gap={3}>
                      {splits.map((s, i) => (
                        <Box key={i} borderWidth="1px" borderColor="border" borderRadius="lg" p={3}>
                          <HStack justify="space-between" mb={2}>
                            <Text fontSize="sm" color="fg.muted">Pago {i + 1}</Text>
                            {splits.length > 1 && (
                              <IconButton aria-label="Quitar pago" size="xs" variant="ghost" colorPalette="red"
                                onClick={() => removeSplit(i)}><LuTrash2 /></IconButton>
                            )}
                          </HStack>
                          <SimpleGrid columns={4} gap={1} mb={2}>
                            {methods.map((m) => {
                              const on = s.methodId === m.id;
                              return (
                                <Button key={m.id} h="44px" px={1} fontSize="xs" whiteSpace="normal"
                                  variant={on ? 'solid' : 'outline'} colorPalette={on ? undefined : 'gray'}
                                  onClick={() => setSplitMethod(i, m.id)}>
                                  {m.name}
                                </Button>
                              );
                            })}
                          </SimpleGrid>
                          <HStack>
                            <Input flex="1" size="lg" type="number" inputMode="decimal" placeholder="0.00"
                              value={s.amount} onChange={(e) => setSplitAmount(i, e.target.value)} />
                            <Button size="sm" minH="40px" variant="outline" colorPalette="gray" onClick={() => fillRest(i)}>
                              Resto
                            </Button>
                          </HStack>
                        </Box>
                      ))}
                      <Button variant="outline" colorPalette="gray" onClick={addSplit}>
                        <LuPlus /> Agregar método
                      </Button>
                      <Flex justify="space-between" align="baseline">
                        <Text color="fg.muted">Restante</Text>
                        <Text fontSize="xl" fontWeight="800" color={splitValid ? 'green.500' : 'orange.500'}>
                          {money(splitRemaining)}
                        </Text>
                      </Flex>
                      {splitRemaining < -0.005 && (
                        <Text fontSize="xs" color="red.400">Los pagos superan el total del pedido.</Text>
                      )}
                    </VStack>
                  )}
                </Box>

                {/* Propina */}
                <Box>
                  <Text fontWeight="600" mb={2}>Propina</Text>
                  <HStack mb={2}>
                    {[0, 0.1, 0.15, 0.2].map((pct) => {
                      const amt = Math.round(total * pct * 100) / 100;
                      const active = tipAmount === amt;
                      return (
                        <Button key={pct} flex="1"
                          variant={active ? 'solid' : 'outline'} colorPalette={active ? undefined : 'gray'}
                          onClick={() => setTip(amt ? String(amt) : '')}>
                          {pct === 0 ? 'Sin' : `${pct * 100}%`}
                        </Button>
                      );
                    })}
                  </HStack>
                  <Input size="lg" type="number" inputMode="decimal" placeholder="Otra cantidad"
                    value={tip} onChange={(e) => setTip(e.target.value)} />
                </Box>

                {/* Efectivo (modo simple): recibido + billetes rápidos + cambio */}
                {!splitMode && esEfectivo(metodoElegido) && (
                  <Box>
                    <HStack justify="space-between" mb={2}>
                      <Text fontWeight="600">Recibido</Text>
                      {!isExact && (
                        <Button size="sm" minH="40px" variant="ghost" colorPalette="gray" onClick={() => setTendered('')}>
                          <LuX /> Borrar
                        </Button>
                      )}
                    </HStack>
                    <SimpleGrid columns={3} gap={2} mb={2}>
                      <Button h="52px" variant={isExact ? 'solid' : 'outline'} colorPalette={isExact ? undefined : 'gray'}
                        onClick={() => setTendered('')}>
                        Exacto
                      </Button>
                      {BILLS.map((v) => {
                        const active = tendered === String(v);
                        return (
                          <Button key={v} h="52px" variant={active ? 'solid' : 'outline'} colorPalette={active ? undefined : 'gray'}
                            onClick={() => setTendered(String(v))}>
                            {money(v)}
                          </Button>
                        );
                      })}
                    </SimpleGrid>
                    <Input size="lg" type="number" inputMode="decimal" placeholder={money(grandTotal)}
                      value={tendered} onChange={(e) => setTendered(e.target.value)} />
                    <Flex justify="space-between" mt={3} align="baseline">
                      <Text color="fg.muted">Cambio</Text>
                      <Text fontSize="4xl" fontWeight="800" color={cashShort ? 'red.400' : 'green.500'}>
                        {money(change)}
                      </Text>
                    </Flex>
                  </Box>
                )}
              </VStack>

              {/* Columna del pedido: lo que se le ajusta antes de cobrarlo. */}
              <VStack align="stretch" gap={4}>
                {/* Envío: solo a domicilio. Pre-llenado con el ajuste de negocio, editable (0 = gratis). */}
                {isDelivery && (
                  <Box>
                    <Text fontWeight="600" mb={2}>Costo de envío</Text>
                    <Input size="lg" type="number" inputMode="decimal" placeholder={money(Number(defaultFee))}
                      value={feeInput} onChange={(e) => setFeeOverride(e.target.value)} />
                    <Text fontSize="xs" color="fg.muted" mt={1}>0 = envío gratis</Text>
                  </Box>
                )}

                {(deliveryFee > 0 || tipAmount > 0) && (
                  <Box>
                    <Flex justify="space-between"><Text color="fg.muted">Subtotal</Text><Text>{money(total)}</Text></Flex>
                    {deliveryFee > 0 && (
                      <Flex justify="space-between"><Text color="fg.muted">Envío</Text><Text>{money(deliveryFee)}</Text></Flex>
                    )}
                    {tipAmount > 0 && (
                      <Flex justify="space-between"><Text color="fg.muted">Propina</Text><Text>{money(tipAmount)}</Text></Flex>
                    )}
                    <Flex justify="space-between" align="baseline" mt={1}>
                      <Text fontWeight="700">Total</Text>
                      <Text fontSize="xl" fontWeight="800">{money(grandTotal)}</Text>
                    </Flex>
                  </Box>
                )}

                {pedidosAbiertos.length > 0 && (
                  <Box borderTopWidth="1px" pt={3}>
                    <Text fontWeight="600" mb={1}>Agregar a un pedido en curso</Text>
                    <Picker
                      value={agregarA}
                      onChange={setAgregarA}
                      options={pedidosAbiertos.map((o) => ({
                        value: String(o.id),
                        // Nombre primero (es lo que se ve en el tablero y lo que dice el cliente),
                        // número después para quien busque por ticket.
                        label: `${o.folioName ? `${o.folioName} · #${o.number}` : `#${o.number}`}${o.customerName ? ` · ${o.customerName}` : ''}`,
                        hint: money(o.total, o.currency),
                      }))}
                      placeholder="Pedido nuevo"
                      title="¿A qué pedido se agrega?"
                      clearable
                      clearLabel="Pedido nuevo"
                    />
                  </Box>
                )}
              </VStack>
            </SimpleGrid>

          </VStack>
        </DrawerBody>
        <DrawerFooter borderTopWidth="1px">
          {/* Con un pedido elegido hay UN solo camino: agregarle lo de la cuenta. Cobrar es del
              pedido completo y se hace cuando el cliente se va, no por cada agregado. */}
          {agregarA ? (
            <Button w="100%" size="lg" disabled={chargeLines.length === 0} loading={sending || charging}
              onClick={() => mutation.mutate(false)}>
              Agregar a {(() => {
                const o = pedidosAbiertos.find((p) => String(p.id) === agregarA);
                return o?.folioName || `#${o?.number ?? ''}`;
              })()}
            </Button>
          ) : (
            /* Una sola salida. Mandar a cocina sin cobrar vive en el panel del pedido: teniéndolo
               aquí, esta pantalla pedía método de pago y propina para algo que después descartaba,
               y las dos salidas se veían igual de definitivas. */
            <Button w="100%" size="lg" colorPalette="green" fontWeight="800"
              disabled={chargeLines.length === 0 || (splitMode ? !splitValid : cashShort)}
              loading={charging} onClick={() => mutation.mutate(true)}>
              COBRAR {money(grandTotal)}
            </Button>
          )}
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}
