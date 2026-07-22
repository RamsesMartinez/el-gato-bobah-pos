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
  backofficeApi, type Expense, type ExpenseStatus, type Supplier, type ExpenseCategory, type FinancialGroup,
} from '../../api/backoffice';
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
        </Tabs.List>
        <Tabs.Content value="gastos" px={0} pt={4}><GastosTab /></Tabs.Content>
        <Tabs.Content value="proveedores" px={0} pt={4}><ProveedoresTab /></Tabs.Content>
        <Tabs.Content value="categorias" px={0} pt={4}><CategoriasTab /></Tabs.Content>
      </Tabs.Root>
    </Page>
  );
}

// ---- Tab: gastos (registrar, listar, pagar, cancelar) ----
function GastosTab() {
  const qc = useQueryClient();
  const [filter, setFilter] = useState<ExpenseStatus | ''>('');
  const [page, setPage] = useState(0);
  const pageSize = 20;
  const { data, isLoading } = useQuery({
    queryKey: ['expenses', filter, page],
    queryFn: () => backofficeApi.expenses({ status: filter || undefined, page, pageSize }),
    placeholderData: (prev) => prev, // sin parpadeo al cambiar de página
  });
  const { data: methods } = useQuery({ queryKey: ['payment-methods'], queryFn: posApi.paymentMethods });

  const [newOpen, setNewOpen] = useState(false);
  const [payTarget, setPayTarget] = useState<Expense | null>(null);

  const invalidate = () => { qc.invalidateQueries({ queryKey: ['expenses'] }); qc.invalidateQueries({ queryKey: ['cash'] }); };
  const cancel = useMutation({
    mutationFn: (v: { id: number; reason: string }) => backofficeApi.cancelExpense(v.id, v.reason),
    onSuccess: () => invalidate(),
    onError: (e) => toaster.create({ title: 'No se pudo cancelar', description: String(e), type: 'error' }),
  });

  const setFilterReset = (f: ExpenseStatus | '') => { setFilter(f); setPage(0); };
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
              <Table.ColumnHeader>Fecha</Table.ColumnHeader>
              <Table.ColumnHeader>Estado</Table.ColumnHeader>
              <Table.ColumnHeader>Categoría</Table.ColumnHeader>
              <Table.ColumnHeader>Proveedor</Table.ColumnHeader>
              <Table.ColumnHeader>Descripción</Table.ColumnHeader>
              <Table.ColumnHeader>Método</Table.ColumnHeader>
              <Table.ColumnHeader textAlign="end">Importe</Table.ColumnHeader>
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
                  <Table.Cell textAlign="end" fontWeight="600" whiteSpace="nowrap">{money(e.amount, e.currency)}</Table.Cell>
                  <Table.Cell>
                    {e.status === 'pendiente' && (
                      <HStack gap={1} justify="end">
                        <Button size="xs" colorPalette="green" onClick={() => setPayTarget(e)}>Pagar</Button>
                        <Button size="xs" variant="outline" colorPalette="red" onClick={() => {
                          const reason = prompt('Motivo de cancelación (opcional):');
                          if (reason === null) return;
                          cancel.mutate({ id: e.id, reason });
                        }}>Cancelar</Button>
                      </HStack>
                    )}
                  </Table.Cell>
                </Table.Row>
              ))}
              {items.length === 0 && (
                <Table.Row><Table.Cell colSpan={8}><Center py={10}><Text color="fg.muted">Sin gastos en esta vista.</Text></Center></Table.Cell></Table.Row>
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

      <NewExpenseDialog open={newOpen} onClose={() => setNewOpen(false)} onCreated={() => { setNewOpen(false); invalidate(); }} />
      <PayDialog expense={payTarget} methods={(methods?.items ?? []).map((m) => ({ id: m.id, name: m.name }))}
        onClose={() => setPayTarget(null)} onPaid={invalidate} />
    </VStack>
  );
}

