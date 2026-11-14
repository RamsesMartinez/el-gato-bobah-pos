import { ApiError } from '../../api/client';
import { useState, Fragment, type ReactNode } from 'react';
import { DatePickerButton } from '../../components/DatePickerButton';
import { Box, Text, Button, VStack, HStack, Table, Center, Spinner, Badge, SimpleGrid, Grid, useBreakpointValue, IconButton } from '@chakra-ui/react';
import { ANCHO_PC, ANCHO_2K, desde } from '../../hooks/useAnchoMinimo';
import { LuChevronDown, LuChevronUp, LuChevronLeft, LuChevronRight } from 'react-icons/lu';
import { toaster } from '../../components/ui/toaster';
import { useQuery } from '@tanstack/react-query';
import { backofficeApi, type CashSessionDetail, type CorteSale, type CashMovement, type CashExpenseLine, type MethodTotal, type CorteBreakdown, type ConteosDelTurno, type ArqueoDelCajon, type VoidedPayment, type SessionRefund, type CutDiff, type CashSessionRow } from '../../api/backoffice';
import { Diferencia, ResumenDeDiferencia } from './diferencia';
import { montoDeCaja } from './montosDeCaja';
import { rotuloDeMotivo } from './motivosDeApertura';
import { etiquetaEstado } from '../../domain/etiquetasDeVenta';
import { diferenciaDelHistorico } from './historicoDeCaja';
import type { TerminalCount } from '../../api/backoffice';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';
import { DEFAULT_TIMEZONE } from '../../utils/zonaPorDefecto';
import { DialogRoot, DialogBackdrop, DialogContent, DialogBody, DialogHeader, DialogTitle, DialogCloseTrigger } from '../../components/ui/dialog';
import { mensajeDeError } from '../../api/mensajes';
import { ArqueoDelCorte, DesgloseDelConteo } from './arqueo';
import { TotalsTable } from './conciliacion';
import { MovementsTable, ExpensesTable } from './movimientos';
import { hhmm } from './piezasDeCaja';
import { CorreccionesDelCorte } from './CorreccionesDelCorte';

// Cuántas ventas trae cada "Ver más". El mismo tamaño de página que el resto de las listas: pedir
// de 200 en 200 no cabe en la caja y pedir de 5 en 5 obliga a diez toques para ver un día.
const VENTAS_POR_PAGINA = 20;

