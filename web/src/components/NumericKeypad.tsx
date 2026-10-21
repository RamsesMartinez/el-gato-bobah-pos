import { Button, Grid } from '@chakra-ui/react';

// Teclado numérico en pantalla (spec 038, conteo). Contando con una mano y la otra en el anaquel,
// el teclado del sistema tapa media pantalla y cambia de lugar entre tabletas; éste no se mueve y
// sus teclas miden 56 px. Las casillas que lo usan van con `inputMode="none"` para que el del
// sistema no se abra encima.

const KEYS = ['1', '2', '3', '4', '5', '6', '7', '8', '9'];

export function NumericKeypad({ allowDecimal, onKey, onNext }: {
  allowDecimal: boolean;
  onKey: (key: string) => void;
  onNext?: () => void;
}) {
  const key = (k: string, label = k) => (
    <Button key={k} minH="56px" fontSize="xl" variant="outline" aria-label={label}
      // Sin quitar el foco de la casilla: la tecla escribe donde ya se estaba escribiendo.
      onMouseDown={(e) => e.preventDefault()} onClick={() => onKey(k)}>
      {k === 'back' ? '⌫' : k}
    </Button>
  );
  return (
    <Grid templateColumns="repeat(3, 1fr)" gap={2} data-testid="teclado">
      {KEYS.map((k) => key(k))}
      {allowDecimal ? key('.', 'punto') : <span />}
      {key('0')}
      {key('back', 'Borrar')}
      {onNext && (
        <Button gridColumn="span 3" minH="56px" variant="outline" onMouseDown={(e) => e.preventDefault()} onClick={onNext}>
          Siguiente casilla ›
        </Button>
      )}
    </Grid>
  );
}