// Alta de gasto en diálogo: mantiene la tabla como protagonista. Categoría/proveedor con alta
// inline (Picker) para no romper el flujo si falta uno.
function NewExpenseDialog({ open, onClose, onCreated }: { open: boolean; onClose: () => void; onCreated: () => void }) {
  const qc = useQueryClient();
  const { data: cats } = useQuery({ queryKey: ['expense-cats'], queryFn: backofficeApi.expenseCategories });
  const { data: sups } = useQuery({ queryKey: ['suppliers'], queryFn: backofficeApi.suppliers });
  const { data: methods } = useQuery({ queryKey: ['payment-methods'], queryFn: posApi.paymentMethods });

  const [categoryId, setCategoryId] = useState('');
  const [supplierId, setSupplierId] = useState('');
  const [amount, setAmount] = useState('');
  const [description, setDescription] = useState('');
  const [payNow, setPayNow] = useState(false);
  const [methodId, setMethodId] = useState('');

  const activeCats = (cats?.items ?? []).filter((c) => c.isActive);
  const activeSups = (sups?.items ?? []).filter((s) => s.isActive);

  const reset = () => { setCategoryId(''); setSupplierId(''); setAmount(''); setDescription(''); setPayNow(false); setMethodId(''); };
  const create = useMutation({
    mutationFn: () => backofficeApi.createExpense({
      categoryId: Number(categoryId),
      supplierId: supplierId ? Number(supplierId) : undefined,
      amount: parseFloat(amount) || 0,
      description: description || undefined,
      status: payNow ? 'pagada' : 'pendiente',
      methodId: payNow ? Number(methodId) : undefined,
    }),
    onSuccess: () => { reset(); onCreated(); },
    onError: (e) => toaster.create({ title: 'No se pudo registrar', description: String(e), type: 'error' }),
  });
  const canCreate = !!categoryId && (parseFloat(amount) || 0) > 0 && (!payNow || !!methodId);

  return (
    <DialogRoot open={open} onOpenChange={(e) => { if (!e.open) { onClose(); reset(); } }} placement="center" size="md" scrollBehavior="inside">
      <DialogBackdrop />
      <DialogContent>
        <DialogHeader><DialogTitle>Nuevo gasto</DialogTitle></DialogHeader>
        <DialogCloseTrigger />
        <DialogBody pb={6}>
          <VStack align="stretch" gap={3}>
            <Field label="Categoría">
              <Picker value={categoryId} onChange={setCategoryId} placeholder="Elegir categoría" title="Categoría"
                options={activeCats.map((c) => ({ value: String(c.id), label: c.name }))}
                onCreate={async (name) => {
                  // Alta rápida: grupo 'operacional' por defecto, se reclasifica en la pestaña Categorías.
                  const c = await backofficeApi.createExpenseCategory({ name, financialGroup: 'operacional' });
                  qc.setQueryData(['expense-cats'], (old?: { items: ExpenseCategory[] }) => ({ items: [...(old?.items ?? []), c] }));
                  return { value: String(c.id), label: c.name };
                }} />
            </Field>
            <Field label="Proveedor (opcional)">
              <Picker value={supplierId} onChange={setSupplierId} placeholder="— Sin proveedor —" title="Proveedor"
                clearable clearLabel="— Sin proveedor —"
                options={activeSups.map((s) => ({ value: String(s.id), label: s.name }))}
                onCreate={async (name) => {
                  const s = await backofficeApi.createSupplier({ name });
                  qc.setQueryData(['suppliers'], (old?: { items: Supplier[] }) => ({ items: [...(old?.items ?? []), s] }));
                  return { value: String(s.id), label: s.name };
                }} />
            </Field>
            <Field label="Importe">
              <Input type="number" inputMode="decimal" placeholder="0" value={amount} onChange={(e) => setAmount(e.target.value)} />
            </Field>
            <Field label="Descripción / comentario">
              <Input placeholder="Concepto" value={description} onChange={(e) => setDescription(e.target.value)} />
            </Field>
            <Switch checked={payNow} onCheckedChange={(e) => setPayNow(e.checked)}>Pagar ahora</Switch>
            {payNow && (
              <Field label="Método de pago">
                <Picker value={methodId} onChange={setMethodId} placeholder="Método de pago" title="Método de pago"
                  options={(methods?.items ?? []).map((m) => ({ value: String(m.id), label: m.name }))} />
              </Field>
            )}
            <Button mt={1} disabled={!canCreate} loading={create.isPending} onClick={() => create.mutate()}>
              {payNow ? 'Registrar y pagar' : 'Registrar pendiente'}
            </Button>
          </VStack>
        </DialogBody>
      </DialogContent>
    </DialogRoot>
  );
}

function PayDialog({ expense, methods, onClose, onPaid }: {
  expense: Expense | null;
  methods: Array<{ id: number; name: string }>;
  onClose: () => void;
  onPaid: () => void;
}) {
  const [methodId, setMethodId] = useState('');
  const pay = useMutation({
    mutationFn: () => backofficeApi.payExpense(expense!.id, Number(methodId)),
    onSuccess: () => { onPaid(); onClose(); setMethodId(''); },
    onError: (e) => toaster.create({ title: 'No se pudo pagar', description: String(e), type: 'error' }),
  });
  return (
    <DialogRoot open={expense !== null} onOpenChange={(e) => { if (!e.open) { onClose(); setMethodId(''); } }} placement="center" size="sm">
      <DialogBackdrop />
      <DialogContent>
        <DialogHeader><DialogTitle>Pagar gasto</DialogTitle></DialogHeader>
        <DialogCloseTrigger />
        <DialogBody pb={6}>
          <Text mb={3}>{expense?.category} · <b>{expense && money(expense.amount, expense.currency)}</b></Text>
          <Text fontSize="sm" color="fg.muted" mb={2}>Si pagas en efectivo con caja abierta, se registra la salida en el corte.</Text>
          <Box mb={4}>
            <Picker value={methodId} onChange={setMethodId} placeholder="Método de pago" title="Método de pago"
              options={methods.map((m) => ({ value: String(m.id), label: m.name }))} />
          </Box>
          <Button w="100%" colorPalette="green" disabled={!methodId} loading={pay.isPending} onClick={() => pay.mutate()}>
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
