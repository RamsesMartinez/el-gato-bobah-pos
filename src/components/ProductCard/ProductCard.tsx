import React from 'react';
import { Box, Text, Stack, useColorModeValue } from '@chakra-ui/react';
import { FudoProduct } from '../../types/fudo';

interface ProductCardProps {
  product: FudoProduct;
  onClick: () => void;
}

export const ProductCard: React.FC<ProductCardProps> = ({ product, onClick }) => {
  const borderColor = useColorModeValue('gray.200', 'gray.700');
  const bgColor = useColorModeValue('white', 'gray.800');
  const textColor = useColorModeValue('gray.800', 'gray.100');
  const priceColor = useColorModeValue('green.600', 'green.400');

  return (
    <Box
      as="button"
      onClick={onClick}
      borderWidth="1px"
      borderColor={borderColor}
      borderRadius="lg"
      overflow="hidden"
      bg={bgColor}
      _hover={{
        transform: 'translateY(-2px)',
        shadow: 'md',
        borderColor: 'green.400',
      }}
      transition="all 0.2s"
      w="100%"
      textAlign="left"
      p={4}
    >
      <Stack spacing={2}>
        <Text
          fontSize="lg"
          fontWeight="600"
          color={textColor}
          noOfLines={2}
        >
          {product.attributes.name}
        </Text>
        
        <Text
          fontSize="xl"
          fontWeight="700"
          color={priceColor}
        >
          ${product.attributes.price.toFixed(2)}
        </Text>

        {product.attributes.preparationTime && (
          <Text
            fontSize="sm"
            color="gray.500"
          >
            Tiempo: {product.attributes.preparationTime} min
          </Text>
        )}
      </Stack>
    </Box>
  );
}; 