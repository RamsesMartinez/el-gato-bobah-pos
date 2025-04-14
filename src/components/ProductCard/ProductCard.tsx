import React from 'react';
import { Box, Text, Image, Stack, useColorModeValue } from '@chakra-ui/react';
import { FudoProduct } from '../../types/fudo';

interface ProductCardProps {
  product: FudoProduct;
  onClick: () => void;
}

const toTitleCase = (str: string): string => {
  return str
    .toLowerCase()
    .split(' ')
    .map(word => word.charAt(0).toUpperCase() + word.slice(1))
    .join(' ');
};

const getProductImageUrl = (product: FudoProduct): string => {
  const baseUrl = 'https://dev.fu.do';
  if (product.attributes.imageUrl) {
    // Si la URL ya es completa, la usamos directamente
    if (product.attributes.imageUrl.startsWith('http')) {
      return product.attributes.imageUrl;
    }
    // Si es una ruta relativa, la concatenamos con la URL base
    return `${baseUrl}${product.attributes.imageUrl}`;
  }
  return 'https://placehold.co/300x400?text=Producto';
};

export const ProductCard: React.FC<ProductCardProps> = ({ product, onClick }) => {
  const borderColor = useColorModeValue('gray.200', 'gray.700');
  const priceColor = useColorModeValue('green.600', 'green.400');
  const shadowColor = useColorModeValue('rgba(0,0,0,0.1)', 'rgba(0,0,0,0.3)');

  return (
    <Box
      as="button"
      onClick={onClick}
      position="relative"
      width="full"
      aspectRatio="3/4"
      borderRadius="xl"
      overflow="hidden"
      transition="all 0.3s cubic-bezier(0.4, 0, 0.2, 1)"
      role="button"
      _focus={{
        outline: "none",
        boxShadow: `0 0 0 3px ${priceColor}40`,
      }}
      _hover={{
        transform: 'scale(1.02)',
        boxShadow: 'lg',
      }}
    >
      {/* Fondo con imagen y overlay */}
      <Box
        position="absolute"
        inset={0}
        borderRadius="xl"
        border="1px solid"
        borderColor={borderColor}
        overflow="hidden"
        transition="all 0.3s cubic-bezier(0.4, 0, 0.2, 1)"
        boxShadow={`0 4px 6px -1px ${shadowColor}, 0 2px 4px -1px ${shadowColor}`}
      >
        <Image
          src={getProductImageUrl(product)}
          alt={product.attributes.name}
          objectFit="cover"
          width="100%"
          height="100%"
          fallback={
            <Box
              width="100%"
              height="100%"
              bg="gray.100"
              display="flex"
              alignItems="center"
              justifyContent="center"
            >
              <Text color="gray.500" fontSize="sm">
                {product.attributes.name}
              </Text>
            </Box>
          }
        />
        {/* Overlay gradiente */}
        <Box
          position="absolute"
          inset={0}
          bg="linear-gradient(to top, rgba(0,0,0,0.8) 0%, rgba(0,0,0,0.3) 50%, rgba(0,0,0,0.1) 100%)"
        />
      </Box>

      {/* Contenido */}
      <Stack
        position="absolute"
        bottom={0}
        left={0}
        right={0}
        p={4}
        spacing={1}
        align="flex-start"
      >
        <Text
          fontSize={{ base: "md", sm: "lg" }}
          fontWeight="600"
          color="white"
          textAlign="left"
          lineHeight="shorter"
          noOfLines={2}
          textShadow="0 2px 4px rgba(0,0,0,0.3)"
        >
          {toTitleCase(product.attributes.name)}
        </Text>
        
        <Text
          fontSize={{ base: "lg", sm: "xl" }}
          fontWeight="700"
          color="white"
          textShadow="0 2px 4px rgba(0,0,0,0.3)"
        >
          ${product.attributes.price.toFixed(2)}
        </Text>

        {product.attributes.description && (
          <Text
            fontSize="sm"
            color="gray.100"
            noOfLines={2}
            textShadow="0 1px 2px rgba(0,0,0,0.5)"
          >
            {product.attributes.description}
          </Text>
        )}
      </Stack>
    </Box>
  );
}; 