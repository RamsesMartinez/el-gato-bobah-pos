import React from 'react';
import { Box, Input, Button, Flex, SimpleGrid } from '@chakra-ui/react';
import { SearchIcon } from '@chakra-ui/icons';
import { FudoCategory } from '../../types/fudo';
import { Product } from '../../types/sales';
import { CategoryGrid } from '../CategoryGrid/CategoryGrid';

interface ProductsPanelProps {
  categories: FudoCategory[];
  selectedCategory: string;
  onCategoryChange: (category: FudoCategory) => void;
  products: Product[];
  onProductSelect: (product: Product) => void;
}

export const ProductsPanel: React.FC<ProductsPanelProps> = ({
  categories,
  selectedCategory,
  onCategoryChange,
  products,
  onProductSelect,
}) => {
  return (
    <Box flex="1" display="flex" flexDirection="column" height="100%" overflow="hidden">
      {/* Barra de búsqueda y herramientas */}
      <Flex p={2} gap={2} borderBottom="1px" borderColor="gray.200" bg="white">
        <Input
          placeholder="Buscar producto..."
          size="md"
          flex={1}
        />
        <Button
          leftIcon={<SearchIcon />}
          variant="outline"
          size="md"
        >
          Escanear
        </Button>
        <Button
          variant="outline"
          size="md"
        >
          Promociones
        </Button>
      </Flex>

      {/* Grid de categorías */}
      <Box p={2} borderBottom="1px" borderColor="gray.200" bg="white">
        <CategoryGrid
          categories={categories}
          selectedCategory={selectedCategory}
          onCategoryClick={onCategoryChange}
        />
      </Box>

      {/* Grid de productos */}
      <Box flex="1" overflow="auto" p={2} bg="gray.50">
        <SimpleGrid
          columns={{ base: 3, md: 4, lg: 6, xl: 8 }}
          spacing={2}
        >
          {products.map((product) => (
            <Box
              key={product.id}
              onClick={() => onProductSelect(product)}
              cursor="pointer"
              borderRadius="md"
              overflow="hidden"
              bg="white"
              boxShadow="sm"
              transition="all 0.2s"
              _hover={{
                transform: 'translateY(-2px)',
                boxShadow: 'md'
              }}
            >
              <Box position="relative" paddingTop="66.67%">
                <Box
                  as="img"
                  src={product.image_url || 'https://placehold.co/300x200?text=Producto'}
                  alt={product.name}
                  position="absolute"
                  top={0}
                  left={0}
                  w="100%"
                  h="100%"
                  objectFit="cover"
                />
              </Box>
              <Box p={2}>
                <Box fontWeight="semibold" fontSize="sm" noOfLines={2}>
                  {product.name}
                </Box>
                {product.description && (
                  <Box fontSize="xs" color="gray.600" noOfLines={1}>
                    {product.description}
                  </Box>
                )}
                <Box color="gray.800" fontSize="sm" fontWeight="bold" mt={1}>
                  ${product.price.toFixed(2)}
                </Box>
              </Box>
            </Box>
          ))}
        </SimpleGrid>
      </Box>
    </Box>
  );
}; 