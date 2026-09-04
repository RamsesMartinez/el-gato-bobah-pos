import { useState, Fragment, type ReactNode } from 'react';
import {
  Box, Heading, Text, Button, VStack, HStack, Table, Input, Textarea,
  Center, Spinner, Stat, Tabs, Badge, SimpleGrid, Wrap, useBreakpointValue,
} from '@chakra-ui/react';
import { LuArrowDownLeft, LuArrowUpRight, LuArrowLeftRight, LuPlus, LuChevronDown, LuChevronUp } from 'react-icons/lu';
import { ApiError } from '../../api/client';
import { toaster } from '../../components/ui/toaster';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  backofficeApi, type CashSession, type CashSessionDetail, type CashRegister, type CashMovement, type CashExpenseLine, type MethodTotal, type CorteBreakdown,
} from '../../api/backoffice';
import { Picker } from '../../components/Picker';
import { Switch } from '../../components/ui/switch';
import { money } from '../../utils/format';
import { faltanPorContar } from './cierreDeCaja';
import { Page } from '../../components/Page';
import { useSessionStore } from '../../stores/session';
import { soloHora } from '../../utils/horaDelNegocio';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';
import { DEFAULT_TIMEZONE } from '../../utils/zonaPorDefecto';
import {
  DialogRoot, DialogBackdrop, DialogContent, DialogBody, DialogHeader, DialogTitle, DialogCloseTrigger,
} from '../../components/ui/dialog';
import { montoTecleado } from '../../domain/numeros';

// Sobrante (>0) verde, faltante (<0) rojo, cuadrado gris.
function diffColor(v: string) {
  // Del servidor, no de un teclado: Number() alcanza y no hay formato que validar.
  const n = Number(v);
  if (n > 0.005) return 'green.500';
  if (n < -0.005) return 'red.500';
  return 'fg.muted';
}

// La zona llega como parámetro: esta es una función de módulo.
function hhmm(iso: string, zona: string) {
  return soloHora(iso, zona);
}
// Tipo del movimiento para la columna "Tipo": traspaso (azul) o entrada/salida (verde/rojo).
function movementType(m: CashMovement): { label: string; palette: string } {
  if (m.transferId !== null) return { label: 'Traspaso', palette: 'blue' };
  return m.kind === 'entrada' ? { label: 'Entrada', palette: 'green' } : { label: 'Salida', palette: 'red' };
}

// ---- Tablas del resumen (compartidas entre caja en vivo, histórico y resumen post-cierre) ----

// Totales por método: esperado (sistema) vs declarado (usuario) vs diferencia (solo lectura).
// withTotalRow agrega una fila de totales (Sistema / Según usuario / Diferencia) al pie.
export function TotalsTable({ totals, currency, withTotalRow }: { totals: MethodTotal[]; currency: string; withTotalRow?: boolean }) {
  if (!totals?.length) return null;
  const sum = (pick: (t: MethodTotal) => string) => totals.reduce((s, t) => s + (Number(pick(t)) || 0), 0);
  const diffTotal = sum((t) => t.difference);
  return (
    <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" overflowX="auto">
      <Table.Root size="sm">
        <Table.Header><Table.Row>
          <Table.ColumnHeader>Método</Table.ColumnHeader>
          <Table.ColumnHeader textAlign="end">Sistema</Table.ColumnHeader>
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
          {withTotalRow && (
            <Table.Row fontWeight="700">
              <Table.Cell>Total</Table.Cell>
              <Table.Cell textAlign="end">{money(sum((t) => t.expected), currency)}</Table.Cell>
              <Table.Cell textAlign="end">{money(sum((t) => t.declared), currency)}</Table.Cell>
              <Table.Cell textAlign="end" color={diffColor(String(diffTotal))}>{money(diffTotal, currency)}</Table.Cell>
            </Table.Row>
          )}
        </Table.Body>
      </Table.Root>
    </Box>
  );
}

