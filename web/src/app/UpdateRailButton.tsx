import { Button, Text } from '@chakra-ui/react';
import { LuCircleArrowUp } from 'react-icons/lu';
import { useAppUpdate } from '../stores/appUpdate';

// UpdateRailButton es el aviso de versión nueva: ocupa el lugar del logo en el riel mientras haya
// una esperando (spec 064, maqueta A). Ahí no tapa ningún botón de la pantalla ni le quita alto,
// cosa que el toast de arriba al centro sí hacía. Mide lo mismo que el logo (44 px) para que la
// lista del riel no pierda alto: con el rol admin a 600 px apenas le sobran unos píxeles.
// La flecha hacia arriba y no la circular de refrescar: ésa se lee como «recargar esta pantalla».
//
// `size="login"` es el mismo aviso en el lugar del logo de 88 px del inicio de sesión, que no
// tiene riel: una versión que no deja entrar se arregla pasando a la siguiente.
export function UpdateRailButton({ size = 'rail' }: { size?: 'rail' | 'login' }) {
  const needRefresh = useAppUpdate((s) => s.needRefresh);
  const apply = useAppUpdate((s) => s.apply);
  if (!needRefresh) return null;
  const login = size === 'login';
  return (
    <Button
      aria-label="Actualizar a la versión nueva" onClick={apply}
      colorPalette="green" px={0} gap={login ? 1.5 : 0.5} flexDirection="column" flexShrink={0}
      {...(login
        ? { w: '88px', minH: '88px', h: '88px' }
        : { w: '64px', minH: '44px', h: '44px', alignSelf: 'center', mb: 2 })}
    >
      <LuCircleArrowUp size={login ? 32 : 18} />
      <Text fontSize={login ? '13px' : '10px'} lineHeight="1.1">Actualizar</Text>
    </Button>
  );
}
