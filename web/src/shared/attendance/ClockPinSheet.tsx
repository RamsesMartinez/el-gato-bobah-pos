import { useCallback, useEffect, useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import { Button, Flex, Text, VStack } from '@chakra-ui/react';
import { DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader } from '../../components/ui/drawer';
import { PinPad } from '../../components/PinPad';
import { attendanceApi, type ClockKind, type ClockResult } from '../../api/attendance';
import { useTabletaBloqueada } from '../../hooks/tabletaBloqueada';
import { clockError } from './clockErrors';

interface Props {
  open: boolean;
  person: { id: number; name: string };
  kind: ClockKind;
  onClose: () => void;
  onDone: (r: ClockResult) => void;
  // Otra tableta ya marcó a esta persona: quien abrió la hoja vuelve a preguntar el estado.
  onStateChanged: () => void;
}

// ClockPinSheet marca la entrada o la salida de UNA persona desde la caja (spec 046, plan ›
// «Pantallas y maquetas», punto 3). Es el mismo registro que la pantalla de Asistencia —mismo
// endpoint, mismo bloqueo por PIN, mismos textos— y no sabe nada de la caja: cerrarla después de
// marcar no deshace la marca ni toca la caja.
export function ClockPinSheet({ open, person, kind, onClose, onDone, onStateChanged }: Props) {
  const bloqueada = useTabletaBloqueada();
  const [pin, setPin] = useState('');
  const [error, setError] = useState('');

  // El PIN no sobrevive a la hoja ni al bloqueo: quien desbloquea no tiene por qué ver puntos de
  // alguien más esperando un Enter.
  useEffect(() => {
    if (!open || bloqueada) {
      setPin(''); // eslint-disable-line react-hooks/set-state-in-effect -- se limpia al cerrarse o bloquearse
      setError('');
    }
  }, [open, bloqueada]);

  const mutation = useMutation({
    networkMode: 'always',
    mutationFn: () => attendanceApi.clock(person.id, pin, kind),
    onSuccess: (r) => { setPin(''); onDone(r); },
    onError: (e) => {
      setPin('');
      const { kind: why, message } = clockError(e);
      if (why === 'state_changed') {
        onStateChanged();
        return;
      }
      setError(message);
    },
  });

  const typeDigit = useCallback((d: string) => { setError(''); setPin((p) => (p.length >= 12 ? p : p + d)); }, []);
  const backspace = useCallback(() => { setError(''); setPin((p) => p.slice(0, -1)); }, []);
  const submit = useCallback(() => mutation.mutate(), [mutation]);

  const action = kind === 'out' ? 'Salida' : 'Entrada';
  return (
    <DrawerRoot open={open} placement="bottom" onOpenChange={(e) => { if (!e.open) onClose(); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="l3" maxH="94dvh" display="flex" flexDirection="column">
        <DrawerHeader pb={1}>
          <Flex w="100%" justify="space-between" align="start" gap={3}>
            <VStack align="start" gap={0} flex="1" minW={0}>
              <Text fontSize="lg" fontWeight="700">
                {kind === 'out' ? 'Marcar salida' : 'Marcar entrada'} · {person.name}
              </Text>
              <Text fontSize="sm" color="fg.muted">Teclea el PIN de {person.name}.</Text>
            </VStack>
            <Button variant="ghost" minH="44px" onClick={onClose}>Cerrar</Button>
          </Flex>
        </DrawerHeader>
        <DrawerBody flex="1" minH={0} overflowY="auto" pb={5}>
          <VStack w="340px" maxW="100%" mx="auto" gap={2}>
            {/* Arriba del teclado y con su alto apartado: debajo quedaba fuera de los 600 px y quien
                se equivocaba solo veía que los puntos se borraban. */}
            <Text color="fg.error" fontWeight="600" textAlign="center" minH="1.5em" role="alert">{error}</Text>
            <PinPad pin={pin} onDigit={typeDigit} onBackspace={backspace} onSubmit={submit}
              canSubmit={pin.length >= 4} loading={mutation.isPending} active={open && !bloqueada}
              actionLabel={action} actionColor={kind === 'out' ? 'red' : 'green'} />
          </VStack>
        </DrawerBody>
      </DrawerContent>
    </DrawerRoot>
  );
}
