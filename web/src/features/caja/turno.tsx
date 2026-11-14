import { aperturaDelConteo } from './piezasDeCaja';
import { cajasConTurno } from '../../shared/cajaDelGasto';
import { useState } from 'react';
import { useNavigate } from 'react-router';
import { ConfirmSheet } from '../../components/ConfirmSheet';
import { ReasonSheet } from '../../components/ReasonSheet';
import { posApi } from '../../api/pos';
import { descartarCuenta } from '../pos/descartarCuenta';
import type { AccountItem } from '../../types/pos';
import { ESTADO, nombreDeCuenta } from '../../domain/cuentas';
import { Box, Text, Button, VStack, HStack, Table, Textarea, Center, Spinner, Stat, Badge, SimpleGrid } from '@chakra-ui/react';
import { LuArrowLeftRight, LuChevronDown, LuChevronUp } from 'react-icons/lu';
import { medirAccion } from '../../api/uso';
import { ApiError } from '../../api/client';
import { toaster } from '../../components/ui/toaster';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { backofficeApi, type CashSession, type CashRegister, type PendingOrder, type OwingOrder, type AperturaInput } from '../../api/backoffice';
import { ContadorDeEfectivo } from './ContadorDeEfectivo';
import type { ResultadoDelConteo } from './conteo';
import { Diferencia, ResumenDeDiferencia } from './diferencia';
import { montoDeCaja } from './montosDeCaja';
import { RepartirPropinas, PropinasDelCierre } from './RepartirPropinas';
import { MotivoDeApertura } from './MotivoDeApertura';
import { AperturaConAsistencia, AvisoDelCierre, CierreConAsistencia } from './AsistenciaDeCaja';
import { AvisosDeCaja } from '../../shared/AvisosDeCaja';
import { FondoQueSeDeja } from './FondoQueSeDeja';
import { useVolverAlCierre } from './volverAlCierre';
import { fondoValido, fondoExcedeLoContado, retiroDelCierre, resumenDelRetiro, avisoDeQuienSeLoLleva } from './fondoQueSeDeja';
import { ConteoDeTerminales } from './ConteoDeTerminales';
import { faltanTerminales } from './terminalesPorContar';
import { faltaDecidirPropinas } from './propinas';
import type { TipPayoutInput, TipsDecision } from '../../api/backoffice';
import { faltanPorContar, diferenciasDelCierre, faltaContarElCajon, faltantesDelCierre } from './cierreDeCaja';
import { useSessionStore } from '../../stores/session';
import { can } from '../../app/permissions';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';
import { DialogRoot, DialogBackdrop, DialogContent, DialogBody, DialogHeader, DialogTitle, DialogCloseTrigger } from '../../components/ui/dialog';
import { montoTecleado, round2 } from '../../domain/numeros';
import { mensajeDeError } from '../../api/mensajes';
import { AvisoDeFaltantes, TablaDelCierre, DiferenciaDelCierre } from './arqueo';
import { TotalsTable } from './conciliacion';
import { ExpensesTable, MovementsPanel, TransferDialog } from './movimientos';
import { Section, IngresosEgresosCard, VoidedPaymentsList, RefundsList } from './reporte';

