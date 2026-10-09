import { Box, HStack, Text } from '@chakra-ui/react';
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

  const pendiente = resumen.pending ?? { count: 0, amount: '0' };
  return (
    // UNA sola fila con scroll horizontal, en este orden: Total → Por cobrar → medios → separador →
    // lo demás. Medido a 1024×600: dos filas (tiles y medios) dejaban la tabla en DOS renglones. Y
    // el orden no es estético: los medios son lo que prueba que el Total cuadra, así que van pegados
    // a él y no pasado el borde derecho. El degradado del borde avisa que la fila sigue.
    <Box position="relative">
      <HStack gap={2} overflowX="auto" pb={1} pr={8} css={{ scrollbarWidth: 'none' }} align="stretch">
        <Tile label="Total" valor={money(resumen.total)} nota="cobrado − devuelto" destacado />
        {pendiente.count > 0 && (
          <Tile label="Por cobrar" valor={money(pendiente.amount)} tono="orange"
            nota={`no entra al total · ${pendiente.count} ${pendiente.count === 1 ? 'pedido' : 'pedidos'}`} />
        )}
        {resumen.byMethod.map((m) => (
          <Medio key={m.methodId} m={m} />
        ))}

        <Box alignSelf="stretch" borderLeftWidth="1px" mx={1} flexShrink={0} />
        <Tile label="Ventas" valor={String(resumen.count)} nota="sin canceladas" />
        <Tile label="Ticket promedio" valor={money(resumen.average)} nota="de los pedidos" />
        {Number(resumen.tips) > 0 && <Tile label="Propinas" valor={money(resumen.tips)} nota="no entra al total" />}
        {resumen.cancelled.count > 0 && (
          <Tile label="Canceladas" valor={money(resumen.cancelled.amount)} nota={`${resumen.cancelled.count}`} />
        )}
        {resumen.refunded.count > 0 && (
          <Tile label="Devoluciones" valor={money(resumen.refunded.amount)} nota={`${resumen.refunded.count}`} />
        )}
        {resumen.cancelledLines.count > 0 && (
          <Tile label="Renglones cancelados" valor={money(resumen.cancelledLines.amount)} nota={`${resumen.cancelledLines.count}`} />
        )}

        {/* Lo que impide que las cifras de plataformas se resten con las de arriba es que cada una
            lleva SOBRE CUÁNTOS PEDIDOS habla: los conjuntos son distintos —hay pedidos vendidos
            cuyo documento todavía no llega— y sin el conteo tres importes hermanos se restan a ojo.
            Es la forma exacta del fondo de caja que dejó un turno con $4,500 de faltante. */}
        {plataformas && plataformas.llegoAlBanco.orders > 0 && (
          <>
            <Box alignSelf="stretch" borderLeftWidth="1px" mx={1} flexShrink={0} />
            <CifraDePlataformaTile c={plataformas.vendido} label="Vendido por plataformas" />
            <CifraDePlataformaTile c={plataformas.seQuedoLaPlataforma} label="Se quedó la plataforma" />
            <CifraDePlataformaTile c={plataformas.llegoAlBanco} label="Llegó al banco" />
            {plataformas.sinLiquidar.orders > 0 && (
              <Tile label="Sin liquidar" valor={String(plataformas.sinLiquidar.orders)}
                nota="falta su documento" />
            )}
            {plataformas.sinFolio.orders > 0 && (
              <Tile label="Sin folio de plataforma" valor={String(plataformas.sinFolio.orders)}
                nota="no se pueden conciliar" />
            )}
          </>
        )}
      </HStack>
      <Box position="absolute" top={0} right={0} bottom={1} w={8} pointerEvents="none"
        bgGradient="to-l" gradientFrom="bg" gradientTo="transparent" />
    </Box>
  );
}

// Un medio de pago: lo cobrado neto. Lo devuelto se dice como YA RESTADO y en gris: en rojo y con
// un «−» se leía como algo pendiente de restar, y el operador lo restaba otra vez a ojo.
function Medio({ m }: { m: SalesSummary['byMethod'][number] }) {
  const devuelto = Number(m.refunds ?? 0);
  const propina = Number(m.tipRefunds ?? 0);
  return (
    <Box borderWidth="1px" borderRadius="lg" px={3} py={2} minW="130px" bg="bg.subtle" flexShrink={0}>
      <Text fontSize="xs" color="fg.muted" whiteSpace="nowrap">{m.method}</Text>
      <Text fontSize="lg" fontWeight="700" whiteSpace="nowrap" color={Number(m.total) < 0 ? 'red.600' : undefined}>
        {money(m.total)}
      </Text>
      {devuelto > 0 && (
        <Text fontSize="2xs" color="fg.muted" whiteSpace="nowrap">ya restados {money(devuelto)} devueltos</Text>
      )}
      {propina > 0 && (
        <Text fontSize="2xs" color="fg.muted" whiteSpace="nowrap">+ {money(propina)} de propina devuelta</Text>
      )}
    </Box>
  );
}

// El detalle de qué incluye y qué excluye va detrás de un ICONO DE AYUDA, no como texto fijo:
// cuatro líneas de prosa por tarjeta en cada refresco gastan el alto que la tabla necesita, y es la
// explicación de un cálculo — justo lo que la constitución manda guardar detrás de la ayuda.
function CifraDePlataformaTile({ c, label }: { c: PlatformMoney['vendido']; label: string }) {
  return (
    <Box borderWidth="1px" borderRadius="lg" px={3} py={2} minW="150px" bg="bg.panel" flexShrink={0}>
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

function Tile({ label, valor, nota, destacado, tono }: {
  label: string; valor: string; nota?: string; destacado?: boolean; tono?: 'orange';
}) {
  return (
    <Box borderWidth="1px" borderRadius="lg" px={3} py={2} minW="120px" flexShrink={0}
      borderColor={tono === 'orange' ? 'orange.300' : undefined}
      bg={destacado ? 'colorPalette.subtle' : 'bg.panel'}>
      <Text fontSize="xs" color="fg.muted" whiteSpace="nowrap">{label}</Text>
      <Text fontSize={destacado ? '2xl' : 'xl'} fontWeight="800" whiteSpace="nowrap">{valor}</Text>
      {nota && <Text fontSize="2xs" color="fg.muted" whiteSpace="nowrap">{nota}</Text>}
    </Box>
  );
}
