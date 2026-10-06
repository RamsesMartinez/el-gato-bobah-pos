import { useEffect, useState } from 'react';
import { createPortal } from 'react-dom';
import { Badge, Box, Button, HStack, Text, VStack } from '@chakra-ui/react';
import { LuBike, LuCheck, LuStore } from 'react-icons/lu';
import type { PedidoDePlataforma } from '../../api/pedidosDePlataforma';
import { money } from '../../utils/format';
import { minutosQueQuedan, urgenciaDe } from './urgencia';

// Alto mínimo de todo lo que se toca.
const TAP = '44px';

// POR ENCIMA DE TODO LO DEMÁS, Y EN SU PROPIO PORTAL.
//
// Es la parte que decide si esta feature sirve. Las hojas del POS —el ticket a pantalla completa,
// los modificadores, el cobro— se montan en un portal fuera del árbol de la página. Un aviso
// pintado como una parte más de la pantalla queda DEBAJO de la hoja que se abra después: tapado
// justo mientras alguien captura una venta de mostrador, que es exactamente el momento en que
// llega un pedido y el único en que este aviso hace falta.
//
// El número es mayor que la capa de las hojas de Chakra a propósito, y el test lo fija: si alguien
// lo baja, la prueba se cae antes que la tableta.
export const CAPA_DEL_AVISO = 2000;

/**
 * AvisoDePedidoEntrante muestra el pedido MÁS URGENTE con sus acciones directas, y cuántos más
 * esperan.
 *
 * Uno solo y no la lista completa: en hora pica pueden llegar tres a la vez, y tres tarjetas
 * apiladas tapan la pantalla entera. El más urgente se acepta de UN toque; para los demás hay un
 * contador que abre la lista — dos toques, y se cuenta así a propósito.
 */
export function AvisoDePedidoEntrante({ pedidos, onAceptar, onVerTodos, aceptando }: {
  pedidos: PedidoDePlataforma[];
  onAceptar: (pedido: PedidoDePlataforma) => void;
  onVerTodos: () => void;
  aceptando: boolean;
}) {
  // El reloj avanza cada 15 segundos, no cada segundo: lo que la pantalla muestra son MINUTOS y dos
  // umbrales, así que repintar cada segundo una pantalla que también dibuja el catálogo no cambia
  // nada de lo que se ve.
  const [ahora, setAhora] = useState(() => Date.now());
  useEffect(() => {
    const t = setInterval(() => setAhora(Date.now()), 15000);
    return () => clearInterval(t);
  }, []);

  if (pedidos.length === 0) return null;
  const pedido = pedidos[0];
  const urgencia = urgenciaDe(pedido.decideBefore, ahora);
  const minutos = minutosQueQuedan(pedido.decideBefore, ahora);
  const paraRecoger = pedido.serviceType === 'para_llevar';

  const color = urgencia === 'por_expirar' ? 'red' : urgencia === 'sonando' ? 'orange' : 'green';

  return createPortal(
    <Box
      data-testid="aviso-de-pedido-entrante"
      position="fixed"
      top={0}
      left={0}
      right={0}
      zIndex={CAPA_DEL_AVISO}
      bg={`${color}.600`}
      color="white"
      px={4}
      py={2}
      boxShadow="lg"
    >
      <HStack justify="space-between" wrap="wrap" gap={3}>
        <VStack align="start" gap={0} minW={0}>
          <HStack gap={2}>
            <Text fontWeight="bold" fontSize="lg" truncate>
              {pedido.platformName} · {pedido.displayId || pedido.customerName || 'Pedido nuevo'}
            </Text>
            <Badge colorPalette={paraRecoger ? 'purple' : 'blue'}>
              {paraRecoger ? <><LuStore /> Pasan por él</> : <><LuBike /> A domicilio</>}
            </Badge>
          </HStack>
          <Text fontSize="sm">
            {(pedido.lines ?? []).length} platillos · {money(Number(pedido.total))}
            {/* MINUTOS Y NUNCA SEGUNDOS. Lo que cambia el comportamiento son dos momentos, no el
                paso de un segundo al siguiente. */}
            {' · '}
            {urgencia === 'por_expirar'
              ? 'Se cancela en cualquier momento'
              : `Quedan ${minutos} min`}
          </Text>
        </VStack>
        <HStack gap={4}>
          {pedidos.length > 1 && (
            // El contador abre la lista. No se apilan tres tarjetas: taparían la pantalla entera.
            <Button minH={TAP} variant="outline" colorPalette="whiteAlpha" onClick={onVerTodos}>
              +{pedidos.length - 1} más
            </Button>
          )}
          {/* ACEPTAR ES EL BOTÓN DOMINANTE Y VA SOLO, sin un «Rechazar» del mismo tamaño al lado:
              con la cuenta corriendo el operador va rápido, y dos botones iguales hacen que el toque
              caiga en el equivocado. Rechazar vive dentro de la lista, que ya cuesta un toque
              llegar — y rechazar además exige elegir un motivo. */}
          <Button
            minH="52px"
            minW="160px"
            fontSize="lg"
            fontWeight="bold"
            colorPalette="whiteAlpha"
            variant="solid"
            loading={aceptando}
            onClick={() => onAceptar(pedido)}
          >
            <LuCheck /> Aceptar
          </Button>
        </HStack>
      </HStack>
    </Box>,
    document.body,
  );
}
