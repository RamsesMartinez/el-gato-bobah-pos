import { Fragment, useRef, useState } from 'react';
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { Box, Button, Center, HStack, Spinner, Table, Text } from '@chakra-ui/react';

import { PreviewGroup as Group, PreviewTable, StaleNotice, num as n, signed, tone } from '../../shared/preview/PreviewTable';

import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
} from '../../components/ui/drawer';
import type { EditDecision, EditPreview, EditResult } from '../../api/backoffice';
import { ApiError } from '../../api/client';
import { mensajeDeError } from '../../api/mensajes';
import { money } from '../../utils/format';
import { formatBaseQty } from '../../shared/units';
import { dia } from './expenseEdit';

// Vista previa de una edición o una entrega (spec 053, maqueta «Vista previa», opción B: tabla
// antes/después). La calcula el servidor con la misma transacción que confirma, revertida; aquí solo
// se pinta. Cada decisión va en una fila justo debajo de la que afecta, y «Confirmar» espera a todas.

const OPTION_LABEL: Record<string, string> = {
  adjust: 'Ajustar el inventario',
  keep: 'No mover',
  pay: 'Pagar al confirmar',
  leave: 'Dejar pendiente',
  pending: 'Sigue pendiente de entrega',
  'shortfall:adjust': 'Ajustar el importe a lo recibido',
};

// Una opción o un campo que el servidor agregue después sale con un texto genérico, nunca con su
// clave interna.
const optionLabel = (d: EditDecision, o: string) => (d.kind === 'shortfall' && o === 'adjust' ? OPTION_LABEL['shortfall:adjust'] : OPTION_LABEL[o] ?? 'Otra opción');

const FIELD_LABEL: Record<string, string> = {
  supplier: 'Proveedor', category: 'Categoría', description: 'Comentario', expenseDate: 'Fecha del gasto',
};


