import { Box, HStack, SimpleGrid, Stack, Text } from '@chakra-ui/react';
import { LuCheck, LuTriangle } from 'react-icons/lu';
import type { CutDiff } from '../../api/backoffice';
import { CERO, diffColor, montoDeCaja } from './montosDeCaja';

// Diferencia escribe una diferencia de caja con palabra, signo, ícono y color (#290, FR-004; WCAG
// 1.4.1): en la Caja B un faltante distinguido solo por el color se leyó como cuadrado.
//
// `apilada` pone la palabra arriba y el monto abajo: en una tarjeta angosta (el diálogo de 512 px
// parte el ancho en tres) «Faltante −$12,345.00» en una línea se salía de su tarjeta.
export function Diferencia({ valor, currency, grande, etiqueta, apilada }: {
  valor: string | number | null; currency: string; grande?: boolean; etiqueta?: string; apilada?: boolean;
}) {
  const fontSize = grande ? 'xl' : 'sm';
  if (valor === null || valor === '' || !Number.isFinite(Number(valor))) {
    return <Text aria-label={etiqueta} color="fg.muted" fontSize={fontSize} textAlign="end">—</Text>;
  }
  const n = Number(valor);
  if (Math.abs(n) < CERO) {
    return (
      <HStack aria-label={etiqueta} gap={1} justify="flex-end" color="fg.muted" fontSize={fontSize} fontWeight="600">
        <LuCheck aria-hidden /> <Text>Cuadra</Text>
      </HStack>
    );
  }
  const falta = n < 0;
  // Signo menos tipográfico (U+2212), no el guion: en columnas de dinero el guion se pierde.
  const monto = (falta ? '−' : '+') + montoDeCaja(Math.abs(n), currency);
  return (
    <Stack direction={apilada ? 'column' : 'row'} aria-label={etiqueta} gap={apilada ? 0 : 1}
      justify="flex-end" align={apilada ? 'flex-start' : 'center'} color={diffColor(n)} fontSize={fontSize}
      fontWeight="700" whiteSpace="nowrap">
      <Text fontSize={apilada ? 'sm' : undefined}>{falta ? 'Faltante' : 'Sobrante'}</Text>
      <Text fontVariantNumeric="tabular-nums">{monto}</Text>
      <Box as="span" display="inline-flex" fontSize="0.8em" transform={falta ? 'rotate(180deg)' : undefined} aria-hidden>
        <LuTriangle />
      </Box>
    </Stack>
  );
}

// ResumenDeDiferencia: la diferencia de un corte en sus tres partes, arriba del detalle y en el
// diálogo de cierre (Maquetas M1). Sin arqueo no se pinta la parte del cajón: un «Cuadra» ahí
// afirmaría un conteo que no existió.
export function ResumenDeDiferencia({ diferencia, currency }: { diferencia: CutDiff; currency: string }) {
  // Sin cajón aparte, «Otros métodos» y «Total» serían la misma cifra dos veces: va solo el total.
  const partes: [string, string | null][] = diferencia.drawer !== null
    ? [['Cajón (arqueo)', diferencia.drawer], ['Otros métodos (conciliación)', diferencia.otherMethods], ['Total', diferencia.total]]
    : [['Total', diferencia.total]];
  return (
    <SimpleGrid columns={partes.length} gap={3}>
      {partes.map(([k, v]) => (
        <Box key={k} borderWidth="1px" borderRadius="lg" p={3} bg="bg.panel" minW={0}>
          <Text fontSize="sm" color="fg.muted">{k}</Text>
          <Diferencia valor={v} currency={currency} grande={k === 'Total'} etiqueta={`Diferencia: ${k}`} apilada />
        </Box>
      ))}
    </SimpleGrid>
  );
}
