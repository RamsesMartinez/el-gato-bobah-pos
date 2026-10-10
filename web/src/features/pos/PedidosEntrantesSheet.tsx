import { useEffect, useState } from 'react';
import { Badge, Box, Button, Flex, HStack, Text, VStack } from '@chakra-ui/react';
import { LuBike, LuCheck, LuStore } from 'react-icons/lu';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader,
} from '../../components/ui/drawer';
import type { PedidoDePlataforma } from '../../api/pedidosDePlataforma';
import { money } from '../../utils/format';
import { minutosQueQuedan, urgenciaDe } from './urgencia';

const TAP = '44px';

/**
 * PedidosEntrantesSheet es la lista completa de los pedidos que esperan decisión, la que abre el
 * contador «+N» del aviso. Sigue el patrón de la hoja de pedidos por cobrar: un renglón por pedido
 * con su acción a la derecha.
 *
 * Llegan ordenados por el servidor, el que vence primero arriba: es el mismo orden que decide cuál
 * se muestra en la franja, y así el de la franja es siempre el primero de aquí.
 */
export function PedidosEntrantesSheet({ abierta, pedidos, aceptandoId, onCerrar, onAceptar }: {
  abierta: boolean;
  pedidos: PedidoDePlataforma[];
  aceptandoId: number | null;
  onCerrar: () => void;
  onAceptar: (p: PedidoDePlataforma) => void;
}) {
  // El mismo reloj de 15 s que la franja: lo que se pinta son minutos y dos umbrales.
  const [ahora, setAhora] = useState(() => Date.now());
  // Corre aunque la hoja esté cerrada: si solo avanzara abierta, al reabrirla mostraría los
  // minutos de la última vez que se vio.
  useEffect(() => {
    const t = setInterval(() => setAhora(Date.now()), 15000);
    return () => clearInterval(t);
  }, []);
  return (
    <DrawerRoot open={abierta} placement="bottom" size="md"
      onOpenChange={(e) => { if (!e.open) onCerrar(); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="2xl">
        <DrawerHeader borderBottomWidth="1px" py={3}>
          <Text fontWeight="800" fontSize="lg">Pedidos esperando respuesta ({pedidos.length})</Text>
        </DrawerHeader>
        <DrawerBody py={3}>
          <VStack align="stretch" gap={2}>
            {pedidos.map((p) => {
              const urgencia = urgenciaDe(p.decideBefore, ahora);
              const folio = p.displayId || p.customerName || `#${p.id}`;
              const paraRecoger = p.serviceType === 'para_llevar';
              return (
                <Flex key={p.id} borderWidth="1px" borderRadius="lg" px={3} py={2} align="center"
                  justify="space-between" gap={3}
                  borderColor={urgencia === 'por_expirar' ? 'red.400' : urgencia === 'sonando' ? 'orange.400' : 'green.400'}>
                  <Box minW={0}>
                    <HStack gap={2}>
                      <Text fontWeight="700" lineClamp={1}>{p.platformName} · {folio}</Text>
                      <Badge colorPalette={paraRecoger ? 'purple' : 'blue'}>
                        {paraRecoger ? <><LuStore /> Pasan por él</> : <><LuBike /> A domicilio</>}
                      </Badge>
                    </HStack>
                    <Text fontSize="sm" color="fg.muted" lineClamp={1}>
                      {(p.lines ?? []).length} platillos · {money(Number(p.total))} ·{' '}
                      {urgencia === 'por_expirar'
                        ? 'Se cancela en cualquier momento'
                        : `Quedan ${minutosQueQuedan(p.decideBefore, ahora)} min`}
                    </Text>
                  </Box>
                  <Button minH={TAP} minW="140px" px={5} colorPalette="green" flexShrink={0}
                    aria-label={`Aceptar ${folio}`}
                    loading={aceptandoId === p.id}
                    disabled={aceptandoId !== null && aceptandoId !== p.id}
                    onClick={() => onAceptar(p)}>
                    <LuCheck /> Aceptar
                  </Button>
                </Flex>
              );
            })}
          </VStack>
        </DrawerBody>
      </DrawerContent>
    </DrawerRoot>
  );
}
