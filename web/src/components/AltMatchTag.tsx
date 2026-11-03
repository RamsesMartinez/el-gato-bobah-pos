import { Badge } from '@chakra-ui/react';

// AltMatchTag señala que un resultado salió por uno de sus otros nombres (spec 063, maqueta
// «Coincidió con», opción B). Una sola para los seis buscadores: contorno gris y «también:» delante,
// para que no se lea como un estado (naranja «por revisar», morado de grupos) ni como el nombre.
// Truncada: nunca le quita al nombre más ancho del que le dejan.
export function AltMatchTag({ name, maxW = '100%' }: { name: string; maxW?: string }) {
  return (
    <Badge variant="outline" colorPalette="gray" fontWeight="500" maxW={maxW} minW={0} flexShrink={1}
      overflow="hidden" textOverflow="ellipsis" whiteSpace="nowrap" display="inline-block" title={name}>
      también: {name}
    </Badge>
  );
}
