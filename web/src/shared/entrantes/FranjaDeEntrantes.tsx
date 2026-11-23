import { Box, Button, Flex, Text } from '@chakra-ui/react';
import { LuBellRing, LuVolumeX } from 'react-icons/lu';
import type { PedidoEntrante } from '../../api/pedidosEntrantes';
import type { EstadoAlarma } from './alarma';
import { CAPA_FRANJA } from './capas';
import { tiempoDe } from './urgencia';

/**
 * FranjaDeEntrantes es el aviso de pedidos por aceptar en cualquier pantalla (M-Indicador, opción A).
 *
 * UNA FILA Y NO UN FLOTANTE: empuja la pantalla 52 px en vez de tapar un buscador o un botón, y solo
 * existe mientras hay algo por aceptar. Va en la capa más alta (`CAPA_FRANJA`): encima de cualquier
 * hoja abierta y encima del bloqueo por PIN, que es cuando más suele llegar un pedido.
 *
 * CON LA TABLETA BLOQUEADA no dice de quién es ni qué pidió —quien la ve no ha entrado— y no ofrece
 * entrar: se desbloquea con el PIN. Tampoco hay botón para el sonido, porque debajo del bloqueo nada
 * se puede tocar; cualquier toque en la pantalla de PIN lo activa.
 */
export function FranjaDeEntrantes({ pedidos, alarma, tabletaBloqueada, ahora, onVer, onActivarSonido }: {
  pedidos: PedidoEntrante[];
  alarma: EstadoAlarma;
  tabletaBloqueada: boolean;
  ahora: number;
  onVer: () => void;
  onActivarSonido: () => void;
}) {
  if (pedidos.length === 0) return null;
  const urgente = pedidos[0];
  const uno = pedidos.length === 1;
  const sinSonido = alarma === 'bloqueada';

  return (
    <Flex data-testid="franja-de-entrantes" data-alarma={alarma} position="relative" zIndex={CAPA_FRANJA}
      minH="52px" align="center" gap={3} px={3} bg="red.600" color="white" flexShrink={0} role="status">
      <LuBellRing size={22} aria-hidden />
      <Box flex="1" minW={0}>
        <Text fontWeight="800" lineHeight="1.2">{uno ? '1 pedido por aceptar' : `${pedidos.length} pedidos por aceptar`}</Text>
        {tabletaBloqueada ? (
          sinSonido && <Text fontSize="sm">Toca la pantalla para activar el sonido</Text>
        ) : (
          <Text fontSize="sm" lineClamp={1}>
            {urgente.sourceName} · {urgente.customerName || urgente.displayId} · {tiempoDe(urgente, ahora)}
          </Text>
        )}
      </Box>
      {!tabletaBloqueada && sinSonido && (
        <Button minH="44px" bg="yellow.300" color="yellow.950" fontWeight="700" onClick={onActivarSonido}>
          <LuVolumeX /> Toca para activar el sonido
        </Button>
      )}
      {!tabletaBloqueada && (
        <Button minH="44px" px={5} bg="white" color="red.700" fontWeight="800" onClick={onVer}>
          {uno ? 'Ver pedido' : 'Ver pedidos'}
        </Button>
      )}
    </Flex>
  );
}
