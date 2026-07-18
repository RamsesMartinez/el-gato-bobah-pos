import { Box, type BoxProps } from '@chakra-ui/react';

// Contenedor estándar de páginas admin/backoffice: padding + ancho máximo CENTRADO
// (mx="auto"). Las pantallas operativas (POS, Pedidos) NO lo usan: van al 100% del ancho.
// Cada página puede pasar su propio maxW (formularios angostos vs. tablas anchas).
//
// fill: la página ocupa toda la altura como columna flex. Úsalo cuando un hijo deba
// hacer scroll interno (flex="1" minH={0} overflowY="auto") para que header/footer
// queden fijos y siempre visibles (p. ej. tabla con paginador al pie).
export function Page({ fill, ...props }: BoxProps & { fill?: boolean }) {
  const fillProps = fill ? { h: '100%', display: 'flex', flexDirection: 'column' as const, minH: 0 } : {};
  return <Box p={6} mx="auto" w="100%" maxW="1150px" {...fillProps} {...props} />;
}
