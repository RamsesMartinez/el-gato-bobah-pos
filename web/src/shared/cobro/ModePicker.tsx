import { Button, HStack } from '@chakra-ui/react';

// SplitMode es cómo se divide lo que falta. 'none' es cobrarle todo a una persona, que es el caso de
// casi todos los pedidos y por eso no gasta un solo píxel en el selector.
export type SplitMode = 'none' | 'products' | 'people' | 'amount';

const LABELS: Record<Exclude<SplitMode, 'none'>, string> = {
  products: 'Por productos',
  people: 'Entre personas',
  amount: 'Por monto',
};

interface Props {
  mode: Exclude<SplitMode, 'none'>;
  // Los modos que este pedido admite. Por productos necesita el pedido ya creado; uno de plataforma
  // solo se reparte por monto.
  available: Array<Exclude<SplitMode, 'none'>>;
  onChange: (mode: Exclude<SplitMode, 'none'>) => void;
  onStop: () => void;
  disabled?: boolean;
}

// ModePicker es el selector de cómo se divide, y aparece solo tras tocar «Dividir»: quien cobra a
// una sola persona no le paga alto.
export function ModePicker({ mode, available, onChange, onStop, disabled }: Props) {
  return (
    <HStack gap={2} role="group" aria-label="Cómo se divide">
      {available.map((m) => (
        <Button key={m} flex="1" minH="44px" size="sm" aria-pressed={mode === m} disabled={disabled}
          variant={mode === m ? 'solid' : 'outline'} colorPalette={mode === m ? undefined : 'gray'}
          onClick={() => onChange(m)}>
          {LABELS[m]}
        </Button>
      ))}
      {/* Con palabras: la hoja ya tiene su ✕ de cerrar, y otra ✕ aquí se confundía con ella. */}
      <Button minH="44px" size="sm" variant="ghost" colorPalette="gray" flexShrink={0}
        disabled={disabled} onClick={onStop}>
        Dejar de dividir
      </Button>
    </HStack>
  );
}
