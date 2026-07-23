import { useState, type ReactNode } from 'react';
import {
  Box, Heading, Text, Button, VStack, HStack, Table, Input, Textarea,
  Center, Spinner, Stat, Tabs, Badge, SimpleGrid,
} from '@chakra-ui/react';
import { LuArrowDownLeft, LuArrowUpRight, LuArrowLeftRight, LuPlus } from 'react-icons/lu';
import { toaster } from '../../components/ui/toaster';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  backofficeApi, type CashSession, type CashRegister, type CashMovement, type CashExpenseLine, type MethodTotal,
} from '../../api/backoffice';
import { Picker } from '../../components/Picker';
import { Switch } from '../../components/ui/switch';
import { money } from '../../utils/format';
import { Page } from '../../components/Page';
import { useSessionStore } from '../../stores/session';
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

function hhmm(iso: string) {
  return new Date(iso).toLocaleTimeString('es-MX', { hour: '2-digit', minute: '2-digit' });
}
// Tipo del movimiento para la columna "Tipo": traspaso (azul) o entrada/salida (verde/rojo).
function movementType(m: CashMovement): { label: string; palette: string } {
  if (m.transferId !== null) return { label: 'Traspaso', palette: 'blue' };
  return m.kind === 'entrada' ? { label: 'Entrada', palette: 'green' } : { label: 'Salida', palette: 'red' };
}

// ---- Tablas del resumen (compartidas entre caja en vivo, histórico y resumen post-cierre) ----

// Totales por método: esperado vs declarado vs diferencia (solo lectura).
function TotalsTable({ totals, currency }: { totals: MethodTotal[]; currency: string }) {
  if (totals.length === 0) return null;
  return (
    <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" overflowX="auto">
      <Table.Root size="sm">
        <Table.Header><Table.Row>
          <Table.ColumnHeader>Método</Table.ColumnHeader>
          <Table.ColumnHeader textAlign="end">Esperado</Table.ColumnHeader>
          <Table.ColumnHeader textAlign="end">Declarado</Table.ColumnHeader>
          <Table.ColumnHeader textAlign="end">Dif.</Table.ColumnHeader>
        </Table.Row></Table.Header>
        <Table.Body>
          {totals.map((t) => (
            <Table.Row key={t.methodId}>
              <Table.Cell>{t.name}</Table.Cell>
              <Table.Cell textAlign="end">{money(t.expected, currency)}</Table.Cell>
              <Table.Cell textAlign="end">{money(t.declared, currency)}</Table.Cell>
              <Table.Cell textAlign="end" color={diffColor(t.difference)} fontWeight="600">{money(t.difference, currency)}</Table.Cell>
            </Table.Row>
          ))}
        </Table.Body>
      </Table.Root>
    </Box>
  );
}

// Movimientos de efectivo en tabla: Hora · Tipo · Concepto · Usuario · Monto. Excluye las salidas
// de gastos (van en su propia sección) para no contarlas dos veces.
function MovementsTable({ movements, currency }: { movements: CashMovement[]; currency: string }) {
  const rows = movements.filter((m) => m.expenseId === null);
  if (rows.length === 0) return <Text fontSize="sm" color="fg.muted">Sin movimientos de efectivo.</Text>;
  return (
    <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" overflowX="auto">
      <Table.Root size="sm">
        <Table.Header><Table.Row>
          <Table.ColumnHeader>Hora</Table.ColumnHeader>
          <Table.ColumnHeader>Tipo</Table.ColumnHeader>
          <Table.ColumnHeader>Concepto</Table.ColumnHeader>
          <Table.ColumnHeader>Usuario</Table.ColumnHeader>
          <Table.ColumnHeader textAlign="end">Monto</Table.ColumnHeader>
        </Table.Row></Table.Header>
        <Table.Body>
          {rows.map((m) => {
            const t = movementType(m);
            return (
              <Table.Row key={m.id}>
                <Table.Cell whiteSpace="nowrap" color="fg.muted">{hhmm(m.createdAt)}</Table.Cell>
                <Table.Cell><Badge colorPalette={t.palette}>{t.label}</Badge></Table.Cell>
                <Table.Cell><Text truncate maxW="220px">{m.concept}</Text></Table.Cell>
                <Table.Cell color="fg.muted" whiteSpace="nowrap">{m.userName}</Table.Cell>
                <Table.Cell textAlign="end" fontWeight="600" whiteSpace="nowrap"
                  color={m.kind === 'entrada' ? 'green.500' : 'red.500'}>
                  {m.kind === 'entrada' ? '+' : '−'}{money(m.amount, currency)}
                </Table.Cell>
              </Table.Row>
            );
          })}
        </Table.Body>
      </Table.Root>
    </Box>
  );
}

