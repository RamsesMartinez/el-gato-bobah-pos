import { aperturaDelConteo } from './piezasDeCaja';
import { cajasConTurno } from '../../shared/cajaDelGasto';
import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router';
import { ConfirmSheet } from '../../components/ConfirmSheet';
import { ReasonSheet } from '../../components/ReasonSheet';
import { posApi } from '../../api/pos';
import { descartarCuenta } from '../pos/descartarCuenta';
import type { AccountItem } from '../../types/pos';
import { ESTADO, nombreDeCuenta } from '../../domain/cuentas';
import { Box, Text, Button, Stack, VStack, HStack, Table, Textarea, Center, Spinner, Stat, Badge, SimpleGrid, Grid } from '@chakra-ui/react';
import { useAnchoMinimo, ANCHO_2K, ANCHO_PC, desde } from '../../hooks/useAnchoMinimo';
import { LuArrowLeftRight, LuChevronDown, LuChevronUp, LuChevronLeft, LuChevronRight, LuCheck, LuLock } from 'react-icons/lu';
import { InfoButton } from '../../components/InfoButton';
import { MOTIVOS_DE_CORRECCION } from './motivosDeCorreccion';
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
import { AperturaConAsistencia, AvisoDelCierre, type EstadoDeAsistencia } from './AsistenciaDeCaja';
import { AvisosDeCaja } from '../../shared/AvisosDeCaja';
import { FondoQueSeDeja } from './FondoQueSeDeja';
import { useVolverAlCierre } from './volverAlCierre';
import { FIN_DEL_CIERRE, fondoValido, fondoExcedeLoContado, retiroDelCierre, resumenDelRetiro, avisoDeQuienSeLoLleva } from './fondoQueSeDeja';
import { ConteoDeTerminales } from './ConteoDeTerminales';
import { faltanTerminales } from './terminalesPorContar';
import { faltaDecidirPropinas } from './propinas';
import type { TipPayoutInput, TipsDecision } from '../../api/backoffice';
import {
  faltanPorContar, diferenciasDelCierre, faltaContarElCajon, diferenciaDelCajon,
  pendientesDelCierre, estadoDelPaso, asistenciaDelCierre, type PasoDelCierre, type PendienteDelCierre,
} from './cierreDeCaja';
import { useSessionStore } from '../../stores/session';
import { can } from '../../app/permissions';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';
import { DialogRoot, DialogBackdrop, DialogContent, DialogBody, DialogHeader, DialogTitle, DialogCloseTrigger } from '../../components/ui/dialog';
import { montoTecleado, round2 } from '../../domain/numeros';
import { mensajeDeError } from '../../api/mensajes';
import { TablaDelCierre, DiferenciaDelCierre } from './arqueo';
import { TotalsTable } from './conciliacion';
import { ExpensesTable, MovementsPanel, TransferDialog } from './movimientos';
import { Section, IngresosEgresosCard, VoidedPaymentsList, RefundsList } from './reporte';

export function RegistersTab({ cajaId, onCaja, onCajaNoEncontrada, cerrando = false, onCerrando }: {
  cajaId: number | null; onCaja: (id: number) => void; onCajaNoEncontrada: () => void;
  cerrando?: boolean; onCerrando?: (id: number, v: boolean) => void;
}) {
  const lado = useAnchoMinimo(ANCHO_2K);
  const { data, isLoading } = useQuery({ queryKey: ['cash', 'registers'], queryFn: backofficeApi.cashRegisters });
  // El resumen tras cerrar vive aquí y no en el panel: en 2K, salir del cierre cambia del panel único a
  // la cuadrícula, el panel se vuelve a montar y el resumen se perdía.
  const [closed, setClosed] = useState<CashSession | null>(null);
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
  // Sin caja elegida, la que tiene turno: abrir en la principal cerrada escondía el turno vivo.
  const current = elegida ?? registers.find((r) => r.openSessionId !== null) ?? registers[0];

  return (
    <VStack align="stretch" gap={4}>
      {/* Selector de cajas: chip por caja con su estado. Mientras se cierra no se pinta: le quitaba ~60 px
          al paso en 600 de alto, y «Volver al turno» ya está en los pasos. */}
      {!cerrando && <HStack gap={2} flexWrap="wrap">
        {registers.map((r) => (
          <Button key={r.id} size="sm" minH="44px" variant={current.id === r.id ? 'solid' : 'outline'}
            colorPalette={current.id === r.id ? undefined : 'gray'} onClick={() => onCaja(r.id)}>
            {r.name}
            <Badge ml={2} colorPalette={r.openSessionId !== null ? 'green' : 'gray'}>
              {r.openSessionId !== null ? 'Abierta' : 'Cerrada'}
            </Badge>
          </Button>
        ))}
      </HStack>}

      {/* En 2K las cajas abiertas van lado a lado, cada una con su turno (#290, FR-036). */}
      {/* Solo si la elegida está abierta: una caja cerrada no está en la cuadrícula y no se podía abrir. */}
      {lado && !cerrando && openRegisters.length > 1 && current.openSessionId !== null ? (
        <SimpleGrid columns={2} gap={6}>
          {openRegisters.map((r) => (
            <Box key={r.id} borderWidth="1px" borderRadius="lg" p={4} minW={0}>
              <RegisterPanel register={r} openRegisters={openRegisters} cerrando={false}
                onCerrando={(v) => onCerrando?.(r.id, v)} onCerrado={setClosed} />
            </Box>
          ))}
        </SimpleGrid>
      ) : (
        <RegisterPanel register={current} openRegisters={openRegisters} cerrando={cerrando}
          onCerrando={(v) => onCerrando?.(current.id, v)} onCerrado={setClosed} />
      )}
      <ResumenDelCierre closed={closed} onClose={() => setClosed(null)} />
    </VStack>
  );
}

