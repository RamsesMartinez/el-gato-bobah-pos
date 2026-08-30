import { Box, Text, Badge } from '@chakra-ui/react';
import { LuPencil } from 'react-icons/lu';
import type { MenuProduct } from '../../types/pos';
import { money, categoryColor } from '../../utils/format';
import { RADIUS, BORDER_W, ACCENT_W } from '../../theme/ui';
import { useLongPress } from '../../hooks/useLongPress';

interface Props {
  product: MenuProduct;
  count: number; // cuántos hay en el ticket
  onTap: (p: MenuProduct) => void;
  showPrice: boolean;
  // Precio de la lista activa. En mostrador es el base; con una plataforma elegida, el de esa
  // lista. Va calculado desde arriba porque el mosaico no conoce el menú completo.
  price: number;
  // Precio capturado a mano en la lista activa: se marca para que se distinga del calculado.
  esManual: boolean;
  // Mantener presionado para corregir el precio. undefined en mostrador o sin permiso.
  onEditPrice?: (p: MenuProduct) => void;
}

export function ProductTile({ product, count, onTap, showPrice, price, esManual, onEditPrice }: Props) {
  const hue = categoryColor(product.categoryId);
  const press = useLongPress(
    onEditPrice ? () => onEditPrice(product) : undefined,
    () => onTap(product),
  );
  return (
    <Box
      as="button"
      {...press}
      position="relative"
      textAlign="left"
      bg="bg.panel"
      borderWidth={BORDER_W}
      borderColor="border"
      borderLeftWidth={ACCENT_W}
      borderLeftColor={hue}
      borderRadius={RADIUS}
      p={3}
      minH="104px"
      display="flex"
      flexDir="column"
      justifyContent="space-between"
      transition="transform 0.05s"
      css={{ touchAction: 'manipulation', userSelect: 'none', '&:active': { transform: 'scale(0.97)' } }}
      _hover={{ borderColor: 'border.emphasized' }}
    >
      {count > 0 && (
        <Badge position="absolute" top={1} right={1} borderRadius="full" px={2}>
          {count}×
        </Badge>
      )}
      <Box>
        <Text fontWeight="600" fontSize="md" lineClamp={3} lineHeight="1.25">
          {product.name}
        </Text>
        {product.description && (
          <Text fontSize="xs" color="fg.muted" lineClamp={1} mt={0.5}>
            {product.description}
          </Text>
        )}
      </Box>
      {showPrice && (
        <Text fontWeight="700" fontSize="lg" mt={2} display="flex" alignItems="center" gap={1}>
          {money(price)}
          {esManual && <Box as="span" color="fg.muted" fontSize="sm"><LuPencil /></Box>}
        </Text>
      )}
    </Box>
  );
}
