import type { ReactNode } from 'react';
import { Button, HStack } from '@chakra-ui/react';

export interface SegmentedOption<T extends string> {
  value: T;
  label: string;
  icon?: ReactNode;
}

// Segmented es la fila de botones pegados para elegir una opción entre pocas (2 a 4): almacén,
// estado de un renglón, a dónde se guarda. Lo activo va sólido con el color de la paleta del negocio
// (heredado de Apariencia, no fijo en `brand`), y lo demás sin fondo: con el rojo de marca en un
// `subtle` lo elegido se leía como un aviso de error junto a «Revisar» (revisión de la 094). Con
// nombres largos la fila se desplaza a lo ancho en vez de salirse de la pantalla.
export function Segmented<T extends string>({ label, value, options, onChange }: {
  label: string;
  value: T;
  options: SegmentedOption<T>[];
  onChange: (v: T) => void;
}) {
  return (
    <HStack gap={0} borderWidth="1px" borderRadius="md" role="group" aria-label={label}
      overflowX="auto" maxW="100%" flexShrink={0} alignSelf="start">
      {options.map((o) => {
        const on = o.value === value;
        return (
          <Button key={o.value} minH="44px" flexShrink={0} variant={on ? 'solid' : 'ghost'}
            colorPalette={on ? undefined : 'gray'} aria-pressed={on} onClick={() => onChange(o.value)}>
            {o.icon}{o.label}
          </Button>
        );
      })}
    </HStack>
  );
}
