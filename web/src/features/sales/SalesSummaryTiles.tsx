import { Box, HStack, Text, VStack } from '@chakra-ui/react';

import type { SalesSummary } from '../../api/sales';
import { money } from '../../utils/format';

// El resumen de arriba de la pantalla.
//
// Cada cifra dice qué incluye, y la separación no es estética: la propina es dinero del personal
// que pasa por la caja, la cancelada es ingreso que no ocurrió, y el envío ya está DENTRO del
// total. Ponerlos como renglones hermanos sin decirlo invita a sumarlos y a reportar una venta que
// el negocio no tuvo.
export function SalesSummaryTiles({ resumen, cargando }: { resumen?: SalesSummary; cargando?: boolean }) {
  if (!resumen) {
    return (
      <Box borderWidth="1px" borderRadius="lg" p={4}>
        <Text color="fg.muted">{cargando ? 'Calculando…' : 'Sin datos del periodo'}</Text>
      </Box>
    );
  }

  return (
    <VStack align="stretch" gap={2}>
      {/* Scroll horizontal propio: en una tablet de 7" cuatro cifras no caben, y que se desborde la
          página entera obligaría a mover la tabla para leer el total. */}
      <HStack gap={2} overflowX="auto" pb={1} css={{ scrollbarWidth: 'none' }}>
        <Tile label="Ventas" valor={String(resumen.count)} />
        <Tile label="Total" valor={money(resumen.total)} destacado />
        <Tile label="Promedio" valor={money(resumen.average)} />
        {Number(resumen.tips) > 0 && <Tile label="Propinas" valor={money(resumen.tips)} nota="no entra al total" />}
        {resumen.cancelled.count > 0 && (
          <Tile label="Canceladas" valor={money(resumen.cancelled.amount)} nota={`${resumen.cancelled.count}`} />
        )}
        {resumen.refunded.count > 0 && (
          <Tile label="Reembolsadas" valor={money(resumen.refunded.amount)} nota={`${resumen.refunded.count}`} />
        )}
        {resumen.cancelledLines.count > 0 && (
          <Tile label="Renglones cancelados" valor={money(resumen.cancelledLines.amount)} nota={`${resumen.cancelledLines.count}`} />
        )}
      </HStack>

      {resumen.byMethod.length > 0 && (
        <HStack gap={2} overflowX="auto" pb={1} css={{ scrollbarWidth: 'none' }}>
          {resumen.byMethod.map((m) => (
            <Box key={m.methodId} borderWidth="1px" borderRadius="lg" px={3} py={2} minW="150px" bg="bg.subtle">
              <Text fontSize="xs" color="fg.muted" whiteSpace="nowrap">{m.method}</Text>
              <Text fontWeight="700" whiteSpace="nowrap">{money(m.total)}</Text>
            </Box>
          ))}
        </HStack>
      )}
    </VStack>
  );
}

function Tile({ label, valor, nota, destacado }: { label: string; valor: string; nota?: string; destacado?: boolean }) {
  return (
    <Box borderWidth="1px" borderRadius="lg" px={4} py={3} minW="140px"
      bg={destacado ? 'colorPalette.subtle' : 'bg.panel'}>
      <Text fontSize="xs" color="fg.muted" whiteSpace="nowrap">{label}</Text>
      <Text fontSize={destacado ? '2xl' : 'xl'} fontWeight="800" whiteSpace="nowrap">{valor}</Text>
      {nota && <Text fontSize="2xs" color="fg.muted" whiteSpace="nowrap">{nota}</Text>}
    </Box>
  );
}
