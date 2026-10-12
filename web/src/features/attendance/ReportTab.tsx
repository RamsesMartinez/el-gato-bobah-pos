import { useState } from 'react';
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Badge, Box, Button, Flex, HStack, Input, Table, Text, VStack } from '@chakra-ui/react';
import {
  attendanceAdminApi, attendanceCorrectionApi, type ReportDay, type ReportEntry, type ReportPerson,
} from '../../api/attendance';
import { auditApi } from '../../api/audit';
import { ApiError } from '../../api/client';
import { can } from '../../app/permissions';
import { isOffline } from '../../api/mensajes';
import { LoadError } from '../../shared/LoadError';
import { Picker } from '../../components/Picker';
import { DatePickerButton } from '../../components/DatePickerButton';
import { RangoDeFechas } from '../../components/RangoDeFechas';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';
import { useSessionStore } from '../../stores/session';
import { soloHora } from '../../utils/horaDelNegocio';
import { QUARTERS } from '../../shared/attendance/schedule';
import {
  daysBetween, instantInZone, loadView, minutesLabel, rowsByDay, saveView, weekViewAllowed, type ReportView,
} from './report';

const PRESETS = [
  { id: 'semana', label: 'Esta semana' },
  { id: 'pasada', label: 'Semana pasada' },
  { id: 'mes', label: 'Este mes' },
];

function mondayOf(iso: string): string {
  const d = new Date(`${iso}T12:00:00Z`);
  d.setUTCDate(d.getUTCDate() - ((d.getUTCDay() + 6) % 7));
  return d.toISOString().slice(0, 10);
}

function addDays(iso: string, n: number): string {
  const d = new Date(`${iso}T12:00:00Z`);
  d.setUTCDate(d.getUTCDate() + n);
  return d.toISOString().slice(0, 10);
}

const WEEKDAY_ABBR = ['dom', 'lun', 'mar', 'mié', 'jue', 'vie', 'sáb'];

const MONTH_ABBR = ['ene', 'feb', 'mar', 'abr', 'may', 'jun', 'jul', 'ago', 'sep', 'oct', 'nov', 'dic'];

// diaSemana: «lun 12», lo que se lee en el encabezado de la cuadrícula de la semana.
function weekdayDay(iso: string): string {
  return `${WEEKDAY_ABBR[new Date(`${iso}T12:00:00Z`).getUTCDay()]} ${Number(iso.slice(8))}`;
}

// diaLargo: «lun 12 oct», para los renglones; «2026-10-12» es formato de base de datos.
function longDay(iso: string): string {
  return `${weekdayDay(iso)} ${MONTH_ABBR[Number(iso.slice(5, 7)) - 1]}`;
}

function statusLabel(d: ReportDay): string {
  if (d.missingOut) return 'Falta salida';
  if (d.entries.some((e) => e.source === 'manual')) return 'A mano';
  if (d.entries.some((e) => e.corrected)) return 'Corregido';
  return '';
}

function entryRange(e: ReportEntry, zone: string): string {
  return `${soloHora(e.in, zone)} – ${e.out ? soloHora(e.out, zone) : 'Falta salida'}`;
}

function deviceLabel(dev?: Record<string, string>): string {
  return [dev?.platform, dev?.browser, dev?.model].filter(Boolean).join(' · ');
}

