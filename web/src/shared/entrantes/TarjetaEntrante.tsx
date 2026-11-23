import { useState } from 'react';
import { Box, Button, Flex, HStack, Text, VStack } from '@chakra-ui/react';
import { LuBike, LuChevronDown, LuGlobe, LuMapPin, LuPhone, LuStore } from 'react-icons/lu';
import { SiUbereats } from 'react-icons/si';
import type { IconType } from 'react-icons';
import type { PedidoEntrante, RenglonEntrante } from '../../api/pedidosEntrantes';
import { money } from '../../utils/format';
import { tiempoDe } from './urgencia';

const TAP = '44px';
/** Renglones a la vista en la tarjeta abierta: con más, sus botones salían de la pantalla a 600 px. */
const RENGLONES_A_LA_VISTA = 3;

// El ícono del origen junto al nombre (M-PorAceptar): se reconoce de un vistazo de dónde viene.
const ICONO: Record<string, { icon: IconType; bg: string; color: string }> = {
  'Uber Eats': { icon: SiUbereats, bg: 'gray.900', color: 'white' },
  Web: { icon: LuGlobe, bg: 'blue.subtle', color: 'blue.fg' },
};
const ICONO_GENERICO = { icon: LuBike, bg: 'gray.subtle', color: 'fg' };

function OrigenIcono({ nombre }: { nombre: string }) {
  const o = ICONO[nombre] ?? ICONO_GENERICO;
  const Icon = o.icon;
  return (
    <Flex flexShrink={0} w="36px" h="36px" borderRadius="md" align="center" justify="center" bg={o.bg} color={o.color} aria-hidden>
      <Icon size={20} />
    </Flex>
  );
}

const piezas = (ls: RenglonEntrante[]) => ls.reduce((s, l) => s + Number(l.quantity), 0);

/**
 * TarjetaEntrante es un pedido por aceptar con la misma forma que las tarjetas de «En preparación»:
 * nombre en grande, total a la derecha, renglones con borde. `plegada` esconde lo que pidieron
 * detrás de un toque para que, en hora pico, quepan los botones de más de un pedido.
 */
export function TarjetaEntrante({ p, ahora, plegada, aceptando, ocupada, onAceptar, onRechazar }: {
  p: PedidoEntrante;
  ahora: number;
  plegada: boolean;
  aceptando: boolean;
  /** Otra decisión está en curso: no se ofrece una segunda a la vez. */
  ocupada: boolean;
  onAceptar: () => void;
  onRechazar: () => void;
}) {
  // Abierta si es la más urgente o si alguien la abrió: cuando la de arriba se decide, la que sube
  // queda abierta sola, y una que se abrió a mano no se vuelve a plegar.
  const [abiertaAMano, setAbierta] = useState(false);
  const abierta = !plegada || abiertaAMano;
  const [todos, setTodos] = useState(false);
  const lines = p.lines ?? [];
  const recoger = p.serviceType === 'para_llevar';
  const Svc = recoger ? LuStore : LuBike;
  const sinPareja = lines.filter((l) => !l.matched).length;
  const n = piezas(lines);
  const visibles = todos ? lines : lines.slice(0, RENGLONES_A_LA_VISTA);

  return (
    <Box data-entrante bg="bg.panel" borderWidth="1px" borderColor="border" borderRadius="lg" p={2.5}>
      <Flex justify="space-between" align="start" gap={2} mb={2}>
        <HStack align="start" gap={2} minW={0}>
          <OrigenIcono nombre={p.sourceName} />
          <Box minW={0}>
            <Text fontWeight="800" fontSize="lg" lineHeight="1.2" lineClamp={1}>{p.customerName || p.displayId}</Text>
            <Text fontSize="xs" color="fg.muted">{p.sourceName} · {p.displayId}</Text>
            <HStack fontSize="xs" color="fg.muted" gap={1}><Svc size={12} /><Text as="span">{recoger ? 'Pasan por él' : 'A domicilio'}</Text></HStack>
          </Box>
        </HStack>
        <VStack align="end" gap={0} flexShrink={0}>
          <Text fontWeight="700" lineHeight="1.2">{money(p.total)}</Text>
          <Text fontSize="xs" fontWeight={p.decideBefore ? '700' : '400'} color={p.decideBefore ? 'orange.600' : 'fg.muted'}>
            {tiempoDe(p, ahora)}
          </Text>
        </VStack>
      </Flex>

      {abierta && (p.customerPhone || p.address) && (
        <VStack align="stretch" gap={0.5} mb={2} fontSize="sm">
          {p.customerPhone && <HStack gap={1.5}><LuPhone size={14} /><Text fontWeight="600">{p.customerPhone}</Text></HStack>}
          {p.address && <HStack gap={1.5} align="start"><Box pt={0.5}><LuMapPin size={14} /></Box><Text lineClamp={2}>{p.address}</Text></HStack>}
        </VStack>
      )}

      {!abierta ? (
        <Button w="100%" minH={TAP} mb={2} variant="outline" colorPalette="gray" justifyContent="space-between"
          onClick={() => setAbierta(true)}>
          {n === 1 ? '1 producto' : `${n} productos`}{sinPareja > 0 ? ` · ${sinPareja} no está en el menú` : ''}
          <LuChevronDown />
        </Button>
      ) : (
        <VStack role="list" align="stretch" gap={1} mb={2}>
          {visibles.map((l, i) => (
            <HStack role="listitem" key={`${l.name}-${i}`} gap={1.5} borderWidth="1px" borderColor="border" borderRadius="md" px={1.5} py={1}>
              <Text fontWeight="800" fontSize="sm" minW="1.75rem" textAlign="center" flexShrink={0}>{Number(l.quantity)}</Text>
              <Box flex="1" minW={0}>
                <Text fontWeight="600" fontSize="sm" lineHeight="1.25" lineClamp={1}>{l.name}</Text>
                {(l.options ?? []).length > 0 && (
                  <Text fontSize="xs" color="fg.muted" lineClamp={1}>{(l.options ?? []).map((o) => o.name).join(', ')}</Text>
                )}
                {!l.matched && <Text fontSize="xs" fontWeight="700" color="orange.600">No está en el menú</Text>}
              </Box>
            </HStack>
          ))}
          {lines.length > visibles.length && (
            <Button minH={TAP} variant="ghost" size="sm" onClick={() => setTodos(true)}>+{lines.length - visibles.length} más</Button>
          )}
        </VStack>
      )}

      {/* Como «Entregar todo | Abrir cuenta» del tablero: la acción frecuente a la izquierda y
          sólida; Rechazar aparte, porque pide un motivo y no se deshace. */}
      <HStack gap={4}>
        <Button flex="1" minH={TAP} colorPalette="green" loading={aceptando} disabled={ocupada && !aceptando} onClick={onAceptar}>
          Aceptar
        </Button>
        <Button minH={TAP} px={4} variant="outline" colorPalette="red" disabled={ocupada} onClick={onRechazar}>
          Rechazar
        </Button>
      </HStack>
    </Box>
  );
}
