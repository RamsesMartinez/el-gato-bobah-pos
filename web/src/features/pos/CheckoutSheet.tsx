import { useState } from 'react';
import {
  Button, VStack, HStack, SimpleGrid, Text, Input, Box, Flex, IconButton,
} from '@chakra-ui/react';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
  DrawerCloseTrigger, DrawerTitle, DrawerGrabber,
} from '../../components/ui/drawer';
import { useSwipeDownToClose } from '../../hooks/useSwipeDownToClose';
import {
  LuBanknote, LuCreditCard, LuLandmark, LuStore, LuShoppingBag, LuBike, LuX, LuTriangleAlert,
  LuSplit, LuPlus, LuTrash2,
} from 'react-icons/lu';
import type { IconType } from 'react-icons';
import { toaster } from '../../components/ui/toaster';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useMenu } from '../../hooks/useMenu';
import { useTicketStore, useActiveTicket, ticketTotal } from '../../stores/ticket';
import { useUiStore } from '../../stores/ui';
import { posApi, type CreateOrderBody } from '../../api/pos';
import type { OrderView, ServiceType } from '../../types/pos';
import { money } from '../../utils/format';
import { uuid } from '../../utils/uuid';
import { ApiError } from '../../api/client';

// IDs de medios de pago según seeds/migraciones (0010 + 0013). MVP: ids fijos.
const METHODS: Array<{ id: number; label: string; icon: IconType }> = [
  { id: 2, label: 'Débito', icon: LuCreditCard },       // default
  { id: 7, label: 'Crédito', icon: LuCreditCard },
  { id: 1, label: 'Efectivo', icon: LuBanknote },
  { id: 3, label: 'Transferencia', icon: LuLandmark },
];
const SERVICE: Array<{ v: ServiceType; label: string; icon: IconType }> = [
  { v: 'mostrador', label: 'Mostrador', icon: LuStore },
  { v: 'para_llevar', label: 'Para llevar', icon: LuShoppingBag },
  { v: 'domicilio', label: 'Domicilio', icon: LuBike },
];
// Billetes MXN para pago rápido en efectivo (además de "Exacto").
const BILLS = [50, 100, 200, 500, 1000];

interface Props {
  isOpen: boolean;
  onClose: () => void;
  onDone: (order: OrderView) => void;
}