export function RegistersTab({ cajaId, onCaja, onCajaNoEncontrada }: {
  cajaId: number | null; onCaja: (id: number) => void; onCajaNoEncontrada: () => void;
}) {
  const { data, isLoading } = useQuery({ queryKey: ['cash', 'registers'], queryFn: backofficeApi.cashRegisters });
  // Una caja sin arqueo no tiene turno que abrir ni contar: aquí solo se vería «Cerrada» para siempre.
  const registers = cajasConTurno(data?.items ?? []);
  // Selección derivada: sin elección explícita (o si la caja elegida desaparece) cae en la primera
  // (la primaria). Evita un useEffect+setState solo para inicializar el default.

  if (isLoading) return <Center h="40vh"><Spinner size="xl" /></Center>;
  if (registers.length === 0) return <Text color="fg.muted">No hay cajas configuradas. Créalas en «Administrar».</Text>;

  const openRegisters = registers.filter((r) => r.openSessionId !== null);
  const elegida = registers.find((r) => r.id === cajaId);
  // Una caja de la dirección que no está (otra empresa, archivada, mal escrita) no cae a la principal
  // en silencio: se vuelve a Cajas y se avisa (constitución V).
  if (cajaId !== null && !elegida) {
    queueMicrotask(onCajaNoEncontrada);
    return null;
  }
  const current = elegida ?? registers[0];

  return (
    <VStack align="stretch" gap={4}>
      {/* Selector de cajas: chip por caja con su estado (abierta/cerrada). */}
      <HStack gap={2} flexWrap="wrap">
        {registers.map((r) => (
          <Button key={r.id} size="sm" minH="44px" variant={current.id === r.id ? 'solid' : 'outline'}
            colorPalette={current.id === r.id ? undefined : 'gray'} onClick={() => onCaja(r.id)}>
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
  const [contando, setContando] = useState(false);
  const user = useSessionStore((s) => s.user);
  // Los gastos sin corte se piden solo al ir a abrir: es el único lugar donde se muestran (R11).
  const { data: pendingExpenses } = useQuery({
    queryKey: ['cash', 'pending-expenses', register.id],
    queryFn: () => backofficeApi.cashPendingExpenses(register.id),
    enabled: !session && contando && register.countsCash !== false,
  });
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
  const { ref: refDelCierre, marcar: marcarVueltaAlCierre, volver: volverAlCierre } = useVolverAlCierre<HTMLDivElement>();
  // Apertura que no coincide con el cierre anterior: se guarda el conteo y se pide el motivo.
  const [aperturaPendiente, setAperturaPendiente] = useState<AperturaInput | null>(null);
  const [decisionPropinas, setDecisionPropinas] = useState<TipsDecision | null>(null);
  const faltaDecidir = faltaDecidirPropinas(session?.tipsPending, decisionPropinas);
  const [conteoTerminales, setConteoTerminales] = useState<Record<number, string>>({});
  // El fondo que se queda para el siguiente turno (2026-10-10). Solo con cajón: sin efectivo no hay fondo.
  // Mientras nadie lo toca, es el fijo de la caja (spec 035); null = no se ha tocado.
  const [fondoTecleado, setFondo] = useState<string | null>(null);
  const fijo = session?.fixedFloat != null ? Number(session.fixedFloat) : null;
  const fondo = fondoTecleado ?? (fijo !== null ? String(fijo) : '');
  // «¿Se retira dinero del cajón?» abre en Sí (decisión del dueño sobre la maqueta A).
  const [retirar, setRetirar] = useState(true);
  const yo = useSessionStore((s) => s.user?.id ?? null);
  const personas = session?.tipsPending?.people ?? [];
  const [quienTocado, setQuien] = useState<number | null>(null);
  // Por omisión se lo lleva quien cierra, si está en la lista; si no, hay que elegir.
  const quien = quienTocado ?? (personas.some((p) => p.id === yo) ? yo : null);
  const contadoAlCerrar = conteoDelCierre?.total ?? null;
  const retiro = retirar ? retiroDelCierre(contadoAlCerrar, fondo) : 0;
  const faltaFondo = (session?.drawer ?? null) !== null && retirar && !fondoValido(fondo);
  const faltaQuien = (session?.drawer ?? null) !== null && retirar && (retiro ?? 0) > 0 && quien === null;
  const avisoQuien = avisoDeQuienSeLoLleva(faltaQuien, retiro, session?.currency ?? 'MXN');
  const terminalesSinCifra = faltanTerminales(session?.terminalsToCount ?? [], conteoTerminales);
  // Lo que pide la asistencia para abrir o cerrar (spec 046). Cuelga de ['cash'], así que se vuelve
  // a pedir con todo lo demás al abrir o cerrar. Un estado de la otra etapa (la caja cambió y la
  // consulta no ha vuelto) se trata como «cargando», no como cierto.
  const asistencia = useQuery({
    queryKey: ['cash', 'attendance', register.id],
    queryFn: () => backofficeApi.cashAttendance(register.id),
    retry: false,
    enabled: !isLoading,
  });
  const etapa = session ? 'close' : 'open';
  const estadoDeAsistencia = {
    gate: asistencia.data?.stage === etapa ? asistencia.data : undefined,
    failed: asistencia.isError,
  };
  // El motivo del cierre autorizado: se decide en la hoja y viaja con el cierre.
  const [autorizacion, setAutorizacion] = useState<string | null>(null);

  const invalidate = () => qc.invalidateQueries({ queryKey: ['cash'] });
  const actualizarAsistencia = () => qc.invalidateQueries({ queryKey: ['cash', 'attendance', register.id] });
  // El cierre autorizado se recuerda al confirmar: es lo último que se lee antes de firmar.
  const conAvisoDeAutorizacion = (descripcion: string | undefined) => {
    const g = estadoDeAsistencia.gate;
    if (autorizacion === null || !g || g.ready) return descripcion;
    return `La entrada de ${g.person.name} quedará «sin salida». ${descripcion ?? 'No podrás modificarla después.'}`;
  };
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
    onSuccess: () => { medirAccion('caja', 'abrir-turno'); setContando(false); setAperturaPendiente(null); invalidate(); },
    onError: (e, apertura) => {
      if (e instanceof ApiError && e.code === 'OPENING_REASON_REQUIRED') {
        setContando(false); setAperturaPendiente(apertura);
        return;
      }
      // La entrada cambió en otra tableta mientras se contaba: se vuelve a la tarjeta, que ya pide
      // marcarla.
      if (e instanceof ApiError && e.code === 'CLOCK_IN_REQUIRED') {
        setContando(false); setAperturaPendiente(null); void actualizarAsistencia();
      }
      toaster.create({ title: 'No se pudo abrir la caja', description: mensajeDeError(e), type: 'error' });
    },
  });
  const payoutMut = useMutation({
    mutationFn: (input: TipPayoutInput) => backofficeApi.cashTipPayout(register.id, input),
    onSuccess: (r) => {
      setRepartiendo(false); invalidate(); volverAlCierre();
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
        // «No, todo se queda»: el fondo es lo contado y no hay retiro.
        floatLeft: !retirar ? contadoAlCerrar ?? undefined : fondoValido(fondo) ? Number(fondo) : undefined,
        fixedFloatShown: fijo ?? undefined,
        withdrawalUserId: retirar && (retiro ?? 0) > 0 ? quien ?? undefined : undefined,
        terminalCounts: Object.fromEntries((session?.terminalsToCount ?? []).map((t) => [String(t.terminalId), Number(conteoTerminales[t.terminalId])])),
        authorizedClose: autorizacion !== null ? { reason: autorizacion } : undefined,
      });
    },
    onSuccess: (s) => {
      medirAccion('caja', 'cerrar-turno');
      setClosed(s); setDeclared({}); setConteoDelCierre(null); setNotes(''); setDecisionPropinas(null); setConteoTerminales({}); setFondo(null); setRetirar(true); setQuien(null); setAutorizacion(null); invalidate();
    },
    // El servidor distingue "hay pedidos sin terminar" de cualquier otro fallo y manda los folios
    // en el mensaje. Se pinta con su propio título porque no es un error del cierre: es una tarea
    // pendiente, y el operador tiene que saber que la puede resolver y volver.
    onError: (e) => {
      // Quien abrió volvió a marcar entrada en otra tableta, o la autorización ya no aplica: el aviso
      // del cierre se vuelve a pintar con lo que hay ahora.
      if (e instanceof ApiError && (e.code === 'OPENER_NOT_CLOCKED_OUT' || e.code === 'CLOCK_OUT_WITH_PIN')) {
        setAutorizacion(null); void actualizarAsistencia();
      }
      // Otro cierre autorizado dejó la entrada «sin salida» mientras tanto: al volver a preguntar ya
      // no hace falta autorizar y el siguiente intento cierra.
      if (e instanceof ApiError && e.code === 'ATTENDANCE_STATE_CHANGED') void actualizarAsistencia();
      avisarDelCierre(e);
    },
  });
  const avisarDelCierre = (e: unknown) => toaster.create({
      title: e instanceof ApiError && e.code === 'OPEN_ORDERS' ? 'Faltan pedidos por terminar'
        : e instanceof ApiError && e.code === 'UNPAID_ORDERS' ? 'Faltan pedidos por cobrar' : 'No se pudo cerrar la caja',
      description: mensajeDeError(e),
      type: 'error',
      duration: 8000,
      // Un CONFLICT al cerrar es "alguien más ya cerró o ya guardó su conteo", y las dos tabletas
      // comparten cuenta: pasa en un turno normal. Lo accionable es traer el corte que sí quedó, no
      // volver a intentar sobre un turno que ya no está abierto.
      action: e instanceof ApiError && e.code === 'CONFLICT'
        ? { label: 'Recargar', onClick: () => invalidate() }
        : undefined,
    });

  if (isLoading) return <Center h="30vh"><Spinner size="xl" /></Center>;

  return (
    <>
      {!session ? (
        <AperturaConAsistencia nombreCaja={register.name} estado={estadoDeAsistencia} abriendo={openMut.isPending}
          onContar={() => setContando(true)} onActualizar={() => void actualizarAsistencia()} />
      ) : (
        <VStack align="stretch" gap={5}>
          <HStack justify="space-between" flexWrap="wrap" gap={2}>
            <Text fontWeight="700">{register.name}{session.isPrimary ? ' · recibe ventas' : ''}</Text>
            <Button size="sm" minH="44px" variant="outline" onClick={() => setTransferOpen(true)}
              disabled={openRegisters.length < 2}>
              <LuArrowLeftRight /> Traspaso
            </Button>
          </HStack>

          <AvisoDelCierre estado={estadoDeAsistencia} autorizacion={autorizacion}
            onAutorizar={setAutorizacion} onQuitarAutorizacion={() => setAutorizacion(null)}
            onActualizar={() => void actualizarAsistencia()} />

          <SimpleGrid columns={{ base: 1, sm: 3 }} gap={3}>
            <Stat.Root bg="bg.panel" p={4} borderRadius="lg" borderWidth="1px">
              <Stat.Label>Fondo inicial</Stat.Label>
              <Stat.ValueText>{montoDeCaja(session.openingCash, session.currency)}</Stat.ValueText>
            </Stat.Root>
            <Stat.Root bg="bg.panel" p={4} borderRadius="lg" borderWidth="1px">
              <Stat.Label>Movimientos (neto)</Stat.Label>
              <Stat.ValueText fontSize="lg">{montoDeCaja(session.netMovements ?? '0', session.currency)}</Stat.ValueText>
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
                <Text fontWeight="700">Propinas por entregar: {montoDeCaja(session.tipsPending?.total ?? '0', session.currency)}</Text>
                {Number(session.cardTipsPaidInCash ?? 0) > 0 && (
                  <Text fontSize="sm" color="fg.muted">
                    Propina de tarjeta pagada en efectivo: {montoDeCaja(session.cardTipsPaidInCash ?? '0', session.currency)}
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

          <CierreConAsistencia estado={estadoDeAsistencia} autorizacion={autorizacion}>
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
            <ConteoDeTerminales terminales={session.terminalsToCount ?? []} valores={conteoTerminales} onChange={setConteoTerminales} />

            {cajon && <FondoQueSeDeja value={fondo} onChange={setFondo} contado={contadoAlCerrar} fijo={fijo}
              retirar={retirar} onRetirar={setRetirar} personas={personas} quien={quien} onQuien={setQuien}
              quienCierra={yo} currency={session.currency} />}

            <AvisoDeFaltantes
              faltantes={faltantesDelCierre({ cajon: faltaElCajon, metodos: porContar, terminales: terminalesSinCifra, fondo: faltaFondo })}
              onContar={() => setContando(true)} />
            {avisoQuien && (
              <Box borderWidth="1px" borderColor="border" borderRadius="lg" p={3} colorPalette="orange" bg="colorPalette.subtle">
                <Text fontSize="sm" fontWeight="600">{avisoQuien}</Text>
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
                        <Table.Cell textAlign="end" fontWeight="700">{montoDeCaja(c.cash, 'MXN')}</Table.Cell>
                        <Table.Cell textAlign="end" color="fg.muted">{montoDeCaja(c.other, 'MXN')}</Table.Cell>
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
              <Box borderWidth="1px" borderColor="border.error" bg="bg.error"
   borderRadius="lg" p={3}>
                <Text fontWeight="700" color="fg.error">
                  Sin cobrar: {montoDeCaja(session.uncollected, 'MXN')}
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
                Perdido: {montoDeCaja(session.writtenOff ?? '0', 'MXN')} (se canceló lo que faltaba)
              </Text>
            )}

            <DiferenciaDelCierre diferencias={diferencias} cajon={cajon} conteo={conteoDelCierre}
              currency={session.currency} />

            {(session.cashOutsWithoutConcept ?? 0) > 0 && (
              <AvisosDeCaja tipsPending="0" cashOutsWithoutConcept={session.cashOutsWithoutConcept ?? 0} enCaja />
            )}

            <Box ref={refDelCierre}>
              <PropinasDelCierre pendiente={session.tipsPending} currency={session.currency} decision={decisionPropinas}
                onEntregarAhora={() => { marcarVueltaAlCierre(); setRepartiendo(true); }} onDecidir={setDecisionPropinas} />
            </Box>

            <BotonCerrarCaja nombre={register.name} loading={closeMut.isPending}
              descripcion={conAvisoDeAutorizacion(cajon ? resumenDelRetiro({ retirar, retiro, quien: personas.find((p) => p.id === quien)?.name ?? null,
                contado: contadoAlCerrar, currency: session.currency }) : undefined)}
              disabled={porContar.length > 0 || faltaElCajon || faltaDecidir || faltaFondo || faltaQuien || (retirar && fondoExcedeLoContado(fondo, contadoAlCerrar)) || terminalesSinCifra.length > 0 || session.pending.length > 0 || (session.owing?.length ?? 0) > 0}
              onCerrar={() => closeMut.mutate()} />
          </CierreConAsistencia>
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
          pendingExpenses={pendingExpenses?.items ?? []} canIncludePending={can('cash.adjust_count', user)}
          onConfirmar={(r, excludePending) => { medirAccion('caja', 'contar-efectivo'); openMut.mutate({ ...aperturaDelConteo(r), excludePending }); }} />
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

      <MotivoDeApertura isOpen={aperturaPendiente !== null} guardando={openMut.isPending}
        onVolver={() => { setAperturaPendiente(null); setContando(true); }}
        onConfirmar={(motivo, nota) => aperturaPendiente && openMut.mutate({ ...aperturaPendiente, openingReason: motivo, openingReasonNote: nota || undefined })} />

      <TransferDialog open={transferOpen} onClose={() => setTransferOpen(false)}
        from={register} openRegisters={openRegisters} onDone={() => { setTransferOpen(false); invalidate(); }} />

      {/* Resumen tras cerrar: esperado vs declarado vs diferencia */}
      <DialogRoot open={closed !== null} onOpenChange={(e) => { if (!e.open) setClosed(null); }} placement="center" size="md">
        <DialogBackdrop />
        <DialogContent>
          <DialogHeader><DialogTitle>Caja cerrada</DialogTitle></DialogHeader>
          <DialogCloseTrigger />
          <DialogBody pb={6}>
            {/* La misma cifra que la lista y el detalle (#290): el diálogo era el que decía $0. */}
            {closed?.difference && (
              <Box mb={3}><ResumenDeDiferencia diferencia={closed.difference} currency={closed.currency} /></Box>
            )}
            <TotalsTable totals={closed?.totals ?? []} currency={closed?.currency ?? 'MXN'}
              drawerMethodIds={closed?.drawer?.methodIds} difference={closed?.difference} drawer={closed?.drawer} />
            {(closed?.terminalCounts ?? []).length > 0 && (
              <Box mt={3}>
                <Text fontWeight="700" mb={1}>Terminales</Text>
                {(closed?.terminalCounts ?? []).map((t) => (
                  <HStack key={t.terminalId} justify="space-between">
                    <Text>{t.name}</Text>
                    <HStack gap={2}>
                      <Text>{montoDeCaja(t.declared, closed?.currency ?? 'MXN')}</Text>
                      <Diferencia valor={t.difference} currency={closed?.currency ?? 'MXN'} />
                    </HStack>
                  </HStack>
                ))}
              </Box>
            )}
          </DialogBody>
        </DialogContent>
      </DialogRoot>
    </>
  );
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
        <Box borderWidth="1px" borderColor="border.error" bg="bg.error"
 borderRadius="lg" p={3}>
          <Text fontWeight="700" color="fg.error" mb={1}>
            Falta cobrar {owing.length === 1 ? '1 pedido' : `${owing.length} pedidos`}
          </Text>
          <Text fontSize="sm" color="fg.muted" mb={2}>
            Ya se entregaron. La caja no cierra hasta que se cobren o se cancelen.
          </Text>
          <VStack align="stretch" gap={1} maxH="40dvh" overflowY="auto">
            {owing.map((o) => (
              <HStack key={o.id} justify="space-between" gap={2} flexWrap="wrap">
                <Text fontWeight="600" truncate flex="1" minW="8rem">{o.name ? `${o.name} · #${o.number}` : `#${o.number}`}</Text>
                <Text fontWeight="700" flexShrink={0}>{montoDeCaja(round2(Number(o.total) - Number(o.paid) - Number(o.writtenOff ?? 0)), 'MXN')}</Text>
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
        <Box borderWidth="1px" colorPalette="orange" borderColor="colorPalette.muted" bg="colorPalette.subtle"
          borderRadius="lg" p={3}>
          {/* Tokens semánticos y no orange.50: en oscuro el fondo claro dejaba el texto casi ilegible (#290). */}
          <Text fontWeight="700" color="colorPalette.fg" mb={1}>
            Falta entregar {pending.length === 1 ? '1 pedido' : `${pending.length} pedidos`}
          </Text>
          <Text fontSize="sm" color="fg.muted" mb={2}>
            La caja no cierra hasta que salgan o se cancelen.
          </Text>
          <VStack align="stretch" gap={1}>
            {pending.map((o) => (
              <HStack key={o.number} justify="space-between" gap={2}>
                <Text fontWeight="600" truncate flex="1" minW={0}>{o.name ? `${o.name} · #${o.number}` : `#${o.number}`}</Text>
                {o.total !== undefined && <Text fontWeight="700" flexShrink={0}>{montoDeCaja(o.total, 'MXN')}</Text>}
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
          <Text fontSize="xs" color="fg.muted" px={4}>No impiden cerrar: siguen en la fila de cuentas.</Text>
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
                  {/* Un pedido cerrado no debe nada: lo vivo es lo que falta mandar, no un saldo de $0. */}
                  <Text fontWeight="700" flexShrink={0}>
                    {c.state === 'closed_with_new' ? `${c.pendingCount} sin mandar` : montoDeCaja(c.state === 'capturing' ? c.total : c.outstanding, 'MXN')}
                  </Text>
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
        description={perdiendo ? `Se dan por perdidos ${montoDeCaja(round2(Number(perdiendo.total) - Number(perdiendo.paid) - Number(perdiendo.writtenOff ?? 0)), 'MXN')}. Lo cobrado (${montoDeCaja(perdiendo.paid, 'MXN')}) se queda como venta.` : undefined}
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
export function BotonCerrarCaja({ nombre, disabled, loading, onCerrar, descripcion = 'No podrás modificarla después.' }: {
  nombre: string; disabled: boolean; loading: boolean; onCerrar: () => void;
  // Lo que se lee antes de confirmar: con cajón, el retiro y quién se lo lleva (spec 035).
  descripcion?: string;
}) {
  const [preguntando, setPreguntando] = useState(false);
  return (
    <>
      <Button colorPalette="brand" size="lg" minH="52px" loading={loading} disabled={disabled}
        onClick={() => setPreguntando(true)}>
        Cerrar caja
      </Button>
      <ConfirmSheet isOpen={preguntando} title={`¿Cerrar «${nombre}»?`}
        description={descripcion} cancelLabel="Volver" confirmLabel="Cerrar caja"
        onCancel={() => setPreguntando(false)}
        onConfirm={() => { setPreguntando(false); onCerrar(); }} />
    </>
  );
}
