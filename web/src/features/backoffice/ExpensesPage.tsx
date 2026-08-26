import { useState, type ReactNode } from 'react';
import {
  Box, Heading, Text, Button, HStack, VStack, Table, Input, Textarea,
  Center, Spinner, Tabs, Badge,
} from '@chakra-ui/react';
import { LuPlus, LuChevronLeft, LuChevronRight } from 'react-icons/lu';
import { Picker } from '../../components/Picker';
import { Switch } from '../../components/ui/switch';
import { toaster } from '../../components/ui/toaster';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  backofficeApi, type Expense, type ExpenseStatus, type ExpenseSort, type Supplier,
  type ExpenseCategory, type FinancialGroup,
} from '../../api/backoffice';
import { SortHead } from '../../components/SortHead';
import { ExpenseDialog } from './ExpenseDialog';
import { ExpenseDetailDialog } from './ExpenseDetailDialog';
import { posApi } from '../../api/pos';
import { money } from '../../utils/format';
import { Page } from '../../components/Page';
import {
  DialogRoot, DialogBackdrop, DialogContent, DialogBody, DialogHeader, DialogTitle, DialogCloseTrigger,
} from '../../components/ui/dialog';

const GROUPS: FinancialGroup[] = ['operacional', 'administrativo', 'otro'];
const STATUS_LABEL: Record<ExpenseStatus, string> = { pendiente: 'Pendiente', pagada: 'Pagada', cancelada: 'Cancelada' };
const STATUS_COLOR: Record<ExpenseStatus, string> = { pendiente: 'orange', pagada: 'green', cancelada: 'gray' };

export function ExpensesPage() {
  return (
    <Page maxW="1000px">
      <Heading size="lg" mb={4}>Gastos</Heading>
      <Tabs.Root defaultValue="gastos">
        <Tabs.List>
          <Tabs.Trigger value="gastos">Gastos</Tabs.Trigger>
          <Tabs.Trigger value="proveedores">Proveedores</Tabs.Trigger>
          <Tabs.Trigger value="categorias">Categorías</Tabs.Trigger>
          <Tabs.Trigger value="mapeos">Mapeos</Tabs.Trigger>
        </Tabs.List>
        <Tabs.Content value="gastos" px={0} pt={4}><GastosTab /></Tabs.Content>
        <Tabs.Content value="proveedores" px={0} pt={4}><ProveedoresTab /></Tabs.Content>
        <Tabs.Content value="categorias" px={0} pt={4}><CategoriasTab /></Tabs.Content>
        <Tabs.Content value="mapeos" px={0} pt={4}><MapeosTab /></Tabs.Content>
      </Tabs.Root>
    </Page>
  );
}