// ---- Panel de una caja: abrir (si cerrada) u operar/cerrar (si abierta) ----
// La clave del borrador del cierre en el aparato (#290, FR-032): por caja y por turno, para que el de
// un turno ya cerrado no se cuele en el siguiente.
const claveDelBorrador = (registerId: number, sessionId: number) => `egb:cierre:${registerId}:${sessionId}`;
const prefijoDelBorrador = (registerId: number) => `egb:cierre:${registerId}:`;

interface BorradorDelCierre {
  declared: Record<string, string>;
  conteoDelCierre: ResultadoDelConteo | null;
  conteoTerminales: Record<number, string>;
  fondoTecleado: string | null;
  retirar: boolean;
  quienTocado: number | null;
  notes: string;
  decisionPropinas: TipsDecision | null;
  paso: PasoDelCierre;
  visitados: PasoDelCierre[];
  explicacion: { reason: string; note: string };
}

// leerBorrador: lo capturado en este aparato para este turno. El almacenamiento del navegador puede no
// estar (modo privado, datos borrados): sin él, el cierre empieza de cero y funciona igual.
function leerBorrador(clave: string): Partial<BorradorDelCierre> {
  try {
    const t = localStorage.getItem(clave);
    return t ? (JSON.parse(t) as Partial<BorradorDelCierre>) : {};
  } catch {
    return {};
  }
}

