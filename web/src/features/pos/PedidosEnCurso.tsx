import { useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Box, Button, Flex, HStack, Text, VStack } from '@chakra-ui/react';
import { LuWallet, LuPlus } from 'react-icons/lu';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader,
} from '../../components/ui/drawer';
import { posApi } from '../../api/pos';
import type { BoardOrder } from '../../types/pos';
import { CobrarSheet } from '../../shared/CobrarSheet';
import { money } from '../../utils/format';

// Alto mínimo de todo lo que se toca. Por debajo el dedo falla y la siguiente venta cae en el
// pedido de otra mesa.
const TAP = '44px';

// Lo que el POS mandó y todavía se puede cobrar, detrás de UN botón.
//
// Uno solo, y no un control por pedido: medido en un navegador sin cabeza contra el presupuesto
// real de la tableta (1024×600, con el panel del pedido abierto y rol de gerente), la fila pedía
// 667.6 px sobre 612.6 disponibles — se desbordaba estando vacía, y el `overflow: hidden` del padre
// dejaba los botones de la derecha al 0% visible.
//
// La lista vive en una hoja, con espacio para el nombre completo y para las dos acciones que se le
// pueden hacer: agregarle y cobrarlo.
export function PedidosEnCurso({ onAbrir, hayQueAgregar }: {
  onAbrir: (pedido: BoardOrder) => void;
  // Si la cuenta activa tiene algo que llevarle a un pedido. Sin esto, "Agregar" era un control de
  // 44 px que no hacía nada ni decía por qué.
  hayQueAgregar: boolean;
}) {
  const qc = useQueryClient();
  const [cobrando, setCobrando] = useState<BoardOrder | null>(null);
  const [abierta, setAbierta] = useState(false);

  // El servidor manda solo lo que falta por cobrar (`porCobrar=true` en `api/pos.ts`). Recortarlo
  // aquí en vez de allá dejaría el total del encabezado —que viene del mismo recorrido— sumando
  // filas que esta lista no muestra.
  const { data } = useQuery({
    queryKey: ['orders', 'open'],
    queryFn: posApi.openOrders,
    refetchInterval: 30_000,
  });
  const porCobrar = data?.items ?? [];

  const refrescar = () => qc.invalidateQueries({ queryKey: ['orders'] });

  // El total sale de la MISMA lista que el botón abre.
  //
  // Con dos fuentes, la tableta llegó a decir $2,141 en la píldora y $1,928 en la lista: el operador
  // ve dos cifras del mismo dinero y no tiene forma de saber cuál miente. Es el corolario del
  // principio III, y ya costó un turno con $4,500 de faltante inexplicable.
  const totalPendiente = porCobrar.reduce((s, o) => s + Number(o.outstanding), 0);

  // Sin nada que cobrar no se pinta: un contador en cero es chrome que le quita ancho a la barra, y
  // la caja vacía llegaba a cobrar 112 px de una fila que ya se desbordaba.
  if (porCobrar.length === 0) return null;

  return (
    <>
      <Button
        flexShrink={0} minH={TAP} px={3}
        colorPalette="orange"
        variant="solid"
        onClick={() => setAbierta(true)}
      >
        <LuWallet />
        <Text as="span" ml={1} fontWeight="800" whiteSpace="nowrap">
          {money(String(totalPendiente))}
        </Text>
        <Text as="span" ml={1} fontSize="xs" opacity={0.9}>({porCobrar.length})</Text>
      </Button>

      <PedidosEnCursoSheet
        abierta={abierta}
        pedidos={porCobrar}
        total={totalPendiente}
        hayQueAgregar={hayQueAgregar}
        onCerrar={() => setAbierta(false)}
        onCobrar={(o) => { setAbierta(false); setCobrando(o); }}
        onAgregar={(o) => { setAbierta(false); onAbrir(o); }}
      />

      {/* `key` por pedido: la hoja lleva estado —cuánto se cobra, con qué, y qué pedazos ya
          entraron— y con otro pedido nada de eso aplica. Remontarla lo limpia sin un efecto que
          resincronice, que es de donde salen los estados a medias. */}
      <CobrarSheet key={cobrando?.id} order={cobrando}
        onClose={() => setCobrando(null)} onCobrado={() => refrescar()} />
    </>
  );
}

// La lista de pedidos por cobrar, con sus dos acciones por renglón.
//
// El pedido ENTREGADO y sin cobrar es el caso caro —el cliente ya se fue con la comida— y necesita
// decirse con todas sus letras, no depender del color.
function PedidosEnCursoSheet({ abierta, pedidos, total, hayQueAgregar, onCerrar, onCobrar, onAgregar }: {
  abierta: boolean;
  pedidos: BoardOrder[];
  total: number;
  hayQueAgregar: boolean;
  onCerrar: () => void;
  onCobrar: (o: BoardOrder) => void;
  onAgregar: (o: BoardOrder) => void;
}) {
  return (
    <DrawerRoot open={abierta} placement="bottom" size="md"
      onOpenChange={(e) => { if (!e.open) onCerrar(); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="2xl">
        <DrawerHeader borderBottomWidth="1px" py={3}>
          <HStack justify="space-between">
            <Text fontWeight="800" fontSize="lg">Pedidos por cobrar</Text>
            <Text fontWeight="800" fontSize="lg">{money(String(total))}</Text>
          </HStack>
        </DrawerHeader>
        <DrawerBody py={3}>
          {!hayQueAgregar && pedidos.some((o) => o.enPreparacion) && (
            <Text fontSize="sm" color="fg.muted" mb={2}>
              Captura los productos en una cuenta para poder agregarlos a un pedido.
            </Text>
          )}
          <VStack align="stretch" gap={2}>
            {pedidos.map((o) => (
              <Flex key={o.id} borderWidth="1px" borderColor="orange.300" borderRadius="lg"
                px={3} py={2} align="center" justify="space-between" gap={2}>
                <Box minW={0}>
                  <Text fontWeight="700" lineClamp={1}>{o.folioName || `#${o.number}`}</Text>
                  <Text fontSize="xs" color="fg.muted" lineClamp={1}>
                    #{o.number} · {o.renglones} · {money(o.total, o.currency)}
                    {o.customerName ? ` · ${o.customerName}` : ''}
                    {/* El cliente ya se fue con la comida: se dice, no se deja adivinar. */}
                    {!o.enPreparacion ? ' · ya se entregó' : ''}
                  </Text>
                </Box>
                <HStack gap={2} flexShrink={0}>
                  {/* Al entregado no se le puede agregar: el servidor rechaza renglones nuevos. */}
                  {o.enPreparacion && (
                    <Button minH={TAP} variant="outline" colorPalette="blue" flexShrink={0}
                      disabled={!hayQueAgregar} onClick={() => onAgregar(o)}>
                      <LuPlus /> Agregar
                    </Button>
                  )}
                  <Button minH={TAP} colorPalette="orange" flexShrink={0} onClick={() => onCobrar(o)}>
                    Cobrar {money(o.outstanding, o.currency)}
                  </Button>
                </HStack>
              </Flex>
            ))}
          </VStack>
        </DrawerBody>
      </DrawerContent>
    </DrawerRoot>
  );
}
