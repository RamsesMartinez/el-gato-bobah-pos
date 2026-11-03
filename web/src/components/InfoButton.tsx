import { IconButton } from '@chakra-ui/react';
import { LuInfo } from 'react-icons/lu';

// InfoButton es el (i) de un renglón opcional (spec 058): 44 px para el dedo, y el texto que abre lo
// pinta quien lo usa, en su lugar y debajo del renglón, para que el botón no se mueva al abrirlo.
export function InfoButton({ label, open, onToggle }: { label: string; open: boolean; onToggle: () => void }) {
  return (
    <IconButton aria-label={label} aria-expanded={open} minH="44px" minW="44px" size="sm" variant="ghost" colorPalette="gray"
      onClick={(e) => { e.stopPropagation(); onToggle(); }}>
      <LuInfo />
    </IconButton>
  );
}
