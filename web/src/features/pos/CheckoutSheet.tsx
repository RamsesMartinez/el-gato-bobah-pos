import { useState } from 'react';
import {
  Button, VStack, HStack, SimpleGrid, Text, Input, Box, Flex,
} from '@chakra-ui/react';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
  DrawerCloseTrigger, DrawerTitle,
} from '../../components/ui/drawer';
import {
  LuBanknote, LuCreditCard, LuLandmark, LuStore, LuShoppingBag, LuBike,
  LuArrowRight, LuArrowLeft,
} from 'react-icons/lu';
import type { IconType } from 'react-icons';
import { toaster } from '../../components/ui/toaster';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useTicketStore, useActiveTicket, ticketTotal } from '../../stores/ticket';
import { useUiStore } from '../../stores/ui';
import { posApi, type CreateOrderBody } from '../../api/pos';
import type { OrderView, ServiceType } from '../../types/pos';
import { money } from '../../utils/format';
import { uuid } from '../../utils/uuid';
import { ApiError } from '../../api/client';

// IDs de medios de pago según el seed (0010_seeds.sql). MVP.
const METHODS: Array<{ id: number; label: string; icon: IconType }> = [
  { id: 1, label: 'Efectivo', icon: LuBanknote },
  { id: 2, label: 'Tarjeta', icon: LuCreditCard },
  { id: 3, label: 'Transferencia', icon: LuLandmark },
];
const SERVICE: Array<{ v: ServiceType; label: string; icon: IconType }> = [
  { v: 'mostrador', label: 'Mostrador', icon: LuStore },
  { v: 'para_llevar', label: 'Para llevar', icon: LuShoppingBag },
  { v: 'domicilio', label: 'Domicilio', icon: LuBike },
];

interface Props {
  isOpen: boolean;
  onClose: () => void;
  onDone: (order: OrderView) => void;
}

export function CheckoutSheet({ isOpen, onClose, onDone }: Props) {
  const { id: activeId, lines, serviceType, customerName } = useActiveTicket();
  const setServiceType = useTicketStore((s) => s.setServiceType);
  const setCustomerName = useTicketStore((s) => s.setCustomerName);
  const closeTab = useTicketStore((s) => s.closeTab);
  const total = ticketTotal(lines);
  const qc = useQueryClient();

  const palette = useUiStore((s) => s.palette);
  const [step, setStep] = useState<1 | 2>(1);
  const [methodId, setMethodId] = useState(1);
  const [tendered, setTendered] = useState('');
  const [tip, setTip] = useState('');

  const tipAmount = Math.max(0, Math.round((parseFloat(tip) || 0) * 100) / 100);
  const grandTotal = Math.round((total + tipAmount) * 100) / 100;

  const build = (withPayment: boolean): CreateOrderBody => ({
    clientUuid: uuid(),
    serviceType,
    customerName: customerName || undefined,
    lines: lines.map((l) => ({
      productId: l.productId,
      qty: l.qty,
      notes: l.notes,
      modifiers: l.modifiers.map((m) => ({ optionId: m.optionId, qty: m.qty, portion: m.portion })),
    })),
    payment: withPayment ? { methodId, amount: total, tip: tipAmount || undefined } : undefined,
  });

  const mutation = useMutation({
    mutationFn: (withPayment: boolean) => posApi.createOrder(build(withPayment)),
    onSuccess: (order) => {
      closeTab(activeId); // la cuenta se envió/cobró: se cierra y queda la siguiente activa
      setStep(1);
      setTendered('');
      setTip('');
      qc.invalidateQueries({ queryKey: ['orders', 'active'] });
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

  const change = Math.max(0, (parseFloat(tendered) || 0) - grandTotal);
  const cashShort = methodId === 1 && (parseFloat(tendered) || 0) < grandTotal;

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
        <DrawerHeader><DrawerTitle>{step === 1 ? 'Tipo de pedido' : 'Cobrar'} · {money(total)}</DrawerTitle></DrawerHeader>
        <DrawerBody>
          {step === 1 ? (
            <VStack align="stretch" gap={5}>
              <SimpleGrid columns={3} gap={3}>
                {SERVICE.map((s) => {
                  const Icon = s.icon;
                  return (
                    <Button key={s.v} h="72px" flexDir="column" gap={1} whiteSpace="normal"
                      variant={serviceType === s.v ? 'solid' : 'outline'}
                      colorPalette={serviceType === s.v ? undefined : 'gray'}
                      onClick={() => setServiceType(s.v)}>
                      <Icon size={24} />
                      {s.label}
                    </Button>
                  );
                })}
              </SimpleGrid>
              <Box>
                <Text fontWeight="600" mb={2}>Nombre del cliente (opcional)</Text>
                <Input size="lg" placeholder="Ej. Karla" value={customerName} onChange={(e) => setCustomerName(e.target.value)} />
              </Box>
            </VStack>
          ) : (
            <VStack align="stretch" gap={5}>
              <SimpleGrid columns={3} gap={3}>
                {METHODS.map((m) => {
                  const Icon = m.icon;
                  return (
                    <Button key={m.id} h="72px" flexDir="column" gap={1} whiteSpace="normal"
                      variant={methodId === m.id ? 'solid' : 'outline'}
                      colorPalette={methodId === m.id ? undefined : 'gray'}
                      onClick={() => setMethodId(m.id)}>
                      <Icon size={24} />
                      {m.label}
                    </Button>
                  );
                })}
              </SimpleGrid>
              <Box>
                <Text fontWeight="600" mb={2}>Propina</Text>
                <HStack mb={2}>
                  {[0, 0.1, 0.15, 0.2].map((pct) => {
                    const amt = Math.round(total * pct * 100) / 100;
                    const active = tipAmount === amt;
                    return (
                      <Button key={pct} flex="1"
                        variant={active ? 'solid' : 'outline'}
                        colorPalette={active ? undefined : 'gray'}
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

              {methodId === 1 && (
                <Box>
                  <Text fontWeight="600" mb={2}>Recibido</Text>
                  <HStack mb={2}>
                    {[grandTotal, 100, 200, 500].map((v, i) => (
                      <Button key={i} onClick={() => setTendered(String(v))}>
                        {i === 0 ? 'Exacto' : money(v)}
                      </Button>
                    ))}
                  </HStack>
                  <Input size="lg" type="number" inputMode="decimal" placeholder="0.00"
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
          )}
        </DrawerBody>
        <DrawerFooter borderTopWidth="1px">
          {step === 1 ? (
            <HStack w="100%">
              <Button flex="1" size="lg" variant="outline" loading={mutation.isPending}
                onClick={() => mutation.mutate(false)}>
                Enviar a cocina
              </Button>
              <Button flex="1" size="lg" onClick={() => setStep(2)}>
                Cobrar <LuArrowRight />
              </Button>
            </HStack>
          ) : (
            <HStack w="100%">
              <Button size="lg" variant="ghost" onClick={() => setStep(1)}><LuArrowLeft /> Atrás</Button>
              <Button flex="1" size="lg" colorPalette="green" disabled={cashShort}
                loading={mutation.isPending} onClick={() => mutation.mutate(true)}>
                CONFIRMAR {money(grandTotal)}
              </Button>
            </HStack>
          )}
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}