// ---- Tab: gastos (registrar, listar, pagar, cancelar) ----
function GastosTab() {
  const qc = useQueryClient();
  const [filter, setFilter] = useState<ExpenseStatus | ''>('');
  const [page, setPage] = useState(0);
  const [sort, setSort] = useState<ExpenseSort>('date');
  const [dir, setDir] = useState<'asc' | 'desc'>('desc');
  const pageSize = 20;
  const { data, isLoading } = useQuery({
    queryKey: ['expenses', filter, page, sort, dir],
    queryFn: () => backofficeApi.expenses({ status: filter || undefined, page, pageSize, sort, dir }),
    placeholderData: (prev) => prev, // sin parpadeo al cambiar de página
  });
  const { data: methods } = useQuery({ queryKey: ['payment-methods'], queryFn: posApi.paymentMethods });

  const [newOpen, setNewOpen] = useState(false);
  const [payTarget, setPayTarget] = useState<Expense | null>(null);
  const [detailId, setDetailId] = useState<number | null>(null);

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ['expenses'] });
    qc.invalidateQueries({ queryKey: ['cash'] });
    qc.invalidateQueries({ queryKey: ['stock'] }); // recibir mercancía mueve el almacén
  };
  const cancel = useMutation({
    mutationFn: (v: { id: number; reason: string }) => backofficeApi.cancelExpense(v.id, v.reason),
    onSuccess: () => invalidate(),
    onError: (e) => toaster.create({ title: 'No se pudo cancelar', description: String(e), type: 'error' }),
  });

  const setFilterReset = (f: ExpenseStatus | '') => { setFilter(f); setPage(0); };
  // Orden por columna: 1er clic ordena (texto asc / fecha e importe desc); reclics alternan.
  const onSort = (col: ExpenseSort, numeric = false) => {
    if (sort === col) setDir((d) => (d === 'asc' ? 'desc' : 'asc'));
    else { setSort(col); setDir(numeric ? 'desc' : 'asc'); }
    setPage(0); // el nuevo orden cambia qué cae en cada página
  };
  const items = data?.items ?? [];
  const total = data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / pageSize));

  return (
    <VStack align="stretch" gap={4}>
      {/* Cabecera: filtros por estado + acción principal (la tabla es la protagonista) */}
      <HStack justify="space-between" flexWrap="wrap" gap={3}>
        <HStack gap={2} flexWrap="wrap">
          {([['', 'Todos'], ['pendiente', 'Pendientes'], ['pagada', 'Pagadas'], ['cancelada', 'Canceladas']] as const).map(([v, label]) => (
            <Button key={v} size="sm" variant={filter === v ? 'solid' : 'outline'} colorPalette={filter === v ? undefined : 'gray'}
              onClick={() => setFilterReset(v)}>{label}</Button>
          ))}
        </HStack>
        <Button colorPalette="green" onClick={() => setNewOpen(true)}><LuPlus /> Nuevo gasto</Button>
      </HStack>

      <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" overflowX="auto" minH="220px">
        {isLoading ? (
          <Center py={16}><Spinner size="xl" /></Center>
        ) : (
          <Table.Root size="sm" interactive stickyHeader>
            <Table.Header><Table.Row>
              <SortHead label="Fecha" col="date" sort={sort} dir={dir} onSort={onSort} numeric />
              <SortHead label="Estado" col="status" sort={sort} dir={dir} onSort={onSort} />
              <SortHead label="Categoría" col="category" sort={sort} dir={dir} onSort={onSort} />
              <SortHead label="Proveedor" col="supplier" sort={sort} dir={dir} onSort={onSort} />
              <SortHead label="Descripción" col="description" sort={sort} dir={dir} onSort={onSort} />
              <Table.ColumnHeader>Método</Table.ColumnHeader>
              <Table.ColumnHeader>Mercancía</Table.ColumnHeader>
              <SortHead label="Importe" col="amount" sort={sort} dir={dir} onSort={onSort} numeric align="end" />
              <Table.ColumnHeader></Table.ColumnHeader>
            </Table.Row></Table.Header>
            <Table.Body>
              {items.map((e) => (
                <Table.Row key={e.id}>
                  <Table.Cell whiteSpace="nowrap">{e.expenseDate}</Table.Cell>
                  <Table.Cell><Badge colorPalette={STATUS_COLOR[e.status]}>{STATUS_LABEL[e.status]}</Badge></Table.Cell>
                  <Table.Cell>{e.category}</Table.Cell>
                  <Table.Cell>{e.supplier ?? '—'}</Table.Cell>
                  <Table.Cell>{e.description ?? '—'}</Table.Cell>
                  <Table.Cell>{e.paymentMethod ?? '—'}</Table.Cell>
                  <Table.Cell whiteSpace="nowrap">
                    {e.itemCount === 0 ? (
                      <Text color="fg.subtle" fontSize="sm">—</Text>
                    ) : e.receivedAt ? (
                      <Badge colorPalette="green">recibida {e.receivedAt}</Badge>
                    ) : (
                      <Badge colorPalette="orange">{e.itemCount} por recibir</Badge>
                    )}
                  </Table.Cell>
                  <Table.Cell textAlign="end" fontWeight="600" whiteSpace="nowrap">{money(e.amount, e.currency)}</Table.Cell>
                  <Table.Cell>
                    <HStack gap={1} justify="end">
                      <Button size="xs" variant="ghost" onClick={() => setDetailId(e.id)}>Ver</Button>
                      {e.status === 'pendiente' && (
                        <>
                          <Button size="xs" colorPalette="green" onClick={() => setPayTarget(e)}>Pagar</Button>
                          <Button size="xs" variant="outline" colorPalette="red" onClick={() => {
                            const reason = prompt('Motivo de cancelación (opcional):');
                            if (reason === null) return;
                            cancel.mutate({ id: e.id, reason });
                          }}>Cancelar</Button>
                        </>
                      )}
                    </HStack>
                  </Table.Cell>
                </Table.Row>
              ))}
              {items.length === 0 && (
                <Table.Row><Table.Cell colSpan={9}><Center py={10}><Text color="fg.muted">Sin gastos en esta vista.</Text></Center></Table.Cell></Table.Row>
              )}
            </Table.Body>
          </Table.Root>
        )}
      </Box>

      {/* Paginador */}
      <HStack justify="space-between" flexWrap="wrap" gap={2}>
        <Text fontSize="sm" color="fg.muted">{total} gasto{total === 1 ? '' : 's'}</Text>
        <HStack gap={2}>
          <Button size="sm" variant="outline" disabled={page === 0} onClick={() => setPage((p) => Math.max(0, p - 1))}>
            <LuChevronLeft /> Anterior
          </Button>
          <Text fontSize="sm" minW="130px" textAlign="center">Página {page + 1} de {pages}</Text>
          <Button size="sm" variant="outline" disabled={page + 1 >= pages} onClick={() => setPage((p) => p + 1)}>
            Siguiente <LuChevronRight />
          </Button>
        </HStack>
      </HStack>

      <ExpenseDialog open={newOpen} onClose={() => setNewOpen(false)}
        onSaved={() => { setNewOpen(false); invalidate(); }} />
      <ExpenseDetailDialog key={detailId ?? 'none'} id={detailId} onClose={() => setDetailId(null)} onChanged={invalidate} />
      <PayDialog expense={payTarget} methods={methods?.items ?? []}
        onClose={() => setPayTarget(null)} onPaid={invalidate} />
    </VStack>
  );
}