// Sección con título compacto para agrupar las tablas del resumen.
export function Section({ title, children }: { title: string; children: ReactNode }) {
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
        <Text fontSize="sm" fontWeight={weight} color={negative ? 'fg.error' : color} whiteSpace="nowrap"
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
      <SummaryLine label="Monto inicial" amount={montoDeCaja(openingCash, currency)} weight="600" />
      <SummaryLine label="Ingresos" amount={montoDeCaja(breakdown?.ingresosTotal ?? '0', currency)} weight="700" color="fg.success" top />
      {ingresos.map((m) => (
        <Fragment key={m.method}>
          <SummaryLine label={m.method} amount={montoDeCaja(m.total, currency)} indent={1} weight="600" />
          {m.note && <Text fontSize="xs" color="fg.muted" px={3} pl={8} pb={1}>{m.note}</Text>}
          {m.items.map((it) => (
            // Un concepto que resta (Devoluciones) se pinta como los egresos: en gris se leería como
            // otro ingreso.
            <SummaryLine key={it.concept} label={it.concept} amount={montoDeCaja(it.amount, currency)} indent={2} color="fg.muted"
              negative={Number(it.amount) < 0} />
          ))}
        </Fragment>
      ))}
      {ingresos.length === 0 && <SummaryLine label="Sin ingresos" indent={1} color="fg.muted" />}
      <SummaryLine label="Egresos" amount={`−${montoDeCaja(breakdown?.egresosTotal ?? '0', currency)}`} weight="700" color="fg.error" top />
      {egresos.map((it) => (
        <SummaryLine key={it.concept} label={it.concept} amount={`−${montoDeCaja(it.amount, currency)}`} indent={1} color="fg.muted" />
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
            <SummaryLine key={p.platform} label={p.platform} amount={montoDeCaja(p.total, currency)} indent={1} weight="600" />
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
                {Number(p.tip) > 0 && <Text color="fg.muted">+ propina {montoDeCaja(p.tip, currency)}</Text>}
                <Text fontWeight="600">{montoDeCaja(p.amount, currency)}</Text>
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
                  {Number(r.tip) > 0 && <Text color="fg.muted">+ propina {montoDeCaja(r.tip, currency)}</Text>}
                  <Text fontWeight="600">{montoDeCaja(r.amount, currency)}</Text>
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
  arqueoHidden?: boolean;
  difference?: CutDiff | null;
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
      {data.arqueoHidden ? (
        <Box borderWidth="1px" borderColor="border" borderRadius="lg" p={3}>
          <Text fontWeight="700">Arqueo oculto</Text>
          <Text fontSize="sm" color="fg.muted">Se ve cuando se vuelva a abrir la caja.</Text>
        </Box>
      ) : <IngresosEgresosCard openingCash={data.openingCash} breakdown={data.breakdown} currency={cur} />}
      {totals.length > 0 && !abierto && (
        <Section title="Conciliación">
          <TotalsTable totals={totals} currency={cur} withTotalRow drawerMethodIds={data.drawer?.methodIds} difference={data.difference} drawer={data.drawer} />
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

// Detalle completo de un corte (carga por id). Lo usan el diálogo (7") y el panel lateral (pantallas
// grandes). Arriba va la diferencia en sus tres partes y lo que la explica (Maquetas M1, opción A):
// es lo que faltó en el caso de la Caja B, donde la explicación existía y ninguna pantalla la
// mostraba (#290).
export function CorteDetail({ id, onAbrirCorte }: { id: number; onAbrirCorte?: (id: number) => void }) {
  const horaNegocio = useHoraDelNegocio();
  const { data, isLoading, isError } = useQuery({
    queryKey: ['cash', 'session', id], queryFn: () => backofficeApi.cashSession(id),
    // Un «no encontrado» no se arregla reintentando: tres reintentos eran segundos de spinner.
    retry: (n, e) => !(e instanceof ApiError && e.status === 404) && n < 3,
  });
  // Con la dirección, el id puede no existir o ser de otra empresa (404): el spinner se quedaba fijo.
  if (isError) return <Text color="fg.muted" py={4}>No encontramos ese corte.</Text>;
  if (isLoading || !data) return <Center py={8}><Spinner /></Center>;
  const cur = data.currency;
  return (
    // En 2K el detalle se reparte en dos columnas: resumen e historia a un lado, movimientos y ventas
    // al otro (#290, FR-036). Debajo de eso, una sola columna como siempre.
    <VStack align="stretch" gap={4} css={{ [desde(ANCHO_2K)]: { display: 'grid', gridTemplateColumns: 'repeat(2, minmax(0, 1fr))', alignItems: 'start' } }}>
      {data.difference && <ResumenDeDiferencia diferencia={data.difference} currency={cur} />}
      {data.needsExplanation && (
        <Text fontSize="sm" color="orange.fg" fontWeight="600">Por explicar: la diferencia pasa la tolerancia y nadie la ha explicado.</Text>
      )}
      {data.explainedByNext && (
        <Box borderWidth="1px" borderRadius="lg" p={3} colorPalette="blue" bg="colorPalette.subtle" maxW="70ch">
          <Text fontWeight="700">Diferencia explicada al abrir el turno #{data.explainedByNext.sessionId}</Text>
          <Text fontSize="sm">«{rotuloDeMotivo(data.explainedByNext.reason)}»</Text>
          {data.explainedByNext.reasonNote && <Text fontSize="sm" color="fg.muted">{data.explainedByNext.reasonNote}</Text>}
          {onAbrirCorte && (
            <Button size="sm" minH="44px" mt={2} variant="outline" onClick={() => onAbrirCorte(data.explainedByNext!.sessionId)}>
              Ver el turno #{data.explainedByNext.sessionId}
            </Button>
          )}
        </Box>
      )}
      <CorreccionesDelCorte corte={data} />
      <Section title="El turno">
        <SimpleGrid columns={2} gap={2} fontSize="sm">
          <Text color="fg.muted">Caja</Text><Text textAlign="end" fontWeight="600">{data.registerName}</Text>
          <Text color="fg.muted">Abrió</Text><Text textAlign="end">{data.openedByName} · {horaNegocio.fechaYHora(data.openedAt)} · con {montoDeCaja(data.openingCash, cur)}</Text>
          {data.closedAt && (<>
            <Text color="fg.muted">Cerró</Text>
            <Text textAlign="end">{data.closedByName ?? '—'} · {horaNegocio.fechaYHora(data.closedAt)}</Text>
          </>)}
          {data.floatLeft != null && (<>
            <Text color="fg.muted">Fondo que se dejó</Text>
            <Text textAlign="end">{montoDeCaja(data.floatLeft, cur)}{data.fixedFloatAtClose != null ? ` (fondo fijo ${montoDeCaja(data.fixedFloatAtClose, cur)})` : ''}</Text>
          </>)}
          {data.withdrawal && (<>
            <Text color="fg.muted">Retiro</Text>
            <Text textAlign="end">{montoDeCaja(data.withdrawal.amount, cur)} · se lo llevó {data.withdrawal.userName}</Text>
          </>)}
          {data.openingReason && (<>
            <Text color="fg.muted">Motivo al abrir</Text>
            <Text textAlign="end">{rotuloDeMotivo(data.openingReason)}{data.openingReasonNote ? `: ${data.openingReasonNote}` : ''}</Text>
          </>)}
        </SimpleGrid>
      </Section>
      <CorteSummary data={data} abierto={!data.closedAt} />
      <TerminalesYPropinasDelCorte session={data} currency={cur} />
      <VentasDelCorte session={data} zona={horaNegocio.zona} />
      {data.notes && (
        <Box><Text fontWeight="700" fontSize="sm">Notas</Text><Text fontSize="sm" color="fg.muted">{data.notes}</Text></Box>
      )}
    </VStack>
  );
}

function cambioTrasElArqueo(t: TerminalCount): boolean {
  return t.expectedNow !== undefined && Number(t.expectedNow) !== Number(t.expected);
}

// Terminales y propinas de un corte (spec 032): lo que el turno abierto mostraba y el cierre firmó.
// Sin esto, un faltante en una terminal dejaba de verse en cuanto se cerraba el turno.
export function TerminalesYPropinasDelCorte({ session, currency }: {
  session: Pick<CashSessionDetail, 'terminalCounts' | 'tipsPaidOut' | 'cardTipsPaidInCash' | 'tipsCarriedOver'>;
  currency: string;
}) {
  const terminales = session.terminalCounts ?? [];
  const entregadas = Number(session.tipsPaidOut ?? 0);
  const enEfectivo = Number(session.cardTipsPaidInCash ?? 0);
  const enCaja = Number(session.tipsCarriedOver ?? 0);
  return (
    <>
      {terminales.length > 0 && (
        <Section title="Terminales">
          <Table.Root size="sm">
            <Table.Header><Table.Row>
              <Table.ColumnHeader>Terminal</Table.ColumnHeader>
              <Table.ColumnHeader textAlign="end">Esperado</Table.ColumnHeader>
              <Table.ColumnHeader textAlign="end">Contado</Table.ColumnHeader>
              <Table.ColumnHeader textAlign="end">Diferencia</Table.ColumnHeader>
            </Table.Row></Table.Header>
            <Table.Body>
              {terminales.map((t) => (
                <Table.Row key={t.terminalId}>
                  <Table.Cell>{t.name}</Table.Cell>
                  <Table.Cell textAlign="end">{montoDeCaja(t.expected, currency)}</Table.Cell>
                  <Table.Cell textAlign="end">{montoDeCaja(t.declared, currency)}</Table.Cell>
                  <Table.Cell><Diferencia valor={t.difference} currency={currency} /></Table.Cell>
                </Table.Row>
              ))}
            </Table.Body>
          </Table.Root>
          {/* Lo firmado no se reescribe: si después del arqueo se corrigió la terminal de un pago,
              se dice aquí cuánto se espera hoy y la diferencia contra lo declarado. */}
          {terminales.filter(cambioTrasElArqueo).map((t) => (
            <Text key={t.terminalId} role="status" fontSize="sm" color="orange.fg" mt={2}>
              {`${t.name}: el esperado pasó de ${montoDeCaja(t.expected, currency)} a ${montoDeCaja(t.expectedNow as string, currency)} después del arqueo. Diferencia ahora: ${montoDeCaja(t.differenceNow ?? '0', currency)}`}
            </Text>
          ))}
        </Section>
      )}
      {(entregadas > 0 || enEfectivo > 0 || enCaja > 0) && (
        <Section title="Propinas">
          <Table.Root size="sm">
            <Table.Body>
              <Table.Row><Table.Cell>Propinas entregadas</Table.Cell><Table.Cell textAlign="end">{montoDeCaja(session.tipsPaidOut ?? '0', currency)}</Table.Cell></Table.Row>
              {enEfectivo > 0 && (
                <Table.Row><Table.Cell>De ellas, propina de tarjeta pagada en efectivo</Table.Cell><Table.Cell textAlign="end">{montoDeCaja(session.cardTipsPaidInCash ?? '0', currency)}</Table.Cell></Table.Row>
              )}
              <Table.Row><Table.Cell>Se quedó en caja por entregar</Table.Cell><Table.Cell textAlign="end">{montoDeCaja(session.tipsCarriedOver ?? '0', currency)}</Table.Cell></Table.Row>
            </Table.Body>
          </Table.Root>
        </Section>
      )}
    </>
  );
}

// ---- Tab: histórico de cortes (lista + detalle: panel lateral en pantallas grandes, diálogo en 7") ----
const CORTES_POR_PAGINA = 20;

export function HistoryTab({ corteId, onCorte }: { corteId: number | null; onCorte: (id: number | null) => void }) {
  const horaNegocio = useHoraDelNegocio();
  const [page, setPage] = useState(0);
  const [rango, setRango] = useState({ desde: '', hasta: '' });
  const { data, isLoading } = useQuery({
    queryKey: ['cash', 'history', page, rango.desde, rango.hasta],
    queryFn: () => backofficeApi.cashHistory({ page, pageSize: CORTES_POR_PAGINA, from: rango.desde, to: rango.hasta }),
    placeholderData: (prev) => prev,
  });
  const detailId = corteId;
  const setDetailId = onCorte;
  // Panel lateral solo en pantallas anchas (xl+); en tablet de 7" se usa el diálogo a pantalla completa.
  // Lista y detalle lado a lado desde 1024 px (#290, Maquetas M1 A); en la tableta vertical, el
  // detalle en diálogo como siempre.
  const wide = useBreakpointValue({ base: false, lg: true }, { ssr: false });

  if (isLoading) return <Center h="40vh"><Spinner size="xl" /></Center>;
  const rows = data?.items ?? [];
  const filtrando = rango.desde !== '' || rango.hasta !== '';
  // Con la dirección de un corte, el corte se abre aunque la lista no lo traiga (otra página, otras
  // fechas): el enlace compartido no depende de lo que esta pantalla tenga cargado.
  if (rows.length === 0 && !filtrando && page === 0) {
    return (<>
      <Text color="fg.muted">Aún no hay cortes registrados.</Text>
      <SessionDetailDialog id={detailId} onClose={() => setDetailId(null)} onAbrirCorte={setDetailId} />
    </>);
  }

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
        {/* Con el detalle al lado, la lista es angosta (340 px): caja y estado en un renglón, fecha y
            personas en el segundo, para que nada se recorte ni se salga (#290, hallazgo 17). */}
        <Table.Header><Table.Row>
          <Table.ColumnHeader>{wide ? 'Corte' : 'Caja'}</Table.ColumnHeader>
          {!wide && <Table.ColumnHeader>Abierta</Table.ColumnHeader>}
          {!wide && <Table.ColumnHeader>Estado</Table.ColumnHeader>}
          {!wide && <Table.ColumnHeader>Abrió / Cerró</Table.ColumnHeader>}
          <Table.ColumnHeader textAlign="end">Diferencia</Table.ColumnHeader>
          <Table.ColumnHeader></Table.ColumnHeader>
        </Table.Row></Table.Header>
        <Table.Body>
          {rows.map((r) => (
            <Table.Row key={r.id} cursor="pointer" onClick={() => setDetailId(r.id)} h="56px"
              bg={wide && detailId === r.id ? 'bg.muted' : undefined}>
              {wide ? (
                <Table.Cell maxW="0" w="100%">
                  <Text fontWeight="600" truncate>#{r.id} · {r.registerName} · {r.status === 'abierta' ? 'Abierta' : 'Cerrada'}</Text>
                  <Text fontSize="sm" color="fg.muted" truncate>
                    {horaNegocio.fechaYHora(r.openedAt)} · {r.openedByName}{r.closedByName ? ` → ${r.closedByName}` : ''}
                  </Text>
                </Table.Cell>
              ) : (<>
                <Table.Cell fontWeight="600">{r.registerName}</Table.Cell>
                <Table.Cell whiteSpace="nowrap">{horaNegocio.fechaYHora(r.openedAt)}</Table.Cell>
                <Table.Cell>
                  <Badge colorPalette={r.status === 'abierta' ? 'green' : 'gray'}>
                    {r.status === 'abierta' ? 'Abierta' : 'Cerrada'}
                  </Badge>
                </Table.Cell>
                <Table.Cell fontSize="sm">{r.openedByName}{r.closedByName ? ` → ${r.closedByName}` : ''}</Table.Cell>
              </>)}
              <Table.Cell>
                <DiferenciaDeLaFila fila={r} />
              </Table.Cell>
              <Table.Cell textAlign="end">
                <Button size="sm" minH="48px" px={4} variant="outline" onClick={(e) => { e.stopPropagation(); setDetailId(r.id); }}>Ver</Button>
              </Table.Cell>
            </Table.Row>
          ))}
        </Table.Body>
      </Table.Root>
      {/* Sobrantes y faltantes de TODO el filtro, ya corregidos (#290, P4): no es venta ni gasto. */}
      {data?.periodDifference != null && (
        <HStack justify="space-between" px={3} py={3} borderTopWidth="1px" bg="bg.muted" gap={2} flexWrap="wrap">
          <Text fontWeight="700" fontSize="sm">Sobrantes y faltantes del periodo</Text>
          <Diferencia valor={data.periodDifference} currency={rows[0]?.currency ?? 'MXN'} etiqueta="Sobrantes y faltantes del periodo" />
        </HStack>
      )}
    </Box>
  );

  if (wide) {
    return (
      <Grid templateColumns="340px minmax(0, 1fr)" gap={4} alignItems="start"
        css={{ [desde(ANCHO_PC)]: { gridTemplateColumns: '400px minmax(0, 1fr)' } }}>
        <VStack minW={0} align="stretch" gap={2}>{controles}{list}</VStack>
        <Box minW={0} bg="bg.panel" borderRadius="lg" borderWidth="1px" p={4} maxH="calc(100dvh - 200px)" overflowY="auto">
          {detailId
            ? <CorteDetail id={detailId} onAbrirCorte={setDetailId} />
            : <Center py={12}><Text color="fg.muted">Selecciona un corte para ver el detalle.</Text></Center>}
        </Box>
      </Grid>
    );
  }
  return (
    <VStack align="stretch" gap={2}>
      {controles}
      {list}
      <SessionDetailDialog id={detailId} onClose={() => setDetailId(null)} onAbrirCorte={setDetailId} />
    </VStack>
  );
}

// Los controles del histórico: un rango opcional de días y la página. Van ARRIBA de la lista y en un
// solo renglón: a 1024×600 abajo quedaban fuera de la pantalla, y cada renglón extra se le quita a
// la lista que se vino a leer. Las fechas se eligen con `DatePickerButton`, como en Ventas: el
// calendario del sistema tiene celdas de ~30 px y la constitución lo prohíbe en la tableta.
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
        <DatePickerButton label="Desde" max={hoy} value={borrador.desde} onChange={(v) => cambiar(v, borrador.hasta)} />
        <Text fontSize="sm" color="fg.muted">al</Text>
        <DatePickerButton label="Hasta" max={hoy} value={borrador.hasta} onChange={(v) => cambiar(borrador.desde, v)} />
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

// La columna Diferencia del Histórico (#290): la misma cifra que el detalle, con palabra y signo, y
// la marca de lo que la explica. Oculto no es cero.
function DiferenciaDeLaFila({ fila }: { fila: CashSessionRow }) {
  if (fila.status !== 'cerrada') return <Text textAlign="end" color="fg.muted">—</Text>;
  if (fila.arqueoHidden || (fila.difference == null && fila.totalDifference === null)) {
    return <Text textAlign="end" color="fg.muted">{diferenciaDelHistorico(fila)}</Text>;
  }
  return (
    <HStack justify="flex-end" gap={2} flexWrap="wrap">
      <Diferencia valor={fila.difference?.total ?? fila.totalDifference} currency={fila.currency} />
      {fila.explainedByNext && <Badge colorPalette="gray">Explicado al abrir</Badge>}
      {fila.adjusted && <Badge colorPalette="blue">Corregido</Badge>}
      {fila.needsExplanation && <Badge colorPalette="orange">Por explicar</Badge>}
    </HStack>
  );
}

function SessionDetailDialog({ id, onClose, onAbrirCorte }: { id: number | null; onClose: () => void; onAbrirCorte: (id: number) => void }) {
  return (
    <DialogRoot open={id !== null} onOpenChange={(e) => { if (!e.open) onClose(); }} placement="center" size="lg" scrollBehavior="inside">
      <DialogBackdrop />
      <DialogContent>
        <DialogHeader><DialogTitle>Corte #{id}</DialogTitle></DialogHeader>
        <DialogCloseTrigger />
        <DialogBody pb={6}>
          {id !== null && <CorteDetail id={id} onAbrirCorte={onAbrirCorte} />}
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
        {total === 1 ? '1 venta' : `${total} ventas`} · {montoDeCaja(session.salesTotal ?? '0', session.currency)}
        {' '}sin canceladas, reembolsadas ni propinas
        {/* Es lo VENDIDO, no lo cobrado: los ingresos de arriba son lo que entró (spec 029). */}
        {' '}· importe vendido, incluye lo que falta por cobrar
        {recortadas && ` · se muestran las ${ventas.length} más recientes`}
      </Text>
      {Number(session.writtenOff ?? 0) > 0 && (
        <Text fontSize="sm" color="fg.muted" mb={2}>
          Perdido: {montoDeCaja(session.writtenOff ?? '0', session.currency)} (se canceló lo que faltaba)
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
                  <Badge colorPalette={colorDeEstadoDeVenta(v.status)}>{etiquetaEstado(v.status)}</Badge>
                </Table.Cell>
                <Table.Cell textAlign="end" fontWeight="600">
                  {montoDeCaja(v.total, session.currency)}
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