function RegisterPanel({ register, openRegisters, cerrando, onCerrando, onCerrado }: {
  register: CashRegister; openRegisters: CashRegister[];
  cerrando: boolean; onCerrando: (v: boolean) => void; onCerrado: (s: CashSession) => void;
}) {
  const qc = useQueryClient();
  const { data: session, isLoading, isSuccess } = useQuery({
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
  // Apertura que no coincide con el cierre anterior: se guarda el conteo y se pide el motivo.
  const [aperturaPendiente, setAperturaPendiente] = useState<AperturaInput | null>(null);
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
  const invalidate = () => qc.invalidateQueries({ queryKey: ['cash'] });
  const actualizarAsistencia = () => qc.invalidateQueries({ queryKey: ['cash', 'attendance', register.id] });

  // UN BORRADOR DE UN TURNO QUE YA NO ESTÁ ABIERTO SE DESCARTA AVISANDO (#290, FR-032): otra tableta
  // cerró la caja mientras aquí se contaba. Solo con la respuesta del servidor: una consulta que falló
  // por la red no dice que la caja se cerró, y el borrador existe justo para ese caso.
  const sinTurno = isSuccess && !session;
  // La dirección del cierre de una caja sin turno vuelve a la caja: si no, el turno que se abra
  // después entraría directo al asistente de cierre.
  useEffect(() => {
    if (cerrando && sinTurno) onCerrando(false);
  }, [cerrando, sinTurno, onCerrando]);
  useEffect(() => {
    if (!sinTurno) return;
    try {
      const viejas = Object.keys(localStorage).filter((k) => k.startsWith(prefijoDelBorrador(register.id)));
      if (viejas.length === 0) return;
      viejas.forEach((k) => localStorage.removeItem(k));
      toaster.create({ title: 'La caja ya se cerró en otra tableta', description: 'Se descartó lo que se había capturado aquí.', type: 'info' });
    } catch {
      // Sin almacenamiento no hay borrador que descartar.
    }
  }, [sinTurno, register.id]);

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

  if (isLoading) return <Center h="30vh"><Spinner size="xl" /></Center>;

  return (
    <>
      {!session ? (
        <AperturaConAsistencia nombreCaja={register.name} estado={estadoDeAsistencia} abriendo={openMut.isPending}
          onContar={() => setContando(true)} onActualizar={() => void actualizarAsistencia()} />
      ) : (
        <TurnoAbierto key={session.id} register={register} session={session} openRegisters={openRegisters}
          estadoDeAsistencia={estadoDeAsistencia} onActualizarAsistencia={() => void actualizarAsistencia()}
          cerrando={cerrando} onCerrando={onCerrando}
          onCerrado={(s) => { onCerrado(s); onCerrando(false); invalidate(); }} />
      )}

      {!session && (
        <ContadorDeEfectivo isOpen={contando} onClose={() => setContando(false)}
          // El paso en que va la apertura (#290, FR-035): contar, y si no coincide con el cierre
          // anterior, el motivo.
          titulo={`Paso 1 · Contar el fondo de «${register.name}»`}
          // La moneda del turno la fija el servidor con el default de la columna: hoy no hay forma
          // de elegir otra al abrir. Cuando la haya, ESTA línea es la que cambia.
          currency="MXN" etiquetaConfirmar="Abrir caja" guardando={openMut.isPending}
          // Contar el cajón se mide aquí, donde de verdad ocurre, y no en el onSuccess de la
          // mutación: el de abrir mide «abrir-turno», que es otro hecho.
          pendingExpenses={pendingExpenses?.items ?? []} canIncludePending={can('cash.adjust_count', user)}
          onConfirmar={(r, excludePending) => { medirAccion('caja', 'contar-efectivo'); openMut.mutate({ ...aperturaDelConteo(r), excludePending }); }} />
      )}

      <MotivoDeApertura isOpen={aperturaPendiente !== null} guardando={openMut.isPending}
        onVolver={() => { setAperturaPendiente(null); setContando(true); }}
        onConfirmar={(motivo, nota) => aperturaPendiente && openMut.mutate({ ...aperturaPendiente, openingReason: motivo, openingReasonNote: nota || undefined })} />

    </>
  );
}

// ResumenDelCierre: lo que quedó al cerrar, con la misma cifra que la lista y el detalle (#290).
function ResumenDelCierre({ closed, onClose }: { closed: CashSession | null; onClose: () => void }) {
  return (
      <DialogRoot open={closed !== null} onOpenChange={(e) => { if (!e.open) onClose(); }} placement="center" size="md">
        <DialogBackdrop />
        <DialogContent>
          <DialogHeader><DialogTitle>Caja cerrada</DialogTitle></DialogHeader>
          <DialogCloseTrigger />
          <DialogBody pb={6}>
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
  );
}

const NOMBRES_DE_PASOS: Record<PasoDelCierre, string> = { 1: 'Pendientes', 2: 'Cajón', 3: 'Otros métodos', 4: 'Diferencia y cierre' };
const PASOS: PasoDelCierre[] = [1, 2, 3, 4];

// TurnoAbierto: el turno vivo. Operar y Cerrar son vistas distintas (#290, FR-034): operar a la
// izquierda con «Cómo va el turno» fijo a la derecha (Maquetas M2 B), y el cierre como asistente de
// cuatro pasos (M3 B). Se monta por turno (key = id), así que su estado inicial es el borrador de
// ESTE turno.
function TurnoAbierto({ register, session, openRegisters, estadoDeAsistencia, onActualizarAsistencia, cerrando, onCerrando, onCerrado }: {
  register: CashRegister; session: CashSession; openRegisters: CashRegister[];
  estadoDeAsistencia: EstadoDeAsistencia; onActualizarAsistencia: () => void;
  cerrando: boolean; onCerrando: (v: boolean) => void; onCerrado: (s: CashSession) => void;
}) {
  const horaNegocio = useHoraDelNegocio();
  const qc = useQueryClient();
  const user = useSessionStore((s) => s.user);
  const clave = claveDelBorrador(register.id, session.id);
  const [inicial] = useState(() => leerBorrador(clave));
  const [declared, setDeclared] = useState<Record<string, string>>(inicial.declared ?? {});
  // El conteo del cajón para ESTE cierre, tal como lo entregó la hoja. Vive aquí y no en `declared`
  // porque no es una cifra tecleada: es el arqueo, con sus renglones o con su motivo.
  const [conteoDelCierre, setConteoDelCierre] = useState<ResultadoDelConteo | null>(inicial.conteoDelCierre ?? null);
  const [conteoTerminales, setConteoTerminales] = useState<Record<number, string>>(inicial.conteoTerminales ?? {});
  // El fondo que se queda para el siguiente turno (2026-10-10). Solo con cajón: sin efectivo no hay fondo.
  // Mientras nadie lo toca, es el fijo de la caja (spec 035); null = no se ha tocado.
  const [fondoTecleado, setFondo] = useState<string | null>(inicial.fondoTecleado ?? null);
  // «¿Se retira dinero del cajón?» abre en Sí (decisión del dueño sobre la maqueta A).
  const [retirar, setRetirar] = useState(inicial.retirar ?? true);
  const [quienTocado, setQuien] = useState<number | null>(inicial.quienTocado ?? null);
  const [notes, setNotes] = useState(inicial.notes ?? '');
  const [decisionPropinas, setDecisionPropinas] = useState<TipsDecision | null>(inicial.decisionPropinas ?? null);
  const [paso, setPaso] = useState<PasoDelCierre>(inicial.paso ?? 1);
  const [visitados, setVisitados] = useState<PasoDelCierre[]>(inicial.visitados ?? [1]);
  const [explicacion, setExplicacion] = useState<{ reason: string; note: string }>(inicial.explicacion ?? { reason: '', note: '' });
  const [contando, setContando] = useState(false);
  const [transferOpen, setTransferOpen] = useState(false);
  const [repartiendo, setRepartiendo] = useState(false);
  const { ref: refDelCierre, marcar: marcarVueltaAlCierre, volver: volverAlCierre } = useVolverAlCierre<HTMLDivElement>();
  // El motivo del cierre autorizado: se decide en la hoja y viaja con el cierre.
  const [autorizacion, setAutorizacion] = useState<string | null>(null);

  // EL BORRADOR SE GUARDA EN EL APARATO (#290, FR-032): una recarga o la tableta suspendida no pierden
  // el conteo. Es por aparato, como el ticket del POS; no se sincroniza entre tabletas. Solo si se
  // capturó algo: una tableta que solo miraba el turno recibía después el aviso de «se descartó lo
  // que se había capturado aquí».
  const capturoAlgo = Object.values(declared).some((v) => v.trim() !== '') || conteoDelCierre !== null
    || Object.values(conteoTerminales).some((v) => v.trim() !== '') || fondoTecleado !== null || !retirar
    || quienTocado !== null || notes !== '' || decisionPropinas !== null || explicacion.reason !== '' || explicacion.note !== '';
  useEffect(() => {
    try {
      if (!capturoAlgo) { localStorage.removeItem(clave); return; }
      const b: BorradorDelCierre = { declared, conteoDelCierre, conteoTerminales, fondoTecleado, retirar, quienTocado, notes, decisionPropinas, paso, visitados, explicacion };
      localStorage.setItem(clave, JSON.stringify(b));
    } catch {
      // Sin almacenamiento el cierre sigue funcionando; solo no sobrevive a una recarga.
    }
  }, [capturoAlgo, clave, declared, conteoDelCierre, conteoTerminales, fondoTecleado, retirar, quienTocado, notes, decisionPropinas, paso, visitados, explicacion]);
  // Los borradores de turnos anteriores de esta caja se van sin avisar: ese turno ya cerró y lo que
  // quedó ahí no se puede usar. Si se quedaban, el aviso de «otra tableta» salía en un cierre propio.
  useEffect(() => {
    try {
      Object.keys(localStorage)
        .filter((k) => k.startsWith(prefijoDelBorrador(register.id)) && k !== clave)
        .forEach((k) => localStorage.removeItem(k));
    } catch {
      // Sin almacenamiento no hay borradores viejos.
    }
  }, [register.id, clave]);

  const cajon = session.drawer ?? null;
  // QUIÉN EXIGE QUÉ LO DICE EL SERVIDOR (`requiresEntry`, `requiresCount`). Deducirlo de si el
  // esperado es cero se rompe con el arqueo ciego, donde el esperado no viaja.
  const porContar = faltanPorContar(session.totals ?? [], declared);
  const faltaElCajon = faltaContarElCajon(cajon, conteoDelCierre);
  // La diferencia en vivo, con la MISMA cifra que se va a mandar.
  const declaradoPorMetodo: Record<number, number | undefined> = {};
  for (const t of session.totals ?? []) {
    declaradoPorMetodo[t.methodId] = montoTecleado(declared[t.methodId] ?? '');
  }
  const diferencias = diferenciasDelCierre(session.totals ?? [], declaradoPorMetodo);
  const faltaDecidir = faltaDecidirPropinas(session.tipsPending, decisionPropinas);
  const fijo = session.fixedFloat != null ? Number(session.fixedFloat) : null;
  const fondo = fondoTecleado ?? (fijo !== null ? String(fijo) : '');
  const yo = user?.id ?? null;
  const personas = session.tipsPending?.people ?? [];
  // Por omisión se lo lleva quien cierra, si está en la lista; si no, hay que elegir.
  // Lo elegido antes (quizá restaurado del borrador) vale solo si esa persona sigue en la lista.
  const quien = (quienTocado !== null && personas.some((p) => p.id === quienTocado) ? quienTocado : null)
    ?? (personas.some((p) => p.id === yo) ? yo : null);
  const contadoAlCerrar = conteoDelCierre?.total ?? null;
  const retiro = retirar ? retiroDelCierre(contadoAlCerrar, fondo) : 0;
  const faltaFondo = cajon !== null && retirar && !fondoValido(fondo);
  const faltaQuien = cajon !== null && retirar && (retiro ?? 0) > 0 && quien === null;
  const avisoQuien = avisoDeQuienSeLoLleva(faltaQuien, retiro, session.currency);
  const terminalesSinCifra = faltanTerminales(session.terminalsToCount ?? [], conteoTerminales);
  // LA EXPLICACIÓN AL CERRAR (#290, P1; FR-033): para quien cierra viendo el esperado, configure o no la
  // caja, y solo si la diferencia pasa la tolerancia. Con arqueo ciego no se pide ni se insinúa: el corte queda «por explicar».
  const { data: ajustesDeCierre } = useQuery({ queryKey: ['cash', 'close-settings'], queryFn: backofficeApi.cashCloseSettings });
  const difCajon = diferenciaDelCajon(cajon, conteoDelCierre);
  const totalDelArqueo = diferencias.completo && !(cajon !== null && difCajon === undefined)
    ? round2(diferencias.total + (difCajon ?? 0)) : null;
  const pideExplicacion = !session.blind && totalDelArqueo !== null
    && ajustesDeCierre !== undefined && Math.abs(totalDelArqueo) > Number(ajustesDeCierre.tolerance);
  // Explicar es opcional, pero a medias (motivo sin nota o nota sin motivo) no se manda: es pendiente.
  const explicacionAMedias = pideExplicacion && (explicacion.reason !== '') !== (explicacion.note.trim() !== '');
  const pendientes = pendientesDelCierre({
    asistencia: asistenciaDelCierre(estadoDeAsistencia, autorizacion),
    pedidosSinEntregar: session.pending.length,
    pedidosQueDeben: session.owing?.length ?? 0,
    propinasPorDecidir: faltaDecidir,
    cajon: faltaElCajon,
    metodos: porContar,
    terminales: terminalesSinCifra,
    fondo: faltaFondo,
    quien: faltaQuien,
    fondoExcedeLoContado: retirar && fondoExcedeLoContado(fondo, contadoAlCerrar),
    explicacionAMedias,
  });

  const invalidate = () => qc.invalidateQueries({ queryKey: ['cash'] });
  const navigate = useNavigate();
  // El cierre autorizado se recuerda al confirmar: es lo último que se lee antes de firmar.
  const conAvisoDeAutorizacion = (descripcion: string | undefined) => {
    const g = estadoDeAsistencia.gate;
    if (autorizacion === null || !g || g.ready) return descripcion;
    return `La entrada de ${g.person.name} quedará «sin salida». ${descripcion ?? FIN_DEL_CIERRE}`;
  };
  // Descartar una cuenta que se capturaba y nadie mandó. No bloquea el cierre (D-8).
  const descartarDelCierre = async (draftId: string, version: number) => {
    try {
      await descartarCuenta(qc, draftId, version);
    } catch (e) {
      toaster.create({ title: 'No se pudo descartar', description: mensajeDeError(e), type: 'error' });
    }
    invalidate();
    qc.invalidateQueries({ queryKey: ['pos', 'accounts'] });
  };
  // Cancelar un entregado que debe y no pagó nada («se fue sin pagar»).
  const cancelarDelCierre = async (orderId: number, motivo: string, resto = false) => {
    try {
      await (resto ? posApi.writeOffOrder(orderId, motivo) : posApi.cancelOrder(orderId, motivo));
    } catch (e) {
      toaster.create({ title: 'No se pudo cancelar', description: mensajeDeError(e), type: 'error' });
    }
    invalidate();
    qc.invalidateQueries({ queryKey: ['pos', 'accounts'] });
  };
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
      // `declared` lleva SOLO lo que no está en el cajón: un método de cajón aquí lo rechaza el servidor.
      const delCajon = new Set(cajon?.methodIds ?? []);
      const d: Record<string, number> = {};
      Object.entries(declared).forEach(([k, v]) => {
        if (delCajon.has(Number(k))) return;
        const monto = montoTecleado(v);
        if (monto !== undefined) d[k] = monto;
      });
      const porPiezas = conteoDelCierre !== null && 'counts' in conteoDelCierre ? conteoDelCierre : null;
      const aMano = conteoDelCierre !== null && !('counts' in conteoDelCierre) ? conteoDelCierre : null;
      return backofficeApi.cashClose(register.id, d, {
        counts: porPiezas?.counts,
        countedCash: aMano?.total,
        manualReason: aMano?.manualReason,
        notes: notes || undefined,
        tipsDecision: Number(session.tipsPending?.total ?? 0) >= 1 ? decisionPropinas ?? undefined : undefined,
        // «No, todo se queda»: el fondo es lo contado y no hay retiro.
        floatLeft: !retirar ? contadoAlCerrar ?? undefined : fondoValido(fondo) ? Number(fondo) : undefined,
        fixedFloatShown: fijo ?? undefined,
        withdrawalUserId: retirar && (retiro ?? 0) > 0 ? quien ?? undefined : undefined,
        terminalCounts: Object.fromEntries((session.terminalsToCount ?? []).map((t) => [String(t.terminalId), Number(conteoTerminales[t.terminalId])])),
        authorizedClose: autorizacion !== null ? { reason: autorizacion } : undefined,
        explanation: pideExplicacion && explicacion.reason !== '' && explicacion.note.trim() !== ''
          // Un salto de línea es un carácter de control que el servidor rechaza: tumbaba el cierre entero.
          ? { reason: explicacion.reason, note: explicacion.note.replace(/\s+/g, ' ').trim() } : undefined,
      });
    },
    onSuccess: (s) => {
      medirAccion('caja', 'cerrar-turno');
      try { localStorage.removeItem(clave); } catch { /* sin almacenamiento no hay borrador */ }
      onCerrado(s);
    },
    // El servidor distingue "hay pedidos sin terminar" de cualquier otro fallo.
    onError: (e) => {
      if (e instanceof ApiError && (e.code === 'OPENER_NOT_CLOCKED_OUT' || e.code === 'CLOCK_OUT_WITH_PIN')) {
        setAutorizacion(null); onActualizarAsistencia();
      }
      if (e instanceof ApiError && e.code === 'ATTENDANCE_STATE_CHANGED') onActualizarAsistencia();
      toaster.create({
        title: e instanceof ApiError && e.code === 'OPEN_ORDERS' ? 'Faltan pedidos por terminar'
          : e instanceof ApiError && e.code === 'UNPAID_ORDERS' ? 'Faltan pedidos por cobrar' : 'No se pudo cerrar la caja',
        description: mensajeDeError(e), type: 'error', duration: 8000,
        // Un CONFLICT al cerrar es "alguien más ya cerró": lo accionable es traer el corte que sí quedó.
        action: e instanceof ApiError && e.code === 'CONFLICT' ? { label: 'Recargar', onClick: () => invalidate() } : undefined,
      });
    },
  });

  const irAlPaso = (p: PasoDelCierre) => { setPaso(p); setVisitados((v) => (v.includes(p) ? v : [...v, p])); };
  const resumenDelPaso = (p: PasoDelCierre) => {
    const n = pendientes.filter((x) => x.paso === p).length;
    if (n > 0) return n === 1 ? '1 pendiente' : `${n} pendientes`;
    if (p === 2 && contadoAlCerrar !== null) return `Contado ${montoDeCaja(contadoAlCerrar, session.currency)}`;
    return visitados.includes(p) ? 'Listo' : 'Sin revisar';
  };

  const sheets = (
    <>
      <ContadorDeEfectivo isOpen={contando} onClose={() => setContando(false)}
        titulo={`Efectivo en «${register.name}»`} currency={session.currency}
        etiquetaConfirmar="Usar este conteo" guardando={false}
        onConfirmar={(r) => { medirAccion('caja', 'contar-efectivo'); setConteoDelCierre(r); setContando(false); }} />
      <RepartirPropinas isOpen={repartiendo} pendiente={session.tipsPending ?? null} currency={session.currency}
        guardando={payoutMut.isPending} onEntregar={(i) => payoutMut.mutate(i)} onClose={() => setRepartiendo(false)} />
      <TransferDialog open={transferOpen} onClose={() => setTransferOpen(false)}
        from={register} openRegisters={openRegisters} onDone={() => { setTransferOpen(false); invalidate(); }} />
    </>
  );

  if (!cerrando) {
    return (
      <>
        {/* OPERAR Y «CÓMO VA EL TURNO» (Maquetas M2 B): desde 1024 px el resumen queda fijo al lado,
            con «Cerrar caja» al pie; nunca hay que cambiar de pestaña para ver cómo va el turno. */}
        <Grid templateColumns={{ base: '1fr', lg: 'minmax(0, 1fr) 320px' }}
          css={{ [desde(ANCHO_PC)]: { gridTemplateColumns: 'minmax(0, 1.6fr) minmax(320px, 1fr)' } }}
          gap={4} alignItems="start">
          <VStack align="stretch" gap={4} minW={0}>
            <HStack justify="space-between" flexWrap="wrap" gap={2}>
              <Text fontWeight="700">{register.name}{session.isPrimary ? ' · recibe ventas' : ''}</Text>
              <Button size="sm" minH="48px" variant="outline" onClick={() => setTransferOpen(true)}
                disabled={openRegisters.length < 2}>
                <LuArrowLeftRight /> Traspaso
              </Button>
            </HStack>
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
                <Button minH="52px" colorPalette="orange" onClick={() => setRepartiendo(true)}>Entregar propina</Button>
              </HStack>
            )}
            <MovementsPanel session={session} />
            {(session.expenses ?? []).length > 0 && (
              <Section title="Gastos del corte">
                <ExpensesTable expenses={session.expenses ?? []} currency={session.currency} />
              </Section>
            )}
            <VoidedPaymentsList payments={session.voidedPayments} currency={session.currency} zona={horaNegocio.zona} />
          </VStack>
          <Box position={{ lg: 'sticky' }} top={0} minW={0}>
            <ComoVaElTurno session={session} pendientes={pendientes.filter((x) => x.paso === 1).length} onCerrar={() => { irAlPaso(1); onCerrando(true); }} />
          </Box>
        </Grid>
        {sheets}
      </>
    );
  }

  // EL CIERRE COMO ASISTENTE (Maquetas M3 B): pasos al costado con su estado en texto; el paso a la
  // derecha con Atrás/Siguiente al pie. Se puede pasar al conteo con pendientes: se cuenta mientras se
  // resuelven. El botón final se habilita solo con la lista vacía, y la lista dice qué falta.
  const ultimo = paso === 4;
  return (
    <>
      <Grid templateColumns={{ base: '1fr', lg: '220px minmax(0, 1fr)' }}
        css={{ [desde(ANCHO_PC)]: { gridTemplateColumns: '300px minmax(0, 1fr)' } }} gap={4} alignItems="start">
        {/* Debajo de 1024 px los pasos van en fila: apilados, en la tableta vertical el paso empezaba a la
            mitad de la pantalla. */}
        <Stack as="nav" aria-label="Pasos del cierre" direction={{ base: 'row', lg: 'column' }} align="stretch" gap={1}
          position={{ lg: 'sticky' }} top={0} overflowX={{ base: 'auto', lg: 'visible' }}>
          <Text fontWeight="800" mb={1} display={{ base: 'none', lg: 'block' }}>Cerrar {register.name}</Text>
          {PASOS.map((p) => {
            const estado = estadoDelPaso(pendientes, p, visitados.includes(p));
            return (
              <Button key={p} variant={p === paso ? 'subtle' : 'ghost'} justifyContent="flex-start" h="auto" minH="56px" py={2}
                flex={{ base: '1 0 auto', lg: 'none' }}
                aria-current={p === paso ? 'step' : undefined} onClick={() => irAlPaso(p)}>
                <HStack gap={3} align="center" w="100%">
                  <Center w="32px" h="32px" borderRadius="full" flexShrink={0} fontWeight="800"
                    bg={estado === 'falta' ? 'orange.solid' : estado === 'listo' ? 'green.solid' : 'bg.emphasized'}
                    color={estado === 'pendiente' ? 'fg' : 'white'}>
                    {estado === 'listo' ? <LuCheck /> : p}
                  </Center>
                  <VStack align="start" gap={0} minW={0}>
                    <Text fontWeight="700" textAlign="start">{NOMBRES_DE_PASOS[p]}</Text>
                    <Text fontSize="xs" color="fg.muted" textAlign="start">{resumenDelPaso(p)}</Text>
                  </VStack>
                </HStack>
              </Button>
            );
          })}
          <Button variant="ghost" minH="48px" mt={{ lg: 2 }} flexShrink={0} onClick={() => onCerrando(false)}>Volver al turno</Button>
        </Stack>

        <VStack align="stretch" gap={4} minW={0} css={{ [desde(ANCHO_PC)]: { maxWidth: '1100px' } }}>
          <Text fontSize="xl" fontWeight="800">{NOMBRES_DE_PASOS[paso]}</Text>

          {paso === 1 && (<>
            <AvisoDelCierre estado={estadoDeAsistencia} autorizacion={autorizacion}
              onAutorizar={setAutorizacion} onQuitarAutorizacion={() => setAutorizacion(null)}
              onActualizar={onActualizarAsistencia} />
            {/* Cada pendiente de este paso ya tiene su bloque con su acción: repetirlos en una lista eran
                tres bloques para dos asuntos. La lista con «Ir a…» va en el último paso. */}
            {pendientes.every((x) => x.paso !== 1) && <Text color="fg.muted">Nada pendiente: sigue con el cajón.</Text>}
            <CuentasDelCierre pending={session.pending} owing={session.owing ?? []} cuentas={session.liveAccounts ?? []}
              onAbrir={(ruta) => navigate(ruta)} onDescartar={descartarDelCierre} onCancelar={cancelarDelCierre}
              onCancelarResto={(id, motivo) => cancelarDelCierre(id, motivo, true)} />
            <Box ref={refDelCierre}>
              <PropinasDelCierre pendiente={session.tipsPending} currency={session.currency} decision={decisionPropinas}
                onEntregarAhora={() => { marcarVueltaAlCierre(); setRepartiendo(true); }} onDecidir={setDecisionPropinas} />
            </Box>
            {/* Lo que se vendió y nadie pagó: no bloquea, pero el arqueo no lo incluye. */}
            {Number(session.uncollected) > 0 && (
              <Box borderWidth="1px" borderColor="border.error" bg="bg.error" borderRadius="lg" p={3}>
                <Text fontWeight="700" color="fg.error">
                  Sin cobrar: {montoDeCaja(session.uncollected, session.currency)}
                  {' en '}{session.uncollectedCount === 1 ? '1 pedido' : `${session.uncollectedCount} pedidos`}
                </Text>
                <Text fontSize="sm" color="fg.muted">Este dinero no está en el arqueo. Cóbralo antes de cerrar o el corte va a cuadrar sin él.</Text>
              </Box>
            )}
            {Number(session.writtenOff ?? 0) > 0 && (
              <Text fontSize="sm" color="fg.muted">Perdido: {montoDeCaja(session.writtenOff ?? '0', session.currency)} (se canceló lo que faltaba)</Text>
            )}
            {(session.cashOutsWithoutConcept ?? 0) > 0 && (
              <AvisosDeCaja tipsPending="0" cashOutsWithoutConcept={session.cashOutsWithoutConcept ?? 0} enCaja />
            )}
          </>)}

          {paso === 2 && (
            cajon === null ? (
              <Text color="fg.muted">Esta caja no lleva arqueo del cajón: sigue con los otros métodos.</Text>
            ) : (
              <VStack align="stretch" gap={3}>
                <Text color="fg.muted">Cuenta billetes y monedas del cajón.</Text>
                <HStack borderWidth="1px" borderRadius="lg" p={4} justify="space-between" flexWrap="wrap" gap={3} bg="bg.panel">
                  <Box>
                    <Text fontSize="sm" color="fg.muted">Contado en el cajón</Text>
                    <Text fontSize="2xl" fontWeight="800" aria-label="Contado en el cajón">
                      {contadoAlCerrar === null ? '—' : montoDeCaja(contadoAlCerrar, session.currency)}
                    </Text>
                  </Box>
                  <Button minH="52px" colorPalette={contadoAlCerrar === null ? 'brand' : 'gray'} variant={contadoAlCerrar === null ? 'solid' : 'outline'}
                    onClick={() => setContando(true)}>
                    {contadoAlCerrar === null ? 'Contar efectivo' : 'Volver a contar'}
                  </Button>
                </HStack>
              </VStack>
            )
          )}

          {paso === 3 && (<>
            <TablaDelCierre totals={session.totals ?? []} currency={session.currency}
              declared={declared} onDeclared={setDeclared}
              cajon={cajon} conteo={conteoDelCierre} onContar={() => setContando(true)}
              diferencias={diferencias} />
            {/* Un campo en blanco se guardaba como cero declarado: escribir 0 es válido, dejarlo vacío no. */}
            <ConteoDeTerminales terminales={session.terminalsToCount ?? []} valores={conteoTerminales} onChange={setConteoTerminales} />
          </>)}

          {ultimo && (<>
            {cajon && <FondoQueSeDeja value={fondo} onChange={setFondo} contado={contadoAlCerrar} fijo={fijo}
              retirar={retirar} onRetirar={setRetirar} personas={personas} quien={quien} onQuien={setQuien}
              quienCierra={yo} currency={session.currency} />}
            {avisoQuien && (
              <Box borderWidth="1px" borderColor="border" borderRadius="lg" p={3} colorPalette="orange" bg="colorPalette.subtle">
                <Text fontSize="sm" fontWeight="600">{avisoQuien}</Text>
              </Box>
            )}
            <DiferenciaDelCierre diferencias={diferencias} cajon={cajon} conteo={conteoDelCierre} currency={session.currency} />
            {pideExplicacion && (
              <Section title={`Explica la diferencia (pasa la tolerancia de ${montoDeCaja(ajustesDeCierre?.tolerance ?? '0', session.currency)})`}>
                <HStack gap={2} flexWrap="wrap" mb={2}>
                  {MOTIVOS_DE_CORRECCION.filter((m) => m.value !== 'float_error').map((m) => (
                    <Button key={m.value} minH="48px" variant={explicacion.reason === m.value ? 'solid' : 'outline'}
                      aria-pressed={explicacion.reason === m.value} onClick={() => setExplicacion((x) => ({ ...x, reason: m.value }))}>{m.label}</Button>
                  ))}
                </HStack>
                <Textarea aria-label="Qué pasó con la diferencia" placeholder="Qué pasó" maxLength={200} value={explicacion.note}
                  onChange={(e) => setExplicacion((x) => ({ ...x, note: e.target.value }))} />
                <Text fontSize="sm" color="fg.muted" mt={2}>Si no la explicas ahora, el corte queda «Por explicar» en el Histórico.</Text>
              </Section>
            )}
            <RefundsList refunds={session.refunds} currency={session.currency} zona={horaNegocio.zona} />
            {/* Quién cobró qué, solo con más de una persona; con arqueo ciego no (suma lo mismo que el desglose). */}
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
                        <Table.Cell textAlign="end" fontWeight="700">{montoDeCaja(c.cash, session.currency)}</Table.Cell>
                        <Table.Cell textAlign="end" color="fg.muted">{montoDeCaja(c.other, session.currency)}</Table.Cell>
                      </Table.Row>
                    ))}
                  </Table.Body>
                </Table.Root>
              </Box>
            )}
            <Textarea rows={2} resize="none" placeholder="Notas del cierre (opcional)" value={notes} onChange={(e) => setNotes(e.target.value)} />
            <ListaDePendientes pendientes={pendientes} onIr={irAlPaso} actual={paso} vacio={null} titulo="Para cerrar falta" />
          </>)}

          <HStack justify="space-between" gap={3} pt={3} borderTopWidth="1px" position="sticky" bottom={0} bg="bg" pb={2}>
            <Button minH="52px" variant="outline" disabled={paso === 1} onClick={() => irAlPaso((paso - 1) as PasoDelCierre)}>
              <LuChevronLeft /> Atrás
            </Button>
            {/* En el último paso, «Cerrar caja» va aquí, siempre a la vista: al final del contenido quedaba
                varias pantallas abajo en 600 px de alto. */}
            {ultimo ? (
              <BotonCerrarCaja nombre={register.name} loading={closeMut.isPending}
                descripcion={conAvisoDeAutorizacion(cajon ? resumenDelRetiro({ retirar, retiro, quien: personas.find((p) => p.id === quien)?.name ?? null,
                  contado: contadoAlCerrar, currency: session.currency }) : undefined)}
                disabled={pendientes.length > 0}
                onCerrar={() => closeMut.mutate()} />
            ) : (
              <Button minH="52px" colorPalette="brand" onClick={() => irAlPaso((paso + 1) as PasoDelCierre)}>
                Siguiente: {NOMBRES_DE_PASOS[(paso + 1) as PasoDelCierre]} <LuChevronRight />
              </Button>
            )}
          </HStack>
        </VStack>
      </Grid>
      {sheets}
    </>
  );
}