// ReportTab es el reporte de asistencia (maquetas A, B y C con selector): por persona, día por día
// o una semana en cuadrícula, todas de la misma respuesta. Tocar un día abre su detalle con IP,
// aparato y las acciones «Corregir» y «Capturar a mano», según el permiso.
export function ReportTab() {
  const { zona: zone, diaDelNegocio } = useHoraDelNegocio();
  const user = useSessionStore((s) => s.user);
  const today = diaDelNegocio(new Date());
  const [preset, setPreset] = useState('semana');
  const [fromDate, setFromDate] = useState('');
  const [toDate, setToDate] = useState('');
  const [personFilter, setPersonFilter] = useState('');
  const [view, setView] = useState<ReportView>(loadView);
  const [expanded, setExpanded] = useState<number | null>(null);
  const [selectedDay, setSelectedDay] = useState<{ person: ReportPerson; day: ReportDay } | null>(null);
  const [manualOpen, setManualOpen] = useState(false);

  const range = preset === 'semana' ? { from: mondayOf(today), to: today }
    : preset === 'pasada' ? { from: addDays(mondayOf(today), -7), to: addDays(mondayOf(today), -1) }
      : preset === 'mes' ? { from: `${today.slice(0, 8)}01`, to: today }
        : { from: fromDate, to: toDate };
  const ready = Boolean(range.from && range.to);
  const weekViewPossible = weekViewAllowed(range.from, range.to);
  const effectiveView: ReportView = view === 'semana' && !weekViewPossible ? 'persona' : view;

  const actorsQuery = useQuery({ queryKey: ['audit', 'actors'], queryFn: auditApi.actors, enabled: can('audit.view', user) });
  const { data: fetched, isError, error, refetch, isFetching } = useQuery({
    queryKey: ['asistencia', 'report', range, personFilter],
    queryFn: () => attendanceAdminApi.report(range.from, range.to, personFilter ? Number(personFilter) : undefined),
    enabled: ready,
    placeholderData: keepPreviousData,
    // Sin red, TanStack pausa y deja el periodo anterior bajo el nuevo, sin aviso. Con `always`
    // falla y el reporte viejo se descarta: no es el periodo que se pidió.
    networkMode: 'always', retry: 1,
  });
  const data = isError ? undefined : fetched;
  const people = data?.people ?? [];
  const measure = data?.measureSchedule ?? false;
  const chooseView = (v: ReportView) => { setView(v); saveView(v); };

  const segmentButton = (v: ReportView, label: string, disabled = false) => (
    <Button key={v} minH="44px" px={4} borderRadius={0} disabled={disabled}
      variant={effectiveView === v ? 'solid' : 'outline'} aria-pressed={effectiveView === v}
      title={disabled ? 'Solo para una semana' : undefined} onClick={() => chooseView(v)}>
      {label}
    </Button>
  );

  return (
    <Flex direction="column" gap={3} h="100%" minH={0}>
      {/* Dos renglones y no tres: el periodo y la persona arriba, la vista y la captura abajo. En
          600 px de alto cada renglón de filtros es un renglón menos de reporte. */}
      <Flex gap={2} align="flex-start">
        <Box flex="1" minW={0}>
          <RangoDeFechas presets={PRESETS} preset={preset} onPreset={setPreset} desde={fromDate} hasta={toDate}
            onRango={(d, h) => { setFromDate(d); setToDate(h); }} hoy={today} />
        </Box>
        {actorsQuery.data && (
          <Box w="200px" flexShrink={0}>
            <Picker title="Persona" placeholder="Todas las personas" clearable clearLabel="Todas las personas" value={personFilter}
              onChange={setPersonFilter} options={actorsQuery.data.map((a) => ({ value: String(a.id), label: a.name }))} />
          </Box>
        )}
      </Flex>
      <Flex gap={2} justify="space-between" align="center">
        <HStack gap={0} borderRadius="md" overflow="hidden">
          {segmentButton('persona', 'Por persona')}
          {segmentButton('dia', 'Día por día')}
          {segmentButton('semana', 'Semana', !weekViewPossible)}
        </HStack>
        {can('attendance.correct_others', user) && (
          <Button minH="44px" variant="outline" onClick={() => setManualOpen(true)}>Capturar a mano</Button>
        )}
      </Flex>

      <Box flex="1" minH={0} overflow="auto" borderWidth="1px" borderRadius="lg">
        {isError && <LoadError error={error} onRetry={() => void refetch()} retrying={isFetching} />}
        {ready && data && people.length === 0 && <Text p={4} color="fg.muted">Sin registros en este periodo.</Text>}
        {people.length > 0 && effectiveView === 'persona' && (
          <Table.Root size="sm" stickyHeader>
            <Table.Header><Table.Row>
              <Table.ColumnHeader>Persona</Table.ColumnHeader>
              <Table.ColumnHeader>Días</Table.ColumnHeader>
              <Table.ColumnHeader>Horas</Table.ColumnHeader>
              {measure && <Table.ColumnHeader>Retardo</Table.ColumnHeader>}
              {measure && <Table.ColumnHeader>Extra</Table.ColumnHeader>}
              <Table.ColumnHeader>Avisos</Table.ColumnHeader>
            </Table.Row></Table.Header>
            <Table.Body>
              {people.flatMap((p) => [
                <Table.Row key={p.userId} h="48px" cursor="pointer" onClick={() => setExpanded(expanded === p.userId ? null : p.userId)}>
                  <Table.Cell fontWeight="700">{p.name}</Table.Cell>
                  <Table.Cell>{p.days.length}</Table.Cell>
                  <Table.Cell>{minutesLabel(p.workedMinutes)}</Table.Cell>
                  {measure && <Table.Cell>{p.lateMinutes !== undefined ? `${p.lateMinutes} min` : 'Sin horario'}</Table.Cell>}
                  {measure && <Table.Cell>{p.overtimeMinutes !== undefined ? `${p.overtimeMinutes} min` : 'Sin horario'}</Table.Cell>}
                  <Table.Cell>{p.missingOutDays > 0 && <Badge colorPalette="orange">{p.missingOutDays} sin salida</Badge>}</Table.Cell>
                </Table.Row>,
                ...(expanded === p.userId ? p.days.map((d) => (
                  <Table.Row key={`${p.userId}-${d.date}`} h="44px" cursor="pointer" bg="bg.subtle" onClick={() => setSelectedDay({ person: p, day: d })}>
                    <Table.Cell pl={8} color="fg.muted">{longDay(d.date)}</Table.Cell>
                    <Table.Cell>{d.entries.map((e) => `${soloHora(e.in, zone)}–${e.out ? soloHora(e.out, zone) : '?'}`).join(', ')}</Table.Cell>
                    <Table.Cell>{d.missingOut ? '—' : minutesLabel(d.workedMinutes)}</Table.Cell>
                    {measure && <Table.Cell>{d.lateMinutes ?? '—'}</Table.Cell>}
                    {measure && <Table.Cell>{d.overtimeMinutes ?? '—'}</Table.Cell>}
                    <Table.Cell>{statusLabel(d) && <Badge>{statusLabel(d)}</Badge>}</Table.Cell>
                  </Table.Row>
                )) : []),
              ])}
            </Table.Body>
          </Table.Root>
        )}
        {people.length > 0 && effectiveView === 'dia' && (
          <Table.Root size="sm" stickyHeader>
            <Table.Header><Table.Row>
              <Table.ColumnHeader>Día</Table.ColumnHeader>
              <Table.ColumnHeader>Persona</Table.ColumnHeader>
              <Table.ColumnHeader>Entrada – Salida</Table.ColumnHeader>
              <Table.ColumnHeader>Horas</Table.ColumnHeader>
              {measure && <Table.ColumnHeader>Retardo</Table.ColumnHeader>}
              {measure && <Table.ColumnHeader>Extra</Table.ColumnHeader>}
              <Table.ColumnHeader>Avisos</Table.ColumnHeader>
            </Table.Row></Table.Header>
            <Table.Body>
              {rowsByDay(people).map(({ person, day }) => (
                <Table.Row key={`${person.userId}-${day.date}`} h="48px" cursor="pointer" onClick={() => setSelectedDay({ person, day })}>
                  <Table.Cell>{longDay(day.date)}</Table.Cell>
                  <Table.Cell fontWeight="700">{person.name}</Table.Cell>
                  {/* Un renglón por registro: la primera entrada y la última salida escondían el hueco
                      entre dos turnos del mismo día. */}
                  <Table.Cell>
                    {day.entries.map((e) => <Text key={e.id}>{entryRange(e, zone)}</Text>)}
                  </Table.Cell>
                  <Table.Cell>{day.missingOut ? '—' : minutesLabel(day.workedMinutes)}</Table.Cell>
                  {measure && <Table.Cell>{day.lateMinutes ?? '—'}</Table.Cell>}
                  {measure && <Table.Cell>{day.overtimeMinutes ?? '—'}</Table.Cell>}
                  <Table.Cell>{statusLabel(day) && <Badge>{statusLabel(day)}</Badge>}</Table.Cell>
                </Table.Row>
              ))}
            </Table.Body>
          </Table.Root>
        )}
        {people.length > 0 && effectiveView === 'semana' && (
          <Table.Root size="sm" stickyHeader>
            <Table.Header><Table.Row>
              <Table.ColumnHeader>Persona</Table.ColumnHeader>
              {daysBetween(range.from, range.to).map((d) => <Table.ColumnHeader key={d} textAlign="center">{weekdayDay(d)}</Table.ColumnHeader>)}
              <Table.ColumnHeader>Total</Table.ColumnHeader>
            </Table.Row></Table.Header>
            <Table.Body>
              {people.map((p) => (
                <Table.Row key={p.userId} h="52px">
                  <Table.Cell fontWeight="700">{p.name}</Table.Cell>
                  {daysBetween(range.from, range.to).map((date) => {
                    const d = p.days.find((x) => x.date === date);
                    return (
                      <Table.Cell key={date} textAlign="center" cursor={d ? 'pointer' : undefined} onClick={() => d && setSelectedDay({ person: p, day: d })}>
                        {d ? (d.missingOut ? <Badge colorPalette="orange">Falta salida</Badge> : minutesLabel(d.workedMinutes)) : '·'}
                      </Table.Cell>
                    );
                  })}
                  <Table.Cell fontWeight="700">{minutesLabel(p.workedMinutes)}</Table.Cell>
                </Table.Row>
              ))}
            </Table.Body>
          </Table.Root>
        )}
      </Box>

      {selectedDay && <DayDetail person={selectedDay.person} day={selectedDay.day} onClose={() => setSelectedDay(null)} />}
      {manualOpen && <ManualEntrySheet people={actorsQuery.data ?? []} onClose={() => setManualOpen(false)} />}
    </Flex>
  );
}

