import { useState, Fragment, type ReactNode } from 'react';
import { useNavigate } from 'react-router';
import { ConfirmSheet } from '../../components/ConfirmSheet';
import { ReasonSheet } from '../../components/ReasonSheet';
import { posApi } from '../../api/pos';
import { descartarCuenta } from '../pos/descartarCuenta';
import type { AccountItem } from '../../types/pos';
import { ESTADO, nombreDeCuenta } from '../../domain/cuentas';
import {
  Box, Heading, Text, Button, VStack, HStack, Table, Input, Textarea,
  Center, Spinner, Stat, Tabs, Badge, SimpleGrid, useBreakpointValue, IconButton,
} from '@chakra-ui/react';
import { LuArrowDownLeft, LuArrowUpRight, LuArrowLeftRight, LuPlus, LuChevronDown, LuChevronUp, LuChevronLeft, LuChevronRight } from 'react-icons/lu';
import { medirAccion } from '../../api/uso';
import { ApiError } from '../../api/client';
import { toaster } from '../../components/ui/toaster';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  backofficeApi, type CashSession, type CashSessionDetail, type CorteSale, type CashRegister, type PendingOrder, type OwingOrder, type CashMovement, type CashExpenseLine, type MethodTotal, type CorteBreakdown, type AperturaInput, type ConteosDelTurno, type ArqueoDelCajon, type VoidedPayment, type SessionRefund,
} from '../../api/backoffice';
import { ContadorDeEfectivo } from './ContadorDeEfectivo';
import type { ResultadoDelConteo } from './conteo';
import { Picker } from '../../components/Picker';
import { Switch } from '../../components/ui/switch';
import { money } from '../../utils/format';
import { RepartirPropinas, PropinasDelCierre } from './RepartirPropinas';
import { faltaDecidirPropinas } from './propinas';
import type { TipPayoutInput, TipsDecision } from '../../api/backoffice';
import {
  faltanPorContar, diferenciasDelCierre, faltaContarElCajon, diferenciaDelCajon,
  type DiferenciasDelCierre,
} from './cierreDeCaja';
import { Page } from '../../components/Page';
import { useSessionStore } from '../../stores/session';
import { soloHora } from '../../utils/horaDelNegocio';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';
import { DEFAULT_TIMEZONE } from '../../utils/zonaPorDefecto';
import {
  DialogRoot, DialogBackdrop, DialogContent, DialogBody, DialogHeader, DialogTitle, DialogCloseTrigger,
} from '../../components/ui/dialog';
import { montoTecleado, round2 } from '../../domain/numeros';
import { mensajeDeError } from '../../api/mensajes';

// Cuántas ventas trae cada "Ver más". El mismo tamaño de página que el resto de las listas: pedir
// de 200 en 200 no cabe en la caja y pedir de 5 en 5 obliga a diez toques para ver un día.
const VENTAS_POR_PAGINA = 20;

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
  if (m.isRefund) return { label: 'Devolución', palette: 'orange' };
  if (m.kind === 'propina') return { label: 'Propina', palette: 'purple' };
  return m.kind === 'entrada' ? { label: 'Entrada', palette: 'green' } : { label: 'Salida', palette: 'red' };
}

// ---- Tablas del resumen (compartidas entre caja en vivo, histórico y resumen post-cierre) ----

