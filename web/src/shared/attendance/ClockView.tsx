import { useCallback, useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Box, Button, Flex, Heading, SimpleGrid, Text, VStack } from '@chakra-ui/react';
import { attendanceApi, type ClockKind, type ClockResult, type RosterPerson } from '../../api/attendance';
import { ApiError } from '../../api/client';
import { isOffline } from '../../api/mensajes';
import { LoadError } from '../LoadError';
import { PinPad } from '../../components/PinPad';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';
import { soloHora } from '../../utils/horaDelNegocio';

const ROSTER = ['asistencia', 'roster'] as const;

function hoursLabel(min: number): string {
  const h = Math.floor(min / 60);
  return h > 0 ? `${h} h ${String(min % 60).padStart(2, '0')} min` : `${min} min`;
}

// ClockView es la pantalla de asistencia: quién falta por entrar y quién está en el local
// (maqueta C, «Pantallas y maquetas» del plan). Vive en /asistencia y también encima de la
// pantalla de bloqueo, sin desbloquear: checar nunca cambia la sesión de quien cobra.
//
// `role="dialog"` en la vista y en la hoja: el medidor de toques no cuenta lo que pasa dentro de un
// diálogo, y sin eso el teclado del PIN dejaría una zona caliente en la pantalla de abajo.
export function ClockView({ onCerrar }: { onCerrar?: () => void }) {
  const qc = useQueryClient();
  const { zona: zone } = useHoraDelNegocio();
  const { data, isError, error: rosterError, refetch, isFetching } = useQuery({
    queryKey: ROSTER, queryFn: attendanceApi.roster,
    // Sin red, TanStack pausa la consulta y deja la lista vieja sin error: quien llega vería «0 en el
    // local» como si fuera cierto. Con `always` la consulta falla y la pantalla lo dice.
    networkMode: 'always', retry: 1,
  });
  const [chosen, setChosen] = useState<RosterPerson | null>(null);
  const [pin, setPin] = useState('');
  const [error, setError] = useState('');
  const [done, setDone] = useState<(ClockResult & { name: string }) | null>(null);

  const people = data?.people ?? [];
  const inside = people.filter((p) => p.openSince);
  const notIn = people.filter((p) => !p.openSince);

  const choose = async (p: RosterPerson) => {
    setPin('');
    setError('');
    setChosen(p);
    // Se vuelve a pedir: otra tableta pudo checar a esta persona desde que se pintó la lista, y el
    // botón diría lo contrario de lo que el servidor va a registrar.
    const fresh = await qc.fetchQuery({ queryKey: ROSTER, queryFn: attendanceApi.roster }).catch(() => null);
    const existing = fresh?.people.find((x) => x.userId === p.userId);
    if (existing) setChosen(existing);
  };

  const expected: ClockKind = chosen?.openSince ? 'out' : 'in';
  const clockMutation = useMutation({
    // Sin esto, sin red la mutación se pausa y el botón se queda girando sin decir nada.
    networkMode: 'always',
    mutationFn: () => attendanceApi.clock(chosen!.userId, pin, expected),
    onSuccess: (r) => {
      setDone({ ...r, name: chosen!.name });
      setChosen(null);
      setPin('');
      void qc.invalidateQueries({ queryKey: ROSTER });
    },
    onError: (e) => {
      setPin('');
      if (e instanceof ApiError && e.status === 429) {
        const min = Math.max(1, Math.ceil((e.retryAfter ?? 60) / 60));
        setError(`Demasiados intentos. Intenta de nuevo en ${min} min.`);
      } else if (e instanceof ApiError && e.code === 'ATTENDANCE_STATE_CHANGED') {
        setError('Tu registro cambió en otra tableta. Vuelve a tocar tu nombre.');
        setChosen(null);
        void qc.invalidateQueries({ queryKey: ROSTER });
      } else if (isOffline(e)) {
        setError('Sin conexión. Checa cuando vuelva la red.');
      } else {
        // No dice si falló la persona o el PIN, igual que el bloqueo: distinguirlos enumera usuarios.
        setError('No coincide. Inténtalo de nuevo.');
      }
    },
  });

  // La confirmación se cierra sola: quien sigue en la fila no tiene que tocar nada.
  useEffect(() => {
    if (!done) return;
    const t = setTimeout(() => setDone(null), 3000);
    return () => clearTimeout(t);
  }, [done]);

  const typeDigit = useCallback((d: string) => { setError(''); setPin((p) => (p.length >= 12 ? p : p + d)); }, []);
  const backspace = useCallback(() => { setError(''); setPin((p) => p.slice(0, -1)); }, []);
  const submit = useCallback(() => clockMutation.mutate(), [clockMutation]);

  if (data && !data.enabled) {
    return <Text p={6} color="fg.muted">La asistencia no está activa en este negocio.</Text>;
  }

  const personButton = (p: RosterPerson) => (
    <Button key={p.userId} minH="64px" h="auto" py={2} variant="outline" disabled={!p.hasPin}
      flexDirection="column" alignItems="flex-start" justifyContent="center" whiteSpace="normal"
      bg={p.openSince ? 'green.50' : undefined} borderColor={p.openSince ? 'green.300' : undefined}
      onClick={() => void choose(p)}>
      <Text fontWeight="700" fontSize="md">{p.name}</Text>
      <Text fontSize="xs" color={p.openSince ? 'green.700' : 'fg.muted'}>
        {!p.hasPin ? 'Aún no tiene PIN' : p.openSince ? `Dentro desde ${soloHora(p.openSince, zone)}`
          : p.leftAt ? `Salió a las ${soloHora(p.leftAt, zone)}` : 'Sin entrada hoy'}
      </Text>
    </Button>
  );

  return (
    <Flex role="dialog" aria-label="Asistencia" direction="column" h="100%" minH={0} p={5} gap={3} position="relative">
      <Flex justify="space-between" align="center">
        <Heading size="lg">Asistencia</Heading>
        <Flex align="center" gap={3}>
          {!isError && <Text color="fg.muted">{inside.length} en el local</Text>}
          {onCerrar && <Button variant="ghost" minH="44px" onClick={onCerrar}>Cerrar</Button>}
        </Flex>
      </Flex>
      {error && !chosen && <Text color="red.500" fontWeight="600">{error}</Text>}
      {isError ? (
        <LoadError error={rosterError} onRetry={() => void refetch()} retrying={isFetching} />
      ) : (
      <SimpleGrid columns={2} gap={5} flex="1" minH={0}>
        <VStack align="stretch" gap={2} minH={0}>
          <Text fontSize="xs" fontWeight="700" textTransform="uppercase" letterSpacing="0.06em" color="fg.muted">Por entrar</Text>
          <SimpleGrid columns={2} gap={2} overflowY="auto" minH={0} flex="1" alignContent="start">
            {notIn.map(personButton)}
          </SimpleGrid>
        </VStack>
        <VStack align="stretch" gap={2} minH={0}>
          <Text fontSize="xs" fontWeight="700" textTransform="uppercase" letterSpacing="0.06em" color="fg.muted">Dentro</Text>
          <VStack align="stretch" gap={2} overflowY="auto" minH={0} flex="1">
            {inside.map(personButton)}
          </VStack>
        </VStack>
      </SimpleGrid>
      )}

      {chosen && (
        <Flex position="absolute" inset={0} bg="blackAlpha.400" align="flex-end" zIndex={1}>
          <Box role="dialog" aria-label={`PIN de ${chosen.name}`} bg="bg" w="100%" borderTopRadius="xl"
            px={6} pt={4} pb={5} maxH="94dvh" overflowY="auto">
            <Flex justify="space-between" align="center" mb={2}>
              <Heading size="md">{chosen.name}</Heading>
              <Button variant="ghost" minH="44px" onClick={() => { setChosen(null); setPin(''); setError(''); }}>No soy yo</Button>
            </Flex>
            <VStack w="340px" mx="auto" gap={2}>
              <PinPad pin={pin} onDigit={typeDigit} onBackspace={backspace} onSubmit={submit}
                canSubmit={pin.length >= 4} loading={clockMutation.isPending}
                actionLabel={expected === 'out' ? 'Salida' : 'Entrada'}
                actionColor={expected === 'out' ? 'red' : 'green'} />
              {error && <Text color="red.500" fontWeight="600">{error}</Text>}
            </VStack>
          </Box>
        </Flex>
      )}

      {done && (
        <Flex position="absolute" inset={0} bg="bg" opacity={0.97} align="center" justify="center" zIndex={2}
          direction="column" gap={2} role="status" onClick={() => setDone(null)}>
          <Text color="fg.muted">{done.name}</Text>
          <Text fontSize="5xl" fontWeight="800">
            {done.kind === 'in' ? 'Entrada' : 'Salida'} {soloHora(done.at, zone)}
          </Text>
          {done.workedMinutes !== undefined && <Text fontSize="lg" color="green.700">Trabajaste {hoursLabel(done.workedMinutes)}</Text>}
        </Flex>
      )}
    </Flex>
  );
}