function Sheet({ title, onClose, children }: { title: string; onClose: () => void; children: React.ReactNode }) {
  return (
    <Flex position="fixed" inset={0} bg="blackAlpha.400" align="flex-end" zIndex={1400} onClick={(e) => { if (e.target === e.currentTarget) onClose(); }}>
      <Box role="dialog" aria-label={title} bg="bg" w="100%" maxH="90dvh" overflowY="auto" borderTopRadius="xl"
        px={6} pt={4} pb={6}>
        <Flex justify="space-between" align="center" mb={3}>
          <Text fontSize="lg" fontWeight="800">{title}</Text>
          <Button variant="ghost" minH="44px" onClick={onClose}>Cerrar</Button>
        </Flex>
        {children}
      </Box>
    </Flex>
  );
}

// DayDetail es el día de una persona con el origen de cada registro y la corrección.
function DayDetail({ person, day, onClose }: { person: ReportPerson; day: ReportDay; onClose: () => void }) {
  const user = useSessionStore((s) => s.user);
  const { zona: zone } = useHoraDelNegocio();
  const [correcting, setCorrecting] = useState<{ entry: ReportEntry; field: 'clock_in_at' | 'clock_out_at' } | null>(null);
  const isOwn = user?.id === person.userId;
  const canCorrectEntry = can(isOwn ? 'attendance.correct_own' : 'attendance.correct_others', user);

  return (
    <Sheet title={`${person.name} · ${longDay(day.date)}`} onClose={onClose}>
      <VStack align="stretch" gap={3}>
        {day.entries.map((e) => (
          <Box key={e.id} borderWidth="1px" borderRadius="lg" p={3}>
            <HStack justify="space-between" wrap="wrap">
              <Text fontWeight="700">
                {soloHora(e.in, zone)} – {e.out ? soloHora(e.out, zone) : 'Falta salida'}
                {e.source === 'manual' && <Badge ml={2}>A mano</Badge>}
                {e.corrected && <Badge ml={2}>Corregido</Badge>}
              </Text>
              {canCorrectEntry && (
                <HStack gap={2}>
                  <Button minH="44px" variant="outline" onClick={() => setCorrecting({ entry: e, field: 'clock_in_at' })}>Corregir entrada</Button>
                  <Button minH="44px" variant="outline" onClick={() => setCorrecting({ entry: e, field: 'clock_out_at' })}>
                    {e.out ? 'Corregir salida' : 'Capturar salida'}
                  </Button>
                </HStack>
              )}
            </HStack>
            {e.source === 'manual' && !e.inIp ? <Text fontSize="sm" color="fg.muted">Capturado a mano, no desde una tableta</Text> : <>
            <Text fontSize="sm" color="fg.muted">Entrada: {[e.inIp, deviceLabel(e.inDevice), e.inDeviceKey && `Aparato ${e.inDeviceKey.slice(0, 4).toUpperCase()}`].filter(Boolean).join(' · ') || '—'}</Text>
            {e.out && <Text fontSize="sm" color="fg.muted">Salida: {[e.outIp, deviceLabel(e.outDevice), e.outDeviceKey && `Aparato ${e.outDeviceKey.slice(0, 4).toUpperCase()}`].filter(Boolean).join(' · ') || '—'}</Text>}
            </>}
          </Box>
        ))}
      </VStack>
      {correcting && (
        <CorrectionForm personName={person.name} date={day.date} entry={correcting.entry} field={correcting.field}
          onDone={() => { setCorrecting(null); onClose(); }} onCancel={() => setCorrecting(null)} />
      )}
    </Sheet>
  );
}

