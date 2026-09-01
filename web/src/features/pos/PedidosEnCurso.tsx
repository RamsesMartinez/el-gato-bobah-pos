import { useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Button, HStack, Text, VStack } from '@chakra-ui/react';
import { LuWallet } from 'react-icons/lu';
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

  // Sin pedidos no se pinta nada: un contador en cero es chrome que le quita ancho a la barra.
  if (pedidos.length === 0) return null;

  return (
    <>
      {pedidos.map((o) => (
        <Button
          key={o.id}
          flexShrink={0}
          minH={TAP}
          px={3}
          borderRadius={RADIUS}
          borderWidth={BORDER_W}
          variant="outline"
          // El que sigue en cocina se abre para agregarle; el entregado sin cobrar solo se cobra.
          // Son dos acciones distintas y el color las separa antes de leer el renglón chico.
          colorPalette={o.enPreparacion ? 'blue' : 'orange'}
          onClick={() => (o.enPreparacion ? onAbrir(o) : setCobrando(o))}
        >
          <VStack gap={0} align="start">
            {/* El animal primero: es lo que se canta en cocina y lo que dice el cliente. */}
            <Text fontWeight="700" fontSize="sm" lineHeight="1.15" whiteSpace="nowrap">
              {o.folioName || `#${o.number}`}
            </Text>
            <Text fontSize="2xs" color="fg.muted" lineHeight="1.15" whiteSpace="nowrap">
              {money(o.outstanding, o.currency)}
              {/* Entregado y sin cobrar es el caso caro: el cliente ya se fue con la comida, así
                  que se dice y no se deja adivinar. */}
              {o.enPreparacion ? ` · ${o.renglones}` : ' · ya se entregó'}
            </Text>
          </VStack>
        </Button>
      ))}

      {/* El total en riesgo, que es lo que la píldora dejaba leer de un vistazo. Va al final de los
          chips para que no se confunda con el monto de un pedido. */}
      {Number(data?.outstanding ?? 0) > 0 && (
        <HStack flexShrink={0} gap={1} color="orange.600" px={1}>
          <LuWallet />
          <Text fontWeight="800" fontSize="sm" whiteSpace="nowrap">
            {money(String(data?.outstanding ?? '0'))}
          </Text>
        </HStack>
      )}

      <CobrarSheet order={cobrando} onClose={() => setCobrando(null)} onCobrado={refrescar} />
    </>
  );
}
