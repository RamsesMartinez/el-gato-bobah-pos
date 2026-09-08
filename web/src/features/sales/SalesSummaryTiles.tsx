import { Box, HStack, Text, VStack } from '@chakra-ui/react';
import { LuCircleHelp } from 'react-icons/lu';

import type { SalesSummary } from '../../api/sales';
import type { PlatformMoney } from '../../api/settlements';
import { Tooltip } from '../../components/ui/tooltip';
import { money } from '../../utils/format';

// El resumen de arriba de la pantalla.
//
// Cada cifra dice qué incluye, y la separación no es estética: la propina es dinero del personal
// que pasa por la caja, la cancelada es ingreso que no ocurrió, y el envío ya está DENTRO del
// total. Ponerlos como renglones hermanos sin decirlo invita a sumarlos y a reportar una venta que
// el negocio no tuvo.
export function SalesSummaryTiles({ resumen, plataformas, cargando }: {
  resumen?: SalesSummary;
  /** Las tres cifras de plataformas. Solo se pintan si el periodo tiene alguna liquidación. */
  plataformas?: PlatformMoney;
  cargando?: boolean;
}) {
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

      {/* Las tres cifras de plataformas, y SOLO si hay al menos una liquidación capturada. Sin
          liquidaciones no habría nada que decir y la fila costaría alto por nada.

          Cada tile lleva SOBRE CUÁNTOS PEDIDOS habla. No es adorno: los conjuntos son distintos
          —hay pedidos vendidos cuyo documento todavía no llega— y tres importes hermanos sin su
          conteo se restan a ojo. Es la forma exacta del fondo de caja que dejó un turno con $4,500
          de faltante sin explicación. */}
      {plataformas && plataformas.llegoAlBanco.orders > 0 && (
        <HStack gap={2} overflowX="auto" pb={1} css={{ scrollbarWidth: 'none' }}>
          <CifraDePlataformaTile c={plataformas.vendido} label="Vendido por plataformas" />
          <CifraDePlataformaTile c={plataformas.seQuedoLaPlataforma} label="Se quedó la plataforma" />
          <CifraDePlataformaTile c={plataformas.llegoAlBanco} label="Llegó al banco" />
          {plataformas.sinLiquidar.count > 0 && (
            <Tile label="Sin liquidar" valor={String(plataformas.sinLiquidar.count)}
              nota="falta su documento" />
          )}
          {plataformas.sinFolio.count > 0 && (
            <Tile label="Sin folio" valor={String(plataformas.sinFolio.count)}
              nota="no se pueden conciliar" />
          )}
        </HStack>
      )}

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

// El detalle de qué incluye y qué excluye va detrás de un ICONO DE AYUDA, no como texto fijo:
// cuatro líneas de prosa por tarjeta en cada refresco gastan el alto que la tabla necesita, y es la
// explicación de un cálculo — justo lo que la constitución manda guardar detrás de la ayuda.
function CifraDePlataformaTile({ c, label }: { c: PlatformMoney['vendido']; label: string }) {
  return (
    <Box borderWidth="1px" borderRadius="lg" px={4} py={3} minW="170px" bg="bg.panel">
      <HStack gap={1} align="center">
        <Text fontSize="xs" color="fg.muted" whiteSpace="nowrap">{label}</Text>
        <Tooltip content={`Incluye: ${c.incluye}. Excluye: ${c.excluye}.`}>
          <Box as="span" color="fg.muted" aria-label={`Qué incluye ${label}`}><LuCircleHelp size={12} /></Box>
        </Tooltip>
      </HStack>
      <Text fontSize="xl" fontWeight="800" whiteSpace="nowrap">{money(c.amount)}</Text>
      {/* El conteo SIEMPRE a la vista, no en el tooltip: es lo que impide restar las tres cifras. */}
      <Text fontSize="2xs" color="fg.muted" whiteSpace="nowrap">
        {c.orders} {c.orders === 1 ? 'pedido' : 'pedidos'}
      </Text>
    </Box>
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
