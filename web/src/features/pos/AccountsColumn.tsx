import { useEffect, useRef } from 'react';
import { Box, Button, Flex, HStack, Text, VStack } from '@chakra-ui/react';
import { LuList, LuPlus } from 'react-icons/lu';
import { missingFor, nombreDeCuenta } from '../../domain/cuentas';
import type { AccountItem } from '../../types/pos';
import { AccountActions } from './AccountActions';

// LA COLUMNA DE CUENTAS (spec 089, maqueta B elegida por el dueño): todas las cuentas vivas, una por
// renglón, a la izquierda del catálogo. Reemplaza en modo ancho a la fila de fichas, que a 1024×600
// con el ticket abierto enseñaba dos y mandaba el resto a «+N»; aquí caben siete.
//
// El orden es por antigüedad y NO cambia al elegir: en una lista vertical, subir la elegida movía la
// cuenta bajo el dedo. La abierta se marca y se desplaza a la vista.
//
// Cuando a una cuenta le falta UNA sola cosa (entregar una pagada, cobrar una entregada), el renglón
// trae su botón: es la cuenta que se acumulaba porque nadie la marcaba entregada.
export const ANCHO_COLUMNA = 248;

interface Props {
  cuentas: AccountItem[];
  seleccionada: string | null;
  onElegir: (c: AccountItem) => void;
  onNueva: () => void;
  onDeliver: (c: AccountItem) => void;
  onCharge: (c: AccountItem) => void;
  // La hoja con todas las cuentas: la única que agrupa y que mira las deudas de más de 90 días.
  onVerTodas: () => void;
  delivering?: ReadonlySet<string>;
  // La tableta está en una cuenta nueva sin productos: se pinta como primer renglón.
  nueva?: boolean;
}

export function AccountsColumn({ cuentas, seleccionada, onElegir, onNueva, onDeliver, onCharge, onVerTodas, delivering, nueva }: Props) {
  const orden = [...cuentas].sort((a, b) => a.openedAt.localeCompare(b.openedAt));
  const lista = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = lista.current?.querySelector<HTMLElement>('[aria-pressed="true"]');
    if (el && typeof el.scrollIntoView === 'function') el.scrollIntoView({ block: 'nearest' });
  }, [seleccionada, nueva]);

  return (
    <Flex direction="column" w={`${ANCHO_COLUMNA}px`} flexShrink={0} minH={0} h="100%"
      borderRightWidth="1px" borderColor="border" bg="bg.muted">
      <HStack px={2} py={2} gap={2} flexShrink={0}>
        <Button flex="1" minW={0} minH="44px" size="sm" variant="ghost" colorPalette="gray" justifyContent="flex-start" px={1}
          aria-label="Ver todas las cuentas" onClick={onVerTodas}>
          <Text fontWeight="700" fontSize="md">Cuentas ({cuentas.length})</Text>
          <LuList />
        </Button>
        <Button minH="44px" size="sm" variant="outline" bg="bg.panel" aria-label="Cuenta nueva" onClick={onNueva}>
          <LuPlus /> Nueva
        </Button>
      </HStack>
      <VStack ref={lista} aria-label="Cuentas" align="stretch" gap={1} px={2} pb={2} flex="1" minH={0} overflowY="auto"
        css={{ overscrollBehavior: 'contain' }}>
        {/* Dentro de un HStack, igual que las demás: suelto en la columna, su `flex="1"` lo estiraba a
            todo el alto. */}
        {nueva && (
          <HStack gap={1} align="stretch">
            <Renglon activa nombre="Cuenta nueva" texto="Sin productos" color="gray" etiqueta="Cuenta nueva, sin productos" />
          </HStack>
        )}
        {orden.map((c) => {
          const m = missingFor(c);
          const nombre = nombreDeCuenta(c);
          return (
            // 8 px entre el renglón y su botón: «Entregar» es de un toque y la cuenta sale de la fila.
            <HStack key={c.key} gap={2} align="stretch">
              <Renglon activa={c.key === seleccionada} nombre={nombre} texto={m.text} color={m.color}
                etiqueta={`${nombre} · ${m.text}`} onClick={() => onElegir(c)} />
              <Box display="flex" alignItems="center">
                <AccountActions c={c} compact size="sm" onDeliver={onDeliver} onCharge={onCharge}
                  delivering={delivering?.has(c.key)} />
              </Box>
            </HStack>
          );
        })}
        {cuentas.length === 0 && !nueva && (
          <Text color="fg.muted" fontSize="sm" textAlign="center" py={6}>No hay cuentas abiertas</Text>
        )}
      </VStack>
    </Flex>
  );
}

function Renglon({ activa, nombre, texto, color, etiqueta, onClick }: {
  activa: boolean; nombre: string; texto: string; color: string; etiqueta: string; onClick?: () => void;
}) {
  return (
    <Button data-ficha="true" aria-pressed={activa} aria-label={etiqueta} onClick={onClick}
      flex="1" minW={0} minH="56px" h="auto" py={1.5} px={2.5} justifyContent="flex-start"
      variant={activa ? 'subtle' : 'outline'} colorPalette={activa ? undefined : 'gray'}
      bg={activa ? undefined : 'bg.panel'} borderColor={activa ? 'colorPalette.solid' : undefined} borderWidth="1px">
      <VStack align="start" gap={0} minW={0} w="100%">
        <Text fontSize="sm" fontWeight="700" truncate w="100%" textAlign="left">{nombre}</Text>
        <Text fontSize="xs" fontWeight="700" color={`${color}.fg`} truncate w="100%" textAlign="left">{texto}</Text>
      </VStack>
    </Button>
  );
}
