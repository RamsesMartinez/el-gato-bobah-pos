import { IconButton } from '@chakra-ui/react';
import { useTheme } from 'next-themes';
import { LuMoon, LuSun } from 'react-icons/lu';

// TemaRapido cambia entre claro y oscuro de un toque (#290, F). Va en la franja fija del riel y no
// en la lista: a 768 px de alto el menú desplaza y esconde Interfaz. Cambia la MISMA preferencia que
// Interfaz (next-themes la guarda en el aparato); «Como el sistema» se recupera desde ahí.
export function TemaRapido() {
  const { resolvedTheme, setTheme } = useTheme();
  const oscuro = resolvedTheme === 'dark';
  return (
    <IconButton
      aria-label={oscuro ? 'Cambiar a tema claro' : 'Cambiar a tema oscuro'}
      size="sm" minH="44px" w="100%" variant="ghost" colorPalette="whiteAlpha" color="gray.100"
      onClick={() => setTheme(oscuro ? 'light' : 'dark')}
    >
      {oscuro ? <LuSun /> : <LuMoon />}
    </IconButton>
  );
}