// Totales por método: esperado (sistema) vs declarado (usuario) vs diferencia (solo lectura).
// withTotalRow agrega una fila de totales (Sistema / Según usuario / Diferencia) al pie.
//
// `drawerDifference` es la diferencia del CAJÓN, que vive en el conteo y no en los renglones: los
// métodos del cajón guardan declarado = esperado. Sin sumarla, la fila Total decía $0 en un corte con
// el cajón corto, mientras el histórico —que sí la suma— decía la cifra real (spec 031, D13).
export function TotalsTable({ totals, currency, withTotalRow, drawerDifference }: {
  totals: MethodTotal[]; currency: string; withTotalRow?: boolean; drawerDifference?: string | null;
}) {
  if (!totals?.length) return null;
  const sum = (pick: (t: MethodTotal) => string) => totals.reduce((s, t) => s + (Number(pick(t)) || 0), 0);
  const diffTotal = round2(sum((t) => t.difference) + (Number(drawerDifference) || 0));
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
              <Table.Cell textAlign="end">{montoOSinDato(t.expected, currency)}</Table.Cell>
              <Table.Cell textAlign="end">{money(t.declared, currency)}</Table.Cell>
              <Table.Cell textAlign="end" color={diffColor(t.difference)} fontWeight="600">{money(t.difference, currency)}</Table.Cell>
            </Table.Row>
          ))}
          {withTotalRow && (
            <Table.Row fontWeight="700">
              <Table.Cell>Total</Table.Cell>
              <Table.Cell textAlign="end">{money(sum((t) => t.expected ?? '0'), currency)}</Table.Cell>
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
                {/* Sin ancho fijo: a 220 px «Devolución: Producto en mal estado» se cortaba con la
                    tabla a medio llenar, y el motivo es justo lo que se vino a leer. */}
                <Table.Cell><Text lineClamp={2}>{m.concept}</Text></Table.Cell>
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
function SummaryLine({ label, amount, indent = 0, weight = '400', color, top, negative }: {
  label: string; amount?: string; indent?: number; weight?: string; color?: string; top?: boolean; negative?: boolean;
}) {
  return (
    <HStack justify="space-between" px={3} py="6px" pl={3 + indent * 4}
      borderTopWidth={top ? '1px' : undefined} borderColor="border.muted">
      <Text fontSize="sm" fontWeight={weight} color={color}>{label}</Text>
      {amount !== undefined && (
        <Text fontSize="sm" fontWeight={weight} color={negative ? 'red.600' : color} whiteSpace="nowrap"
          data-negative={negative ? 'true' : undefined}>{amount}</Text>
      )}
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
          {m.note && <Text fontSize="xs" color="fg.muted" px={3} pl={8} pb={1}>{m.note}</Text>}
          {m.items.map((it) => (
            // Un concepto que resta (Devoluciones) se pinta como los egresos: en gris se leería como
            // otro ingreso.
            <SummaryLine key={it.concept} label={it.concept} amount={money(it.amount, currency)} indent={2} color="fg.muted"
              negative={Number(it.amount) < 0} />
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
// 44 px: medía 36 y en la tableta el dedo cae en el renglón de al lado.
export function Plegable({ title, children }: { title: string; children: ReactNode }) {
  const [open, setOpen] = useState(false);
  return (
    <Box>
      <Button size="sm" minH="44px" variant="ghost" w="100%" justifyContent="space-between" onClick={() => setOpen((o) => !o)}>
        <Text fontWeight="700">{title}</Text>
        {open ? <LuChevronUp /> : <LuChevronDown />}
      </Button>
      {open && <Box mt={2}>{children}</Box>}
    </Box>
  );
}

// VoidedPaymentsList: los pagos que se devolvieron en el turno (spec 027).
//
// NO SUMA NADA, a propósito: un pago devuelto no es una salida de caja ni dinero del turno, y el
// esperado del servidor ya lo excluye. Un total aquí invitaría a restarlo otra vez del cierre.
// Sin devoluciones no se pinta: es el caso normal y cada renglón vacío le quita alto a la tableta.
// El tope en dvh es para que en 600 px de alto la lista no empuje fuera la tabla del cierre.
export function VoidedPaymentsList({ payments, currency, zona = DEFAULT_TIMEZONE }: {
  payments?: VoidedPayment[]; currency: string; zona?: string;
}) {
  if (!payments?.length) return null;
  return (
    <Section title="Pagos devueltos">
      <Box as="ul" aria-label="Pagos devueltos" listStyleType="none" m={0} p={0}
        bg="bg.panel" borderRadius="lg" borderWidth="1px" maxH="35dvh" overflowY="auto">
        {payments.map((p, i) => (
          <Box as="li" key={i} px={3} py={2} borderTopWidth={i === 0 ? 0 : '1px'} fontSize="sm">
            <HStack justify="space-between" gap={2}>
              <HStack gap={2} minW={0}>
                <Text fontWeight="600">{p.method}</Text>
                <Text color="fg.muted" truncate>Pedido {p.orderFolio}</Text>
              </HStack>
              <HStack gap={2} flexShrink={0}>
                {Number(p.tip) > 0 && <Text color="fg.muted">+ propina {money(p.tip, currency)}</Text>}
                <Text fontWeight="600">{money(p.amount, currency)}</Text>
              </HStack>
            </HStack>
            <Text color="fg.muted">
              {hhmm(p.voidedAt, zona)} · {p.voidedBy}: {p.reason}
            </Text>
          </Box>
        ))}
      </Box>
    </Section>
  );
}

// RefundsList: el dinero que se le devolvió al cliente en el turno (spec 031).
//
// No es la lista de «Pagos devueltos»: aquélla son cobros que no ocurrieron, ésta dinero que salió
// hacia el cliente. Tampoco suma nada: cada devolución ya está en «Devoluciones» de su medio en el
// desglose (spec 029), así que un total aquí invitaría a restarlas otra vez.
// Va PLEGADA con su contador: abierta empujaría fuera de los 600 px de la tableta la tabla donde se
// declara el cierre. Y por ir plegada, abierta NO lleva scroll propio (spec 029): un scroll de 3.5
// renglones dentro de una página que ya hace scroll obligaba a adivinar cuál mover. «Pagos
// devueltos» sí conserva su tope, a propósito: esa lista va abierta siempre.
export function RefundsList({ refunds, currency, zona = DEFAULT_TIMEZONE }: {
  refunds?: SessionRefund[]; currency: string; zona?: string;
}) {
  const [open, setOpen] = useState(false);
  if (!refunds?.length) return null;
  return (
    <Box>
      <Button variant="ghost" w="100%" minH="44px" justifyContent="space-between" onClick={() => setOpen((o) => !o)}>
        <Text fontWeight="700">Devoluciones ({refunds.length})</Text>
        {open ? <LuChevronUp /> : <LuChevronDown />}
      </Button>
      {open && (
        <Box as="ul" aria-label="Devoluciones" listStyleType="none" m={0} mt={2} p={0}
          bg="bg.panel" borderRadius="lg" borderWidth="1px">
          {refunds.map((r, i) => (
            <Box as="li" key={i} px={3} py={2} borderTopWidth={i === 0 ? 0 : '1px'} fontSize="sm">
              <HStack justify="space-between" gap={2}>
                <HStack gap={2} minW={0}>
                  <Text fontWeight="600">{r.method}</Text>
                  <Text color="fg.muted" truncate>Pedido {r.orderFolio}</Text>
                </HStack>
                <HStack gap={2} flexShrink={0}>
                  {Number(r.tip) > 0 && <Text color="fg.muted">+ propina {money(r.tip, currency)}</Text>}
                  <Text fontWeight="600">{money(r.amount, currency)}</Text>
                </HStack>
              </HStack>
              <Text color="fg.muted">
                {hhmm(r.refundedAt, zona)} · {r.refundedBy}: {r.reason}
                {r.fromDrawer && <> · <Text as="span">salió del cajón</Text></>}
              </Text>
            </Box>
          ))}
        </Box>
      )}
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
  counts?: ConteosDelTurno | null;
  drawer?: ArqueoDelCajon | null;
  voidedPayments?: VoidedPayment[];
  refunds?: SessionRefund[];
}

// Resumen del corte reutilizable (histórico y panel lateral): jerarquía + conciliación + drill-down.
// `abierto`: el turno no se ha cerrado, así que no hay nada declarado y la conciliación no se pinta —
// «Declarado $0 · Dif. $0» se leería como un corte cuadrado (spec 029).
export function CorteSummary({ data, abierto = false }: { data: CorteData; abierto?: boolean }) {
  const horaNegocio = useHoraDelNegocio();
  const cur = data.currency;
  const totals = data.totals ?? [];
  const movements = data.movements ?? [];
  const expenses = data.expenses ?? [];
  return (
    <VStack align="stretch" gap={4}>
      <IngresosEgresosCard openingCash={data.openingCash} breakdown={data.breakdown} currency={cur} />
      {totals.length > 0 && !abierto && (
        <Section title="Conciliación (sistema vs declarado)">
          <TotalsTable totals={totals} currency={cur} withTotalRow drawerDifference={data.drawer?.difference} />
        </Section>
      )}
      <ArqueoDelCorte drawer={data.drawer} currency={cur} />
      <DesgloseDelConteo counts={data.counts} currency={cur} />
      <Plegable title={`Movimientos de efectivo (${movements.filter((m) => m.expenseId === null).length})`}>
        <MovementsTable movements={movements} currency={cur} zona={horaNegocio.zona} />
      </Plegable>
      {expenses.length > 0 && (
        <Plegable title={`Gastos (${expenses.length})`}>
          <ExpensesTable expenses={expenses} currency={cur} />
        </Plegable>
      )}
      <VoidedPaymentsList payments={data.voidedPayments} currency={cur} zona={horaNegocio.zona} />
      <RefundsList refunds={data.refunds} currency={cur} zona={horaNegocio.zona} />
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
      <CorteSummary data={data} abierto={!data.closedAt} />
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
          {/* 44 px: medían 40. */}
          <Tabs.Trigger value="operar" minH="44px">Cajas</Tabs.Trigger>
          <Tabs.Trigger value="historico" minH="44px">Histórico</Tabs.Trigger>
          {canManage && <Tabs.Trigger value="gestion" minH="44px">Administrar</Tabs.Trigger>}
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

// ArqueoDelCorte: lo que el cajón debía tener, lo que se contó y la diferencia — una sola.
//
// Sin esto, un corte cerrado con faltante se ve CUADRADO: desde la spec 015 los métodos que
// comparten el cajón guardan su declarado igual a su esperado, así que la tabla por método reporta
// cero en todos y el faltante vive solo aquí. Es la pantalla donde alguien audita.
//
// Nil = corte anterior a la 015, o una caja que no maneja efectivo. No se pinta nada y no se
// inventa un cero.
export function ArqueoDelCorte({ drawer, currency }: {
  drawer?: ArqueoDelCajon | null; currency: string;
}) {
  if (!drawer || drawer.counted === null) return null;
  const dif = drawer.difference;
  return (
    <Box borderWidth="1px" borderRadius="lg" p={3} bg="bg.panel">
      <Text fontWeight="700" mb={2}>Arqueo del cajón</Text>
      <SimpleGrid columns={3} gap={2} fontSize="sm">
        <Text color="fg.muted">Esperado</Text>
        <Text color="fg.muted">Contado</Text>
        <Text color="fg.muted">Diferencia</Text>
        <Text aria-label="Esperado del cajón">{montoOSinDato(drawer.expected, currency)}</Text>
        <Text aria-label="Contado del cajón" fontWeight="600">{money(drawer.counted, currency)}</Text>
        <Text aria-label="Diferencia del arqueo del corte" fontWeight="700"
          color={dif === null ? 'fg.muted' : diffColor(dif)}>
          {montoOSinDato(dif, currency)}
        </Text>
      </SimpleGrid>
    </Box>
  );
}

// DesgloseDelConteo: cuántas piezas de cada denominación se declararon, o por qué no se contó.
//
// Es la razón de guardar el desglose (US3): un corte con faltante y sin él es un número sin
// historia, y no hay forma de distinguir "faltan dos billetes de $500" de "falta dinero". Eso costó
// un turno con $1,662 que nadie pudo explicar.
//
// UN CORTE ANTERIOR A ESTA FUNCIONALIDAD NO PINTA NADA, sin avisos ni explicaciones: son todos los
// que ya existen, y decirle al operador que ese corte "no tiene desglose" es contarle una historia
// del sistema que él no puede accionar.
export function DesgloseDelConteo({ counts, currency }: {
  counts?: ConteosDelTurno | null; currency: string;
}) {
  const momentos = ([['apertura', 'Al abrir'], ['cierre', 'Al cerrar']] as const)
    .map(([k, titulo]) => ({ titulo, conteo: counts?.[k] ?? null }))
    .filter((m) => m.conteo !== null);
  if (momentos.length === 0) return null;
  return (
    <Plegable title="Efectivo contado">
      <VStack align="stretch" gap={4}>
        {momentos.map(({ titulo, conteo }) => (
          <Box key={titulo}>
            <HStack justify="space-between" mb={1}>
              <Text fontWeight="700" fontSize="sm">{titulo}</Text>
              <Text fontWeight="700">{money(conteo!.total, currency)}</Text>
            </HStack>
            {conteo!.manualReason !== null ? (
              // Sin piezas: lo que hay es el porqué, y es lo que vuelve auditable este arqueo.
              <Text fontSize="sm" color="fg.muted">Capturado a mano: {conteo!.manualReason}</Text>
            ) : (
              <Table.Root size="sm">
                <Table.Header><Table.Row>
                  <Table.ColumnHeader>Denominación</Table.ColumnHeader>
                  <Table.ColumnHeader textAlign="end">Piezas</Table.ColumnHeader>
                  <Table.ColumnHeader textAlign="end">Subtotal</Table.ColumnHeader>
                </Table.Row></Table.Header>
                <Table.Body>
                  {conteo!.lines.map((l) => (
                    <Table.Row key={l.value}>
                      <Table.Cell>{etiquetaDeDenominacion(l.value, currency)}</Table.Cell>
                      <Table.Cell textAlign="end">{l.pieces}</Table.Cell>
                      {/* Del servidor: quien lee esto está comparando contra su cajón. */}
                      <Table.Cell textAlign="end">{money(l.subtotal, currency)}</Table.Cell>
                    </Table.Row>
                  ))}
                </Table.Body>
              </Table.Root>
            )}
          </Box>
        ))}
      </VStack>
    </Plegable>
  );
}

// Las piezas de menos de un peso se nombran en centavos: "$0.5" no es como se llama esa moneda.
function etiquetaDeDenominacion(value: string, currency: string): string {
  const n = Number(value);
  return n < 1 ? `${Math.round(n * 100)}¢` : money(n, currency);
}

// montoOSinDato: un importe que puede no viajar.
//
// Con el arqueo ciego encendido el esperado llega en null —lo que la pantalla no debe mostrar no se
// le manda— y una raya es lo honesto. Pintar un cero diría que no se espera nada.
function montoOSinDato(v: string | null, currency: string): string {
  return v === null ? '—' : money(v, currency);
}

// TablaDelCierre: qué se espera, qué se declaró y CUÁNTO FALTA (spec 015).
//
// EL CAJÓN ES UN RENGLÓN, NO CUATRO. Los métodos cuyo dinero cae en el mismo montón de billetes
// —el efectivo del mostrador y los de plataforma en efectivo— se siguen listando porque el corte
// informa por canal, pero sin campo y sin diferencia propia: un turno real quedó con «Efectivo» en
// $0.00 y «Didi efectivo» en −$64.80, dos diferencias del mismo dinero, y nadie podía saber cuál
// era el faltante real.
//
// El tercer estado de la columna «Declarado» dice **«Va al cajón»** y NO reusa «Automático»: un
// método auto-declarado lo resuelve el servidor, y uno de cajón se cuenta físicamente. Confundirlos
// es la clase de ambigüedad que ya costó $4,500.
export function TablaDelCierre({ totals, currency, declared, onDeclared, cajon, conteo, onContar, diferencias }: {
  totals: MethodTotal[];
  currency: string;
  declared: Record<string, string>;
  onDeclared: (d: Record<string, string>) => void;
  cajon: ArqueoDelCajon | null;
  conteo: ResultadoDelConteo | null;
  onContar: () => void;
  diferencias: DiferenciasDelCierre;
}) {
  const delCajon = new Set(cajon?.methodIds ?? []);
  const difCajon = diferenciaDelCajon(cajon, conteo);
  // ARQUEO CIEGO: si el servidor no mandó ningún esperado, la columna entera sobra. No es solo que
  // no haya cifras — a 1024×600 el ancho que libera se lo devuelve a lo que el operador vino a
  // leer, y una columna de rayas invita a preguntarse qué se rompió.
  const seVeElEsperado = totals.some((t) => t.expected !== null) || (cajon?.expected ?? null) !== null;
  return (
    <Box>
      <Text fontWeight="700" mb={2}>Cierre — declarado por método</Text>
      <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" overflow="hidden">
        <Table.Root size="sm">
          <Table.Header><Table.Row>
            <Table.ColumnHeader>Método</Table.ColumnHeader>
            {seVeElEsperado && <Table.ColumnHeader textAlign="end">Esperado</Table.ColumnHeader>}
            <Table.ColumnHeader>Declarado</Table.ColumnHeader>
            <Table.ColumnHeader textAlign="end">Diferencia</Table.ColumnHeader>
          </Table.Row></Table.Header>
          <Table.Body>
            {/* EL CAJÓN, PRIMERO Y UNA SOLA VEZ: es lo que el operador tiene enfrente. */}
            {cajon && (
              <Table.Row bg="bg.subtle">
                <Table.Cell fontWeight="700">Cajón</Table.Cell>
                {seVeElEsperado && (
                  <Table.Cell textAlign="end" fontWeight="700">{montoOSinDato(cajon.expected, currency)}</Table.Cell>
                )}
                <Table.Cell>
                  <Button size="sm" minH="44px" variant={conteo ? 'outline' : 'solid'} onClick={onContar}>
                    {conteo ? money(conteo.total, currency) : 'Contar efectivo'}
                  </Button>
                </Table.Cell>
                <Table.Cell textAlign="end" color={difCajon === undefined ? 'fg.muted' : diffColor(String(difCajon))}
                  fontWeight="700" aria-label="Diferencia del cajón">
                  {difCajon === undefined ? '—' : money(difCajon, currency)}
                </Table.Cell>
              </Table.Row>
            )}
            {totals.map((t) => {
              const dif = diferencias.porMetodo[t.methodId];
              const enElCajon = delCajon.has(t.methodId);
              return (
                <Table.Row key={t.methodId}>
                  <Table.Cell color={enElCajon ? 'fg.muted' : undefined}>{t.name}</Table.Cell>
                  {seVeElEsperado && (
                    <Table.Cell textAlign="end">{montoOSinDato(t.expected, currency)}</Table.Cell>
                  )}
                  <Table.Cell>
                    {enElCajon ? (
                      // Ni campo ni botón: su dinero ya se contó arriba, con el cajón entero.
                      <Text fontSize="sm" color="fg.muted">Va al cajón</Text>
                    ) : t.autoDeclare ? (
                      <Text fontSize="sm" color="fg.muted">Automático</Text>
                    ) : (
                      <Input size="sm" minH="44px" w="130px" inputMode="decimal" placeholder="0"
                        aria-label={`Declarado de ${t.name}`}
                        value={declared[t.methodId] ?? ''}
                        onChange={(e) => onDeclared({ ...declared, [t.methodId]: e.target.value })} />
                    )}
                  </Table.Cell>
                  <Table.Cell textAlign="end" color={dif === undefined ? 'fg.muted' : diffColor(String(dif))}
                    fontWeight={dif ? '700' : undefined} aria-label={`Diferencia de ${t.name}`}>
                    {dif === undefined ? '—' : money(dif, currency)}
                  </Table.Cell>
                </Table.Row>
              );
            })}
          </Table.Body>
        </Table.Root>
      </Box>
    </Box>
  );
}

// DiferenciaDelCierre: lo que el arqueo va a reportar, ANTES de confirmarlo.
//
// Va junto al botón de cerrar y con el mismo peso visual que el aviso de "falta por contar": las dos
// cosas deciden si el operador debe tocar el botón o volver a contar.
export function DiferenciaDelCierre({ diferencias, cajon, conteo, currency }: {
  diferencias: DiferenciasDelCierre;
  cajon: ArqueoDelCajon | null;
  conteo: ResultadoDelConteo | null;
  currency: string;
}) {
  // Mientras falte capturar algo, la cifra sería parcial. Un número que se lee como el resultado del
  // arqueo sin serlo manda a buscar dinero que sí está: el aviso de lo que falta ya está arriba.
  if (!diferencias.completo) return null;
  const difCajon = diferenciaDelCajon(cajon, conteo);
  // Con el cajón sin contar —o con el arqueo ciego, donde el esperado no viaja— no hay total que
  // mostrar. Sumar cero por el cajón diría que cuadra, y eso nadie lo comprobó.
  if (cajon !== null && difCajon === undefined) return null;
  const total = round2(diferencias.total + (difCajon ?? 0));
  const cuadra = Math.abs(total) < 0.005;
  return (
    <Box borderWidth="1px" borderRadius="lg" p={3} bg="bg.panel"
      borderColor={cuadra ? 'border' : 'red.400'}>
      <HStack justify="space-between" flexWrap="wrap" gap={2}>
        <Text fontWeight="700">{cuadra ? 'El arqueo cuadra' : (total < 0 ? 'Faltante' : 'Sobrante')}</Text>
        <Text fontSize="xl" fontWeight="700" color={diffColor(String(total))}
          aria-label="Diferencia del arqueo">
          {money(total, currency)}
        </Text>
      </HStack>
    </Box>
  );
}

// aperturaDelConteo traduce lo que entrega la hoja al cuerpo que espera el endpoint.
//
// La hoja no habla de `openingCash` a propósito: la misma sirve para abrir y para cerrar, y cada
// uno manda el efectivo en un campo distinto. La unión discriminada es la que impide mandar los dos
// caminos juntos, que es un 400 con el cajón ya contado.
function aperturaDelConteo(r: ResultadoDelConteo): AperturaInput {
  return 'counts' in r ? { counts: r.counts } : { openingCash: r.total, manualReason: r.manualReason };
}

// ---- Panel de una caja: abrir (si cerrada) u operar/cerrar (si abierta) ----
function RegisterPanel({ register, openRegisters }: { register: CashRegister; openRegisters: CashRegister[] }) {
  const horaNegocio = useHoraDelNegocio();
  const qc = useQueryClient();
  const { data: session, isLoading } = useQuery({
    queryKey: ['cash', 'current', register.id],
    queryFn: () => backofficeApi.cashCurrent(register.id),
  });
  const [contando, setContando] = useState(false);
  const [declared, setDeclared] = useState<Record<string, string>>({});
  // El conteo del cajón para ESTE cierre, tal como lo entregó la hoja. Vive aquí y no en `declared`
  // porque no es una cifra tecleada: es el arqueo, con sus renglones o con su motivo.
  const [conteoDelCierre, setConteoDelCierre] = useState<ResultadoDelConteo | null>(null);
  const cajon = session?.drawer ?? null;
  // Lo que todavía falta capturar. Las dos reglas viven fuera del componente y con test propio:
  // son las que evitan registrar un faltante inventado, y ese fallo ya costó un corte con $1,662.
  //
  // QUIÉN EXIGE QUÉ LO DICE EL SERVIDOR (`requiresEntry`, `requiresCount`). Deducirlo de si el
  // esperado es cero se rompe con el arqueo ciego, donde el esperado no viaja: `Number(null)` es 0
  // y la pantalla concluiría que no falta nada.
  const porContar = faltanPorContar(session?.totals ?? [], declared);
  const faltaElCajon = faltaContarElCajon(cajon, conteoDelCierre);
  // La diferencia en vivo, con la MISMA cifra que se va a mandar: un resumen que se derive de otro
  // predicado que el cierre miente y quien lo lee no tiene cómo saber cuál de los dos.
  const declaradoPorMetodo: Record<number, number | undefined> = {};
  for (const t of session?.totals ?? []) {
    declaradoPorMetodo[t.methodId] = montoTecleado(declared[t.methodId] ?? '');
  }
  const diferencias = diferenciasDelCierre(session?.totals ?? [], declaradoPorMetodo);
  const [notes, setNotes] = useState('');
  const [closed, setClosed] = useState<CashSession | null>(null); // resumen tras cerrar
  const [transferOpen, setTransferOpen] = useState(false);
  const [repartiendo, setRepartiendo] = useState(false);
  const [decisionPropinas, setDecisionPropinas] = useState<TipsDecision | null>(null);
  const faltaDecidir = faltaDecidirPropinas(session?.tipsPending, decisionPropinas);

  const invalidate = () => qc.invalidateQueries({ queryKey: ['cash'] });
  const navigate = useNavigate();
  // Descartar una cuenta que se capturaba y nadie mandó. No bloquea el cierre, pero quien cierra
  // puede limpiarla desde aquí (D-8).
  const descartarDelCierre = async (draftId: string, version: number) => {
    try {
      await descartarCuenta(qc, draftId, version);
    } catch (e) {
      toaster.create({ title: 'No se pudo descartar', description: mensajeDeError(e), type: 'error' });
    }
    invalidate();
    qc.invalidateQueries({ queryKey: ['pos', 'accounts'] });
  };
  // Cancelar un entregado que debe y no pagó nada («se fue sin pagar»): la otra salida del cierre
  // sin fiados, además de cobrarlo en su cuenta.
  const cancelarDelCierre = async (orderId: number, motivo: string, resto = false) => {
    try {
      await (resto ? posApi.writeOffOrder(orderId, motivo) : posApi.cancelOrder(orderId, motivo));
    } catch (e) {
      toaster.create({ title: 'No se pudo cancelar', description: mensajeDeError(e), type: 'error' });
    }
    invalidate();
    qc.invalidateQueries({ queryKey: ['pos', 'accounts'] });
  };
  const openMut = useMutation({
    mutationFn: (apertura: AperturaInput) => backofficeApi.cashOpen(register.id, apertura),
    // Esto abre el TURNO. Medía «contar-efectivo» y era falso por partida doble: `abrir-turno`
    // —que existe en la lista blanca— nunca se disparaba, y el conteo del cierre no se contaba en
    // ningún lado. Dos ceros permanentes que se leen como «nadie lo hace».
    onSuccess: () => { medirAccion('caja', 'abrir-turno'); setContando(false); invalidate(); },
    onError: (e) => toaster.create({ title: 'No se pudo abrir la caja', description: String(e), type: 'error' }),
  });
  const payoutMut = useMutation({
    mutationFn: (input: TipPayoutInput) => backofficeApi.cashTipPayout(register.id, input),
    onSuccess: (r) => {
      setRepartiendo(false); invalidate();
      toaster.create({ title: r.items.length === 1 ? `Propina entregada a ${r.items[0].recipientName}`
        : `Propina repartida entre ${r.items.length} personas`, type: 'success' });
    },
    onError: (e) => toaster.create({ title: 'No se pudo entregar la propina', description: mensajeDeError(e), type: 'error' }),
  });
  const closeMut = useMutation({
    mutationFn: () => {
      // `declared` lleva SOLO lo que no está en el cajón. Un método de cajón aquí lo rechaza el
      // servidor nombrándolo, y llegar a ese rechazo con el cajón ya contado cuesta recontarlo.
      const delCajon = new Set(cajon?.methodIds ?? []);
      const d: Record<string, number> = {};
      Object.entries(declared).forEach(([k, v]) => {
        if (delCajon.has(Number(k))) return;
        d[k] = montoTecleado(v) ?? 0;
      });
      const porPiezas = conteoDelCierre !== null && 'counts' in conteoDelCierre ? conteoDelCierre : null;
      const aMano = conteoDelCierre !== null && !('counts' in conteoDelCierre) ? conteoDelCierre : null;
      return backofficeApi.cashClose(register.id, d, {
        counts: porPiezas?.counts,
        // El camino manual del cajón va en su propio campo, como `openingCash` en la apertura.
        countedCash: aMano?.total,
        manualReason: aMano?.manualReason,
        notes: notes || undefined,
        tipsDecision: Number(session?.tipsPending?.total ?? 0) >= 1 ? decisionPropinas ?? undefined : undefined,
      });
    },
    onSuccess: (s) => {
      medirAccion('caja', 'cerrar-turno');
      setClosed(s); setDeclared({}); setConteoDelCierre(null); setNotes(''); setDecisionPropinas(null); invalidate();
    },
    // El servidor distingue "hay pedidos sin terminar" de cualquier otro fallo y manda los folios
    // en el mensaje. Se pinta con su propio título porque no es un error del cierre: es una tarea
    // pendiente, y el operador tiene que saber que la puede resolver y volver.
    onError: (e) => toaster.create({
      title: e instanceof ApiError && e.code === 'OPEN_ORDERS' ? 'Faltan pedidos por terminar'
        : e instanceof ApiError && e.code === 'UNPAID_ORDERS' ? 'Faltan pedidos por cobrar' : 'No se pudo cerrar la caja',
      description: e instanceof ApiError ? e.message : String(e),
      type: 'error',
      duration: 8000,
      // Un CONFLICT al cerrar es "alguien más ya cerró o ya guardó su conteo", y las dos tabletas
      // comparten cuenta: pasa en un turno normal. Lo accionable es traer el corte que sí quedó, no
      // volver a intentar sobre un turno que ya no está abierto.
      action: e instanceof ApiError && e.code === 'CONFLICT'
        ? { label: 'Recargar', onClick: () => invalidate() }
        : undefined,
    }),
  });

  if (isLoading) return <Center h="30vh"><Spinner size="xl" /></Center>;

  return (
    <>
      {!session ? (
        <VStack align="stretch" gap={4} bg="bg.panel" p={6} borderRadius="lg" borderWidth="1px" maxW="420px">
          <Text fontWeight="600">«{register.name}» está cerrada.</Text>
          <Text fontSize="sm" color="fg.muted">Cuenta el efectivo con el que arranca el cajón.</Text>
          {/* El conteo NO va aquí. Medido a 1024×600, esta pantalla mide 1,494 px de alto: una
              rejilla de once denominaciones en este punto nace 200 px debajo del fold. */}
          <Button minH="52px" onClick={() => setContando(true)} loading={openMut.isPending}>
            Contar el efectivo
          </Button>
        </VStack>
      ) : (
        <VStack align="stretch" gap={5}>
          <HStack justify="space-between" flexWrap="wrap" gap={2}>
            <Text fontWeight="700">{register.name}{session.isPrimary ? ' · recibe ventas' : ''}</Text>
            <Button size="sm" minH="44px" variant="outline" onClick={() => setTransferOpen(true)}
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

          {/* CONTANDO A CIEGAS NO SE PINTA EL RESUMEN, y no basta con que venga vacío: el
              desglose por método más el fondo y el neto reconstruyen el esperado exacto sumando
              cuatro renglones contiguos, que es justo lo que el ajuste viene a esconder. Se dice
              cuándo vuelve, para que no se lea como una pantalla rota. */}
          <Section title="Resumen del corte">
            {session.blind ? (
              <Text fontSize="sm" color="fg.muted">
                Aparece al confirmar el cierre, junto con la diferencia.
              </Text>
            ) : (
              <IngresosEgresosCard openingCash={session.openingCash} breakdown={session.breakdown} currency={session.currency} />
            )}
          </Section>

          {Number(session.tipsPending?.total ?? 0) >= 1 && (
            <HStack borderWidth="1px" borderRadius="lg" p={3} justify="space-between" flexWrap="wrap" gap={2}>
              <Box>
                <Text fontWeight="700">Propinas por entregar: {money(session.tipsPending?.total ?? '0', session.currency)}</Text>
                {Number(session.cardTipsPaidInCash ?? 0) > 0 && (
                  <Text fontSize="sm" color="fg.muted">
                    Propina de tarjeta pagada en efectivo: {money(session.cardTipsPaidInCash ?? '0', session.currency)}
                  </Text>
                )}
              </Box>
              <Button minH="52px" colorPalette="orange"
                onClick={() => setRepartiendo(true)}>
                Entregar propina
              </Button>
            </HStack>
          )}

          <MovementsPanel session={session} />

          {(session.expenses ?? []).length > 0 && (
            <Section title="Gastos del corte">
              <ExpensesTable expenses={session.expenses ?? []} currency={session.currency} />
            </Section>
          )}

          <VoidedPaymentsList payments={session.voidedPayments} currency={session.currency} zona={horaNegocio.zona} />

          <TablaDelCierre totals={session.totals ?? []} currency={session.currency}
            declared={declared} onDeclared={setDeclared}
            cajon={cajon} conteo={conteoDelCierre} onContar={() => setContando(true)}
            diferencias={diferencias} />

          {/* Después de la tabla del cierre y no antes: es lo que se viene a llenar, y la lista
              abierta la empujaría fuera de los 600 px de la tableta. */}
          <RefundsList refunds={session.refunds} currency={session.currency} zona={horaNegocio.zona} />

          <Textarea rows={2} resize="none" placeholder="Notas del cierre (opcional)"
            value={notes} onChange={(e) => setNotes(e.target.value)} />

          {/* Un campo en blanco se guardaba como cero declarado y quedaba registrado un faltante
              que no existía: pasó con un corte real de $1,662. Escribir 0 sigue siendo válido —
              puede no haber efectivo—; lo que no vale es dejarlo vacío. */}
          {(porContar.length > 0 || faltaElCajon) && (
            <Box borderWidth="1px" borderColor="border" borderRadius="lg" p={3} colorPalette="orange" bg="colorPalette.subtle">
              <Text fontSize="sm" fontWeight="600">
                Falta capturar lo contado en: {[...(faltaElCajon ? ['el cajón'] : []), ...porContar.map((m) => m.name)].join(', ')}
              </Text>
            </Box>
          )}

          {/* Quién cobró qué. Con dos estaciones contra el mismo cajón, es lo único que separa la
              responsabilidad: partir la caja daría dos arqueos contando el mismo dinero. Solo se
              pinta si hubo más de una persona — con una sola, repite el total de arriba. */}
          {/* Con el arqueo ciego tampoco: lo cobrado en efectivo por cada persona suma lo mismo
              que el desglose por método. */}
          {!session.blind && session.cashiers.length > 1 && (
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

          {/* Lo que falta por entregar (bloquea) y las cuentas que siguen vivas (no bloquean), ANTES
              de intentar cerrar: antes solo se sabía al presionar el botón y recibir el error. */}
          <CuentasDelCierre pending={session.pending} owing={session.owing ?? []} cuentas={session.liveAccounts ?? []}
            onAbrir={(ruta) => navigate(ruta)} onDescartar={descartarDelCierre} onCancelar={cancelarDelCierre}
            onCancelarResto={(id, motivo) => cancelarDelCierre(id, motivo, true)} />

          {/* Lo que se vendió y nadie pagó. NO bloquea el cierre —fiar o cobrar por fuera son
              decisiones del negocio— pero el arqueo tiene que decirlo: solo compara pagos contra
              declarado, así que sin esta línea un turno con ventas sin cobrar cierra en cero y el
              faltante solo aparece restando dos cifras de dos pantallas. */}
          {Number(session.uncollected) > 0 && (
            <Box borderWidth="1px" borderColor="red.300" bg="red.50"
              _dark={{ bg: 'red.950' }} borderRadius="lg" p={3}>
              <Text fontWeight="700" color="red.700" _dark={{ color: 'red.200' }}>
                Sin cobrar: {money(session.uncollected)}
                {' en '}
                {session.uncollectedCount === 1 ? '1 pedido' : `${session.uncollectedCount} pedidos`}
              </Text>
              <Text fontSize="sm" color="fg.muted">
                Este dinero no está en el arqueo. Cóbralo antes de cerrar o el corte va a cuadrar
                sin él.
              </Text>
            </Box>
          )}

          {/* Lo dado por perdido («cancelar lo que falta»): ni cobrado ni sin cobrar. */}
          {Number(session.writtenOff ?? 0) > 0 && (
            <Text fontSize="sm" color="fg.muted">
              Perdido: {money(session.writtenOff ?? '0')} (se canceló lo que faltaba)
            </Text>
          )}

          <DiferenciaDelCierre diferencias={diferencias} cajon={cajon} conteo={conteoDelCierre}
            currency={session.currency} />

          <PropinasDelCierre pendiente={session.tipsPending} currency={session.currency} decision={decisionPropinas}
            onEntregarAhora={() => setRepartiendo(true)} onDecidir={setDecisionPropinas} />

          <BotonCerrarCaja nombre={register.name} loading={closeMut.isPending}
            disabled={porContar.length > 0 || faltaElCajon || faltaDecidir || session.pending.length > 0 || (session.owing?.length ?? 0) > 0}
            onCerrar={() => closeMut.mutate()} />
        </VStack>
      )}

      {!session && (
        <ContadorDeEfectivo isOpen={contando} onClose={() => setContando(false)}
          titulo={`Fondo inicial de «${register.name}»`}
          // La moneda del turno la fija el servidor con el default de la columna: hoy no hay forma
          // de elegir otra al abrir. Cuando la haya, ESTA línea es la que cambia.
          currency="MXN" etiquetaConfirmar="Abrir caja" guardando={openMut.isPending}
          // Contar el cajón se mide aquí, donde de verdad ocurre, y no en el onSuccess de la
          // mutación: el de abrir mide «abrir-turno», que es otro hecho.
          onConfirmar={(r) => { medirAccion('caja', 'contar-efectivo'); openMut.mutate(aperturaDelConteo(r)); }} />
      )}

      {session && (
        <ContadorDeEfectivo isOpen={contando} onClose={() => setContando(false)}
          titulo={`Efectivo en «${register.name}»`} currency={session.currency}
          etiquetaConfirmar="Usar este conteo" guardando={false}
          onConfirmar={(r) => { medirAccion('caja', 'contar-efectivo'); setConteoDelCierre(r); setContando(false); }} />
      )}

      {session && (
        <RepartirPropinas isOpen={repartiendo} pendiente={session.tipsPending ?? null} currency={session.currency}
          guardando={payoutMut.isPending} onEntregar={(i) => payoutMut.mutate(i)} onClose={() => setRepartiendo(false)} />
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
export function MovementsPanel({ session }: { session: CashSession }) {
  const horaNegocio = useHoraDelNegocio();
  const qc = useQueryClient();
  const [kind, setKind] = useState<'entrada' | 'salida'>('salida');
  const [amount, setAmount] = useState('');
  const [concept, setConcept] = useState('');

  const mut = useMutation({
    mutationFn: () => backofficeApi.cashMovement(session.registerId, kind, montoTecleado(amount) ?? 0, concept.trim()),
    onSuccess: () => {
      medirAccion('caja', 'traspaso');
      setAmount(''); setConcept(''); qc.invalidateQueries({ queryKey: ['cash'] });
    },
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
          <Input size="sm" minH="44px" w="120px" type="number" inputMode="decimal" placeholder="Monto"
            value={amount} onChange={(e) => setAmount(e.target.value)} />
          <Input size="sm" minH="44px" flex="1" minW="140px" placeholder="Concepto (ej. pago proveedor)"
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
                <Table.Cell textAlign="end"><Button size="sm" minH="44px" variant="outline" onClick={() => setEdit(r)}>Editar</Button></Table.Cell>
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
const CORTES_POR_PAGINA = 20;

function HistoryTab() {
  const horaNegocio = useHoraDelNegocio();
  const [page, setPage] = useState(0);
  const [rango, setRango] = useState({ desde: '', hasta: '' });
  const { data, isLoading } = useQuery({
    queryKey: ['cash', 'history', page, rango.desde, rango.hasta],
    queryFn: () => backofficeApi.cashHistory({ page, pageSize: CORTES_POR_PAGINA, from: rango.desde, to: rango.hasta }),
    placeholderData: (prev) => prev,
  });
  const [detailId, setDetailId] = useState<number | null>(null);
  // Panel lateral solo en pantallas anchas (xl+); en tablet de 7" se usa el diálogo a pantalla completa.
  const wide = useBreakpointValue({ base: false, xl: true }, { ssr: false });

  if (isLoading) return <Center h="40vh"><Spinner size="xl" /></Center>;
  const rows = data?.items ?? [];
  const filtrando = rango.desde !== '' || rango.hasta !== '';
  if (rows.length === 0 && !filtrando && page === 0) return <Text color="fg.muted">Aún no hay cortes registrados.</Text>;

  const controles = (
    <ControlesDelHistorico page={page} total={data?.total ?? 0} pageSize={CORTES_POR_PAGINA}
      desde={rango.desde} hasta={rango.hasta} hoy={horaNegocio.diaDelNegocio(new Date())}
      onPage={setPage} onRango={(desde, hasta) => { setRango({ desde, hasta }); setPage(0); }} />
  );

  const list = rows.length === 0 ? (
    <Text color="fg.muted" py={4}>{filtrando ? 'No hay cortes en esas fechas.' : 'No hay más cortes.'}</Text>
  ) : (
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
                <Button size="sm" minH="44px" px={4} variant="outline" onClick={(e) => { e.stopPropagation(); setDetailId(r.id); }}>Ver</Button>
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
        <VStack flex="1.1" minW={0} align="stretch" gap={2}>{controles}{list}</VStack>
        <Box flex="1" minW={0} bg="bg.panel" borderRadius="lg" borderWidth="1px" p={4} maxH="calc(100dvh - 220px)" overflowY="auto">
          {detailId
            ? <CorteDetail id={detailId} />
            : <Center py={12}><Text color="fg.muted">Selecciona un corte para ver el detalle.</Text></Center>}
        </Box>
      </HStack>
    );
  }
  return (
    <VStack align="stretch" gap={2}>
      {controles}
      {list}
      <SessionDetailDialog id={detailId} onClose={() => setDetailId(null)} />
    </VStack>
  );
}

// Los controles del histórico: un rango opcional de días y la página. Van ARRIBA de la lista y en un
// solo renglón: a 1024×600 abajo quedaban fuera de la pantalla, y cada renglón extra se le quita a
// la lista que se vino a leer. Las fechas son `input type="date"` como en Ventas (el calendario del
// sistema a pantalla completa, no el desplegable de renglones de 20 px que prohíbe la constitución).
export function ControlesDelHistorico({ page, total, pageSize, desde, hasta, hoy, onPage, onRango }: {
  page: number; total: number; pageSize: number; desde: string; hasta: string; hoy: string;
  onPage: (page: number) => void; onRango: (desde: string, hasta: string) => void;
}) {
  // Lo tecleado vive aquí hasta que es un rango válido: un rango al revés no se pide al servidor.
  const [borrador, setBorrador] = useState({ desde, hasta });
  const alReves = borrador.desde !== '' && borrador.hasta !== '' && borrador.desde > borrador.hasta;
  const cambiar = (d: string, h: string) => {
    setBorrador({ desde: d, hasta: h });
    if (!(d !== '' && h !== '' && d > h)) onRango(d, h);
  };
  const paginas = Math.max(1, Math.ceil(total / pageSize));
  return (
    <HStack gap={2} flexWrap="wrap" justify="space-between">
      <HStack gap={2} flexWrap="wrap">
        <Input type="date" size="sm" minH="44px" w="150px" max={hoy} aria-label="Desde"
          value={borrador.desde} onChange={(e) => cambiar(e.target.value, borrador.hasta)} />
        <Text fontSize="sm" color="fg.muted">al</Text>
        <Input type="date" size="sm" minH="44px" w="150px" max={hoy} aria-label="Hasta"
          value={borrador.hasta} onChange={(e) => cambiar(borrador.desde, e.target.value)} />
        {(borrador.desde !== '' || borrador.hasta !== '' || desde !== '' || hasta !== '') && (
          <Button size="sm" minH="44px" variant="ghost" onClick={() => cambiar('', '')}>Quitar fechas</Button>
        )}
        {alReves && <Text fontSize="sm" color="fg.error" role="status">La fecha de inicio va después de la final.</Text>}
      </HStack>
      <HStack gap={1}>
        <IconButton size="sm" minH="44px" minW="44px" variant="outline" aria-label="Página anterior"
          disabled={page === 0} onClick={() => onPage(page - 1)}><LuChevronLeft /></IconButton>
        <Text fontSize="sm" minW="110px" textAlign="center">Página {page + 1} de {paginas}</Text>
        <IconButton size="sm" minH="44px" minW="44px" variant="outline" aria-label="Página siguiente"
          disabled={page + 1 >= paginas} onClick={() => onPage(page + 1)}><LuChevronRight /></IconButton>
      </HStack>
    </HStack>
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
  // Los hooks van ANTES de cualquier salida temprana: React exige el mismo orden en cada render, y
  // una guarda arriba los volvería condicionales.
  //
  // Las páginas que se han pedido además de la que trajo el detalle.
  const [extra, setExtra] = useState<CorteSale[]>([]);
  const [cargando, setCargando] = useState(false);

  // "El campo no vino" NO es "vino en cero". El front se despliega antes que el backend, y en esa
  // ventana un corte que sí cobró aparecería jurando que no cobró nada — una pantalla que miente
  // sobre dinero es peor que una pantalla incompleta. Sin el campo, la sección no se dibuja.
  const total = session.salesCount;
  const ventas = [...(session.sales ?? []), ...extra];

  const verMas = async () => {
    setCargando(true);
    try {
      // La página siguiente se calcula sobre lo que YA se tiene, no sobre un contador propio: si el
      // detalle cambiara de tamaño de página, un contador aparte empezaría a saltarse renglones.
      const pagina = Math.floor(ventas.length / VENTAS_POR_PAGINA);
      const r = await backofficeApi.cashSessionSales(session.id, pagina, VENTAS_POR_PAGINA);
      setExtra((prev) => [...prev, ...r.items]);
    } catch (e) {
      toaster.create({ title: 'No se pudieron traer más ventas', description: mensajeDeError(e), type: 'error' });
    } finally {
      setCargando(false);
    }
  };

  if (session.sales === undefined || total === undefined) return null;

  if (total === 0) {
    return (
      <Section title="Pedidos del corte">
        <Text fontSize="sm" color="fg.muted">Este corte no cobró ninguna venta.</Text>
      </Section>
    );
  }

  const recortadas = total > ventas.length;
  return (
    <Section title="Pedidos del corte">
      <Text fontSize="sm" color="fg.muted" mb={2}>
        {total === 1 ? '1 venta' : `${total} ventas`} · {money(session.salesTotal ?? '0', session.currency)}
        {' '}sin canceladas, reembolsadas ni propinas
        {/* Es lo VENDIDO, no lo cobrado: los ingresos de arriba son lo que entró (spec 029). */}
        {' '}· importe vendido, incluye lo que falta por cobrar
        {recortadas && ` · se muestran las ${ventas.length} más recientes`}
      </Text>
      {Number(session.writtenOff ?? 0) > 0 && (
        <Text fontSize="sm" color="fg.muted" mb={2}>
          Perdido: {money(session.writtenOff ?? '0', session.currency)} (se canceló lo que faltaba)
        </Text>
      )}
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
        {recortadas && (
          <Box p={2} borderTopWidth="1px">
            <Button size="sm" variant="outline" w="100%" minH="44px" loading={cargando} onClick={verMas}>
              Ver más ({total - ventas.length} restantes)
            </Button>
          </Box>
        )}
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

// LAS CUENTAS VIVAS EN EL CIERRE (spec 030, US8; D-10; lienzo V2-7).
//
// Arriba lo que BLOQUEA: los entregados que deben (no hay fiados, 2026-10-09) con «Cobrar» y
// «Cancelar», y los pedidos en cocina o listos con «Abrir». Debajo, plegado, lo que NO bloquea —las
// que se capturan y lo de plataforma—. Plegado porque en 600 px de alto la lista empujaba el botón de
// cerrar fuera de la pantalla.
export function CuentasDelCierre({ pending, owing = [], cuentas: todas, onAbrir, onDescartar, onCancelar, onCancelarResto }: {
  pending: PendingOrder[];
  // Los entregados que deben: bloquean (no hay fiados, 2026-10-09). Se cobran en su cuenta o se
  // cancelan con motivo; cancelar solo si no tienen pagos, como exige el servidor.
  owing?: OwingOrder[];
  cuentas: AccountItem[];
  onAbrir: (ruta: string) => void;
  onDescartar: (draftId: string, version: number) => void;
  onCancelar?: (orderId: number, motivo: string) => void;
  // «Cancelar lo que falta» del pagado a medias: lo pagado se queda y el resto se da por perdido.
  onCancelarResto?: (orderId: number, motivo: string) => void;
}) {
  const [abierta, setAbierta] = useState(false);
  const [descartando, setDescartando] = useState<AccountItem | null>(null);
  const [cancelando, setCancelando] = useState<OwingOrder | null>(null);
  const [perdiendo, setPerdiendo] = useState<OwingOrder | null>(null);
  // Lo que ya va arriba como bloqueo no se repite en la sección plegada.
  const deben = new Set(owing.map((o) => o.id));
  const cuentas = todas.filter((c) => c.orderId == null || !deben.has(c.orderId));
  if (pending.length === 0 && owing.length === 0 && cuentas.length === 0) return null;
  const nombre = nombreDeCuenta;
  const nombreDePedido = (o: { name: string; number: number }) => o.name || `#${o.number}`;
  return (
    <VStack align="stretch" gap={2}>
      {owing.length > 0 && (
        <Box borderWidth="1px" borderColor="red.300" bg="red.50"
          _dark={{ bg: 'red.950' }} borderRadius="lg" p={3}>
          <Text fontWeight="700" color="red.700" _dark={{ color: 'red.200' }} mb={1}>
            Falta cobrar {owing.length === 1 ? '1 pedido' : `${owing.length} pedidos`}
          </Text>
          <Text fontSize="sm" color="fg.muted" mb={2}>
            Ya se entregaron. La caja no cierra hasta que se cobren o se cancelen.
          </Text>
          <VStack align="stretch" gap={1} maxH="40dvh" overflowY="auto">
            {owing.map((o) => (
              <HStack key={o.id} justify="space-between" gap={2} flexWrap="wrap">
                <Text fontWeight="600" truncate flex="1" minW="8rem">{o.name ? `${o.name} · #${o.number}` : `#${o.number}`}</Text>
                <Text fontWeight="700" flexShrink={0}>{money(round2(Number(o.total) - Number(o.paid) - Number(o.writtenOff ?? 0)))}</Text>
                <Box flexShrink={0}>
                  <Button size="sm" minH="44px" colorPalette="orange" aria-label={`Cobrar ${nombreDePedido(o)}`}
                    onClick={() => onAbrir(`/pos?pedido=${o.id}`)}>Cobrar</Button>
                </Box>
                {onCancelar && Number(o.paid) === 0 && (
                  // Separado de «Cobrar»: es destructivo.
                  <Box flexShrink={0} pl={6}>
                    <Button size="sm" minH="44px" variant="ghost" colorPalette="red" aria-label={`Cancelar ${nombreDePedido(o)}`}
                      onClick={() => setCancelando(o)}>Cancelar</Button>
                  </Box>
                )}
                {onCancelarResto && Number(o.paid) > 0 && (
                  <Box flexShrink={0} pl={6}>
                    <Button size="sm" minH="44px" variant="ghost" colorPalette="red" aria-label={`Cancelar lo que falta de ${nombreDePedido(o)}`}
                      onClick={() => setPerdiendo(o)}>Cancelar lo que falta</Button>
                  </Box>
                )}
              </HStack>
            ))}
          </VStack>
        </Box>
      )}
      {pending.length > 0 && (
        <Box borderWidth="1px" borderColor="orange.300" bg="orange.50"
          _dark={{ bg: 'orange.950' }} borderRadius="lg" p={3}>
          <Text fontWeight="700" color="orange.700" _dark={{ color: 'orange.200' }} mb={1}>
            Falta entregar {pending.length === 1 ? '1 pedido' : `${pending.length} pedidos`}
          </Text>
          <Text fontSize="sm" color="fg.muted" mb={2}>
            La caja no cierra hasta que salgan o se cancelen.
          </Text>
          <VStack align="stretch" gap={1}>
            {pending.map((o) => (
              <HStack key={o.number} justify="space-between" gap={2}>
                <Text fontWeight="600" truncate flex="1" minW={0}>{o.name ? `${o.name} · #${o.number}` : `#${o.number}`}</Text>
                {o.total !== undefined && <Text fontWeight="700" flexShrink={0}>{money(o.total)}</Text>}
                {o.id !== undefined && (
                  <Button size="sm" minH="44px" variant="outline" aria-label={`Abrir ${o.name || `#${o.number}`}`}
                    onClick={() => onAbrir(`/pos?pedido=${o.id}`)}>Abrir</Button>
                )}
              </HStack>
            ))}
          </VStack>
        </Box>
      )}
      {cuentas.length > 0 && (
        <Box borderWidth="1px" borderRadius="lg" p={2}>
          <Button variant="ghost" minH="44px" w="100%" justifyContent="space-between"
            onClick={() => setAbierta((v) => !v)}>
            <Text fontWeight="700">Cuentas pendientes ({cuentas.length})</Text>
            {abierta ? <LuChevronUp /> : <LuChevronDown />}
          </Button>
          {!abierta && (
            <Text fontSize="xs" color="fg.muted" px={4}>No impiden cerrar: siguen en la fila de cuentas.</Text>
          )}
          {abierta && (
            <VStack align="stretch" gap={1} maxH="40dvh" overflowY="auto" mt={1}>
              {cuentas.map((c) => (
                <HStack key={c.key} justify="space-between" gap={3} px={2} py={1} borderTopWidth="1px">
                  <Box minW={0} flex="1">
                    <Text fontWeight="600" truncate>{nombre(c)}</Text>
                    <Text fontSize="xs" color="fg.muted" truncate>
                      {c.number !== null ? `#${c.number} · ` : ''}{ESTADO[c.state].texto}
                    </Text>
                  </Box>
                  <Text fontWeight="700" flexShrink={0}>{money(c.state === 'capturing' ? c.total : c.outstanding)}</Text>
                  <Box flexShrink={0}>
                    <Button size="sm" minH="44px" variant="outline" aria-label={`Abrir ${nombre(c)}`}
                      onClick={() => onAbrir(c.kind === 'draft' ? `/pos?cuenta=${c.draftId}` : `/pos?pedido=${c.orderId}`)}>
                      Abrir
                    </Button>
                  </Box>
                  {c.kind === 'draft' && c.draftId && c.draftVersion != null && (
                    // ≥ 24 px de «Abrir»: es destructivo y la fila mide ~52 px.
                    <Box flexShrink={0} pl={6}>
                      <Button size="sm" minH="44px" variant="ghost" colorPalette="red" aria-label={`Descartar ${nombre(c)}`}
                        onClick={() => setDescartando(c)}>
                        Descartar
                      </Button>
                    </Box>
                  )}
                </HStack>
              ))}
            </VStack>
          )}
        </Box>
      )}
      <ReasonSheet isOpen={cancelando !== null} destructive required
        title={`¿Cancelar ${cancelando ? nombreDePedido(cancelando) : ''}?`}
        label="Motivo" placeholder="Ej. se fue sin pagar" confirmLabel="Cancelar pedido"
        onDone={(motivo) => {
          const o = cancelando;
          setCancelando(null);
          if (o && motivo) onCancelar?.(o.id, motivo);
        }} />
      <ReasonSheet isOpen={perdiendo !== null} destructive required
        title={`¿Cancelar lo que falta de ${perdiendo ? nombreDePedido(perdiendo) : ''}?`}
        label="Motivo" placeholder="Ej. se fue sin pagar" confirmLabel="Cancelar lo que falta"
        atajos={['Se fue sin pagar']}
        description={perdiendo ? `Se dan por perdidos ${money(round2(Number(perdiendo.total) - Number(perdiendo.paid) - Number(perdiendo.writtenOff ?? 0)))}. Lo cobrado (${money(perdiendo.paid)}) se queda como venta.` : undefined}
        onDone={(motivo) => {
          const o = perdiendo;
          setPerdiendo(null);
          if (o && motivo) onCancelarResto?.(o.id, motivo);
        }} />
      <ConfirmSheet isOpen={descartando !== null} destructive
        title={`¿Descartar la cuenta de ${descartando ? nombre(descartando) : ''}?`}
        description="No se ha mandado a cocina; su nombre vuelve a quedar libre."
        cancelLabel="Volver" confirmLabel="Descartar"
        onCancel={() => setDescartando(null)}
        onConfirm={() => {
          const c = descartando;
          setDescartando(null);
          if (c?.draftId && c.draftVersion != null) onDescartar(c.draftId, c.draftVersion);
        }} />
    </VStack>
  );
}

// «Cerrar caja» confirma en una hoja de la app, no con el `confirm()` del navegador: el del sistema
// se pinta fuera de la app con botones que en la tableta se aciertan al revés.
export function BotonCerrarCaja({ nombre, disabled, loading, onCerrar }: {
  nombre: string; disabled: boolean; loading: boolean; onCerrar: () => void;
}) {
  const [preguntando, setPreguntando] = useState(false);
  return (
    <>
      <Button colorPalette="red" size="lg" minH="52px" loading={loading} disabled={disabled}
        onClick={() => setPreguntando(true)}>
        Cerrar caja
      </Button>
      <ConfirmSheet isOpen={preguntando} title={`¿Cerrar «${nombre}»?`}
        description="No podrás modificarla después." cancelLabel="Volver" confirmLabel="Cerrar caja"
        onCancel={() => setPreguntando(false)}
        onConfirm={() => { setPreguntando(false); onCerrar(); }} />
    </>
  );
}
