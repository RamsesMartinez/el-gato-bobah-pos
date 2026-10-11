import { useEffect } from 'react';
import { Button, SimpleGrid, Text } from '@chakra-ui/react';
import { LuDelete } from 'react-icons/lu';

// Alto mínimo de cada tecla. Por debajo el dedo falla, y aquí fallar con prisa es teclear el PIN de
// otro.
const TAP = '56px';

// PinPad es el teclado del PIN: el del desbloqueo y el de checar asistencia.
//
// Es el ÚNICO dueño del oyente del teclado físico. Con dos teclados montados (checar abierto
// encima de la pantalla de bloqueo) y dos oyentes, cada dígito caía en los dos PIN y un Enter
// mandaba dos intentos que cuentan doble para el bloqueo por intentos. Por eso escucha solo si
// `active`, y quien lo monta encima de otro desmonta o apaga el de abajo.
export function PinPad({
  pin, onDigit, onBackspace, onSubmit, canSubmit, loading, actionLabel, actionColor = 'green', active = true,
}: {
  pin: string;
  onDigit: (d: string) => void;
  onBackspace: () => void;
  onSubmit: () => void;
  canSubmit: boolean;
  loading?: boolean;
  actionLabel: string;
  actionColor?: string;
  active?: boolean;
}) {
  useEffect(() => {
    if (!active) return;
    const onKeyDown = (e: KeyboardEvent) => {
      // Ctrl+1 cambia de pestaña y Alt+F4 cierra: una combinación no es teclear un PIN.
      if (e.ctrlKey || e.metaKey || e.altKey) return;
      if (/^[0-9]$/.test(e.key)) {
        e.preventDefault();
        onDigit(e.key);
        return;
      }
      if (e.key === 'Backspace') {
        e.preventDefault();
        onBackspace();
        return;
      }
      if (e.key === 'Enter') {
        // Con el foco en un botón, Enter es DE ese botón: es como se sale sin ratón.
        if (document.activeElement instanceof HTMLButtonElement) return;
        // La misma condición que apaga el botón: un PIN a medias acerca al bloqueo por una tecla.
        if (!canSubmit || loading) return;
        e.preventDefault();
        onSubmit();
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [active, canSubmit, loading, onDigit, onBackspace, onSubmit]);

  return (
    <>
      {/* Puntos y no el número: la pantalla se ve desde el mostrador. */}
      <Text fontSize="2xl" letterSpacing="0.4em" minH="2rem" textAlign="center">{'•'.repeat(pin.length)}</Text>
      <SimpleGrid columns={3} gap={2} w="100%">
        {['1', '2', '3', '4', '5', '6', '7', '8', '9'].map((d) => (
          <Button key={d} minH={TAP} fontSize="xl" onClick={() => onDigit(d)}>{d}</Button>
        ))}
        <Button minH={TAP} variant="ghost" aria-label="Borrar" onClick={onBackspace}>
          <LuDelete />
        </Button>
        <Button minH={TAP} fontSize="xl" onClick={() => onDigit('0')}>0</Button>
        <Button minH={TAP} colorPalette={actionColor} fontWeight="800"
          disabled={!canSubmit} loading={loading} onClick={onSubmit}>
          {actionLabel}
        </Button>
      </SimpleGrid>
    </>
  );
}