// PayDialog agrega UN pago al gasto. Si con él los pagos cubren el importe, el gasto pasa a
// pagado; si no, queda pendiente con un abono registrado (así el pago partido no necesita un
// estado "parcial" aparte).
function PayDialog({ expense, methods, onClose, onPaid }: {
  expense: Expense | null;
  methods: Array<{ id: number; name: string; affectsCashDrawer?: boolean }>;
  onClose: () => void;
  onPaid: () => void;
}) {
  const [methodId, setMethodId] = useState('');
  const [registerId, setRegisterId] = useState('');
  const [amount, setAmount] = useState('');
  const [paidOn, setPaidOn] = useState(() => new Date().toISOString().slice(0, 10));
  const { data: registers } = useQuery({ queryKey: ['cash', 'registers'], queryFn: backofficeApi.cashRegisters });
  const openRegisters = (registers?.items ?? []).filter((r) => r.openSessionId !== null);

  // Un método que mueve el cajón EXIGE caja: el backend lo rechaza si no, porque efectivo que
  // sale sin movimiento de caja descuadra el corte.
  const cash = !!methods.find((m) => String(m.id) === methodId)?.affectsCashDrawer;

  const reset = () => { setMethodId(''); setRegisterId(''); setAmount(''); };
  const pay = useMutation({
    mutationFn: () => backofficeApi.payExpense(expense!.id, {
      methodId: Number(methodId),
      amount: amount || expense!.amount,
      paidOn,
      registerId: registerId ? Number(registerId) : undefined,
    }),
    onSuccess: () => { onPaid(); onClose(); reset(); },
    onError: (e) => toaster.create({ title: 'No se pudo pagar', description: String(e), type: 'error' }),
  });
  return (
    <DialogRoot open={expense !== null} onOpenChange={(e) => { if (!e.open) { onClose(); reset(); } }} placement="center" size="sm">
      <DialogBackdrop />
      <DialogContent>
        <DialogHeader><DialogTitle>Pagar gasto</DialogTitle></DialogHeader>
        <DialogCloseTrigger />
        <DialogBody pb={6}>
          <Text mb={3}>{expense?.category} · <b>{expense && money(expense.amount, expense.currency)}</b></Text>
          <VStack align="stretch" gap={3} mb={4}>
            <Field label="Método de pago">
              <Picker value={methodId} onChange={setMethodId} placeholder="Método de pago" title="Método de pago"
                options={methods.map((m) => ({ value: String(m.id), label: m.name }))} />
            </Field>
            <Field label="Importe (vacío = el total del gasto)">
              <Input type="number" inputMode="decimal" placeholder={expense?.amount ?? '0'} value={amount}
                onChange={(e) => setAmount(e.target.value)} />
            </Field>
            <Field label="Fecha del pago">
              <Input type="date" value={paidOn} onChange={(e) => setPaidOn(e.target.value)} />
            </Field>
            <Field label={cash ? 'Caja (obligatoria en efectivo)' : 'Caja (para el arqueo)'}>
              {openRegisters.length === 0 ? (
                <Text fontSize="sm" color={cash ? 'red.500' : 'fg.muted'}>
                  {cash ? 'No hay caja abierta. Abre una en Caja para poder pagar en efectivo.' : 'Sin cajas abiertas.'}
                </Text>
              ) : (
                <Picker value={registerId} onChange={setRegisterId} placeholder={cash ? 'Elegir caja' : '— Sin arqueo —'}
                  title="Caja" clearable={!cash} clearLabel="— No entra al arqueo —"
                  options={openRegisters.map((r) => ({ value: String(r.id), label: r.name }))} />
              )}
            </Field>
          </VStack>
          <Button w="100%" colorPalette="green" disabled={!methodId || (cash && !registerId)}
            loading={pay.isPending} onClick={() => pay.mutate()}>
            Confirmar pago
          </Button>
        </DialogBody>
      </DialogContent>
    </DialogRoot>
  );
}