function CorrectionForm({ personName, date, entry, field, onDone, onCancel }: {
  personName: string; date: string; entry: ReportEntry; field: 'clock_in_at' | 'clock_out_at'; onDone: () => void; onCancel: () => void;
}) {
  const qc = useQueryClient();
  const { zona: zone } = useHoraDelNegocio();
  const [timeValue, setTimeValue] = useState('');
  const [reason, setReason] = useState('');
  const [error, setError] = useState('');
  const save = useMutation({
    networkMode: 'always',
    mutationFn: () => {
      let at = instantInZone(date, Number(timeValue), zone);
      // Una salida que no es posterior a la entrada es del día siguiente (turno nocturno): 22:00 a
      // 06:00 termina mañana, no antes de empezar.
      if (field === 'clock_out_at' && Date.parse(at) <= Date.parse(entry.in)) {
        at = instantInZone(addDays(date, 1), Number(timeValue), zone);
      }
      return attendanceCorrectionApi.correct(entry.id, field, at, reason.trim());
    },
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ['asistencia', 'report'] }); onDone(); },
    onError: (e) => setError(saveErrorMessage(e, personName)),
  });
  return (
    <VStack align="stretch" gap={2} mt={4} borderTopWidth="1px" pt={4}>
      <Text fontWeight="700">
        {field === 'clock_in_at' ? 'Entrada' : 'Salida'} correcta del registro de {entryRange(entry, zone)}
      </Text>
      <HStack gap={2}>
        <Box w="136px"><Picker title="Hora" placeholder="Hora" value={timeValue} options={QUARTERS} onChange={setTimeValue} /></Box>
        <Input minH="44px" flex="1" placeholder="Motivo (obligatorio)" value={reason} onChange={(e) => setReason(e.target.value)} />
      </HStack>
      {error && <Text color="red.500" fontWeight="600">{error}</Text>}
      <HStack justify="flex-end" gap={2}>
        <Button variant="ghost" minH="44px" onClick={onCancel}>Cancelar</Button>
        <Button minH="44px" disabled={!timeValue || reason.trim().length < 3} loading={save.isPending} onClick={() => save.mutate()}>Guardar corrección</Button>
      </HStack>
    </VStack>
  );
}