// Gastos del corte en tabla + total. No renderiza nada si no hay gastos (ahorra espacio).
function ExpensesTable({ expenses, currency }: { expenses: CashExpenseLine[]; currency: string }) {
  if (expenses.length === 0) return null;
  const total = expenses.reduce((s, e) => s + (parseFloat(e.amount) || 0), 0);
  return (
    <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" overflowX="auto">
      <Table.Root size="sm">
        <Table.Header><Table.Row>
          <Table.ColumnHeader>Categoría</Table.ColumnHeader>
          <Table.ColumnHeader>Proveedor</Table.ColumnHeader>
          <Table.ColumnHeader>Método</Table.ColumnHeader>
          <Table.ColumnHeader textAlign="end">Monto</Table.ColumnHeader>
        </Table.Row></Table.Header>
        <Table.Body>
          {expenses.map((e) => (
            <Table.Row key={e.id}>
              <Table.Cell>{e.category}</Table.Cell>
              <Table.Cell color="fg.muted">{e.supplier ?? '—'}</Table.Cell>
              <Table.Cell color="fg.muted">{e.paymentMethod ?? '—'}</Table.Cell>
              <Table.Cell textAlign="end" fontWeight="600" whiteSpace="nowrap">{money(e.amount, currency)}</Table.Cell>
            </Table.Row>
          ))}
          <Table.Row fontWeight="700">
            <Table.Cell colSpan={3}>Total gastos</Table.Cell>
            <Table.Cell textAlign="end" whiteSpace="nowrap">{money(total, currency)}</Table.Cell>
          </Table.Row>
        </Table.Body>
      </Table.Root>
    </Box>
  );
}

// Sección con título compacto para agrupar las tablas del resumen.
function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Box>
      <Text fontWeight="700" mb={2}>{title}</Text>
      {children}
    </Box>
  );
}

export function CashPage() {
  const role = useSessionStore((s) => s.user?.role);
  const canManage = role === 'admin' || role === 'gerente';
  return (
    <Page maxW="920px">
      <Heading size="lg" mb={4}>Caja</Heading>
      <Tabs.Root defaultValue="operar">
        <Tabs.List>
          <Tabs.Trigger value="operar">Cajas</Tabs.Trigger>
          <Tabs.Trigger value="historico">Histórico</Tabs.Trigger>
          {canManage && <Tabs.Trigger value="gestion">Administrar</Tabs.Trigger>}
        </Tabs.List>
        <Tabs.Content value="operar" px={0} pt={4}><RegistersTab /></Tabs.Content>
        <Tabs.Content value="historico" px={0} pt={4}><HistoryTab /></Tabs.Content>
        {canManage && <Tabs.Content value="gestion" px={0} pt={4}><ManageRegistersTab /></Tabs.Content>}
      </Tabs.Root>
    </Page>
  );
}

// ---- Tab: cajas (selector + operar la caja elegida + traspaso) ----
function RegistersTab() {
  const { data, isLoading } = useQuery({ queryKey: ['cash', 'registers'], queryFn: backofficeApi.cashRegisters });
  const registers = data?.items ?? [];
  // Selección derivada: sin elección explícita (o si la caja elegida desaparece) cae en la primera
  // (la primaria). Evita un useEffect+setState solo para inicializar el default.
  const [selectedId, setSelectedId] = useState<number | null>(null);

  if (isLoading) return <Center h="40vh"><Spinner size="xl" /></Center>;
  if (registers.length === 0) return <Text color="fg.muted">No hay cajas configuradas. Créalas en «Administrar».</Text>;

  const openRegisters = registers.filter((r) => r.openSessionId !== null);
  const current = registers.find((r) => r.id === selectedId) ?? registers[0];

  return (
    <VStack align="stretch" gap={4}>
      {/* Selector de cajas: chip por caja con su estado (abierta/cerrada). */}
      <HStack gap={2} flexWrap="wrap">
        {registers.map((r) => (
          <Button key={r.id} size="sm" minH="44px" variant={current.id === r.id ? 'solid' : 'outline'}
            colorPalette={current.id === r.id ? undefined : 'gray'} onClick={() => setSelectedId(r.id)}>
            {r.name}
            <Badge ml={2} colorPalette={r.openSessionId !== null ? 'green' : 'gray'}>
              {r.openSessionId !== null ? 'Abierta' : 'Cerrada'}
            </Badge>
          </Button>
        ))}
      </HStack>

      <RegisterPanel register={current} openRegisters={openRegisters} />
    </VStack>
  );
}