// ComoVaElTurno: el Corte X (#290, FR-034). Solo consulta: no cierra ni guarda nada. Con arqueo ciego
// lee la misma vista filtrada por el servidor, así que no muestra el esperado.
function ComoVaElTurno({ session, pendientes, onCerrar }: { session: CashSession; pendientes: number; onCerrar: () => void }) {
  const [ayuda, setAyuda] = useState(false);
  return (
    <Box borderWidth="1px" borderRadius="lg" p={4} bg="bg.panel">
      <HStack justify="space-between" mb={2}>
        <Text fontWeight="700">Cómo va el turno</Text>
        <InfoButton label="Qué es «Cómo va el turno»" open={ayuda} onToggle={() => setAyuda((v) => !v)} />
      </HStack>
      {ayuda && (
        <Text fontSize="sm" color="fg.muted" mb={2}>
          Es el corte X: las cifras del turno hasta ahora. Consultarlo no cierra la caja ni guarda nada.
        </Text>
      )}
      {session.blind ? (
        <Text fontSize="sm" color="fg.muted">El resumen aparece al confirmar el cierre, junto con la diferencia.</Text>
      ) : (
        <IngresosEgresosCard openingCash={session.openingCash} breakdown={session.breakdown} currency={session.currency} />
      )}
      {Number(session.uncollected) > 0 && (
        <Text fontSize="sm" color="fg.error" mt={2}>
          Sin cobrar: {montoDeCaja(session.uncollected, session.currency)} en {session.uncollectedCount === 1 ? '1 pedido' : `${session.uncollectedCount} pedidos`}
        </Text>
      )}
      <Button mt={3} w="100%" minH="56px" colorPalette="brand" fontSize="lg" onClick={onCerrar}>
        <LuLock /> Cerrar caja
      </Button>
      {pendientes > 0 && (
        <Text fontSize="sm" color="fg.muted" mt={2} textAlign="center">
          {pendientes === 1 ? 'Falta 1 cosa por resolver antes de cerrar' : `Faltan ${pendientes} cosas por resolver antes de cerrar`}
        </Text>
      )}
    </Box>
  );
}

