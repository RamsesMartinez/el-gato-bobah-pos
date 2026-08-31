import { Box, HStack, Spinner, Text, VStack } from '@chakra-ui/react';
import { useQuery } from '@tanstack/react-query';

import { posApi } from '../../api/pos';
import type { SaleRow } from '../../api/sales';
import {
  DialogRoot, DialogBackdrop, DialogContent, DialogHeader, DialogBody,
  DialogTitle, DialogCloseTrigger,
} from '../../components/ui/dialog';
import { useUiStore } from '../../stores/ui';
import { money } from '../../utils/format';
import { etiquetaEstado, etiquetaTipo } from './etiquetas';

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
  const palette = useUiStore((s) => s.palette);
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
              <Dato k="Abierta" v={fechaHora(venta.openedAt)} />
              {venta.completedAt && <Dato k="Cerrada" v={fechaHora(venta.completedAt)} />}
              {venta.openedBy && <Dato k="Atendió" v={venta.openedBy} />}
              {venta.customer && <Dato k="Cliente" v={venta.customer} />}
              <Dato k="Medio de pago" v={venta.methods || 'Sin cobrar'} />
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

function fechaHora(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleString('es-MX', { dateStyle: 'short', timeStyle: 'short' });
}
