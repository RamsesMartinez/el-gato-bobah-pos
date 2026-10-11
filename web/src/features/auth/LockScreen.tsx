import { useCallback, useEffect, useRef, useState } from 'react';
import { useQuery, useMutation } from '@tanstack/react-query';
import { Box, Button, Center, HStack, SimpleGrid, Text, VStack } from '@chakra-ui/react';
import { LuLock } from 'react-icons/lu';
import { PinPad } from '../../components/PinPad';
import { attendanceApi } from '../../api/attendance';
import { ClockView } from '../../shared/attendance/ClockView';
import { posApi } from '../../api/pos';
import { useSessionStore } from '../../stores/session';

// Alto mínimo de todo lo que se toca. Por debajo el dedo falla, y aquí fallar con prisa significa
// teclear el PIN de otro.
const TAP = '56px';

// La pantalla de bloqueo. Es TAMBIÉN la de cambiar de operador, y eso no es un ahorro de código:
// desbloquear ES identificarse, así que no queda una acción aparte de "cambiar usuario" que alguien
// pueda saltarse para cobrar a nombre de quien dejó la tableta abierta.
export function LockScreen({ onDesbloqueado }: { onDesbloqueado: () => void }) {
  const [persona, setPersona] = useState<number | null>(null);
  const [pin, setPin] = useState('');
  const [error, setError] = useState('');
  const setSession = useSessionStore((s) => s.setSession);
  const clear = useSessionStore((s) => s.clear);
  const [saliendo, setSaliendo] = useState(false);

  // Salir REVOCA la cookie de refresh en el servidor antes de limpiar el estado local.
  //
  // Con solo limpiar en memoria, la cookie sobrevive y una recarga entre el tap y el login la
  // canjea: la tableta vuelve sola a la sesión de quien se estaba yendo. En esta pantalla eso es
  // peor que en el logout normal, porque es la salida de quien no puede entrar de otra forma.
  const salir = async () => {
    setSaliendo(true);
    try {
      await posApi.logout();
    } catch {
      /* red caída: se cierra localmente de todos modos */
    }
    clear();
  };

  const { data } = useQuery({ queryKey: ['auth', 'unlock-options'], queryFn: posApi.unlockOptions });
  const { data: attendance } = useQuery({ queryKey: ['asistencia', 'roster'], queryFn: attendanceApi.roster });
  // Checar desde aquí no desbloquea: la vista va ENCIMA y este teclado se desmonta mientras tanto.
  // Con los dos montados, cada dígito del teclado físico caía en los dos PIN y un Enter mandaba
  // dos intentos que cuentan doble para el bloqueo por intentos.
  const [clocking, setClocking] = useState(false);
  const pinOnly = data?.pinOnly ?? false;
  const personas = data?.users ?? [];

  const desbloquear = useMutation({
    mutationFn: () => posApi.pinSwitch(pinOnly ? null : persona, pin),
    onSuccess: (s) => {
      setSession(s.accessToken, s.user);
      setPin('');
      setPersona(null);
      setError('');
      onDesbloqueado();
    },
    onError: () => {
      // El mensaje NO dice si falló la persona o el PIN: distinguirlos convertiría la pantalla en
      // un enumerador de usuarios. El servidor tampoco los distingue.
      setError('No coincide. Inténtalo de nuevo.');
      setPin('');
    },
  });

  // Con solo-PIN no hay a quién elegir; sin él, hasta que se elige persona no tiene sentido teclear.
  const pidiendoPin = pinOnly || persona !== null;
  const largoMinimo = pinOnly ? 6 : 4;

  const typeDigit = useCallback((d: string) => {
    setError('');
    setPin((p) => (p.length >= 12 ? p : p + d));
  }, []);
  const backspace = useCallback(() => {
    setError('');
    setPin((p) => p.slice(0, -1));
  }, []);

  // El foco se trae a esta pantalla al montarla.
  //
  // La hoja de bloqueo va ENCIMA de la app sin desmontarla, así que sin esto el foco se queda en lo
  // último que se tocó antes de que la tableta se durmiera. Con teclado eso es grave de dos maneras:
  // Enter activaría un botón que ya no se ve, y el PIN se iría escribiendo dentro del campo que
  // quedó atrás.
  const caja = useRef<HTMLDivElement>(null);
  useEffect(() => { caja.current?.focus(); }, []);

  if (clocking) {
    return (
      <Box position="fixed" inset={0} zIndex={2000} bg="bg.subtle">
        <ClockView onCerrar={() => setClocking(false)} />
      </Box>
    );
  }

  return (
    // `role="dialog"` y `aria-modal` porque eso ES: una capa que cubre toda la pantalla y no deja
    // usar nada de abajo. Además de ser lo correcto para quien navega con lector, es lo que hace
    // que el medidor de toques (spec 019) NO cuente el teclado del PIN: el bloqueo no cambia de
    // ruta, así que sus toques se atribuirían a la pantalla de abajo —y como el teclado siempre
    // cae en el mismo sitio, dejaría una zona caliente en el centro que dentro de seis meses
    // alguien leería como «un control muy usado».
    <Center ref={caja} tabIndex={-1} outline="none" role="dialog" aria-modal="true"
      aria-label="Pantalla bloqueada"
      position="fixed" inset={0} zIndex={2000} bg="bg.subtle" p={4}>
      <VStack gap={4} w="100%" maxW="480px">
        <HStack color="fg.muted">
          <LuLock />
          <Text fontWeight="700">Pantalla bloqueada</Text>
        </HStack>

        {!pidiendoPin ? (
          // Elegir persona y LUEGO el PIN es el modo por default: el nombre identifica y el PIN
          // solo prueba, así que un dedazo no puede atribuirle la venta a quien no fue.
          <>
            <Text color="fg.muted" fontSize="sm">¿Quién va a cobrar?</Text>
            <SimpleGrid columns={{ base: 2, sm: 3 }} gap={2} w="100%">
              {personas.map((u) => (
                <Button key={u.id} minH={TAP} onClick={() => { setPersona(u.id); setError(''); }}>
                  {u.name}
                </Button>
              ))}
            </SimpleGrid>
          </>
        ) : (
          <>
            <Text color="fg.muted" fontSize="sm">
              {pinOnly ? 'Teclea tu PIN' : personas.find((u) => u.id === persona)?.name}
            </Text>
            <PinPad pin={pin} onDigit={typeDigit} onBackspace={backspace}
              onSubmit={() => desbloquear.mutate()} canSubmit={pin.length >= largoMinimo}
              loading={desbloquear.isPending} actionLabel="Entrar" />
            {!pinOnly && (
              <Button variant="ghost" size="sm" minH="44px" onClick={() => { setPersona(null); setPin(''); setError(''); }}>
                No soy yo
              </Button>
            )}
          </>
        )}

        {error && <Text color="red.500" fontWeight="600">{error}</Text>}

        {/* La salida de quien olvidó su PIN. Va VISIBLE y no escondida detrás de un menú: si no se
            ve, la única persona con acceso a media noche queda encerrada fuera con el local
            abierto, y no puede esperar a que otra llegue. */}
        {attendance?.enabled && (
          <Button variant="outline" minH="44px"
            onClick={() => { setPersona(null); setPin(''); setError(''); setClocking(true); }}>
            Registrar asistencia
          </Button>
        )}

        <Box pt={2}>
          <Button variant="outline" minH="44px" loading={saliendo} onClick={salir}>
            Entrar con usuario y contraseña
          </Button>
        </Box>
      </VStack>
    </Center>
  );
}