// Movimientos de efectivo en tabla: Hora · Tipo · Concepto · Usuario · Monto. Excluye las salidas
// de gastos (van en su propia sección) para no contarlas dos veces.
// La zona llega como PROP y no del hook: esto es una tabla de presentación, y que pidiera los
// ajustes por su cuenta la vuelve imposible de pintar sin montar media aplicación alrededor. Quien
// la usa ya tiene la zona a la mano.
export function MovementsTable({ movements, currency, zona = DEFAULT_TIMEZONE }: {
  movements: CashMovement[];
  currency: string;
  zona?: string;
}) {
  const rows = (movements ?? []).filter((m) => m.expenseId === null);
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
                <Table.Cell whiteSpace="nowrap" color="fg.muted">{hhmm(m.createdAt, zona)}</Table.Cell>
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
export function ExpensesTable({ expenses, currency }: { expenses: CashExpenseLine[]; currency: string }) {
  if (!expenses?.length) return null;
  const total = expenses.reduce((s, e) => s + (Number(e.amount) || 0), 0);
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

// Fila de la tarjeta jerárquica Ingresos/Egresos (label a la izquierda, monto a la derecha).
function SummaryLine({ label, amount, indent = 0, weight = '400', color, top }: {
  label: string; amount?: string; indent?: number; weight?: string; color?: string; top?: boolean;
}) {
  return (
    <HStack justify="space-between" px={3} py="6px" pl={3 + indent * 4}
      borderTopWidth={top ? '1px' : undefined} borderColor="border.muted">
      <Text fontSize="sm" fontWeight={weight} color={color}>{label}</Text>
      {amount !== undefined && <Text fontSize="sm" fontWeight={weight} color={color} whiteSpace="nowrap">{amount}</Text>}
    </HStack>
  );
}

// Tarjeta jerárquica del corte: Monto inicial → Ingresos (por método → concepto) → Egresos.
// Es la "categorización por naturaleza del dinero" que explica cómo el sistema llegó a cada esperado.
export function IngresosEgresosCard({ openingCash, breakdown, currency }: { openingCash: string; breakdown: CorteBreakdown; currency: string }) {
  const ingresos = breakdown?.ingresos ?? [];
  const egresos = breakdown?.egresos ?? [];
  const plataformas = breakdown?.plataformas ?? [];
  return (
    <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" overflow="hidden">
      <SummaryLine label="Monto inicial" amount={money(openingCash, currency)} weight="600" />
      <SummaryLine label="Ingresos" amount={money(breakdown?.ingresosTotal ?? '0', currency)} weight="700" color="green.600" top />
      {ingresos.map((m) => (
        <Fragment key={m.method}>
          <SummaryLine label={m.method} amount={money(m.total, currency)} indent={1} weight="600" />
          {m.items.map((it) => (
            <SummaryLine key={it.concept} label={it.concept} amount={money(it.amount, currency)} indent={2} color="fg.muted" />
          ))}
        </Fragment>
      ))}
      {ingresos.length === 0 && <SummaryLine label="Sin ingresos" indent={1} color="fg.muted" />}
      <SummaryLine label="Egresos" amount={`−${money(breakdown?.egresosTotal ?? '0', currency)}`} weight="700" color="red.600" top />
      {egresos.map((it) => (
        <SummaryLine key={it.concept} label={it.concept} amount={`−${money(it.amount, currency)}`} indent={1} color="fg.muted" />
      ))}
      {egresos.length === 0 && <SummaryLine label="Sin egresos" indent={1} color="fg.muted" />}
      {/* Cada plataforma cobra por dos métodos —en línea y efectivo—, así que su total no se lee de
          un renglón de arriba. Es el número que se concilia contra el depósito que la plataforma
          manda después. Solo salen las que vendieron: un renglón en $0 por cada una configurada
          llena el corte de ruido justo donde se está buscando un descuadre. */}
      {plataformas.length > 0 && (
        <>
          <SummaryLine label="Por plataforma" weight="700" top />
          {plataformas.map((p) => (
            <SummaryLine key={p.platform} label={p.platform} amount={money(p.total, currency)} indent={1} weight="600" />
          ))}
        </>
      )}
    </Box>
  );
}

// Bloque plegable para el drill-down (tablas de movimientos/gastos) — ahorra espacio por defecto.
function Collapsible({ title, children }: { title: string; children: ReactNode }) {
  const [open, setOpen] = useState(false);
  return (
    <Box>
      <Button size="sm" variant="ghost" w="100%" justifyContent="space-between" onClick={() => setOpen((o) => !o)}>
        <Text fontWeight="700">{title}</Text>
        {open ? <LuChevronUp /> : <LuChevronDown />}
      </Button>
      {open && <Box mt={2}>{children}</Box>}
    </Box>
  );
}

// Datos mínimos del resumen (los cumplen CashSession y CashSessionDetail por estructura).
interface CorteData {
  openingCash: string;
  currency: string;
  breakdown: CorteBreakdown;
  totals: MethodTotal[];
  movements: CashMovement[];
  expenses: CashExpenseLine[];
}

// Resumen del corte reutilizable (histórico y panel lateral): jerarquía + conciliación + drill-down.
function CorteSummary({ data }: { data: CorteData }) {
  const horaNegocio = useHoraDelNegocio();
  const cur = data.currency;
  const totals = data.totals ?? [];
  const movements = data.movements ?? [];
  const expenses = data.expenses ?? [];
  return (
    <VStack align="stretch" gap={4}>
      <IngresosEgresosCard openingCash={data.openingCash} breakdown={data.breakdown} currency={cur} />
      {totals.length > 0 && (
        <Section title="Conciliación (sistema vs declarado)">
          <TotalsTable totals={totals} currency={cur} withTotalRow />
        </Section>
      )}
      <Collapsible title={`Movimientos de efectivo (${movements.filter((m) => m.expenseId === null).length})`}>
        <MovementsTable movements={movements} currency={cur} zona={horaNegocio.zona} />
      </Collapsible>
      {expenses.length > 0 && (
        <Collapsible title={`Gastos (${expenses.length})`}>
          <ExpensesTable expenses={expenses} currency={cur} />
        </Collapsible>
      )}
    </VStack>
  );
}

// Detalle completo de un corte (carga por id): cabecera + resumen + notas. Lo usan el diálogo (7")
// y el panel lateral (pantallas grandes).
function CorteDetail({ id }: { id: number }) {
  const horaNegocio = useHoraDelNegocio();
  const { data, isLoading } = useQuery({ queryKey: ['cash', 'session', id], queryFn: () => backofficeApi.cashSession(id) });
  if (isLoading || !data) return <Center py={8}><Spinner /></Center>;
  return (
    <VStack align="stretch" gap={4}>
      <SimpleGrid columns={2} gap={2} fontSize="sm">
        <Text color="fg.muted">Caja</Text><Text textAlign="end" fontWeight="600">{data.registerName}</Text>
        <Text color="fg.muted">Abrió</Text><Text textAlign="end">{data.openedByName} · {horaNegocio.fechaYHora(data.openedAt)}</Text>
        {data.closedAt && (<>
          <Text color="fg.muted">Cerró</Text>
          <Text textAlign="end">{data.closedByName ?? '—'} · {horaNegocio.fechaYHora(data.closedAt)}</Text>
        </>)}
      </SimpleGrid>
      <CorteSummary data={data} />
      <VentasDelCorte session={data} zona={horaNegocio.zona} />
      {data.notes && (
        <Box><Text fontWeight="700" fontSize="sm">Notas</Text><Text fontSize="sm" color="fg.muted">{data.notes}</Text></Box>
      )}
    </VStack>
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
  const horaNegocio = useHoraDelNegocio();
  const qc = useQueryClient();
  const { data: session, isLoading } = useQuery({
    queryKey: ['cash', 'current', register.id],
    queryFn: () => backofficeApi.cashCurrent(register.id),
  });
  const [opening, setOpening] = useState('');
  const [declared, setDeclared] = useState<Record<string, string>>({});
  // Lo que todavía no se cuenta. La regla vive fuera del componente y con test propio: es la que
  // evita registrar un faltante inventado, y ese fallo ya costó un corte con $1,662 de descuadre.
  const porContar = faltanPorContar(session?.totals ?? [], declared);
  const [notes, setNotes] = useState('');
  const [closed, setClosed] = useState<CashSession | null>(null); // resumen tras cerrar
  const [transferOpen, setTransferOpen] = useState(false);

  const invalidate = () => qc.invalidateQueries({ queryKey: ['cash'] });
  const openMut = useMutation({
    mutationFn: () => backofficeApi.cashOpen(register.id, montoTecleado(opening) ?? 0),
    onSuccess: () => { setOpening(''); invalidate(); },
    onError: (e) => toaster.create({ title: 'No se pudo abrir la caja', description: String(e), type: 'error' }),
  });
  const closeMut = useMutation({
    mutationFn: () => {
      const d: Record<string, number> = {};
      Object.entries(declared).forEach(([k, v]) => (d[k] = montoTecleado(v) ?? 0));
      return backofficeApi.cashClose(register.id, d, notes || undefined);
    },
    onSuccess: (s) => { setClosed(s); setDeclared({}); setNotes(''); invalidate(); },
    // El servidor distingue "hay pedidos sin terminar" de cualquier otro fallo y manda los folios
    // en el mensaje. Se pinta con su propio título porque no es un error del cierre: es una tarea
    // pendiente, y el operador tiene que saber que la puede resolver y volver.
    onError: (e) => toaster.create({
      title: e instanceof ApiError && e.code === 'OPEN_ORDERS' ? 'Faltan pedidos por terminar' : 'No se pudo cerrar la caja',
      description: e instanceof ApiError ? e.message : String(e),
      type: 'error',
      duration: 8000,
    }),
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
              <Stat.ValueText fontSize="sm">{horaNegocio.fechaYHora(session.openedAt)}</Stat.ValueText>
            </Stat.Root>
          </SimpleGrid>

          <Section title="Resumen del corte">
            <IngresosEgresosCard openingCash={session.openingCash} breakdown={session.breakdown} currency={session.currency} />
          </Section>

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

          {/* Un campo en blanco se guardaba como cero declarado y quedaba registrado un faltante
              que no existía: pasó con un corte real de $1,662. Escribir 0 sigue siendo válido —
              puede no haber efectivo—; lo que no vale es dejarlo vacío. */}
          {porContar.length > 0 && (
            <Box borderWidth="1px" borderColor="border" borderRadius="lg" p={3} colorPalette="orange" bg="colorPalette.subtle">
              <Text fontSize="sm" fontWeight="600">
                Falta capturar lo contado en: {porContar.map((m) => m.name).join(', ')}
              </Text>
            </Box>
          )}

          {/* Quién cobró qué. Con dos estaciones contra el mismo cajón, es lo único que separa la
              responsabilidad: partir la caja daría dos arqueos contando el mismo dinero. Solo se
              pinta si hubo más de una persona — con una sola, repite el total de arriba. */}
          {session.cashiers.length > 1 && (
            <Box borderWidth="1px" borderColor="border" borderRadius="lg" p={3}>
              <Text fontWeight="700" mb={2}>Cobrado por</Text>
              <Table.Root size="sm">
                <Table.Header>
                  <Table.Row>
                    <Table.ColumnHeader>Persona</Table.ColumnHeader>
                    <Table.ColumnHeader textAlign="end">Efectivo</Table.ColumnHeader>
                    <Table.ColumnHeader textAlign="end">Otros</Table.ColumnHeader>
                  </Table.Row>
                </Table.Header>
                <Table.Body>
                  {session.cashiers.map((c) => (
                    <Table.Row key={c.name}>
                      <Table.Cell>{c.name}</Table.Cell>
                      {/* El efectivo con más peso: es lo que está en el cajón y lo único de donde
                          puede salir una diferencia. */}
                      <Table.Cell textAlign="end" fontWeight="700">{money(c.cash)}</Table.Cell>
                      <Table.Cell textAlign="end" color="fg.muted">{money(c.other)}</Table.Cell>
                    </Table.Row>
                  ))}
                </Table.Body>
              </Table.Root>
            </Box>
          )}

          {/* Lo que falta por entregar, ANTES de intentar cerrar. Antes solo se sabía al presionar
              el botón y recibir el error: el operador terminaba de contar el efectivo para
              enterarse entonces de que le faltaba sacar comida. */}
          {session.pending.length > 0 && (
            <Box borderWidth="1px" borderColor="orange.300" bg="orange.50"
              _dark={{ bg: 'orange.950' }} borderRadius="lg" p={3}>
              <Text fontWeight="700" color="orange.700" _dark={{ color: 'orange.200' }} mb={1}>
                Falta entregar {session.pending.length === 1 ? '1 pedido' : `${session.pending.length} pedidos`}
              </Text>
              <Text fontSize="sm" color="fg.muted" mb={2}>
                La caja no cierra hasta que salgan o se cancelen. Estar cobrado no cuenta: lo que
                falta es la comida.
              </Text>
              <Wrap gap={2}>
                {session.pending.map((o) => (
                  <Badge key={o.number} colorPalette="orange" px={2} py={1} fontSize="sm">
                    {o.name ? `${o.name} · #${o.number}` : `#${o.number}`}
                  </Badge>
                ))}
              </Wrap>
            </Box>
          )}

          <Button colorPalette="red" size="lg" loading={closeMut.isPending}
            disabled={porContar.length > 0 || session.pending.length > 0}
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
  const horaNegocio = useHoraDelNegocio();
  const qc = useQueryClient();
  const [kind, setKind] = useState<'entrada' | 'salida'>('salida');
  const [amount, setAmount] = useState('');
  const [concept, setConcept] = useState('');

  const mut = useMutation({
    mutationFn: () => backofficeApi.cashMovement(session.registerId, kind, montoTecleado(amount) ?? 0, concept.trim()),
    onSuccess: () => { setAmount(''); setConcept(''); qc.invalidateQueries({ queryKey: ['cash'] }); },
    onError: (e) => toaster.create({ title: 'No se pudo registrar', description: String(e), type: 'error' }),
  });
  const canAdd = (montoTecleado(amount) ?? 0) > 0 && concept.trim().length > 0;
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
      <MovementsTable movements={movements} currency={session.currency} zona={horaNegocio.zona} />
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
    mutationFn: () => backofficeApi.cashTransfer(from.id, Number(toId), montoTecleado(amount) ?? 0, note || undefined),
    onSuccess: () => { reset(); onDone(); },
    onError: (e) => toaster.create({ title: 'No se pudo traspasar', description: String(e), type: 'error' }),
  });
  const canSend = !!toId && (montoTecleado(amount) ?? 0) > 0;

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

// ---- Tab: histórico de cortes (lista + detalle: panel lateral en pantallas grandes, diálogo en 7") ----
function HistoryTab() {
  const horaNegocio = useHoraDelNegocio();
  const { data, isLoading } = useQuery({ queryKey: ['cash', 'history'], queryFn: backofficeApi.cashHistory });
  const [detailId, setDetailId] = useState<number | null>(null);
  // Panel lateral solo en pantallas anchas (xl+); en tablet de 7" se usa el diálogo a pantalla completa.
  const wide = useBreakpointValue({ base: false, xl: true }, { ssr: false });

  if (isLoading) return <Center h="40vh"><Spinner size="xl" /></Center>;
  const rows = data?.items ?? [];
  if (rows.length === 0) return <Text color="fg.muted">Aún no hay cortes registrados.</Text>;

  const list = (
    <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" overflowX="auto">
      <Table.Root size="sm" interactive>
        <Table.Header><Table.Row>
          <Table.ColumnHeader>Caja</Table.ColumnHeader>
          <Table.ColumnHeader>Abierta</Table.ColumnHeader>
          <Table.ColumnHeader>Estado</Table.ColumnHeader>
          {!wide && <Table.ColumnHeader>Abrió / Cerró</Table.ColumnHeader>}
          <Table.ColumnHeader textAlign="end">Diferencia</Table.ColumnHeader>
          <Table.ColumnHeader></Table.ColumnHeader>
        </Table.Row></Table.Header>
        <Table.Body>
          {rows.map((r) => (
            <Table.Row key={r.id} cursor="pointer" onClick={() => setDetailId(r.id)}
              bg={wide && detailId === r.id ? 'bg.muted' : undefined}>
              <Table.Cell fontWeight="600">{r.registerName}</Table.Cell>
              <Table.Cell whiteSpace="nowrap">{horaNegocio.fechaYHora(r.openedAt)}</Table.Cell>
              <Table.Cell>
                <Badge colorPalette={r.status === 'abierta' ? 'green' : 'gray'}>
                  {r.status === 'abierta' ? 'Abierta' : 'Cerrada'}
                </Badge>
              </Table.Cell>
              {!wide && <Table.Cell fontSize="sm">{r.openedByName}{r.closedByName ? ` → ${r.closedByName}` : ''}</Table.Cell>}
              <Table.Cell textAlign="end" color={diffColor(r.totalDifference)} fontWeight="600">
                {r.status === 'cerrada' ? money(r.totalDifference, r.currency) : '—'}
              </Table.Cell>
              <Table.Cell textAlign="end">
                <Button size="xs" variant="outline" onClick={(e) => { e.stopPropagation(); setDetailId(r.id); }}>Ver</Button>
              </Table.Cell>
            </Table.Row>
          ))}
        </Table.Body>
      </Table.Root>
    </Box>
  );

  if (wide) {
    return (
      <HStack align="start" gap={4}>
        <Box flex="1.1" minW={0}>{list}</Box>
        <Box flex="1" minW={0} bg="bg.panel" borderRadius="lg" borderWidth="1px" p={4} maxH="calc(100dvh - 220px)" overflowY="auto">
          {detailId
            ? <CorteDetail id={detailId} />
            : <Center py={12}><Text color="fg.muted">Selecciona un corte para ver el detalle.</Text></Center>}
        </Box>
      </HStack>
    );
  }
  return (
    <>
      {list}
      <SessionDetailDialog id={detailId} onClose={() => setDetailId(null)} />
    </>
  );
}

function SessionDetailDialog({ id, onClose }: { id: number | null; onClose: () => void }) {
  return (
    <DialogRoot open={id !== null} onOpenChange={(e) => { if (!e.open) onClose(); }} placement="center" size="lg" scrollBehavior="inside">
      <DialogBackdrop />
      <DialogContent>
        <DialogHeader><DialogTitle>Corte #{id}</DialogTitle></DialogHeader>
        <DialogCloseTrigger />
        <DialogBody pb={6}>
          {id !== null && <CorteDetail id={id} />}
        </DialogBody>
      </DialogContent>
    </DialogRoot>
  );
}

// Las ventas que este corte cobró.
//
// Vive dentro del corte y no como filtro de la pantalla de Ventas: ahí tendría que convivir con el
// filtro de fechas, y bastaría elegir un rango que no toque el corte para quedarse mirando una
// pantalla vacía sin nada que explique por qué.
//
// Alto acotado: el detalle ya reparte los 600 px de la tableta entre el resumen, los gastos, lo
// declarado por método y lo cobrado por persona. La lista muestra cinco renglones y desplaza dentro
// de su propia caja, para no empujar fuera de pantalla lo que ya estaba.
export function VentasDelCorte({ session, zona = DEFAULT_TIMEZONE }: {
  session: CashSessionDetail;
  zona?: string;
}) {
  // "El campo no vino" NO es "vino en cero". El front se despliega antes que el backend, y en esa
  // ventana un corte que sí cobró aparecería jurando que no cobró nada — una pantalla que miente
  // sobre dinero es peor que una pantalla incompleta. Sin el campo, la sección no se dibuja.
  if (session.sales === undefined || session.salesCount === undefined) return null;

  const ventas = session.sales;
  const total = session.salesCount;

  if (total === 0) {
    return (
      <Section title="Ventas del corte">
        <Text fontSize="sm" color="fg.muted">Este corte no cobró ninguna venta.</Text>
      </Section>
    );
  }

  const recortadas = total > ventas.length;
  return (
    <Section title="Ventas del corte">
      <Text fontSize="sm" color="fg.muted" mb={2}>
        {total === 1 ? '1 venta' : `${total} ventas`} · {money(session.salesTotal ?? '0', session.currency)}
        {' '}sin canceladas, reembolsadas ni propinas
        {recortadas && ` · se muestran las ${ventas.length} más recientes`}
      </Text>
      <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" maxH="240px" overflowY="auto">
        <Table.Root size="sm" stickyHeader>
          <Table.Header><Table.Row>
            <Table.ColumnHeader>Folio</Table.ColumnHeader>
            <Table.ColumnHeader>Hora</Table.ColumnHeader>
            <Table.ColumnHeader>Estado</Table.ColumnHeader>
            <Table.ColumnHeader textAlign="end">Total</Table.ColumnHeader>
          </Table.Row></Table.Header>
          <Table.Body>
            {ventas.map((v) => (
              <Table.Row key={v.id}>
                <Table.Cell whiteSpace="nowrap">
                  #{v.dailyNumber}{v.folioName ? ` · ${v.folioName}` : ''}
                </Table.Cell>
                <Table.Cell whiteSpace="nowrap">{hhmm(v.openedAt, zona)}</Table.Cell>
                <Table.Cell>
                  <Badge colorPalette={colorDeEstadoDeVenta(v.status)}>{v.status}</Badge>
                </Table.Cell>
                <Table.Cell textAlign="end" fontWeight="600">
                  {money(v.total, session.currency)}
                </Table.Cell>
              </Table.Row>
            ))}
          </Table.Body>
        </Table.Root>
      </Box>
    </Section>
  );
}

// Cancelada y reembolsada se pintan distinto porque su dinero NO está en el total de arriba: verlas
// con el mismo color que una venta cobrada invita a sumarlas.
function colorDeEstadoDeVenta(estado: string) {
  if (estado === 'cancelada') return 'red';
  if (estado === 'reembolsada') return 'orange';
  return 'gray';
}
