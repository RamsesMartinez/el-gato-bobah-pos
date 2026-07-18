import { useState } from 'react';
import {
  Button, VStack, HStack, SimpleGrid, Text, Input, Box, Flex,
} from '@chakra-ui/react';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
  DrawerCloseTrigger, DrawerTitle,
} from '../../components/ui/drawer';
import {
  LuBanknote, LuCreditCard, LuLandmark, LuStore, LuShoppingBag, LuBike, LuX, LuTriangleAlert,
} from 'react-icons/lu';
import type { IconType } from 'react-icons';
import { toaster } from '../../components/ui/toaster';
import { useMutation, useQueryClient } from '@tanstack/react-query';
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

  const tipAmount = Math.max(0, Math.round((parseFloat(tip) || 0) * 100) / 100);
  const grandTotal = Math.round((total + tipAmount) * 100) / 100;

  // "Exacto" es el default: campo vacío ⇒ el cliente pagó justo, sin cambio.
  const isExact = tendered === '';
  const received = isExact ? grandTotal : (parseFloat(tendered) || 0);
  const change = Math.max(0, received - grandTotal);
  const cashShort = methodId === 1 && !isExact && received < grandTotal;

  const build = (withPayment: boolean): CreateOrderBody => ({
    clientUuid: uuid(),
    serviceType,
    customerName: customerName || undefined,
    lines: chargeLines.map((l) => ({
      productId: l.productId,
      qty: l.qty,
      notes: l.notes,
      modifiers: l.modifiers.map((m) => ({ optionId: m.optionId, qty: m.qty })),
    })),
    payment: withPayment ? { methodId, amount: total, tip: tipAmount || undefined } : undefined,
  });

  const mutation = useMutation({
    mutationFn: (withPayment: boolean) => posApi.createOrder(build(withPayment)),
    onSuccess: (order) => {
      closeTab(activeId); // la cuenta se envió/cobró: se cierra y queda la siguiente activa
      setTendered('');
      setTip('');
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
      >
        <DrawerCloseTrigger />
        <DrawerHeader><DrawerTitle>Cobrar · {money(total)}</DrawerTitle></DrawerHeader>
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

            {/* Método de pago */}
            <Box>
              <Text fontWeight="600" mb={2}>Método de pago</Text>
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

            {tipAmount > 0 && (
              <Flex justify="space-between" align="baseline">
                <Text color="fg.muted">Total con propina</Text>
                <Text fontSize="xl" fontWeight="700">{money(grandTotal)}</Text>
              </Flex>
            )}

            {/* Efectivo: recibido + billetes rápidos + cambio */}
            {methodId === 1 && (
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
            <Button flex="1.4" size="lg" colorPalette="green" disabled={cashShort || chargeLines.length === 0}
              loading={charging} onClick={() => mutation.mutate(true)}>
              COBRAR {money(grandTotal)}
            </Button>
          </HStack>
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}
