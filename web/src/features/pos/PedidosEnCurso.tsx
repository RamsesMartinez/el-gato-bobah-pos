import { useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Box, Button, Flex, HStack, Text, VStack } from '@chakra-ui/react';
import { LuWallet } from 'react-icons/lu';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader,
} from '../../components/ui/drawer';
import { posApi } from '../../api/pos';
import type { BoardOrder } from '../../types/pos';
import { CobrarSheet } from '../orders/CobrarSheet';
import { money } from '../../utils/format';
import { RADIUS, BORDER_W } from '../../theme/ui';

// Alto mínimo de todo lo que se toca. Por debajo el dedo falla y la siguiente venta cae en el
// pedido de otra mesa.
const TAP = '44px';

// La barra de pedidos en curso: lo que el POS mandó y todavía tiene que ver.
//
// Reemplaza a la píldora de "Por cobrar" y no se suma a ella. Eran la misma cosa dicha dos veces, y
// en 1024x600 el alto que se gasta repitiendo información se lo quita a la lista de productos, que
// es lo que el operador vino a leer. Va en la fila que ya existe —cuentas, buscador, botones—, así
// que no cuesta alto nuevo.
//
// Son DOS conjuntos y el servidor los distingue con `enPreparacion`:
//
//   * los que siguen en cocina, incluidos los YA COBRADOS: a esos es a los que el cliente le pide
//     algo más, y un toque en el chip los vuelve a abrir. Antes desaparecían al mandarlos y
//     recuperarlos costaba cinco toques por un camino que nadie encontró nunca;
//   * los que deben dinero, incluido el ENTREGADO sin cobrar — el caro, porque el cliente ya se fue
//     con la comida. Ese se cobra desde aquí y no se puede ampliar.
export function PedidosEnCurso({ onAbrir }: { onAbrir: (pedido: BoardOrder) => void }) {
  const qc = useQueryClient();
  const [cobrando, setCobrando] = useState<BoardOrder | null>(null);
  const [listaDeSaldos, setListaDeSaldos] = useState(false);

  const { data } = useQuery({
    queryKey: ['orders', 'open'],
    queryFn: posApi.openOrders,
    refetchInterval: 30_000,
  });
  const pedidos = data?.items ?? [];

  const refrescar = () => {
    qc.invalidateQueries({ queryKey: ['orders', 'open'] });
    qc.invalidateQueries({ queryKey: ['orders', 'active'] });
    qc.invalidateQueries({ queryKey: ['orders', 'delivered'] });
  };

  // EN PREPARACIÓN va como chip; CON SALDO va contado en la píldora.
  //
  // Los dos estaban inline y se comían la barra: en la tableta real, tres pedidos entregados sin
  // cobrar dejaban la cuenta activa cortada a la izquierda y el último chip truncado a media
  // palabra. Y no son la misma cosa: al chip en preparación se le TOCA para agregarle, mientras que
  // el entregado sin cobrar es un aviso de dinero — exactamente lo que la píldora que esto
  // reemplazó hacía bien, y que se perdió al ponerlos todos como chips.
  const enPreparacion = pedidos.filter((o) => o.enPreparacion);
  const conSaldo = pedidos.filter((o) => !o.enPreparacion);

  // Sin nada que mostrar no se pinta: un contador en cero es chrome que le quita ancho a la barra.
  if (pedidos.length === 0) return null;

  return (
    <>
      {/* Los chips scrollean DENTRO de su propia caja, con el ancho acotado.
          Sin esto, cada chip mide ~150px y no encoge, así que con seis pedidos —el máximo de un día
          en producción— la fila crecía hasta empujar los botones de precios y edición fuera del
          `overflow="hidden"` del contenedor: no se veían y tampoco se podían tocar. Y las cuentas
          locales, que son lo único elástico de la fila, se aplastaban a cero. */}
      <HStack
        gap={2}
        // minW: sin un piso, la caja se comprimía a ~30px y no se veía ni un chip completo — el
        // operador tenía que descubrir por scroll que hay pedidos en cocina. Un chip mide ~100px.
        minW="104px"
        maxW="clamp(110px, 26%, 340px)"
        overflowX="auto"
        py={1}
        css={{ scrollbarWidth: 'none', '&::-webkit-scrollbar': { display: 'none' } }}
      >
      {enPreparacion.map((o) => (
        <Button
          key={o.id}
          flexShrink={0}
          minH={TAP}
          px={3}
          borderRadius={RADIUS}
          borderWidth={BORDER_W}
          variant="outline"
          colorPalette="blue"
          onClick={() => onAbrir(o)}
        >
          <VStack gap={0} align="start">
            {/* El animal primero: es lo que se canta en cocina y lo que dice el cliente.
                "Ya se fue" va en ESTE renglón y no en el chico de abajo: el entregado sin cobrar es
                dinero que se fue con el cliente, y depender del color para verlo lo deja invisible
                para quien no distingue naranja de azul — y para cualquiera con prisa, porque el
                renglón de abajo es el que menos se lee. */}
            {/* El animal primero: es lo que se canta en cocina y lo que dice el cliente. */}
            <Text fontWeight="700" fontSize="sm" lineHeight="1.15" whiteSpace="nowrap">
              {o.folioName || `#${o.number}`}
            </Text>
            <Text fontSize="2xs" color="fg.muted" lineHeight="1.15" whiteSpace="nowrap">
              {o.renglones} · {money(o.total, o.currency)}
            </Text>
          </VStack>
        </Button>
      ))}

      </HStack>

      {/* Lo que se debe, en UNA píldora que abre la lista. Es dinero en riesgo, no algo a lo que se
          le agregue: ponerlo como chips lo mezclaba con los pedidos en curso y se comía el ancho de
          la barra. Va fuera de la caja que scrollea porque es la cifra que no se puede perder de
          vista, y adentro se iría con el desplazamiento justo cuando hay muchos pedidos. */}
      {conSaldo.length > 0 && (
        <Button
          flexShrink={0} minH={TAP} px={3} colorPalette="orange" variant="solid"
          onClick={() => setListaDeSaldos(true)}
        >
          <LuWallet />
          <Text as="span" ml={1} fontWeight="800" whiteSpace="nowrap">
            {money(String(data?.outstanding ?? '0'))}
          </Text>
          <Text as="span" ml={1} fontSize="xs" opacity={0.9}>({conSaldo.length})</Text>
        </Button>
      )}

      <SaldosPendientes
        abierto={listaDeSaldos}
        pedidos={conSaldo}
        onCerrar={() => setListaDeSaldos(false)}
        onCobrar={(o) => { setListaDeSaldos(false); setCobrando(o); }}
      />

      <CobrarSheet order={cobrando} onClose={() => setCobrando(null)} onCobrado={refrescar} />
    </>
  );
}

