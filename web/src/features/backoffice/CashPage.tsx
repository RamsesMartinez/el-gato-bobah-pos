import { useState } from 'react';
import {
  Box, Heading, Text, Button, VStack, HStack, Table, Input, Textarea,
  Center, Spinner, Stat, Tabs, Badge, SimpleGrid,
} from '@chakra-ui/react';
import { LuArrowDownLeft, LuArrowUpRight } from 'react-icons/lu';
import { toaster } from '../../components/ui/toaster';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { backofficeApi, type CashSession } from '../../api/backoffice';
import { money } from '../../utils/format';
import { Page } from '../../components/Page';
import {
  DialogRoot, DialogBackdrop, DialogContent, DialogBody, DialogHeader, DialogTitle, DialogCloseTrigger,
} from '../../components/ui/dialog';

// Sobrante (>0) verde, faltante (<0) rojo, cuadrado gris.
function diffColor(v: string) {
  const n = parseFloat(v);
  if (n > 0.005) return 'green.500';
  if (n < -0.005) return 'red.500';
  return 'fg.muted';
}

export function CashPage() {
  return (
    <Page maxW="920px">
      <Heading size="lg" mb={4}>Caja</Heading>
      <Tabs.Root defaultValue="actual">
        <Tabs.List>
          <Tabs.Trigger value="actual">Caja actual</Tabs.Trigger>
          <Tabs.Trigger value="historico">Histórico</Tabs.Trigger>
        </Tabs.List>
        <Tabs.Content value="actual" px={0} pt={4}><CurrentTab /></Tabs.Content>
        <Tabs.Content value="historico" px={0} pt={4}><HistoryTab /></Tabs.Content>
      </Tabs.Root>
    </Page>
  );
}

