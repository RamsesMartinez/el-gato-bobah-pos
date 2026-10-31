import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Badge, Box, Button, Flex, HStack, IconButton, Text, VStack } from '@chakra-ui/react';
import { LuCircleHelp } from 'react-icons/lu';
import { attendanceAdminApi } from '../../api/attendance';
import { ApiError } from '../../api/client';
import { Picker } from '../../components/Picker';
import { DatePickerButton } from '../../components/DatePickerButton';
import { Switch } from '../../components/ui/switch';
import { DialogRoot, DialogBackdrop, DialogContent, DialogBody, DialogHeader, DialogTitle, DialogFooter } from '../../components/ui/dialog';
import { QUARTERS, WEEK, copyToWeekdays, crossesMidnight, emptyWeek, shiftsFromWeek, weekFromShifts, type Week } from '../../shared/attendance/schedule';

// ScheduleEditor es el horario de una persona (maqueta A, ronda 2): un renglón por día con hasta
// dos turnos, «+1 día» cuando la salida es del día siguiente, copiar a lunes–viernes y copiar de
// otra persona. Guardar crea una versión nueva con su vigencia; el pasado no se reescribe.
export function ScheduleEditor({ userId, name, people, onClose }: {
  userId: number;
  name: string;
  people: Array<{ id: number; name: string }>;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const existing = useQuery({ queryKey: ['asistencia', 'schedule', userId], queryFn: () => attendanceAdminApi.schedules(userId) });
  const [week, setWeek] = useState<Week | null>(null);
  const [fromDate, setFromDate] = useState(() => new Date().toISOString().slice(0, 10));
  const [tracks, setTracks] = useState<boolean | null>(null);
  const [error, setError] = useState('');
  const [confirmDiscard, setConfirmDiscard] = useState(false);
  const [help, setHelp] = useState(false);
  const displayedWeek = week ?? (existing.data?.versions[0] ? weekFromShifts(existing.data.versions[0].days) : emptyWeek());
  const tracksValue = tracks ?? existing.data?.tracksAttendance ?? true;
  const unsaved = week !== null || tracks !== null;

  const save = useMutation({
    mutationFn: async () => {
      if (tracks !== null && tracks !== existing.data?.tracksAttendance) await attendanceAdminApi.setTracks(userId, tracks);
      if (week !== null) await attendanceAdminApi.setSchedule(userId, fromDate, shiftsFromWeek(week));
    },
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ['asistencia'] }); onClose(); },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'No se pudo guardar.'),
  });

  const changeShift = (wd: number, i: number, campo: 'start' | 'end', min: number) => {
    const w = { ...displayedWeek, [wd]: displayedWeek[wd].map((s, j) => (j === i ? { ...s, [campo]: min } : s)) };
    setWeek(w);
  };
  const copyFrom = async (other: string) => {
    const r = await attendanceAdminApi.schedules(Number(other)).catch(() => null);
    setWeek(r?.versions[0] ? weekFromShifts(r.versions[0].days) : emptyWeek());
  };

  return (
    <Flex position="fixed" inset={0} bg="blackAlpha.400" align="flex-end" zIndex={1500}>
      <Box role="dialog" aria-label={`Horario de ${name}`} bg="bg" w="100%" maxH="96dvh" overflowY="auto"
        borderTopRadius="xl" px={6} pt={4} pb={5}>
        {/* Fijo arriba: con dos turnos en un día la hoja desplaza, y «Guardar» no puede quedar fuera
            de la vista de quien edita el viernes. */}
        <Flex justify="space-between" align="center" mb={2} gap={2} wrap="wrap" position="sticky" top={0} bg="bg" zIndex={1} py={1}>
          <Text fontSize="lg" fontWeight="800">{name} · Horario</Text>
          <HStack gap={2}>
            <DatePickerButton label="Desde" value={fromDate} onChange={setFromDate} />
            <Button variant="ghost" minH="44px" onClick={() => (unsaved ? setConfirmDiscard(true) : onClose())}>Cerrar</Button>
            <Button minH="44px" loading={save.isPending} disabled={week === null && tracks === null} onClick={() => save.mutate()}>Guardar</Button>
          </HStack>
        </Flex>
        <HStack justify="space-between" mb={2}>
          <HStack gap={1}>
            <Text fontWeight="600">Lleva asistencia</Text>
            <IconButton aria-label="Quién lleva asistencia" variant="ghost" minW="44px" minH="44px"
              onClick={() => setHelp(true)}><LuCircleHelp /></IconButton>
          </HStack>
          <Switch size="lg" py="10px" checked={tracksValue} onCheckedChange={(e) => setTracks(e.checked)} aria-label="Lleva asistencia" />
        </HStack>
        {confirmDiscard && (
          <HStack mb={2} p={3} borderWidth="1px" borderRadius="lg" justify="space-between" wrap="wrap">
            <Text fontWeight="600">Hay cambios sin guardar.</Text>
            <HStack gap={2}>
              <Button variant="ghost" minH="44px" onClick={() => setConfirmDiscard(false)}>Seguir editando</Button>
              <Button variant="outline" colorPalette="red" minH="44px" onClick={onClose}>Salir sin guardar</Button>
            </HStack>
          </HStack>
        )}
        {error && <Text color="fg.error" fontWeight="600" mb={2}>{error}</Text>}
        <HStack gap={2} mb={2}>
          <Button variant="outline" minH="44px" onClick={() => setWeek(copyToWeekdays(displayedWeek))}>Copiar lunes a martes–viernes</Button>
          <Box w="240px">
            <Picker title="Copiar de otra persona" placeholder="Copiar de otra persona" value=""
              options={people.filter((p) => p.id !== userId).map((p) => ({ value: String(p.id), label: p.name }))}
              onChange={(v) => void copyFrom(v)} />
          </Box>
        </HStack>
        <VStack align="stretch" gap={0} borderWidth="1px" borderRadius="lg" px={3}>
          {WEEK.map(({ weekday, label }) => (
            <HStack key={weekday} minH="52px" gap={3} borderBottomWidth="1px" _last={{ borderBottomWidth: 0 }} wrap="wrap">
              <Text w="96px" fontWeight="700">{label}</Text>
              {displayedWeek[weekday].length === 0 && <Text color="fg.muted">Libre</Text>}
              {displayedWeek[weekday].map((s, i) => (
                <HStack key={i} gap={1}>
                  {i > 0 && <Text color="fg.muted">y</Text>}
                  <Box w="136px"><Picker title={`${label} · entrada`} value={String(s.start)} options={QUARTERS} onChange={(v) => changeShift(weekday, i, 'start', Number(v))} /></Box>
                  <Text color="fg.muted">a</Text>
                  <Box w="136px"><Picker title={`${label} · salida`} value={String(s.end)} options={QUARTERS} onChange={(v) => changeShift(weekday, i, 'end', Number(v))} /></Box>
                  {crossesMidnight(s) && <Badge colorPalette="purple">+1 día</Badge>}
                </HStack>
              ))}
              <HStack ml="auto" gap={1}>
                {displayedWeek[weekday].length < 2 && (
                  <Button variant="ghost" minH="44px" onClick={() => setWeek({ ...displayedWeek, [weekday]: [...displayedWeek[weekday],
                    displayedWeek[weekday].length ? { start: 22 * 60, end: 6 * 60 } : { start: 9 * 60, end: 17 * 60 }] })}>
                    {displayedWeek[weekday].length ? '+ Turno' : 'Agregar horario'}
                  </Button>
                )}
                {displayedWeek[weekday].length > 0 && (
                  <Button variant="ghost" minH="44px" onClick={() => setWeek({ ...displayedWeek, [weekday]: [] })}>Día libre</Button>
                )}
              </HStack>
            </HStack>
          ))}
        </VStack>
        <Text fontSize="sm" color="fg.muted" mt={2}>
          Se aplica desde la fecha elegida; los días anteriores conservan su horario.
        </Text>
      </Box>
      {/* Spec 046: quien lleva asistencia marca su entrada antes de abrir la caja. Una cuenta que
          comparten varias personas no es una persona y nunca podría hacerlo. */}
      <DialogRoot open={help} onOpenChange={(e) => setHelp(e.open)} placement="center" size="sm">
        <DialogBackdrop />
        <DialogContent>
          <DialogHeader><DialogTitle>Quién lleva asistencia</DialogTitle></DialogHeader>
          <DialogBody>
            <VStack as="ol" align="stretch" gap={2} pl={5} listStyleType="decimal">
              <li>Prende «Lleva asistencia» a cada persona que marca su entrada y su salida.</li>
              <li>La caja se abre con la entrada marcada, y se cierra cuando quien la abrió ya marcó su salida.</li>
              <li>Si varias personas abren la caja con la misma cuenta, apágale «Lleva asistencia» a esa cuenta: si no, no podrá abrirla.</li>
            </VStack>
          </DialogBody>
          <DialogFooter><Button minH="44px" onClick={() => setHelp(false)}>Entendido</Button></DialogFooter>
        </DialogContent>
      </DialogRoot>
    </Flex>
  );
}