// saveErrorMessage: qué hacer cuando no se guardó. El traslape se dice con el nombre de la persona;
// el texto del servidor venía en minúscula y sin decir dónde mirar.
function saveErrorMessage(e: unknown, personName: string): string {
  if (isOffline(e)) return 'Sin conexión. No se guardó; vuelve a intentar cuando regrese la red.';
  if (e instanceof ApiError && e.code === 'ATTENDANCE_OVERLAP') {
    return `Esa hora se encima con otro registro de ${personName}. Revisa los registros de este día.`;
  }
  return e instanceof ApiError ? e.message : 'No se pudo guardar.';
}

// ManualEntrySheet captura un turno completo de alguien que no checó.
function ManualEntrySheet({ people, onClose }: { people: Array<{ id: number; name: string }>; onClose: () => void }) {
  const qc = useQueryClient();
  const { zona: zone, diaDelNegocio } = useHoraDelNegocio();
  const [personFilter, setPersonFilter] = useState('');
  const [dayValue, setDayValue] = useState(diaDelNegocio(new Date()));
  const [inTime, setInTime] = useState('');
  const [outTime, setOutTime] = useState('');
  const [reason, setReason] = useState('');
  const [error, setError] = useState('');
  const save = useMutation({
    networkMode: 'always',
    mutationFn: () => {
      const inAt = instantInZone(dayValue, Number(inTime), zone);
      // Una salida que no es posterior a la entrada es del día siguiente (turno nocturno).
      const outDate = Number(outTime) <= Number(inTime) ? addDays(dayValue, 1) : dayValue;
      return attendanceCorrectionApi.manual(Number(personFilter), inAt, instantInZone(outDate, Number(outTime), zone), reason.trim());
    },
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ['asistencia', 'report'] }); onClose(); },
    onError: (e) => setError(saveErrorMessage(e, people.find((p) => String(p.id) === personFilter)?.name ?? 'esta persona')),
  });
  return (
    <Sheet title="Capturar a mano" onClose={onClose}>
      <VStack align="stretch" gap={3}>
        <Picker title="Persona" placeholder="Elige a la persona" value={personFilter} onChange={setPersonFilter}
          options={people.map((p) => ({ value: String(p.id), label: p.name }))} />
        <HStack gap={2}>
          <DatePickerButton label="Día" value={dayValue} max={diaDelNegocio(new Date())} onChange={setDayValue} />
          <Box w="136px"><Picker title="Entrada" placeholder="Entrada" value={inTime} options={QUARTERS} onChange={setInTime} /></Box>
          <Box w="136px"><Picker title="Salida" placeholder="Salida" value={outTime} options={QUARTERS} onChange={setOutTime} /></Box>
        </HStack>
        <Input minH="44px" placeholder="Motivo (obligatorio)" value={reason} onChange={(e) => setReason(e.target.value)} />
        {error && <Text color="red.500" fontWeight="600">{error}</Text>}
        <Button minH="44px" alignSelf="flex-end" loading={save.isPending}
          disabled={!personFilter || !inTime || !outTime || reason.trim().length < 3} onClick={() => save.mutate()}>
          Guardar registro
        </Button>
      </VStack>
    </Sheet>
  );
}