// ---- Tab: proveedores ----
function ProveedoresTab() {
  const qc = useQueryClient();
  const { data, isLoading } = useQuery({ queryKey: ['suppliers'], queryFn: backofficeApi.suppliers });
  const [name, setName] = useState('');
  const [phone, setPhone] = useState('');
  const [edit, setEdit] = useState<Supplier | null>(null);

  const create = useMutation({
    mutationFn: () => backofficeApi.createSupplier({ name: name.trim(), phone: phone || undefined }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['suppliers'] }); setName(''); setPhone(''); },
    onError: (e) => toaster.create({ title: 'Error', description: String(e), type: 'error' }),
  });
  const update = useMutation({
    mutationFn: (s: Supplier) => backofficeApi.updateSupplier(s.id, { name: s.name, phone: s.phone ?? undefined, notes: s.notes ?? undefined, isActive: s.isActive }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['suppliers'] }); setEdit(null); },
    onError: (e) => toaster.create({ title: 'Error', description: String(e), type: 'error' }),
  });

  if (isLoading) return <Center h="40vh"><Spinner size="xl" /></Center>;

  return (
    <VStack align="stretch" gap={4}>
      <Box bg="bg.panel" p={4} borderRadius="lg" borderWidth="1px">
        <Text fontWeight="700" mb={3}>Nuevo proveedor</Text>
        <HStack flexWrap="wrap" gap={3}>
          <Input placeholder="Nombre" value={name} onChange={(e) => setName(e.target.value)} flex="1" minW="180px" />
          <Input placeholder="Teléfono (opcional)" value={phone} onChange={(e) => setPhone(e.target.value)} maxW="200px" />
          <Button disabled={!name.trim()} loading={create.isPending} onClick={() => create.mutate()}>Agregar</Button>
        </HStack>
      </Box>
      <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" overflowX="auto">
        <Table.Root size="sm">
          <Table.Header><Table.Row>
            <Table.ColumnHeader>Nombre</Table.ColumnHeader>
            <Table.ColumnHeader>Teléfono</Table.ColumnHeader>
            <Table.ColumnHeader>Activo</Table.ColumnHeader>
            <Table.ColumnHeader></Table.ColumnHeader>
          </Table.Row></Table.Header>
          <Table.Body>
            {(data?.items ?? []).map((s) => (
              <Table.Row key={s.id}>
                <Table.Cell>{s.name}</Table.Cell>
                <Table.Cell>{s.phone ?? '—'}</Table.Cell>
                <Table.Cell>
                  <Switch checked={s.isActive} onCheckedChange={(e) => update.mutate({ ...s, isActive: e.checked })} />
                </Table.Cell>
                <Table.Cell textAlign="end"><Button size="xs" variant="outline" onClick={() => setEdit(s)}>Editar</Button></Table.Cell>
              </Table.Row>
            ))}
          </Table.Body>
        </Table.Root>
      </Box>

      <DialogRoot open={edit !== null} onOpenChange={(e) => { if (!e.open) setEdit(null); }} placement="center" size="sm">
        <DialogBackdrop />
        <DialogContent>
          <DialogHeader><DialogTitle>Editar proveedor</DialogTitle></DialogHeader>
          <DialogCloseTrigger />
          <DialogBody pb={6}>
            {edit && (
              <VStack align="stretch" gap={3}>
                <Input placeholder="Nombre" value={edit.name} onChange={(e) => setEdit({ ...edit, name: e.target.value })} />
                <Input placeholder="Teléfono" value={edit.phone ?? ''} onChange={(e) => setEdit({ ...edit, phone: e.target.value })} />
                <Textarea rows={2} resize="none" placeholder="Notas" value={edit.notes ?? ''} onChange={(e) => setEdit({ ...edit, notes: e.target.value })} />
                <Button disabled={!edit.name.trim()} loading={update.isPending} onClick={() => update.mutate(edit)}>Guardar</Button>
              </VStack>
            )}
          </DialogBody>
        </DialogContent>
      </DialogRoot>
    </VStack>
  );
}

