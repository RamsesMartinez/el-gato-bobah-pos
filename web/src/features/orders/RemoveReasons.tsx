import { Button, Text, VStack } from '@chakra-ui/react';

// Los motivos para quitar productos de un pedido: UNA sola lista para quitar un producto y para
// quitar lo que falta. Dos listas parecidas reparten la misma causa en dos nombres y el reporte de
// cancelaciones las cuenta por separado. Sin texto libre por lo mismo.
const REMOVE_REASONS = ['Ya no lo quiere', 'Se capturó de más', 'Sin insumos', 'Se equivocó el pedido'];

// 44 px es el mínimo con el que un dedo acierta a la primera.
const TAP = '44px';

// RemoveReasons pinta los motivos como filas a la vista y sin ninguno elegido. Con uno ya puesto,
// quitar se vuelve un solo toque y el motivo guardado es el que nadie eligió. Filas y no `Picker`:
// son cuatro, y abrir una hoja encima de otra para elegir entre cuatro cuesta un toque de más.
export function RemoveReasons({ value, onChange }: { value: string | null; onChange: (v: string) => void }) {
  return (
    <VStack align="stretch" gap={1.5} role="radiogroup" aria-label="Por qué">
      <Text fontSize="sm" color="fg.muted">Por qué</Text>
      {REMOVE_REASONS.map((r) => {
        const elegido = r === value;
        return (
          <Button key={r} role="radio" aria-checked={elegido} minH={TAP} justifyContent="flex-start"
            variant={elegido ? 'solid' : 'outline'} colorPalette={elegido ? 'red' : 'gray'}
            onClick={() => onChange(r)}>
            {r}
          </Button>
        );
      })}
    </VStack>
  );
}
