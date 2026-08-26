import { useState } from 'react';
import {
  Box, Button, HStack, VStack, Text, Input, Table, Badge, Center, Spinner, Separator,
} from '@chakra-ui/react';
import { LuPackageCheck } from 'react-icons/lu';
import { useQuery, useMutation } from '@tanstack/react-query';

import { toaster } from '../../components/ui/toaster';
import {
  DialogRoot, DialogBackdrop, DialogContent, DialogBody, DialogHeader, DialogTitle, DialogCloseTrigger,
} from '../../components/ui/dialog';
import { money } from '../../utils/format';
import { backofficeApi, type ExpenseItem } from '../../api/backoffice';

// Detalle del gasto: mercancía, pagos y la RECEPCIÓN.
//
// Recibir es una acción aparte del alta porque una cosa es la fecha en que se pide y otra
// cuándo entra al almacén. Al recibir se confirma cuánto llegó de cada renglón: 0 = no llegó
// (el "No disponible" de un pedido), distinto de lo pedido = entrega parcial o peso ajustado.
// Solo entonces se generan los movimientos de inventario.

const today = () => new Date().toISOString().slice(0, 10);

export function ExpenseDetailDialog({ id, onClose, onChanged }: {
  id: number | null; onClose: () => void; onChanged: () => void;
}) {
  const { data, isLoading, refetch } = useQuery({
    queryKey: ['expense', id],
    queryFn: () => backofficeApi.expenseDetail(id!),
    enabled: id !== null,
  });

  // edits guarda SOLO lo que el operador cambió; el valor efectivo se deriva en render
  // (lo pedido es el default, porque el caso normal es que llegue todo). Copiar los defaults a
  // estado en un efecto provocaría renders en cascada y desincronización al recargar el detalle.
  const [edits, setEdits] = useState<Record<number, string>>({});
  const [receivedAt, setReceivedAt] = useState(today());
  const qtyFor = (i: ExpenseItem) => edits[i.id] ?? i.qtyReceived ?? i.quantity;

  const receive = useMutation({
    mutationFn: () => backofficeApi.receiveExpense(id!, {
      receivedAt,
      received: Object.fromEntries((data?.items ?? []).map((i) => [String(i.id), qtyFor(i) || '0'])),
    }),
    onSuccess: async () => {
      toaster.create({ title: 'Mercancía recibida', description: 'El almacén ya refleja la compra.', type: 'success' });
      await refetch();
      onChanged();
    },
    onError: (e) => toaster.create({ title: 'No se pudo recibir', description: String(e), type: 'error' }),
  });

  const inventoriable = (data?.items ?? []).filter((i) => i.itemType !== null);
  const canReceive = !!data && data.receivedAt === null && data.items.length > 0;

  return (
    <DialogRoot open={id !== null} onOpenChange={(e) => { if (!e.open) onClose(); }}
      placement="center" size="lg" scrollBehavior="inside">
      <DialogBackdrop />
      <DialogContent>
        <DialogHeader><DialogTitle>Gasto {id}</DialogTitle></DialogHeader>
        <DialogCloseTrigger />
        <DialogBody pb={6}>
          {isLoading || !data ? (
            <Center py={16}><Spinner size="xl" /></Center>
          ) : (
            <VStack align="stretch" gap={4}>
              <HStack flexWrap="wrap" gap={3}>
                <Badge colorPalette={data.status === 'pagada' ? 'green' : data.status === 'cancelada' ? 'gray' : 'orange'}>
                  {data.status}
                </Badge>
                {data.docKind && <Badge variant="outline">{data.docKind}{data.docFolio ? ` ${data.docFolio}` : ''}</Badge>}
                {data.receivedAt
                  ? <Badge colorPalette="green">recibida {data.receivedAt}</Badge>
                  : data.items.length > 0 && <Badge colorPalette="orange">sin recibir</Badge>}
              </HStack>
              <Text fontSize="sm" color="fg.muted">
                {data.category} · {data.supplier ?? 'sin proveedor'} · documento {data.expenseDate}
                {data.description ? ` · ${data.description}` : ''}
              </Text>
              <HStack gap={4} flexWrap="wrap">
                <Stat label="Importe" value={money(data.amount, data.currency)} />
                <Stat label="Pagado" value={money(data.paid, data.currency)}
                  tone={Number(data.paid) >= Number(data.amount) ? 'green' : 'orange'} />
              </HStack>

              {data.items.length > 0 && (
                <>
                  <Separator />
                  <Text fontWeight="700">Mercancía</Text>
                  <Box overflowX="auto">
                    <Table.Root size="sm">
                      <Table.Header><Table.Row>
                        <Table.ColumnHeader>Artículo</Table.ColumnHeader>
                        <Table.ColumnHeader textAlign="end">Pedido</Table.ColumnHeader>
                        <Table.ColumnHeader textAlign="end">Importe</Table.ColumnHeader>
                        <Table.ColumnHeader w="120px">{data.receivedAt ? 'Recibido' : 'Llegó'}</Table.ColumnHeader>
                      </Table.Row></Table.Header>
                      <Table.Body>
                        {data.items.map((i) => (
                          <Table.Row key={i.id}>
                            <Table.Cell>
                              <ItemLabel item={i} />
                            </Table.Cell>
                            <Table.Cell textAlign="end" whiteSpace="nowrap">
                              {i.quantity} {i.unitCode ?? ''}
                            </Table.Cell>
                            <Table.Cell textAlign="end" whiteSpace="nowrap">{money(i.amount, data.currency)}</Table.Cell>
                            <Table.Cell>
                              {data.receivedAt ? (
                                <Text color={i.qtyReceived === '0' ? 'orange.600' : undefined}>
                                  {i.qtyReceived ?? '—'}
                                </Text>
                              ) : (
                                <Input size="sm" type="number" inputMode="decimal"
                                  value={qtyFor(i)}
                                  onChange={(e) => setEdits((p) => ({ ...p, [i.id]: e.target.value }))} />
                              )}
                            </Table.Cell>
                          </Table.Row>
                        ))}
                      </Table.Body>
                    </Table.Root>
                  </Box>
                  {canReceive && (
                    <HStack flexWrap="wrap" gap={3} justify="space-between">
                      <Box>
                        <Text fontSize="xs" color="fg.muted" mb={1}>Fecha de recepción</Text>
                        <Input type="date" w="170px" value={receivedAt}
                          onChange={(e) => setReceivedAt(e.target.value)} />
                      </Box>
                      <Button colorPalette="green" loading={receive.isPending} onClick={() => receive.mutate()}>
                        <LuPackageCheck /> Recibir mercancía
                        {inventoriable.length > 0 && ` (${inventoriable.length} al almacén)`}
                      </Button>
                    </HStack>
                  )}
                  {!canReceive && data.receivedAt && (
                    <Text fontSize="xs" color="fg.muted">
                      Ya recibida: las cantidades no se editan porque el almacén ya se movió. Una
                      corrección va como ajuste de inventario.
                    </Text>
                  )}
                </>
              )}

              {data.payments.length > 0 && (
                <>
                  <Separator />
                  <Text fontWeight="700">Pagos</Text>
                  <VStack align="stretch" gap={1}>
                    {data.payments.map((p) => (
                      <HStack key={p.id} justify="space-between" py={1}>
                        <HStack gap={2}>
                          <Text>{p.method}</Text>
                          <Text fontSize="xs" color="fg.subtle">{p.paidOn}</Text>
                          {p.inCashCount && <Badge size="sm" colorPalette="blue">en arqueo</Badge>}
                        </HStack>
                        <Text fontWeight="600">{money(p.amount, data.currency)}</Text>
                      </HStack>
                    ))}
                  </VStack>
                </>
              )}
            </VStack>
          )}
        </DialogBody>
      </DialogContent>
    </DialogRoot>
  );
}

// ItemLabel distingue una línea de inventario (que toca el almacén) de una que no, porque es la
// diferencia entre "esto descuenta stock" y "esto solo suma al gasto".
function ItemLabel({ item }: { item: ExpenseItem }) {
  return (
    <VStack align="start" gap={0}>
      <Text>{item.itemName ?? item.description}</Text>
      {item.itemName && item.itemName !== item.description && (
        <Text fontSize="xs" color="fg.subtle">{item.description}</Text>
      )}
      {item.itemType === null && <Text fontSize="xs" color="fg.subtle">no inventariable</Text>}
      {item.packQtyInBase && (
        <Text fontSize="xs" color="fg.subtle">contenido {item.packQtyInBase} por unidad</Text>
      )}
    </VStack>
  );
}

function Stat({ label, value, tone }: { label: string; value: string; tone?: string }) {
  return (
    <Box>
      <Text fontSize="xs" color="fg.muted">{label}</Text>
      <Text fontSize="lg" fontWeight="700" color={tone ? `${tone}.600` : undefined}>{value}</Text>
    </Box>
  );
}