// ---- Tab: categorías de gasto ----
function CategoriasTab() {
  const qc = useQueryClient();
  const { data, isLoading } = useQuery({ queryKey: ['expense-cats'], queryFn: backofficeApi.expenseCategories });
  const [name, setName] = useState('');
  const [group, setGroup] = useState<FinancialGroup>('operacional');
  const [edit, setEdit] = useState<ExpenseCategory | null>(null);

  const create = useMutation({
    mutationFn: () => backofficeApi.createExpenseCategory({ name: name.trim(), financialGroup: group }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['expense-cats'] }); setName(''); },
    onError: (e) => toaster.create({ title: 'Error', description: String(e), type: 'error' }),
  });
  const update = useMutation({
    mutationFn: (c: ExpenseCategory) => backofficeApi.updateExpenseCategory(c.id, { name: c.name, financialGroup: c.financialGroup, isActive: c.isActive }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['expense-cats'] }); setEdit(null); },
    onError: (e) => toaster.create({ title: 'Error', description: String(e), type: 'error' }),
  });

  if (isLoading) return <Center h="40vh"><Spinner size="xl" /></Center>;

  return (
    <VStack align="stretch" gap={4}>
      <Box bg="bg.panel" p={4} borderRadius="lg" borderWidth="1px">
        <Text fontWeight="700" mb={3}>Nueva categoría</Text>
        <HStack flexWrap="wrap" gap={3}>
          <Input placeholder="Nombre" value={name} onChange={(e) => setName(e.target.value)} flex="1" minW="180px" />
          <GroupChips value={group} onChange={setGroup} />
          <Button disabled={!name.trim()} loading={create.isPending} onClick={() => create.mutate()}>Agregar</Button>
        </HStack>
      </Box>
      <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" overflowX="auto">
        <Table.Root size="sm">
          <Table.Header><Table.Row>
            <Table.ColumnHeader>Nombre</Table.ColumnHeader>
            <Table.ColumnHeader>Grupo financiero</Table.ColumnHeader>
            <Table.ColumnHeader>Activa</Table.ColumnHeader>
            <Table.ColumnHeader></Table.ColumnHeader>
          </Table.Row></Table.Header>
          <Table.Body>
            {(data?.items ?? []).map((c) => (
              <Table.Row key={c.id}>
                <Table.Cell>{c.name}</Table.Cell>
                <Table.Cell>{c.financialGroup}</Table.Cell>
                <Table.Cell>
                  <Switch checked={c.isActive} onCheckedChange={(e) => update.mutate({ ...c, isActive: e.checked })} />
                </Table.Cell>
                <Table.Cell textAlign="end"><Button size="xs" variant="outline" onClick={() => setEdit(c)}>Editar</Button></Table.Cell>
              </Table.Row>
            ))}
          </Table.Body>
        </Table.Root>
      </Box>

      <DialogRoot open={edit !== null} onOpenChange={(e) => { if (!e.open) setEdit(null); }} placement="center" size="sm">
        <DialogBackdrop />
        <DialogContent>
          <DialogHeader><DialogTitle>Editar categoría</DialogTitle></DialogHeader>
          <DialogCloseTrigger />
          <DialogBody pb={6}>
            {edit && (
              <VStack align="stretch" gap={3}>
                <Input placeholder="Nombre" value={edit.name} onChange={(e) => setEdit({ ...edit, name: e.target.value })} />
                <GroupChips value={edit.financialGroup} onChange={(g) => setEdit({ ...edit, financialGroup: g })} />
                <Button disabled={!edit.name.trim()} loading={update.isPending} onClick={() => update.mutate(edit)}>Guardar</Button>
              </VStack>
            )}
          </DialogBody>
        </DialogContent>
      </DialogRoot>
    </VStack>
  );
}

