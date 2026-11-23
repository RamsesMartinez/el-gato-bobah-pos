import { useState } from 'react';
import { Box } from '@chakra-ui/react';
import { FAMILY_SPRITE, familySymbol, smallOf } from '../media/foodFamilies';

interface Props {
  imageUrl: string | null | undefined;
  categoryImageUrl?: string | null;
  family?: string | null;
  size: number;
  alt: string;
  // fluid: ocupa el ancho de su contenedor (cuadrado), como las tarjetas del mosaico; size queda
  // como el ancho que se le pide al navegador para elegir la variante.
  fluid?: boolean;
}


// CatalogImage muestra la foto de un producto o una categoría y, si no hay, la de su categoría o el
// dibujo de su familia: nunca un hueco. El tamaño va declarado para que la lista no salte al cargar.
export function CatalogImage({ imageUrl, categoryImageUrl, family, size, alt, fluid }: Props) {
  // Lo que falló se recuerda por URL y no por posición: cada foto nueva es otra URL (lleva la huella
  // del archivo) y merece su intento aunque la anterior no haya cargado.
  const [bad, setBad] = useState<ReadonlySet<string>>(() => new Set());
  const url = [imageUrl, categoryImageUrl].find((u): u is string => !!u && !bad.has(u));
  const box = fluid
    ? { w: '100%', aspectRatio: 1, flexShrink: 0, overflow: 'hidden' } as const
    : { w: `${size}px`, h: `${size}px`, flexShrink: 0, borderRadius: 'md', overflow: 'hidden' } as const;
  if (url) {
    return (
      <Box {...box} bg="bg.muted">
        <img src={smallOf(url)} srcSet={`${smallOf(url)} 400w, ${url} 1200w`} sizes={`${size}px`}
          width={size} height={size} loading="lazy" decoding="async" alt={alt}
          onError={() => setBad((b) => new Set(b).add(url))}
          style={{ width: '100%', height: '100%', objectFit: 'cover', display: 'block' }} />
      </Box>
    );
  }
  return (
    <Box {...box} bg="brand.subtle" color="brand.fg" display="flex" alignItems="center" justifyContent="center"
      role="img" aria-label={alt}>
      <svg width="58%" height="58%" aria-hidden="true">
        <use href={`${FAMILY_SPRITE}#${familySymbol(family)}`} />
      </svg>
    </Box>
  );
}