// ---- Panel de una caja: abrir (si cerrada) u operar/cerrar (si abierta) ----
function RegisterPanel({ register, openRegisters }: { register: CashRegister; openRegisters: CashRegister[] }) {
  const qc = useQueryClient();
  const { data: session, isLoading } = useQuery({
    queryKey: ['cash', 'current', register.id],
    queryFn: () => backofficeApi.cashCurrent(register.id),
  });
  const [opening, setOpening] = useState('');
  const [declared, setDeclared] = useState<Record<string, string>>({});
  const [notes, setNotes] = useState('');
  const [closed, setClosed] = useState<CashSession | null>(null); // resumen tras cerrar
  const [transferOpen, setTransferOpen] = useState(false);

  const invalidate = () => qc.invalidateQueries({ queryKey: ['cash'] });
  const openMut = useMutation({
    mutationFn: () => backofficeApi.cashOpen(register.id, parseFloat(opening) || 0),
    onSuccess: () => { setOpening(''); invalidate(); },
    onError: (e) => toaster.create({ title: 'No se pudo abrir la caja', description: String(e), type: 'error' }),
  });
  const closeMut = useMutation({
    mutationFn: () => {
      const d: Record<string, number> = {};
      Object.entries(declared).forEach(([k, v]) => (d[k] = parseFloat(v) || 0));
      return backofficeApi.cashClose(register.id, d, notes || undefined);
    },
    onSuccess: (s) => { setClosed(s); setDeclared({}); setNotes(''); invalidate(); },
    onError: (e) => toaster.create({ title: 'No se pudo cerrar la caja', description: String(e), type: 'error' }),
  });

  if (isLoading) return <Center h="30vh"><Spinner size="xl" /></Center>;

  return (
    <>
      {!session ? (
        <VStack align="stretch" gap={4} bg="bg.panel" p={6} borderRadius="lg" borderWidth="1px" maxW="420px">
          <Text fontWeight="600">«{register.name}» está cerrada.</Text>
          <Text fontSize="sm" color="fg.muted">Captura el fondo inicial (efectivo con el que arranca el cajón).</Text>
          <HStack>
            <Input placeholder="Fondo inicial" type="number" inputMode="decimal" value={opening}
              onChange={(e) => setOpening(e.target.value)} />
            <Button onClick={() => openMut.mutate()} loading={openMut.isPending}>Abrir caja</Button>
          </HStack>
        </VStack>
      ) : (
        <VStack align="stretch" gap={5}>
          <HStack justify="space-between" flexWrap="wrap" gap={2}>
            <Text fontWeight="700">{register.name}{session.isPrimary ? ' · recibe ventas' : ''}</Text>
            <Button size="sm" variant="outline" onClick={() => setTransferOpen(true)}
              disabled={openRegisters.length < 2}>
              <LuArrowLeftRight /> Traspaso
            </Button>
          </HStack>

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

          {(session.expenses ?? []).length > 0 && (
            <Section title="Gastos del corte">
              <ExpensesTable expenses={session.expenses ?? []} currency={session.currency} />
            </Section>
          )}

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
            onClick={() => { if (confirm(`¿Cerrar «${register.name}»? No podrás modificarla después.`)) closeMut.mutate(); }}>
            Cerrar caja
          </Button>
        </VStack>
      )}

      <TransferDialog open={transferOpen} onClose={() => setTransferOpen(false)}
        from={register} openRegisters={openRegisters} onDone={() => { setTransferOpen(false); invalidate(); }} />

      {/* Resumen tras cerrar: esperado vs declarado vs diferencia */}
      <DialogRoot open={closed !== null} onOpenChange={(e) => { if (!e.open) setClosed(null); }} placement="center" size="md">
        <DialogBackdrop />
        <DialogContent>
          <DialogHeader><DialogTitle>Caja cerrada</DialogTitle></DialogHeader>
          <DialogCloseTrigger />
          <DialogBody pb={6}>
            <TotalsTable totals={closed?.totals ?? []} currency={closed?.currency ?? 'MXN'} />
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
    mutationFn: () => backofficeApi.cashMovement(session.registerId, kind, parseFloat(amount) || 0, concept.trim()),
    onSuccess: () => { setAmount(''); setConcept(''); qc.invalidateQueries({ queryKey: ['cash'] }); },
    onError: (e) => toaster.create({ title: 'No se pudo registrar', description: String(e), type: 'error' }),
  });
  const canAdd = (parseFloat(amount) || 0) > 0 && concept.trim().length > 0;
  // Go serializa un slice vacío como null; sin esta guarda, `.length`/`.map` revienta el render.
  const movements = session.movements ?? [];

  return (
    <Box>
      <Text fontWeight="700" mb={2}>Movimientos de efectivo</Text>
      <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" p={3} mb={3}>
        <HStack gap={2} flexWrap="wrap">
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
      </Box>
      <MovementsTable movements={movements} currency={session.currency} />
    </Box>
  );
}