export function CheckoutSheet({ isOpen, onClose, onDone }: Props) {
  const swipe = useSwipeDownToClose(onClose);
  const { id: activeId, lines, serviceType, customerName } = useActiveTicket();
  const setServiceType = useTicketStore((s) => s.setServiceType);
  const setCustomerName = useTicketStore((s) => s.setCustomerName);
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
  const [methodId, setMethodId] = useState(2);  // Tarjeta débito por default
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
  const cashShort = !splitMode && methodId === 1 && !isExact && received < grandTotal;

  // --- Pago dividido ---
  const round2 = (n: number) => Math.round(n * 100) / 100;
  const splitSum = round2(splits.reduce((a, s) => a + (parseFloat(s.amount) || 0), 0));
  const splitRemaining = round2(orderTotal - splitSum);
  // Válido cuando la suma cubre EXACTO el total del pedido (tolerancia de 1 centavo).
  const splitValid = splits.length > 0 && splitSum > 0 && Math.abs(splitRemaining) < 0.005;

  const enableSplit = () => {
    // arranca con una línea = método actual y el monto completo (editable); dividir = ir bajando.
    setSplits([{ methodId, amount: String(orderTotal) }]);
    setTendered('');
    setSplitMode(true);
  };
  const firstUnusedMethod = (xs: Array<{ methodId: number }>) => {
    const used = new Set(xs.map((s) => s.methodId));
    return (METHODS.find((m) => !used.has(m.id)) ?? METHODS[0]).id;
  };
  const addSplit = () => setSplits((xs) => [...xs, { methodId: firstUnusedMethod(xs), amount: '' }]);
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
    return [{ methodId, amount: orderTotal, ...(tipAmount > 0 ? { tip: tipAmount } : {}) }];
  };

  const build = (withPayment: boolean): CreateOrderBody => ({
    clientUuid: uuid(),
    serviceType,
    customerName: customerName || undefined,
    deliveryFee,
    lines: chargeLines.map((l) => ({
      productId: l.productId,
      qty: l.qty,
      notes: l.notes,
      modifiers: l.modifiers.map((m) => ({ optionId: m.optionId, qty: m.qty })),
    })),
    payments: withPayment ? buildPayments() : undefined,
  });

  const mutation = useMutation({
    mutationFn: (withPayment: boolean) => posApi.createOrder(build(withPayment)),
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
      toaster.create({
        title: 'No se pudo crear el pedido',
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
        maxW={{ base: '100%', md: '640px' }}
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
          <VStack align="stretch" gap={5}>
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

            {/* Tipo de pedido — 'mostrador' viene pre-seleccionado, no obliga a un paso extra */}
            <Box>
              <Text fontWeight="600" mb={2}>Tipo de pedido</Text>
              <SimpleGrid columns={3} gap={2}>
                {SERVICE.map((s) => {
                  const Icon = s.icon;
                  const on = serviceType === s.v;
                  return (
                    <Button key={s.v} h="56px" flexDir="column" gap={1} whiteSpace="normal" fontSize="sm"
                      variant={on ? 'solid' : 'outline'} colorPalette={on ? undefined : 'gray'}
                      onClick={() => setServiceType(s.v)}>
                      <Icon size={20} />
                      {s.label}
                    </Button>
                  );
                })}
              </SimpleGrid>
            </Box>

            <Input size="lg" placeholder="Nombre del cliente (opcional)"
              value={customerName} onChange={(e) => setCustomerName(e.target.value)} />

            {/* Envío: solo a domicilio. Pre-llenado con el ajuste de negocio, editable (0 = gratis). */}
            {isDelivery && (
              <Box>
                <Text fontWeight="600" mb={2}>Costo de envío</Text>
                <Input size="lg" type="number" inputMode="decimal" placeholder={money(Number(defaultFee))}
                  value={feeInput} onChange={(e) => setFeeOverride(e.target.value)} />
                <Text fontSize="xs" color="fg.muted" mt={1}>0 = envío gratis</Text>
              </Box>
            )}

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
                  {METHODS.map((m) => {
                    const Icon = m.icon;
                    const on = methodId === m.id;
                    return (
                      <Button key={m.id} h="56px" flexDir="column" gap={1} whiteSpace="normal" fontSize="sm"
                        variant={on ? 'solid' : 'outline'} colorPalette={on ? undefined : 'gray'}
                        onClick={() => setMethodId(m.id)}>
                        <Icon size={20} />
                        {m.label}
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
                        {METHODS.map((m) => {
                          const on = s.methodId === m.id;
                          return (
                            <Button key={m.id} h="44px" px={1} fontSize="xs" whiteSpace="normal"
                              variant={on ? 'solid' : 'outline'} colorPalette={on ? undefined : 'gray'}
                              onClick={() => setSplitMethod(i, m.id)}>
                              {m.label}
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

            {/* Efectivo (modo simple): recibido + billetes rápidos + cambio */}
            {!splitMode && methodId === 1 && (
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
        </DrawerBody>
        <DrawerFooter borderTopWidth="1px">
          <HStack w="100%">
            <Button flex="1" size="lg" variant="outline" colorPalette="gray"
              disabled={chargeLines.length === 0} loading={sending} onClick={() => mutation.mutate(false)}>
              Enviar a cocina
            </Button>
            <Button flex="1.4" size="lg" colorPalette="green"
              disabled={chargeLines.length === 0 || (splitMode ? !splitValid : cashShort)}
              loading={charging} onClick={() => mutation.mutate(true)}>
              COBRAR {money(grandTotal)}
            </Button>
          </HStack>
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}
