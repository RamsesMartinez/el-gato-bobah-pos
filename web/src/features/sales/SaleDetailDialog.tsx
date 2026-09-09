import { useState } from 'react';
import { Box, Button, HStack, Input, Spinner, Text, VStack } from '@chakra-ui/react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { posApi } from '../../api/pos';
import type { SaleRow } from '../../api/sales';
import {
  DialogRoot, DialogBackdrop, DialogContent, DialogHeader, DialogBody,
  DialogTitle, DialogCloseTrigger,
} from '../../components/ui/dialog';
import { useUiStore } from '../../stores/ui';
import { LiquidacionSheet } from './LiquidacionSheet';
import { settlementsApi } from '../../api/settlements';
import { money } from '../../utils/format';
import { etiquetaEstado, etiquetaTipo } from './etiquetas';
import { fechaYHora } from '../../utils/horaDelNegocio';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';

// El detalle de una venta: sus renglones con modificadores y de dónde salió el dinero.
//
// Reusa `GET /orders/{id}`, que ya devuelve exactamente esto. Un endpoint propio solo haría falta
// el día que el detalle lleve costo o margen por renglón — eso sí es información de gestión y el
// endpoint de órdenes no exige rol.
//
// Los datos de cabecera (folio, estado, medio de pago) salen del renglón que ya se tiene en la
// tabla, así que el diálogo pinta algo útil desde el primer cuadro y no una pantalla en blanco
// mientras carga.
export function SaleDetailDialog({ venta, isOpen, onClose }: {
  venta: SaleRow;
  isOpen: boolean;
  onClose: () => void;
}) {
  const horaNegocio = useHoraDelNegocio();
  const palette = useUiStore((s) => s.palette);
  const qc = useQueryClient();
  const [folio, setFolio] = useState(venta.platformOrderRef);
  const [error, setError] = useState('');
  const [capturando, setCapturando] = useState(false);
  // La liquidación se consulta aparte del pedido: es otro momento y otro camino, y un pedido de
  // mostrador no la tiene. El 404 es la respuesta legítima de "todavía no llega el documento", así
  // que no se reintenta.
  const liquidacion = useQuery({
    queryKey: ['settlement', venta.id],
    queryFn: () => settlementsApi.get(venta.id),
    enabled: isOpen && !!venta.platform,
    retry: false,
  });
  const guardarFolio = useMutation({
    mutationFn: () => posApi.setPlatformRef(venta.id, folio.trim()),
    onSuccess: (r) => {
      setFolio(r.platformOrderRef);
      setError('');
      // Se invalida la LISTA y el RESUMEN: con el filtro de pendientes puesto, el pedido acaba de
      // salir del conjunto, y dejar la tabla con él dentro haría que el conteo de arriba y las
      // filas de abajo dejaran de cuadrar.
      void qc.invalidateQueries({ queryKey: ['sales'] });
    },
    onError: (e: Error) => setError(e.message),
  });
  const { data, isLoading } = useQuery({
    queryKey: ['order', venta.id],
    queryFn: () => posApi.order(venta.id),
    enabled: isOpen,
  });

  return (
    <DialogRoot open={isOpen} onOpenChange={(e) => { if (!e.open) onClose(); }} size="lg" scrollBehavior="inside">
      <DialogBackdrop />
      <DialogContent colorPalette={palette}>
        <DialogHeader>
          <DialogTitle>
            {venta.folioName ? `${venta.folioName} · #${venta.dailyNumber}` : `Venta #${venta.dailyNumber}`} · {money(venta.total)}
          </DialogTitle>
        </DialogHeader>
        <DialogCloseTrigger />
        <DialogBody>
          <VStack align="stretch" gap={4}>
            <VStack align="stretch" gap={1}>
              <Dato k="Estado" v={etiquetaEstado(venta.status)} />
              <Dato k="Tipo" v={venta.platform || etiquetaTipo(venta.serviceType)} />
              <Dato k="Abierta" v={fechaHora(venta.openedAt, horaNegocio.zona)} />
              {venta.completedAt && <Dato k="Cerrada" v={fechaHora(venta.completedAt, horaNegocio.zona)} />}
              {venta.openedBy && <Dato k="Atendió" v={venta.openedBy} />}
              {venta.customer && <Dato k="Cliente" v={venta.customer} />}
              <Dato k="Medio de pago" v={venta.methods || 'Sin cobrar'} />
              {/* El folio de la plataforma se ve COMPLETO aquí —en la tabla va truncado— y se
                  escribe o corrige desde aquí. Solo aparece en pedidos de plataforma: en uno de
                  mostrador el dato no existe. */}
              {venta.platform && (
                <HStack justify="space-between" align="center" gap={3} py={1}>
                  <Text fontSize="sm" color="fg.muted" flexShrink={0}>
                    Folio de {venta.platform}
                  </Text>
                  <HStack gap={2} flex="1" justify="flex-end">
                    <Input size="sm" minH="44px" maxW="260px"
                      aria-label={`Folio de ${venta.platform}`}
                      placeholder="Sin capturar"
                      value={folio}
                      onChange={(e) => { setFolio(e.target.value); setError(''); }}
                      autoComplete="off" autoCapitalize="off" spellCheck={false} />
                    <Button size="sm" minH="44px" px={4}
                      disabled={!folio.trim() || folio.trim() === venta.platformOrderRef || guardarFolio.isPending}
                      onClick={() => guardarFolio.mutate()}>
                      Guardar
                    </Button>
                  </HStack>
                </HStack>
              )}
              {error && <Text fontSize="sm" color="red.fg">{error}</Text>}
              {/* La liquidación: lo que la plataforma se quedó. NO se suma ni se resta de ninguna
                  cifra de arriba — es dinero que el negocio vendió y no recibió, y mezclarlo con el
                  total reescribiría lo que el POS cobró. */}
              {venta.platform && (
                <HStack justify="space-between" align="center" gap={3} py={1}>
                  <Text fontSize="sm" color="fg.muted" flexShrink={0}>Liquidación</Text>
                  <HStack gap={2} flex="1" justify="flex-end">
                    <Text fontSize="sm" fontWeight={liquidacion.data ? '700' : '400'}
                      color={liquidacion.data ? undefined : 'fg.muted'}>
                      {/* "Sin registrar" y no "$0": todavía no se sabe cuánto cobró la plataforma,
                          que es distinto de saber que no cobró nada. */}
                      {liquidacion.data
                        ? `Se quedó ${money(sumar(liquidacion.data.commissionAmount, liquidacion.data.withholdings))} · llegó ${money(liquidacion.data.netAmount)}`
                        : 'Sin registrar'}
                    </Text>
                    <Button size="sm" minH="44px" px={4} variant="outline"
                      onClick={() => setCapturando(true)}>
                      {liquidacion.data ? 'Corregir' : 'Registrar'}
                    </Button>
                  </HStack>
                </HStack>
              )}
              {Number(venta.tips) > 0 && <Dato k="Propina" v={money(venta.tips)} />}
              {Number(venta.deliveryFee) > 0 && <Dato k="Envío" v={money(venta.deliveryFee)} />}
              {Number(venta.refund) > 0 && <Dato k="Reembolsado" v={money(venta.refund)} />}
            </VStack>

            <Box borderTopWidth="1px" pt={3}>
              <Text fontWeight="700" mb={2}>Qué se vendió</Text>
              {isLoading && <HStack><Spinner size="sm" /><Text color="fg.muted">Cargando renglones…</Text></HStack>}
              {!isLoading && (data?.lines ?? []).length === 0 && (
                <Text color="fg.muted">Esta venta no tiene renglones.</Text>
              )}
              <VStack align="stretch" gap={2}>
                {(data?.lines ?? []).map((l, i) => (
                  <Box key={i}>
                    <HStack justify="space-between" align="baseline">
                      <Text fontWeight="600">{l.quantity}× {l.productName}</Text>
                      <Text fontWeight="600" whiteSpace="nowrap">{money(l.lineTotal)}</Text>
                    </HStack>
                    {(l.modifiers ?? []).map((m, j) => (
                      <HStack key={j} justify="space-between" pl={4}>
                        <Text fontSize="sm" color="fg.muted">
                          + {m.name}{m.quantity > 1 ? ` ×${m.quantity}` : ''}
                        </Text>
                        {Number(m.priceDelta) !== 0 && (
                          <Text fontSize="sm" color="fg.muted">{money(m.priceDelta)}</Text>
                        )}
                      </HStack>
                    ))}
                    {l.notes && <Text fontSize="sm" color="orange.fg" pl={4}>{l.notes}</Text>}
                  </Box>
                ))}
              </VStack>
            </Box>
          </VStack>
        </DialogBody>
        {venta.platform && (
          <LiquidacionSheet orderId={venta.id} plataforma={venta.platform}
            isOpen={capturando} onClose={() => setCapturando(false)} />
        )}
      </DialogContent>
    </DialogRoot>
  );
}

function Dato({ k, v }: { k: string; v: string }) {
  return (
    <HStack justify="space-between" align="baseline">
      <Text color="fg.muted" fontSize="sm">{k}</Text>
      <Text fontWeight="600" fontSize="sm" textAlign="end">{v}</Text>
    </HStack>
  );
}

// La zona llega como parámetro: esta es una función de módulo y el hook solo vive en un componente.
function fechaHora(iso: string, zona: string): string {
  return fechaYHora(iso, zona);
}

// sumar dos importes que vienen como string del servidor. Solo para PINTAR: ninguna cifra que se
// cobre o se guarde se calcula en el front.
function sumar(a: string, b: string): number {
  return (Number(a) || 0) + (Number(b) || 0);
}