// La lista de lo que se debe, detrás de la píldora.
//
// Es la hoja que tenía "Por cobrar" y que se había perdido al volver todo chips inline. El pedido
// ENTREGADO y sin cobrar es el caso caro —el cliente ya se fue con la comida— y necesita decirse con
// todas sus letras, no caber en un chip de 150 px.
function SaldosPendientes({ abierto, pedidos, onCerrar, onCobrar }: {
  abierto: boolean;
  pedidos: BoardOrder[];
  onCerrar: () => void;
  onCobrar: (o: BoardOrder) => void;
}) {
  const total = pedidos.reduce((s, o) => s + Number(o.outstanding), 0);
  return (
    <DrawerRoot open={abierto} placement="bottom" size="md"
      onOpenChange={(e) => { if (!e.open) onCerrar(); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="2xl">
        <DrawerHeader borderBottomWidth="1px" py={3}>
          <HStack justify="space-between">
            <Text fontWeight="800" fontSize="lg">Por cobrar</Text>
            <Text fontWeight="800" fontSize="lg">{money(String(total))}</Text>
          </HStack>
        </DrawerHeader>
        <DrawerBody py={3}>
          <VStack align="stretch" gap={2}>
            {pedidos.map((o) => (
              <Flex key={o.id} borderWidth="1px" borderColor="orange.300" borderRadius="lg"
                px={3} py={2} align="center" justify="space-between" gap={3}>
                <Box minW={0}>
                  <Text fontWeight="700" lineClamp={1}>{o.folioName || `#${o.number}`}</Text>
                  <Text fontSize="xs" color="fg.muted" lineClamp={1}>
                    #{o.number}
                    {o.customerName ? ` · ${o.customerName}` : ''}
                    {/* El cliente ya se fue con la comida: se dice, no se deja adivinar. */}
                    {o.enPreparacion ? '' : ' · ya se entregó'}
                  </Text>
                </Box>
                <Button minH={TAP} colorPalette="orange" flexShrink={0} onClick={() => onCobrar(o)}>
                  Cobrar {money(o.outstanding, o.currency)}
                </Button>
              </Flex>
            ))}
          </VStack>
        </DrawerBody>
      </DrawerContent>
    </DrawerRoot>
  );
}
