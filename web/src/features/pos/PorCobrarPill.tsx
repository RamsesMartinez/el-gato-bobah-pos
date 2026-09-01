import { useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader,
} from '../../components/ui/drawer';
import { Box, Button, Flex, HStack, Text, VStack } from '@chakra-ui/react';
import { LuWallet } from 'react-icons/lu';
import { posApi } from '../../api/pos';
import type { BoardOrder } from '../../types/pos';
import { CobrarSheet } from '../orders/CobrarSheet';
import { money } from '../../utils/format';

// Aviso de lo que se mandó a cocina y nadie ha pagado, con el cobro a dos taps.
//
// Vive en el POS porque cobrar es del POS: el tablero de pedidos prepara y entrega. Y vive en el
// ENCABEZADO porque es dinero en riesgo — antes solo se veía entrando a Pedidos, y el pedido ya
// entregado (donde el cliente se fue con la comida) ni siquiera lo podía ver quien está en la
// caja, porque la lista de entregadas es de admin/gerente.
//
// No se pinta cuando no hay nada pendiente: un contador en cero es chrome que le quita ancho a la
// barra en una tableta de 7".
export function PorCobrarPill() {
  const qc = useQueryClient();
  const [abierto, setAbierto] = useState(false);
  const [cobrando, setCobrando] = useState<BoardOrder | null>(null);

  const { data } = useQuery({
    queryKey: ['orders', 'unpaid'],
    queryFn: posApi.unpaidOrders,
    refetchInterval: 30_000,
  });
  const pendientes = data?.items ?? [];
  const monto = pendientes.reduce((s, o) => s + Number(o.outstanding), 0);

  const refrescar = () => {
    qc.invalidateQueries({ queryKey: ['orders', 'unpaid'] });
    qc.invalidateQueries({ queryKey: ['orders', 'active'] });
    qc.invalidateQueries({ queryKey: ['orders', 'delivered'] });
  };

  if (pendientes.length === 0) return null;

  return (
    <>
      <Button size="lg" colorPalette="orange" variant="solid" flexShrink={0} px={3}
        onClick={() => setAbierto(true)}>
        <LuWallet />
        <Text as="span" ml={1} fontWeight="800">{money(String(monto))}</Text>
      </Button>

      <DrawerRoot open={abierto} placement="bottom" size="md"
        onOpenChange={(e) => { if (!e.open) setAbierto(false); }}>
        <DrawerBackdrop />
        <DrawerContent borderTopRadius="2xl">
          <DrawerHeader borderBottomWidth="1px" py={3}>
            <HStack justify="space-between">
              <Text fontWeight="800" fontSize="lg">Por cobrar</Text>
              <Text fontWeight="800" fontSize="lg">{money(String(monto))}</Text>
            </HStack>
          </DrawerHeader>
          <DrawerBody py={3}>
            <VStack align="stretch" gap={2}>
              {pendientes.map((o) => (
                <Flex key={o.id} borderWidth="1px" borderColor="orange.300" borderRadius="lg"
                  px={3} py={2} align="center" justify="space-between" gap={3}>
                  <Box minW={0}>
                    <Text fontWeight="700" lineClamp={1}>{o.folioName || `#${o.number}`}</Text>
                    <Text fontSize="xs" color="fg.muted" lineClamp={1}>
                      #{o.number}
                      {o.customerName ? ` · ${o.customerName}` : ''}
                      {/* Entregado y sin cobrar es el caso caro: el cliente ya se fue con la
                          comida, así que se dice en la lista y no se deja adivinar. */}
                      {o.status === 'entregada' ? ' · ya se entregó' : ''}
                    </Text>
                  </Box>
                  <Button minH="44px" colorPalette="orange" flexShrink={0}
                    onClick={() => { setAbierto(false); setCobrando(o); }}>
                    Cobrar {money(o.outstanding, o.currency)}
                  </Button>
                </Flex>
              ))}
            </VStack>
          </DrawerBody>
        </DrawerContent>
      </DrawerRoot>

      <CobrarSheet order={cobrando} onClose={() => setCobrando(null)} onCobrado={refrescar} />
    </>
  );
}