export function EditPreviewSheet({ request, onClose, onDone, title = 'Antes de confirmar' }: {
  // request pide la vista previa (preview=true) o confirma con la huella de la última que se vio.
  request: (preview: boolean, decisions: Record<string, string>, fingerprint?: string) => Promise<EditResult>;
  onClose: () => void;
  onDone: (result: EditResult, decisions: Record<string, string>) => void;
  title?: string;
}) {
  const [decisions, setDecisions] = useState<Record<string, string>>({});
  // nonce vuelve a pedir la vista previa con las mismas decisiones (algo cambió en el servidor).
  const [nonce, setNonce] = useState(0);
  const [stale, setStale] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState('');
  const rows = useRef<Record<string, HTMLTableRowElement | null>>({});

  // Cada decisión vuelve a pedir la vista previa: «No mover» cambia el «después» que se ve. Se
  // muestra la anterior mientras llega la nueva.
  const q = useQuery({
    queryKey: ['edit-preview', nonce, decisions],
    queryFn: () => request(true, decisions),
    placeholderData: keepPreviousData, staleTime: 0, gcTime: 0, retry: false, refetchOnWindowFocus: false,
  });
  const preview: EditPreview | null = q.data?.preview ?? null;
  const loading = q.isFetching;
  const error = saveError || (q.error ? mensajeDeError(q.error) : '');

  const pending = (preview?.decisions ?? []).filter((d) => d.chosen === null);
  const choose = (d: EditDecision, o: string) => { setStale(false); setSaveError(''); setDecisions((p) => ({ ...p, [d.key]: o })); };

  const confirmar = async () => {
    if (!preview) return;
    setSaving(true);
    try {
      const r = await request(false, decisions, preview.fingerprint);
      onDone(r, decisions);
    } catch (e) {
      if (e instanceof ApiError && e.code === 'EXPENSE_EDIT_STALE') {
        setStale(true);
        setNonce((v) => v + 1); // vuelve a pedir la vista previa con lo de ahora
      } else {
        setSaveError(mensajeDeError(e));
      }
    } finally {
      setSaving(false);
    }
  };

  const goToPending = () => {
    const first = pending[0];
    if (first) rows.current[first.key]?.scrollIntoView({ behavior: 'smooth', block: 'center' });
  };

  const decisionRow = (d: EditDecision | undefined, question: string) => d && (
    <Table.Row key={`dec-${d.key}`} ref={(el) => { rows.current[d.key] = el; }} bg="orange.subtle" data-testid="fila-decision">
      <Table.Cell colSpan={4} py={2}>
        <HStack gap={3} flexWrap="wrap">
          <Text flex="1" minW="220px" fontSize="sm" fontWeight="600">⚠ {question}</Text>
          {d.options.map((o) => (
            <Button key={o} minH="44px" size="sm" aria-pressed={d.chosen === o}
              variant={d.chosen === o ? 'subtle' : 'outline'} colorPalette={d.chosen === o ? 'brand' : 'gray'}
              onClick={() => choose(d, o)}>
              {optionLabel(d, o)}
            </Button>
          ))}
        </HStack>
      </Table.Cell>
    </Table.Row>
  );

  const decisionOf = (key: string | null) => (key ? preview?.decisions.find((d) => d.key === key) : undefined);
  const moneyChanged = !!preview && (n(preview.money.amountBefore) !== n(preview.money.amountAfter) || n(preview.money.due) > 0 || n(preview.money.overpaid) > 0);

  return (
    <DrawerRoot open placement="bottom" onOpenChange={(e) => { if (!e.open) onClose(); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="2xl" maxH="90dvh">
        <DrawerHeader><Text fontWeight="700" fontSize="lg">{title}</Text></DrawerHeader>
        <DrawerBody>
          {stale && <StaleNotice />}
          {error && <Text p={3} mb={3} borderRadius="lg" bg="red.subtle" fontSize="sm" role="alert">{error}</Text>}
          {!preview ? (
            <Center py={10}><Spinner size="lg" /></Center>
          ) : (
            <Box opacity={loading ? 0.6 : 1}>
              <PreviewTable>
                  {preview.header.length > 0 && <Group label="Datos del gasto" />}
                  {preview.header.map((h) => (
                    <Table.Row key={h.field}>
                      <Table.Cell>{FIELD_LABEL[h.field] ?? 'Otro dato'}</Table.Cell>
                      <Table.Cell textAlign="end">{h.field === 'expenseDate' && h.before ? dia(h.before) : h.before ?? '—'}</Table.Cell>
                      <Table.Cell textAlign="end" fontWeight="600">{h.field === 'expenseDate' && h.after ? dia(h.after) : h.after ?? '—'}</Table.Cell>
                      <Table.Cell />
                    </Table.Row>
                  ))}

                  {preview.lines.length > 0 && <Group label="Renglones" />}
                  {preview.lines.map((l, i) => {
                    const short = l.kind === 'received' ? decisionOf(`shortfall:${l.itemId}`) : undefined;
                    return (
                      <Fragment key={`l-${i}`}>
                        {decisionRow(short, `Llegó menos de lo pedido de ${l.name}.`)}
                        <Table.Row>
                          <Table.Cell colSpan={3}>{lineText(l)}</Table.Cell>
                          <Table.Cell textAlign="end" color={l.kind === 'removed' ? 'fg.error' : l.kind === 'added' ? 'fg.success' : undefined}>
                            {l.kind === 'removed' && l.amount ? `−${money(l.amount)}` : l.kind === 'added' && l.amount ? `+${money(l.amount)}`
                              : l.kind === 'adjusted' && l.amountBefore && l.amountAfter ? `−${money(n(l.amountBefore) - n(l.amountAfter))}` : ''}
                          </Table.Cell>
                        </Table.Row>
                      </Fragment>
                    );
                  })}

                  {preview.inventory.length > 0 && <Group label="Inventario" />}
                  {[...preview.inventory].sort((a, b) => Number(!!b.decisionKey) - Number(!!a.decisionKey)).map((i) => {
                    const d = decisionOf(i.decisionKey);
                    const diff = n(i.after) - n(i.before);
                    return (
                      <Fragment key={`i-${i.ingredientId ?? 'p' + i.productId}`}>
                        {decisionRow(d, `El conteo del ${d?.countedOn ? dia(d.countedOn) : ''} ya contó este artículo. ¿Mover el inventario?`)}
                        <Table.Row>
                          <Table.Cell>{i.name}{i.negativeAfter ? ' · ⚠ quedará en negativo' : ''}</Table.Cell>
                          <Table.Cell textAlign="end">{formatBaseQty(i.before, i.baseUnit)}</Table.Cell>
                          <Table.Cell textAlign="end" fontWeight="600" color={i.negativeAfter ? 'fg.warning' : undefined}>{formatBaseQty(i.after, i.baseUnit)}</Table.Cell>
                          <Table.Cell textAlign="end" color={tone(diff)}>{signed(diff, (v) => formatBaseQty(v, i.baseUnit))}</Table.Cell>
                        </Table.Row>
                      </Fragment>
                    );
                  })}

                  {moneyChanged && <Group label="Dinero" />}
                  {moneyChanged && (
                    <>
                      {decisionRow(decisionOf(preview.money.decisionKey), `Falta pagar ${money(preview.money.due)}. Puedes pagarlo en cuanto confirmes.`)}
                      <Table.Row>
                        <Table.Cell>Importe</Table.Cell>
                        <Table.Cell textAlign="end">{money(preview.money.amountBefore)}</Table.Cell>
                        <Table.Cell textAlign="end" fontWeight="600">{money(preview.money.amountAfter)}</Table.Cell>
                        <Table.Cell textAlign="end" color={n(preview.money.due) > 0 ? 'fg.warning' : tone(n(preview.money.amountAfter) - n(preview.money.amountBefore))}>
                          {signed(n(preview.money.amountAfter) - n(preview.money.amountBefore), (v) => money(v))}
                        </Table.Cell>
                      </Table.Row>
                      <Table.Row>
                        <Table.Cell>Pagado</Table.Cell>
                        <Table.Cell textAlign="end">{money(preview.money.paid)}</Table.Cell>
                        <Table.Cell textAlign="end">{money(preview.money.paid)}</Table.Cell>
                        <Table.Cell textAlign="end" color="fg.muted">sin cambio</Table.Cell>
                      </Table.Row>
                      {n(preview.money.overpaid) > 0 && (
                        <Table.Row><Table.Cell colSpan={4} color="fg.warning" fontWeight="600">⚠ Pagado de más {money(preview.money.overpaid)}</Table.Cell></Table.Row>
                      )}
                    </>
                  )}

                  {preview.costs.length > 0 && <Group label="Costos" />}
                  {preview.costs.map((c) => {
                    const diff = n(c.after) - n(c.before);
                    const unit = preview.inventory.find((i) => i.ingredientId === c.ingredientId)?.baseUnit;
                    // El costo sale de la compra: ni bajar ni subir es un error, así que va sin rojo ni verde.
                    return (
                      <Table.Row key={`c-${c.ingredientId}`}>
                        <Table.Cell>Costo de {c.name}{unit ? ` por ${formatBaseQty(1, unit).replace(/^1 /, '')}` : ''}</Table.Cell>
                        <Table.Cell textAlign="end">{money(c.before)}</Table.Cell>
                        <Table.Cell textAlign="end" fontWeight="600">{money(c.after)}</Table.Cell>
                        <Table.Cell textAlign="end" color="fg.muted">{diff === 0 ? 'se queda igual' : signed(diff, (v) => money(v))}</Table.Cell>
                      </Table.Row>
                    );
                  })}
              </PreviewTable>
            </Box>
          )}
        </DrawerBody>
        <DrawerFooter>
          <HStack w="100%" gap={3}>
            {pending.length > 0 ? (
              <Button variant="ghost" minH="44px" colorPalette="orange" onClick={goToPending}>
                ⚠ Falta decidir {pending.length}
              </Button>
            ) : (
              <Text fontSize="sm" color="fg.muted">{preview ? 'Listo para confirmar' : ''}</Text>
            )}
            <Box flex="1" />
            <Button minH="44px" variant="outline" onClick={onClose}>Volver</Button>
            <Button minH="44px" colorPalette="brand" disabled={!preview || loading || pending.length > 0} loading={saving}
              onClick={() => { void confirmar(); }}>
              Confirmar
            </Button>
          </HStack>
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}

function lineText(l: EditPreview['lines'][number]): string {
  switch (l.kind) {
    case 'added': return `Agrega ${l.name}${l.qty ? ` · ${Number(l.qty)}` : ''}${l.on ? ` · llegó el ${dia(l.on)}` : ' · por recibir'}`;
    case 'removed': return `Quita ${l.name}`;
    case 'received': return `Llegó ${l.name} · ${Number(l.qty)}${l.on ? ` el ${dia(l.on)}` : ''}${l.missingAfter ? ` · faltan ${Number(l.missingAfter)}` : ''}`;
    case 'adjusted': return `${l.name}: el importe baja a lo recibido`;
  }
}