// ---- Traspaso de efectivo entre dos cajas abiertas ----
function TransferDialog({ open, onClose, from, openRegisters, onDone }: {
  open: boolean; onClose: () => void; from: CashRegister; openRegisters: CashRegister[]; onDone: () => void;
}) {
  const [toId, setToId] = useState('');
  const [amount, setAmount] = useState('');
  const [note, setNote] = useState('');
  const destinations = openRegisters.filter((r) => r.id !== from.id);

  const reset = () => { setToId(''); setAmount(''); setNote(''); };
  const mut = useMutation({
    mutationFn: () => backofficeApi.cashTransfer(from.id, Number(toId), parseFloat(amount) || 0, note || undefined),
    onSuccess: () => { reset(); onDone(); },
    onError: (e) => toaster.create({ title: 'No se pudo traspasar', description: String(e), type: 'error' }),
  });
  const canSend = !!toId && (parseFloat(amount) || 0) > 0;

  return (
    <DialogRoot open={open} onOpenChange={(e) => { if (!e.open) { onClose(); reset(); } }} placement="center" size="sm">
      <DialogBackdrop />
      <DialogContent>
        <DialogHeader><DialogTitle>Traspaso desde «{from.name}»</DialogTitle></DialogHeader>
        <DialogCloseTrigger />
        <DialogBody pb={6}>
          <VStack align="stretch" gap={3}>
            <Text fontSize="sm" color="fg.muted">
              Mueve efectivo a otra caja abierta: sale de «{from.name}» y entra en la caja destino automáticamente.
            </Text>
            <Box>
              <Text fontSize="xs" color="fg.muted" mb={1}>Caja destino</Text>
              <Picker value={toId} onChange={setToId} placeholder="Elegir caja destino" title="Caja destino"
                options={destinations.map((r) => ({ value: String(r.id), label: r.name }))} />
            </Box>
            <Box>
              <Text fontSize="xs" color="fg.muted" mb={1}>Monto</Text>
              <Input type="number" inputMode="decimal" placeholder="0" value={amount} onChange={(e) => setAmount(e.target.value)} />
            </Box>
            <Box>
              <Text fontSize="xs" color="fg.muted" mb={1}>Nota (opcional)</Text>
              <Input placeholder="Motivo del traspaso" value={note} onChange={(e) => setNote(e.target.value)} />
            </Box>
            <Button colorPalette="blue" disabled={!canSend} loading={mut.isPending} onClick={() => mut.mutate()}>
              Confirmar traspaso
            </Button>
          </VStack>
        </DialogBody>
      </DialogContent>
    </DialogRoot>
  );
}