// ---- Tab: caja actual (abrir / operar / cerrar) ----
function CurrentTab() {
  const qc = useQueryClient();
  const { data: session, isLoading } = useQuery({ queryKey: ['cash', 'current'], queryFn: backofficeApi.cashCurrent });
  const [opening, setOpening] = useState('');
  const [declared, setDeclared] = useState<Record<string, string>>({});
  const [notes, setNotes] = useState('');
  const [closed, setClosed] = useState<CashSession | null>(null); // resumen tras cerrar

  const openMut = useMutation({
    mutationFn: () => backofficeApi.cashOpen(parseFloat(opening) || 0),
    onSuccess: () => { setOpening(''); qc.invalidateQueries({ queryKey: ['cash'] }); },
    onError: (e) => toaster.create({ title: 'No se pudo abrir la caja', description: String(e), type: 'error' }),
  });
  const closeMut = useMutation({
    mutationFn: () => {
      const d: Record<string, number> = {};
      Object.entries(declared).forEach(([k, v]) => (d[k] = parseFloat(v) || 0));
      return backofficeApi.cashClose(d, notes || undefined);
    },
    onSuccess: (s) => {
      setClosed(s); setDeclared({}); setNotes('');
      qc.invalidateQueries({ queryKey: ['cash'] });
    },
    onError: (e) => toaster.create({ title: 'No se pudo cerrar la caja', description: String(e), type: 'error' }),
  });

  if (isLoading) return <Center h="40vh"><Spinner size="xl" /></Center>;

  return (
    <>
      {!session ? (
        <VStack align="stretch" gap={4} bg="bg.panel" p={6} borderRadius="lg" borderWidth="1px" maxW="420px">
          <Text fontWeight="600">No hay caja abierta.</Text>
          <Text fontSize="sm" color="fg.muted">Captura el fondo inicial (efectivo con el que arranca el cajón).</Text>
          <HStack>
            <Input placeholder="Fondo inicial" type="number" inputMode="decimal" value={opening}
              onChange={(e) => setOpening(e.target.value)} />
            <Button onClick={() => openMut.mutate()} loading={openMut.isPending}>Abrir caja</Button>
          </HStack>
        </VStack>
      ) : (
        <VStack align="stretch" gap={5}>
          <SimpleGrid columns={{ base: 1, sm: 3 }} gap={3}>
            <Stat.Root bg="bg.panel" p={4} borderRadius="lg" borderWidth="1px">
              <Stat.Label>Fondo inicial</Stat.Label>
              <Stat.ValueText>{money(session.openingCash, session.currency)}</Stat.ValueText>
            </Stat.Root>
            <Stat.Root bg="bg.panel" p={4} borderRadius="lg" borderWidth="1px">
              <Stat.Label>Movimientos (neto)</Stat.Label>
              <Stat.ValueText fontSize="lg">{money(session.netMovements ?? '0', session.currency)}</Stat.ValueText>
            </Stat.Root>
            <Stat.Root bg="bg.panel" p={4} borderRadius="lg" borderWidth="1px">
              <Stat.Label>Abierta desde</Stat.Label>
              <Stat.ValueText fontSize="sm">{new Date(session.openedAt).toLocaleString('es-MX')}</Stat.ValueText>
            </Stat.Root>
          </SimpleGrid>

          <MovementsPanel session={session} />

          <Box>
            <Text fontWeight="700" mb={2}>Cierre — declarado por método</Text>
            <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" overflow="hidden">
              <Table.Root size="sm">
                <Table.Header><Table.Row>
                  <Table.ColumnHeader>Método</Table.ColumnHeader>
                  <Table.ColumnHeader textAlign="end">Esperado</Table.ColumnHeader>
                  <Table.ColumnHeader>Declarado</Table.ColumnHeader>
                </Table.Row></Table.Header>
                <Table.Body>
                  {(session.totals ?? []).map((t) => (
                    <Table.Row key={t.methodId}>
                      <Table.Cell>{t.name}</Table.Cell>
                      <Table.Cell textAlign="end">{money(t.expected, session.currency)}</Table.Cell>
                      <Table.Cell>
                        {t.autoDeclare ? (
                          <Text fontSize="sm" color="fg.muted">Automático</Text>
                        ) : (
                          <Input size="sm" w="130px" type="number" inputMode="decimal" placeholder="0"
                            value={declared[t.methodId] ?? ''}
                            onChange={(e) => setDeclared({ ...declared, [t.methodId]: e.target.value })} />
                        )}
                      </Table.Cell>
                    </Table.Row>
                  ))}
                </Table.Body>
              </Table.Root>
            </Box>
          </Box>

          <Textarea rows={2} resize="none" placeholder="Notas del cierre (opcional)"
            value={notes} onChange={(e) => setNotes(e.target.value)} />

          <Button colorPalette="red" size="lg" loading={closeMut.isPending}
            onClick={() => { if (confirm('¿Cerrar la caja? No podrás modificarla después.')) closeMut.mutate(); }}>
            Cerrar caja
          </Button>
        </VStack>
      )}

      {/* Resumen tras cerrar: esperado vs declarado vs diferencia */}
      <DialogRoot open={closed !== null} onOpenChange={(e) => { if (!e.open) setClosed(null); }} placement="center" size="md">
        <DialogBackdrop />
        <DialogContent>
          <DialogHeader><DialogTitle>Caja cerrada</DialogTitle></DialogHeader>
          <DialogCloseTrigger />
          <DialogBody pb={6}>
            <Table.Root size="sm">
              <Table.Header><Table.Row>
                <Table.ColumnHeader>Método</Table.ColumnHeader>
                <Table.ColumnHeader textAlign="end">Esperado</Table.ColumnHeader>
                <Table.ColumnHeader textAlign="end">Declarado</Table.ColumnHeader>
                <Table.ColumnHeader textAlign="end">Diferencia</Table.ColumnHeader>
              </Table.Row></Table.Header>
              <Table.Body>
                {(closed?.totals ?? []).map((t) => (
                  <Table.Row key={t.methodId}>
                    <Table.Cell>{t.name}</Table.Cell>
                    <Table.Cell textAlign="end">{money(t.expected, closed!.currency)}</Table.Cell>
                    <Table.Cell textAlign="end">{money(t.declared, closed!.currency)}</Table.Cell>
                    <Table.Cell textAlign="end" color={diffColor(t.difference)} fontWeight="600">
                      {money(t.difference, closed!.currency)}
                    </Table.Cell>
                  </Table.Row>
                ))}
              </Table.Body>
            </Table.Root>
          </DialogBody>
        </DialogContent>
      </DialogRoot>
    </>
  );
}

