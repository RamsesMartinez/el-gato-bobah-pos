import { useState } from 'react';
import { useQuery, useMutation } from '@tanstack/react-query';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
} from '../../components/ui/drawer';
import { Box, Button, HStack, VStack, Text, Input, SimpleGrid } from '@chakra-ui/react';
import { toaster } from '../../components/ui/toaster';
import { posApi } from '../../api/pos';
import type { BoardOrder } from '../../types/pos';
import { money } from '../../utils/format';
import { esEfectivo, metodosDeLaLista } from '../pos/metodosDePago';

// Billetes para cobrar en efectivo sin teclear. Los mismos del cobro normal: quien cobra aquí ya
// tiene el gesto aprendido de la otra pantalla.
const BILLETES = [50, 100, 200, 500, 1000];

interface Props {
  order: BoardOrder | null;
  onClose: () => void;
  onCobrado: () => void;
}

// Cobra un pedido que se mandó a cocina sin cobrar.
//
// Es una hoja aparte y no el cobro completo: aquí no se eligen productos ni se divide la cuenta —
// eso ya pasó cuando se levantó el pedido. Lo único que falta es el dinero.
export function CobrarSheet({ order, onClose, onCobrado }: Props) {
  const [metodo, setMetodo] = useState<number | null>(null);
  const [recibido, setRecibido] = useState('');

  const { data: metodos } = useQuery({
    queryKey: ['payment-methods'],
    queryFn: posApi.paymentMethods,
    enabled: order !== null,
  });

  const cobrar = useMutation({
    mutationFn: ({ id, methodId, amount }: { id: number; methodId: number; amount: number }) =>
      posApi.chargeOrder(id, { methodId, amount }),
    onSuccess: () => {
      toaster.create({ title: 'Cobrado', type: 'success' });
      setMetodo(null);
      setRecibido('');
      onCobrado();
      onClose();
    },
    onError: (e) => toaster.create({ title: 'No se pudo cobrar', description: String(e), type: 'error' }),
  });

  if (!order) return null;

  const falta = Number(order.outstanding);
  // Misma regla que el cobro normal, y por eso el mismo helper: es el espejo de
  // domain.MetodoCorrespondeALaPlataforma en el servidor. Ofrecer un método que va a rebotar manda
  // al operador a adivinar cuál sirve, con el cliente enfrente.
  const elegibles = metodosDeLaLista(metodos?.items ?? [], order.deliveryPlatformId);
  const efectivo = esEfectivo(elegibles.find((m) => m.id === metodo));
  const entregado = Number(recibido || 0);
  const cambio = efectivo && entregado > falta ? entregado - falta : 0;

  return (
    <DrawerRoot open placement="bottom" onOpenChange={(e) => { if (!e.open) onClose(); }} size="md">
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="2xl">
        <DrawerHeader borderBottomWidth="1px" py={3}>
          <HStack justify="space-between">
            <Box>
              <Text fontWeight="800" fontSize="lg">{order.folioName || `#${order.number}`}</Text>
              <Text fontSize="sm" color="fg.muted">#{order.number}</Text>
            </Box>
            <Text fontWeight="800" fontSize="2xl">{money(String(falta))}</Text>
          </HStack>
        </DrawerHeader>

        <DrawerBody py={3}>
          <VStack align="stretch" gap={3}>
            <Box>
              <Text fontSize="sm" fontWeight="600" mb={2}>¿Con qué paga?</Text>
              {/* Botones y no un desplegable: son pocos y se tocan con el dedo. */}
              <SimpleGrid columns={{ base: 2, sm: 3 }} gap={2}>
                {elegibles.map((m) => (
                  <Button key={m.id} minH="52px" variant={metodo === m.id ? 'solid' : 'outline'}
                    colorPalette={metodo === m.id ? 'green' : 'gray'}
                    onClick={() => setMetodo(m.id)}>
                    {m.name}
                  </Button>
                ))}
              </SimpleGrid>
            </Box>

            {/* Con qué billete paga, solo para efectivo: es lo único que produce cambio. */}
            {efectivo && (
              <Box>
                <Text fontSize="sm" fontWeight="600" mb={2}>¿Con cuánto paga?</Text>
                <HStack gap={2} flexWrap="wrap">
                  <Button minH="52px" variant={recibido === '' ? 'solid' : 'outline'}
                    colorPalette={recibido === '' ? 'green' : 'gray'}
                    onClick={() => setRecibido('')}>
                    Exacto
                  </Button>
                  {BILLETES.filter((b) => b >= falta).map((b) => (
                    <Button key={b} minH="52px" variant={recibido === String(b) ? 'solid' : 'outline'}
                      colorPalette={recibido === String(b) ? 'green' : 'gray'}
                      onClick={() => setRecibido(String(b))}>
                      {money(String(b))}
                    </Button>
                  ))}
                  <Input w="7rem" minH="52px" inputMode="decimal" placeholder="Otro"
                    value={recibido} onChange={(e) => setRecibido(e.target.value)} />
                </HStack>
                {cambio > 0 && (
                  <Text mt={2} fontWeight="700" fontSize="lg" color="orange.600">
                    Cambio {money(String(cambio))}
                  </Text>
                )}
              </Box>
            )}
          </VStack>
        </DrawerBody>

        <DrawerFooter borderTopWidth="1px">
          {/* Se cobra lo que FALTA, no lo que entregó el cliente: el excedente es cambio, no
              ingreso. Registrarlo como ingreso inflaría la venta y descuadraría el corte. */}
          <Button w="100%" size="lg" minH="56px" colorPalette="green"
            disabled={metodo === null} loading={cobrar.isPending}
            onClick={() => metodo !== null && cobrar.mutate({ id: order.id, methodId: metodo, amount: falta })}>
            Cobrar {money(String(falta))}
          </Button>
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}
