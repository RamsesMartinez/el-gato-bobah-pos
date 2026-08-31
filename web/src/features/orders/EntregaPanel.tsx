import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Box, Text, HStack, VStack, Button, IconButton, Center, Spinner } from '@chakra-ui/react';
import { LuMinus, LuPlus, LuCheck } from 'react-icons/lu';
import { toaster } from '../../components/ui/toaster';
import { posApi } from '../../api/pos';
import type { OrderLine } from '../../types/pos';
import { faltante, renglonesPendientes } from './entrega';

// Alto mínimo de todo lo que se toca. En una tableta de 7" con las manos ocupadas, por debajo de
// esto el dedo falla y en este panel fallar significa dar por entregado lo que no salió.
const TAP = '44px';

interface Props {
  orderId: number;
  onEntregado: () => void;
}

// Panel de entrega renglón a renglón. Se abre desde la tarjeta y pide el pedido completo: el
// tablero solo trae el avance ("3 de 5"), no las líneas, porque traerlas para cada tarjeta haría
// una consulta por pedido en cada refresco.
export function EntregaPanel({ orderId, onEntregado }: Props) {
  const qc = useQueryClient();
  const { data: order, isLoading } = useQuery({
    queryKey: ['order', orderId],
    queryFn: () => posApi.order(orderId),
  });

  const entregar = useMutation({
    mutationFn: ({ lineId, qty }: { lineId: number; qty: number }) =>
      posApi.deliverLine(orderId, lineId, qty),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['order', orderId] });
      onEntregado();
    },
    onError: (e) => toaster.create({ title: 'No se pudo entregar', description: String(e), type: 'error' }),
  });

  if (isLoading || !order) {
    return <Center py={6}><Spinner /></Center>;
  }

  const pendientes = renglonesPendientes(order);
  if (pendientes.length === 0) {
    return (
      <HStack py={3} color="green.600" justify="center">
        <LuCheck />
        <Text fontWeight="600">Todo entregado</Text>
      </HStack>
    );
  }

  return (
    <VStack align="stretch" gap={2} pt={3} borderTopWidth="1px" borderColor="border">
      {pendientes.map((l) => (
        <RenglonPendiente
          key={l.id}
          linea={l}
          enviando={entregar.isPending}
          onEntregar={(qty) => entregar.mutate({ lineId: l.id, qty })}
        />
      ))}
    </VStack>
  );
}

// Un renglón que todavía debe comida.
//
// El botón entrega TODO lo que falta con un tap, que es el caso de siempre. El contador solo
// aparece cuando falta más de uno: es la excepción —salen 3 de 5 alitas y las otras 2 siguen en la
// freidora— y cobrarle dos taps al caso común para servir a la excepción está al revés.
function RenglonPendiente({ linea, enviando, onEntregar }: {
  linea: OrderLine;
  enviando: boolean;
  onEntregar: (qty: number) => void;
}) {
  const falta = faltante(linea);
  const [cantidad, setCantidad] = useState(falta);
  const parcial = falta > 1;
  // Lo ya entregado solo se dice cuando hubo una entrega previa; en un renglón intacto sería ruido.
  const yaSalio = Number(linea.delivered) > 0;

  return (
    <Box borderWidth="1px" borderColor="border" borderRadius="lg" p={3}>
      <HStack justify="space-between" align="start" mb={parcial ? 2 : 0} gap={3}>
        <Box minW={0}>
          <Text fontWeight="600" lineClamp={2}>
            {Number(linea.quantity)} {linea.productName}
          </Text>
          {yaSalio && (
            <Text fontSize="xs" color="fg.muted">
              Ya salieron {Number(linea.delivered)} de {Number(linea.quantity)}
            </Text>
          )}
        </Box>
        {!parcial && (
          <Button size="md" minH={TAP} colorPalette="green" loading={enviando}
            onClick={() => onEntregar(falta)}>
            Entregar
          </Button>
        )}
      </HStack>

      {parcial && (
        <HStack gap={2}>
          <IconButton aria-label="Uno menos" variant="outline" minH={TAP} minW={TAP}
            disabled={cantidad <= 1}
            onClick={() => setCantidad((c) => Math.max(1, c - 1))}>
            <LuMinus />
          </IconButton>
          <Text minW="2.5rem" textAlign="center" fontWeight="700" fontSize="lg">{cantidad}</Text>
          <IconButton aria-label="Uno más" variant="outline" minH={TAP} minW={TAP}
            disabled={cantidad >= falta}
            onClick={() => setCantidad((c) => Math.min(falta, c + 1))}>
            <LuPlus />
          </IconButton>
          <Button flex="1" minH={TAP} colorPalette="green" loading={enviando}
            onClick={() => onEntregar(cantidad)}>
            Entregar {cantidad}
          </Button>
        </HStack>
      )}
    </Box>
  );
}