// ---- Movimientos de efectivo (entrada/salida) de la sesión abierta ----
function MovementsPanel({ session }: { session: CashSession }) {
  const qc = useQueryClient();
  const [kind, setKind] = useState<'entrada' | 'salida'>('salida');
  const [amount, setAmount] = useState('');
  const [concept, setConcept] = useState('');

  const mut = useMutation({
    mutationFn: () => backofficeApi.cashMovement(kind, parseFloat(amount) || 0, concept.trim()),
    onSuccess: () => { setAmount(''); setConcept(''); qc.invalidateQueries({ queryKey: ['cash'] }); },
    onError: (e) => toaster.create({ title: 'No se pudo registrar', description: String(e), type: 'error' }),
  });
  const canAdd = (parseFloat(amount) || 0) > 0 && concept.trim().length > 0;
  // Go serializa un slice vacío como null; sin esta guarda, `.length`/`.map` revienta el render.
  const movements = session.movements ?? [];

  return (
    <Box>
      <Text fontWeight="700" mb={2}>Movimientos de efectivo</Text>
      <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" p={3}>
        <HStack gap={2} mb={movements.length ? 3 : 0} flexWrap="wrap">
          <Button size="sm" minH="44px" variant={kind === 'entrada' ? 'solid' : 'outline'}
            colorPalette={kind === 'entrada' ? 'green' : 'gray'} onClick={() => setKind('entrada')}>
            <LuArrowDownLeft /> Entrada
          </Button>
          <Button size="sm" minH="44px" variant={kind === 'salida' ? 'solid' : 'outline'}
            colorPalette={kind === 'salida' ? 'red' : 'gray'} onClick={() => setKind('salida')}>
            <LuArrowUpRight /> Salida
          </Button>
          <Input size="sm" w="120px" type="number" inputMode="decimal" placeholder="Monto"
            value={amount} onChange={(e) => setAmount(e.target.value)} />
          <Input size="sm" flex="1" minW="140px" placeholder="Concepto (ej. pago proveedor)"
            value={concept} onChange={(e) => setConcept(e.target.value)} />
          <Button size="sm" minH="44px" disabled={!canAdd} loading={mut.isPending} onClick={() => mut.mutate()}>
            Registrar
          </Button>
        </HStack>
        {movements.length > 0 && (
          <VStack align="stretch" gap={1}>
            {movements.map((m) => (
              <HStack key={m.id} justify="space-between" fontSize="sm" py={1} borderTopWidth="1px" borderColor="border.muted">
                <HStack gap={2} minW={0}>
                  <Badge colorPalette={m.kind === 'entrada' ? 'green' : 'red'}>
                    {m.kind === 'entrada' ? 'Entrada' : 'Salida'}
                  </Badge>
                  <Text truncate>{m.concept}</Text>
                  <Text color="fg.subtle" flexShrink={0}>· {m.userName}</Text>
                </HStack>
                <Text fontWeight="600" flexShrink={0} color={m.kind === 'entrada' ? 'green.500' : 'red.500'}>
                  {m.kind === 'entrada' ? '+' : '−'}{money(m.amount, session.currency)}
                </Text>
              </HStack>
            ))}
          </VStack>
        )}
      </Box>
    </Box>
  );
}

// ---- Tab: histórico de cortes ----
function HistoryTab() {
  const { data, isLoading } = useQuery({ queryKey: ['cash', 'history'], queryFn: backofficeApi.cashHistory });
  const [detailId, setDetailId] = useState<number | null>(null);

  if (isLoading) return <Center h="40vh"><Spinner size="xl" /></Center>;
  const rows = data?.items ?? [];
  if (rows.length === 0) return <Text color="fg.muted">Aún no hay cortes registrados.</Text>;

  return (
    <>
      <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" overflowX="auto">
        <Table.Root size="sm">
          <Table.Header><Table.Row>
            <Table.ColumnHeader>Abierta</Table.ColumnHeader>
            <Table.ColumnHeader>Estado</Table.ColumnHeader>
            <Table.ColumnHeader>Abrió / Cerró</Table.ColumnHeader>
            <Table.ColumnHeader textAlign="end">Fondo</Table.ColumnHeader>
            <Table.ColumnHeader textAlign="end">Diferencia</Table.ColumnHeader>
            <Table.ColumnHeader></Table.ColumnHeader>
          </Table.Row></Table.Header>
          <Table.Body>
            {rows.map((r) => (
              <Table.Row key={r.id}>
                <Table.Cell>{new Date(r.openedAt).toLocaleString('es-MX')}</Table.Cell>
                <Table.Cell>
                  <Badge colorPalette={r.status === 'abierta' ? 'green' : 'gray'}>
                    {r.status === 'abierta' ? 'Abierta' : 'Cerrada'}
                  </Badge>
                </Table.Cell>
                <Table.Cell fontSize="sm">{r.openedByName}{r.closedByName ? ` → ${r.closedByName}` : ''}</Table.Cell>
                <Table.Cell textAlign="end">{money(r.openingCash, r.currency)}</Table.Cell>
                <Table.Cell textAlign="end" color={diffColor(r.totalDifference)} fontWeight="600">
                  {r.status === 'cerrada' ? money(r.totalDifference, r.currency) : '—'}
                </Table.Cell>
                <Table.Cell><Button size="xs" variant="outline" onClick={() => setDetailId(r.id)}>Ver</Button></Table.Cell>
              </Table.Row>
            ))}
          </Table.Body>
        </Table.Root>
      </Box>
      <SessionDetailDialog id={detailId} onClose={() => setDetailId(null)} />
    </>
  );
}