// Grupo financiero: conjunto fijo de 3 → chips tappables (no dropdown, ver regla de UX táctil).
function GroupChips({ value, onChange }: { value: FinancialGroup; onChange: (g: FinancialGroup) => void }) {
  return (
    <HStack gap={1} flexWrap="wrap">
      {GROUPS.map((g) => (
        <Button key={g} size="sm" minH="40px" textTransform="capitalize"
          variant={value === g ? 'solid' : 'outline'} colorPalette={value === g ? undefined : 'gray'}
          onClick={() => onChange(g)}>{g}</Button>
      ))}
    </HStack>
  );
}

// Campo con etiqueta compacta para los formularios inline.
function Field({ label, children, ...box }: { label: string; children: ReactNode } & Record<string, unknown>) {
  return (
    <Box {...box}>
      <Text fontSize="xs" color="fg.muted" mb={1}>{label}</Text>
      {children}
    </Box>
  );
}

// ---- Tab: mapeos aprendidos ----

// Lo que el sistema aprendió de cada proveedor: qué decía el papel y a qué artículo se resolvió.
// Existe porque un mapeo equivocado se repite silenciosamente en cada compra de ese proveedor —
// aquí se ve y se deshace. "Olvidar" no borra nada del gasto: solo hace que el próximo documento
// vuelva a sugerir desde cero.
function MapeosTab() {
  const qc = useQueryClient();
  const [page, setPage] = useState(0);
  const pageSize = 30;
  const { data, isLoading } = useQuery({
    queryKey: ['supplier-items', page],
    queryFn: () => backofficeApi.supplierItems({ page, pageSize }),
    placeholderData: (prev) => prev,
  });
  const forget = useMutation({
    mutationFn: (id: number) => backofficeApi.forgetSupplierItem(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['supplier-items'] }),
    onError: (e) => toaster.create({ title: 'No se pudo olvidar', description: String(e), type: 'error' }),
  });

  if (isLoading) return <Center h="40vh"><Spinner size="xl" /></Center>;
  const items = data?.items ?? [];
  const total = data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / pageSize));

  return (
    <VStack align="stretch" gap={4}>
      <Text fontSize="sm" color="fg.muted">
        Lo que el sistema reconoce de cada proveedor. Al capturar una compra estos renglones se
        autollenan; si alguno quedó mal asignado, olvídalo y el siguiente documento volverá a
        sugerir.
      </Text>
      <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" overflowX="auto" minH="200px">
        <Table.Root size="sm">
          <Table.Header><Table.Row>
            <Table.ColumnHeader>Proveedor</Table.ColumnHeader>
            <Table.ColumnHeader>Dice el papel</Table.ColumnHeader>
            <Table.ColumnHeader>Se resuelve a</Table.ColumnHeader>
            <Table.ColumnHeader>Contenido</Table.ColumnHeader>
            <Table.ColumnHeader>Visto</Table.ColumnHeader>
            <Table.ColumnHeader></Table.ColumnHeader>
          </Table.Row></Table.Header>
          <Table.Body>
            {items.map((m) => (
              <Table.Row key={m.id}>
                <Table.Cell>{m.supplier}</Table.Cell>
                <Table.Cell>
                  <Text>{m.rawName}</Text>
                  {m.rawCode && <Text fontSize="xs" color="fg.subtle">código {m.rawCode}</Text>}
                </Table.Cell>
                <Table.Cell>
                  {/* 'personal' antes que el artículo: si se marcó como de la casa, eso es lo que
                      se aplicará en la próxima compra aunque quede el mapeo viejo debajo. */}
                  {m.status === 'personal'
                    ? <Badge colorPalette="purple">de la casa</Badge>
                    : m.itemName
                      ? <Text>{m.itemName}</Text>
                      : <Badge colorPalette="gray">no inventariable</Badge>}
                </Table.Cell>
                <Table.Cell>{m.packQtyInBase ?? '—'}</Table.Cell>
                <Table.Cell whiteSpace="nowrap">{m.lastSeenAt.slice(0, 10)}</Table.Cell>
                <Table.Cell textAlign="end">
                  <Button size="xs" variant="outline" colorPalette="red"
                    loading={forget.isPending && forget.variables === m.id}
                    onClick={() => forget.mutate(m.id)}>Olvidar</Button>
                </Table.Cell>
              </Table.Row>
            ))}
            {items.length === 0 && (
              <Table.Row><Table.Cell colSpan={6}>
                <Center py={10}>
                  <Text color="fg.muted">
                    Todavía nada aprendido. Captura una compra con proveedor y sus artículos.
                  </Text>
                </Center>
              </Table.Cell></Table.Row>
            )}
          </Table.Body>
        </Table.Root>
      </Box>
      <HStack justify="space-between" flexWrap="wrap" gap={2}>
        <Text fontSize="sm" color="fg.muted">{total} mapeo{total === 1 ? '' : 's'}</Text>
        <HStack gap={2}>
          <Button size="sm" variant="outline" disabled={page === 0} onClick={() => setPage((p) => Math.max(0, p - 1))}>
            <LuChevronLeft /> Anterior
          </Button>
          <Text fontSize="sm" minW="130px" textAlign="center">Página {page + 1} de {pages}</Text>
          <Button size="sm" variant="outline" disabled={page + 1 >= pages} onClick={() => setPage((p) => p + 1)}>
            Siguiente <LuChevronRight />
          </Button>
        </HStack>
      </HStack>
    </VStack>
  );
}
