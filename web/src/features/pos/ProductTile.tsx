import { Box, Text, Badge } from '@chakra-ui/react';
import type { MenuProduct } from '../../types/pos';
import { money, categoryColor } from '../../utils/format';
import { RADIUS, BORDER_W, ACCENT_W } from '../../theme/ui';

interface Props {
  product: MenuProduct;
  count: number; // cuántos hay en el ticket
  onTap: (p: MenuProduct) => void;
  showPrice: boolean;
}

export function ProductTile({ product, count, onTap, showPrice }: Props) {
  const hue = categoryColor(product.categoryId);
  return (
    <Box
      as="button"
      onClick={() => onTap(product)}
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
        <Text fontWeight="700" fontSize="lg" mt={2}>
          {money(product.price)}
        </Text>
      )}
    </Box>
  );
}