// ListaDePendientes: lo que impide cerrar, cada cosa con un toque que lleva a su paso. Es la misma
// lista que apaga el botón (FR-030).
function ListaDePendientes({ pendientes, onIr, actual, vacio, titulo }: {
  pendientes: PendienteDelCierre[]; onIr?: (p: PasoDelCierre) => void; actual?: PasoDelCierre; vacio: string | null; titulo?: string;
}) {
  if (pendientes.length === 0) return vacio === null ? null : <Text color="fg.muted">{vacio}</Text>;
  return (
    <Box borderWidth="1px" borderRadius="lg" p={3} colorPalette="orange" bg="colorPalette.subtle" role="status">
      {titulo && <Text fontWeight="700" mb={2}>{titulo}:</Text>}
      <VStack align="stretch" gap={2}>
        {pendientes.map((x) => (
          <HStack key={x.clave} justify="space-between" gap={2} flexWrap="wrap">
            <Text fontWeight="600">{x.texto}</Text>
            {onIr && x.paso !== actual && <Button size="sm" minH="48px" variant="outline" onClick={() => onIr(x.paso)}>Ir a {NOMBRES_DE_PASOS[x.paso]}</Button>}
          </HStack>
        ))}
      </VStack>
    </Box>
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
export function BotonCerrarCaja({ nombre, disabled, loading, onCerrar, descripcion = FIN_DEL_CIERRE }: {
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
