import { Box, Button, HStack, Text } from '@chakra-ui/react';
import { LuList, LuPlus } from 'react-icons/lu';
import type { AccountItem } from '../../types/pos';
import { useContainerWidth } from '../../hooks/useContainerWidth';
import { BOTON, ESTADO, FICHA, GAP, fichasQueCaben, loQueFalta, nombreDeCuenta, ordenDeLaFila } from './estadosDeCuenta';

// LA FILA DE CUENTAS (spec 030, US1): todas las cuentas vivas de todas las tabletas, arriba del
// menú. Reemplaza a las pestañas «Cuenta 1 · 2» —que vivían solo en una tableta— y al botón naranja
// de «Pedidos por cobrar»: para quien atiende las dos eran «la cuenta de la mesa».
//
// Fichas de ANCHO FIJO y solo las que caben completas: a 1024×600 con el panel abierto la fila
// tiene ~612 px, y un scroll horizontal escondido es una cuenta que nadie ve. Lo que no cabe se
// cuenta en «+N», que abre la lista completa. «+N» y «+» siempre se ven.
//
// Sin ✕: descartar vive en el ⋮ del ticket, lejos de lo que se toca todo el día.
//
// La cuenta nueva que todavía no tiene productos NO existe en el servidor (nace con el primero),
// pero sí se pinta: tras «+» el operador tiene que ver que está en una cuenta vacía, y que la que
// capturaba sigue junto a ella.

interface Props {
  cuentas: AccountItem[];
  seleccionada: string | null;
  onElegir: (c: AccountItem) => void;
  onNueva: () => void;
  onVerTodas: () => void;
  // Las cuentas que esta tableta abrió, de la más reciente a la más vieja.
  recientes?: string[];
  // La tableta está en una cuenta nueva sin productos: se pinta como ficha activa.
  nueva?: boolean;
  // Solo para los tests: jsdom no hace layout.
  ancho?: number;
}

export function FilaDeCuentas({ cuentas, seleccionada, onElegir, onNueva, onVerTodas, recientes, nueva, ancho }: Props) {
  const { ref, width } = useContainerWidth<HTMLDivElement>();
  const medido = ancho ?? width;
  const caben = Math.max(0, fichasQueCaben(medido) - (nueva ? 1 : 0));
  const orden = ordenDeLaFila(cuentas, seleccionada, recientes);
  const visibles = orden.slice(0, caben);
  const fuera = orden.length - visibles.length;

  return (
    <HStack ref={ref} gap={`${GAP}px`} minW={0} w="100%" overflow="hidden" aria-label="Cuentas">
      {nueva && fichasQueCaben(medido) > 0 && (
        <Button data-ficha="true" aria-pressed aria-label="Cuenta nueva, sin productos"
          w={`${FICHA}px`} minW={`${FICHA}px`} minH="44px" h="44px" px={2} py={0.5} flexShrink={0}
          display="flex" flexDirection="column" alignItems="flex-start" justifyContent="center" gap={0}>
          <Text fontSize="xs" fontWeight="700" truncate w="100%" textAlign="left" lineHeight="1.2">Cuenta nueva</Text>
          <Text as="span" fontSize="2xs" lineHeight="1.1" opacity={0.85}>Sin productos</Text>
        </Button>
      )}
      {visibles.map((c) => {
        const e = ESTADO[c.state];
        const nombre = nombreDeCuenta(c);
        const activa = c.key === seleccionada;
        return (
          <Button key={c.key} data-ficha="true" aria-pressed={activa}
            aria-label={`${nombre} · ${e.texto}`}
            w={`${FICHA}px`} minW={`${FICHA}px`} minH="44px" h="44px" px={2} py={0.5}
            flexShrink={0} variant={activa ? 'solid' : 'outline'}
            colorPalette={activa ? undefined : 'gray'}
            display="flex" flexDirection="column" alignItems="flex-start" justifyContent="center" gap={0}
            onClick={() => onElegir(c)}>
            <Text fontSize="xs" fontWeight="700" truncate w="100%" textAlign="left" lineHeight="1.2">
              {nombre}
            </Text>
            {/* La cifra no cede: con la ficha angosta se corta la etiqueta, nunca «falta $114». */}
            <HStack gap={1} w="100%" minW={0} lineHeight="1.1">
              <Text as="span" fontSize="2xs" fontWeight="700" color={activa ? undefined : `${e.color}.fg`}
                truncate minW={0} flexShrink={1}>
                {e.texto}
              </Text>
              <Text as="span" fontSize="2xs" fontWeight="700" whiteSpace="nowrap" flexShrink={0} opacity={0.9}>
                {loQueFalta(c)}
              </Text>
            </HStack>
          </Button>
        );
      })}
      <Box flex="1" />
      <Button flexShrink={0} minW={`${BOTON}px`} minH="44px" h="44px" px={2} variant="outline" colorPalette="gray"
        aria-label={fuera > 0 ? `Ver todas las cuentas (${fuera} más)` : 'Ver todas las cuentas'}
        onClick={onVerTodas}>
        {fuera > 0 ? <Text fontWeight="800">+{fuera}</Text> : <LuList />}
      </Button>
      <Button flexShrink={0} minW={`${BOTON}px`} minH="44px" h="44px" px={2} variant="outline"
        aria-label="Cuenta nueva" onClick={onNueva}>
        <LuPlus />
      </Button>
    </HStack>
  );
}
