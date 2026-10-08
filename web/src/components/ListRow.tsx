import { Button, HStack, Text } from '@chakra-ui/react';
import { LuCheck } from 'react-icons/lu';

// Por debajo de esto un dedo no acierta a la primera (constitución, restricciones del producto).
const MIN_TOUCH_PX = 44;

interface Props {
  label: string;
  hint?: string; // texto secundario junto al nombre (p.ej. grupo, precio)
  selected?: boolean;
  muted?: boolean;
  // Alto mínimo en px. 52 es el de las hojas del Picker; una lista que se recorre con prisa
  // (los pedidos de «Pasar a otro pedido») pide 56. Nunca baja de 44, se pida lo que se pida.
  minH?: number;
  onClick: () => void;
}

// ListRow es la fila tocable de las hojas inferiores: nombre, pista opcional y palomita si está
// elegida.
export function ListRow({ label, hint, selected, muted, minH = 52, onClick }: Props) {
  return (
    <Button variant={selected ? 'subtle' : 'ghost'} colorPalette="gray" size="lg"
      minH={`${Math.max(MIN_TOUCH_PX, minH)}px`}
      justifyContent="space-between" fontWeight="500" onClick={onClick}>
      <HStack gap={2} minW={0}>
        <Text truncate color={muted ? 'fg.muted' : 'fg'}>{label}</Text>
        {hint && <Text fontSize="xs" color="fg.subtle" flexShrink={0}>{hint}</Text>}
      </HStack>
      {selected && <LuCheck />}
    </Button>
  );
}