// ---- Tab: administrar cajas (admin/gerente) ----
function ManageRegistersTab() {
  const qc = useQueryClient();
  const { data, isLoading } = useQuery({ queryKey: ['cash', 'registers', 'all'], queryFn: backofficeApi.allCashRegisters });
  const [name, setName] = useState('');
  const [edit, setEdit] = useState<CashRegister | null>(null);

  const invalidate = () => qc.invalidateQueries({ queryKey: ['cash'] });
  const create = useMutation({
    mutationFn: () => backofficeApi.createCashRegister(name.trim()),
    onSuccess: () => { invalidate(); setName(''); },
    onError: (e) => toaster.create({ title: 'Error', description: String(e), type: 'error' }),
  });
  const update = useMutation({
    mutationFn: (r: CashRegister) => backofficeApi.updateCashRegister(r.id, { name: r.name.trim(), isActive: r.isActive }),
    onSuccess: () => { invalidate(); setEdit(null); },
    onError: (e) => toaster.create({ title: 'Error', description: String(e), type: 'error' }),
  });

  if (isLoading) return <Center h="40vh"><Spinner size="xl" /></Center>;

  return (
    <VStack align="stretch" gap={4}>
      <Box bg="bg.panel" p={4} borderRadius="lg" borderWidth="1px">
        <Text fontWeight="700" mb={3}>Nueva caja</Text>
        <HStack flexWrap="wrap" gap={3}>
          <Input placeholder="Nombre (ej. Caja fuerte)" value={name} onChange={(e) => setName(e.target.value)} flex="1" minW="180px" />
          <Button disabled={!name.trim()} loading={create.isPending} onClick={() => create.mutate()}><LuPlus /> Agregar</Button>
        </HStack>
      </Box>
      <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" overflowX="auto">
        <Table.Root size="sm">
          <Table.Header><Table.Row>
            <Table.ColumnHeader>Nombre</Table.ColumnHeader>
            <Table.ColumnHeader>Tipo</Table.ColumnHeader>
            <Table.ColumnHeader>Activa</Table.ColumnHeader>
            <Table.ColumnHeader></Table.ColumnHeader>
          </Table.Row></Table.Header>
          <Table.Body>
            {(data?.items ?? []).map((r) => (
              <Table.Row key={r.id}>
                <Table.Cell>{r.name}</Table.Cell>
                <Table.Cell>{r.isPrimary ? <Badge colorPalette="purple">Principal</Badge> : 'Secundaria'}</Table.Cell>
                <Table.Cell>
                  {/* La caja principal no se puede desactivar (el POS necesita dónde cuadrar). */}
                  <Switch checked={r.isActive} disabled={r.isPrimary}
                    onCheckedChange={(e) => update.mutate({ ...r, isActive: e.checked })} />
                </Table.Cell>
                <Table.Cell textAlign="end"><Button size="xs" variant="outline" onClick={() => setEdit(r)}>Editar</Button></Table.Cell>
              </Table.Row>
            ))}
          </Table.Body>
        </Table.Root>
      </Box>

      <DialogRoot open={edit !== null} onOpenChange={(e) => { if (!e.open) setEdit(null); }} placement="center" size="sm">
        <DialogBackdrop />
        <DialogContent>
          <DialogHeader><DialogTitle>Editar caja</DialogTitle></DialogHeader>
          <DialogCloseTrigger />
          <DialogBody pb={6}>
            {edit && (
              <VStack align="stretch" gap={3}>
                <Input placeholder="Nombre" value={edit.name} onChange={(e) => setEdit({ ...edit, name: e.target.value })} />
                {!edit.isPrimary && (
                  <Switch checked={edit.isActive} onCheckedChange={(e) => setEdit({ ...edit, isActive: e.checked })}>Activa</Switch>
                )}
                <Button disabled={!edit.name.trim()} loading={update.isPending} onClick={() => update.mutate(edit)}>Guardar</Button>
              </VStack>
            )}
          </DialogBody>
        </DialogContent>
      </DialogRoot>
    </VStack>
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
            <Table.ColumnHeader>Caja</Table.ColumnHeader>
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
                <Table.Cell fontWeight="600">{r.registerName}</Table.Cell>
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
        <DialogHeader><DialogTitle>Corte #{id}{data ? ` · ${data.registerName}` : ''}</DialogTitle></DialogHeader>
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
                <Section title="Totales por método"><TotalsTable totals={data.totals ?? []} currency={data.currency} /></Section>
              )}

              <Section title="Movimientos de efectivo">
                <MovementsTable movements={data.movements ?? []} currency={data.currency} />
              </Section>

              {(data.expenses ?? []).length > 0 && (
                <Section title="Gastos"><ExpensesTable expenses={data.expenses ?? []} currency={data.currency} /></Section>
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
