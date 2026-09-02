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

// Los pedidos que el POS mandó y todavía tiene que ver, detrás de UN botón.
//
// Antes eran dos cosas en la misma fila: un chip por pedido en cocina, más una píldora con lo que se
// debe. Se midió en un navegador sin cabeza contra el presupuesto real de la tableta (1024×600, con
// el panel del pedido abierto y rol de gerente) y **la fila no cabía ni con cero chips**: pedía
// 667.6 px sobre 612.6 disponibles, y el contenedor tiene `overflow: hidden`. Con dos chips los dos
// botones de la derecha quedaban al 0% visible.
//
// Y el chip cortado que se veía pegado al botón naranja no era un z-index: el envoltorio de la fila
// no encoge, así que el `maxW` de la caja de chips —un porcentaje de ese envoltorio— crecía con los
// propios chips en vez de acotarlos. Con dos chips se veían 9.47 px del segundo, exactamente a la
// izquierda de la píldora. Chips completos visibles: uno, a cualquier ancho.
//
// Además decían dos cifras distintas del mismo pedido —el chip el total, la píldora el saldo— y el
// pedido YA PAGADO que sigue en cocina, que es el caso más común de "agrégame una más", no tenía
// chip ni renglón: se quedó sin ningún camino.
//
// Ahora la lista vive en una hoja, con espacio para el nombre completo y para las dos acciones que
// se le pueden hacer a un pedido en curso: agregarle y cobrarlo.
export function PedidosEnCurso({ onAbrir, hayQueAgregar }: {
  onAbrir: (pedido: BoardOrder) => void;
  // Si la cuenta activa tiene algo que llevarle a un pedido. Sin esto, "Agregar" era un control de
  // 44 px que no hacía nada ni decía por qué.
  hayQueAgregar: boolean;
}) {
  const qc = useQueryClient();
  const [cobrando, setCobrando] = useState<BoardOrder | null>(null);
  const [abierta, setAbierta] = useState(false);

  const { data } = useQuery({
    queryKey: ['orders', 'open'],
    queryFn: posApi.openOrders,
    refetchInterval: 30_000,
  });
  const enCurso = data?.items ?? [];

  const refrescar = () => qc.invalidateQueries({ queryKey: ['orders'] });

  // El total sale de la MISMA lista que el botón abre.
  //
  // Con dos fuentes, la tableta llegó a decir $2,141 en la píldora y $1,928 en la lista: el operador
  // ve dos cifras del mismo dinero y no tiene forma de saber cuál miente. Es el corolario del
  // principio III, y ya costó un turno con $4,500 de faltante inexplicable.
  const totalPendiente = enCurso.reduce((s, o) => s + Number(o.outstanding), 0);

  // Sin nada que mostrar no se pinta: un contador en cero es chrome que le quita ancho a la barra, y
  // la caja vacía llegaba a cobrar 112 px de una fila que ya se desbordaba.
  if (enCurso.length === 0) return null;

  return (
    <>
      <Button
        flexShrink={0} minH={TAP} px={3}
        colorPalette={totalPendiente > 0 ? 'orange' : 'blue'}
        variant="solid"
        onClick={() => setAbierta(true)}
      >
        <LuWallet />
        {/* Con saldo manda el dinero; sin saldo, lo que queda es cuántos siguen en cocina. Decir
            "$0" sobre una lista que sí tiene pedidos haría que el botón se lea como vacío. */}
        {totalPendiente > 0 ? (
          <>
            <Text as="span" ml={1} fontWeight="800" whiteSpace="nowrap">
              {money(String(totalPendiente))}
            </Text>
            <Text as="span" ml={1} fontSize="xs" opacity={0.9}>({enCurso.length})</Text>
          </>
        ) : (
          <Text as="span" ml={1} fontWeight="700" whiteSpace="nowrap">
            {enCurso.length} en curso
          </Text>
        )}
      </Button>

      <PedidosEnCursoSheet
        abierta={abierta}
        pedidos={enCurso}
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

// La lista de pedidos en curso, con sus dos acciones por renglón.
//
// El pedido ENTREGADO y sin cobrar es el caso caro —el cliente ya se fue con la comida— y necesita
// decirse con todas sus letras, no caber en un chip de 100 px.
function PedidosEnCursoSheet({ abierta, pedidos, hayQueAgregar, onCerrar, onCobrar, onAgregar }: {
  abierta: boolean;
  pedidos: BoardOrder[];
  hayQueAgregar: boolean;
  onCerrar: () => void;
  onCobrar: (o: BoardOrder) => void;
  onAgregar: (o: BoardOrder) => void;
}) {
  const total = pedidos.reduce((s, o) => s + Number(o.outstanding), 0);
  return (
    <DrawerRoot open={abierta} placement="bottom" size="md"
      onOpenChange={(e) => { if (!e.open) onCerrar(); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="2xl">
        <DrawerHeader borderBottomWidth="1px" py={3}>
          <HStack justify="space-between">
            <Text fontWeight="800" fontSize="lg">Pedidos en curso</Text>
            {total > 0 && (
              <Text fontWeight="800" fontSize="lg">Por cobrar {money(String(total))}</Text>
            )}
          </HStack>
        </DrawerHeader>
        <DrawerBody py={3}>
          {!hayQueAgregar && pedidos.some((o) => o.enPreparacion) && (
            <Text fontSize="sm" color="fg.muted" mb={2}>
              Captura los productos en una cuenta para poder agregarlos a un pedido.
            </Text>
          )}
          <VStack align="stretch" gap={2}>
            {pedidos.map((o) => {
              const debe = Number(o.outstanding) > 0;
              return (
                <Flex key={o.id} borderWidth="1px"
                  borderColor={debe ? 'orange.300' : 'border'} borderRadius="lg"
                  px={3} py={2} align="center" justify="space-between" gap={2}>
                  <Box minW={0}>
                    <Text fontWeight="700" lineClamp={1}>{o.folioName || `#${o.number}`}</Text>
                    <Text fontSize="xs" color="fg.muted" lineClamp={1}>
                      #{o.number} · {o.renglones} · {money(o.total, o.currency)}
                      {o.customerName ? ` · ${o.customerName}` : ''}
                      {/* El cliente ya se fue con la comida: se dice, no se deja adivinar. */}
                      {debe && !o.enPreparacion ? ' · ya se entregó' : ''}
                    </Text>
                  </Box>
                  <HStack gap={2} flexShrink={0}>
                    {/* Agregar va en TODO lo que sigue en cocina, cobrado o no. El pedido ya pagado
                        que sigue esperando es justo al que el cliente le pide una más, y antes no
                        tenía ningún camino: el filtro de la barra exigía saldo pendiente. */}
                    {o.enPreparacion && (
                      <Button minH={TAP} variant="outline" colorPalette="blue" flexShrink={0}
                        disabled={!hayQueAgregar} onClick={() => onAgregar(o)}>
                        <LuPlus /> Agregar
                      </Button>
                    )}
                    {debe && (
                      <Button minH={TAP} colorPalette="orange" flexShrink={0} onClick={() => onCobrar(o)}>
                        Cobrar {money(o.outstanding, o.currency)}
                      </Button>
                    )}
                  </HStack>
                </Flex>
              );
            })}
          </VStack>
        </DrawerBody>
      </DrawerContent>
    </DrawerRoot>
  );
}