function SessionDetailDialog({ id, onClose }: { id: number | null; onClose: () => void }) {
  const { data, isLoading } = useQuery({
    queryKey: ['cash', 'session', id],
    queryFn: () => backofficeApi.cashSession(id!),
    enabled: id !== null,
  });
  return (
    <DialogRoot open={id !== null} onOpenChange={(e) => { if (!e.open) onClose(); }} placement="center" size="lg" scrollBehavior="inside">
      <DialogBackdrop />
      <DialogContent>
        <DialogHeader><DialogTitle>Corte #{id}</DialogTitle></DialogHeader>
        <DialogCloseTrigger />
        <DialogBody pb={6}>
          {isLoading || !data ? (
            <Center py={8}><Spinner /></Center>
          ) : (
            <VStack align="stretch" gap={4}>
              <SimpleGrid columns={2} gap={2} fontSize="sm">
                <Text color="fg.muted">Fondo inicial</Text><Text textAlign="end">{money(data.openingCash, data.currency)}</Text>
                <Text color="fg.muted">Abrió</Text><Text textAlign="end">{data.openedByName} · {new Date(data.openedAt).toLocaleString('es-MX')}</Text>
                {data.closedAt && (<>
                  <Text color="fg.muted">Cerró</Text>
                  <Text textAlign="end">{data.closedByName ?? '—'} · {new Date(data.closedAt).toLocaleString('es-MX')}</Text>
                </>)}
              </SimpleGrid>

              {(data.totals ?? []).length > 0 && (
                <Table.Root size="sm">
                  <Table.Header><Table.Row>
                    <Table.ColumnHeader>Método</Table.ColumnHeader>
                    <Table.ColumnHeader textAlign="end">Esperado</Table.ColumnHeader>
                    <Table.ColumnHeader textAlign="end">Declarado</Table.ColumnHeader>
                    <Table.ColumnHeader textAlign="end">Dif.</Table.ColumnHeader>
                  </Table.Row></Table.Header>
                  <Table.Body>
                    {(data.totals ?? []).map((t) => (
                      <Table.Row key={t.methodId}>
                        <Table.Cell>{t.name}</Table.Cell>
                        <Table.Cell textAlign="end">{money(t.expected, data.currency)}</Table.Cell>
                        <Table.Cell textAlign="end">{money(t.declared, data.currency)}</Table.Cell>
                        <Table.Cell textAlign="end" color={diffColor(t.difference)} fontWeight="600">{money(t.difference, data.currency)}</Table.Cell>
                      </Table.Row>
                    ))}
                  </Table.Body>
                </Table.Root>
              )}

              {(data.movements ?? []).length > 0 && (
                <Box>
                  <Text fontWeight="700" mb={1}>Movimientos</Text>
                  <VStack align="stretch" gap={1}>
                    {(data.movements ?? []).map((m) => (
                      <HStack key={m.id} justify="space-between" fontSize="sm">
                        <HStack gap={2} minW={0}>
                          <Badge colorPalette={m.kind === 'entrada' ? 'green' : 'red'}>{m.kind === 'entrada' ? 'Entrada' : 'Salida'}</Badge>
                          <Text truncate>{m.concept}</Text>
                        </HStack>
                        <Text color={m.kind === 'entrada' ? 'green.500' : 'red.500'} flexShrink={0}>
                          {m.kind === 'entrada' ? '+' : '−'}{money(m.amount, data.currency)}
                        </Text>
                      </HStack>
                    ))}
                  </VStack>
                </Box>
              )}

              {data.notes && (
                <Box><Text fontWeight="700" fontSize="sm">Notas</Text><Text fontSize="sm" color="fg.muted">{data.notes}</Text></Box>
              )}
            </VStack>
          )}
        </DialogBody>
      </DialogContent>
    </DialogRoot>
  );
}
