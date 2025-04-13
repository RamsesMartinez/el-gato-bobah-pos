import React from 'react';
import { Box, Text, VisuallyHidden, useColorModeValue } from '@chakra-ui/react';
import { FudoCategory } from '../../types/fudo';
import { imageService } from '../../services/images';
import { categoryService } from '../../services/api/categories';

interface CategoryCardProps {
  category: FudoCategory;
  isSelected?: boolean;
  onClick: () => void;
  index: number;
  totalCategories: number;
}

const toTitleCase = (str: string): string => {
  return str
    .toLowerCase()
    .split(' ')
    .map(word => word.charAt(0).toUpperCase() + word.slice(1))
    .join(' ');
};

export const CategoryCard: React.FC<CategoryCardProps> = ({
  category,
  isSelected = false,
  onClick,
  index,
  totalCategories
}) => {
  const style = imageService.getCategoryStyle(index, totalCategories);
  const borderColor = useColorModeValue('gray.200', 'gray.700');
  const textColor = useColorModeValue('gray.800', 'gray.100');
  const shadowColor = useColorModeValue('rgba(0,0,0,0.1)', 'rgba(0,0,0,0.3)');

  const handleClick = async () => {
    try {
      // Obtener las subcategorías usando el nuevo método
      const response = await categoryService.getSubCategories(category.id);
      const subCategories = response.data;
      
      console.log('🌳 CATEGORÍAS HIJAS 🌳\n', JSON.stringify({
        categoríaPadre: {
          id: category.id,
          nombre: category.attributes.name
        },
        subcategorías: subCategories.map((sub: FudoCategory) => ({
          id: sub.id,
          nombre: sub.attributes.name,
          cantidadProductos: sub.relationships.products.data.length
        }))
      }, null, 2));

      onClick();
    } catch (error) {
      console.error('Error al obtener subcategorías:', error);
      onClick();
    }
  };

  return (
    <Box
      as="button"
      display="flex"
      flexDirection="column"
      alignItems="center"
      justifyContent="center"
      borderRadius="xl"
      cursor="pointer"
      width="full"
      position="relative"
      aspectRatio="1"
      transition="all 0.3s cubic-bezier(0.4, 0, 0.2, 1)"
      onClick={handleClick}
      role="button"
      aria-pressed={isSelected}
      transform={isSelected ? 'scale(0.98)' : 'scale(1)'}
      _focus={{
        outline: "none",
        boxShadow: `0 0 0 3px ${style.color}40`,
      }}
      _hover={{
        transform: isSelected ? 'scale(0.98)' : 'scale(1.02)',
        boxShadow: 'lg',
      }}
    >
      {/* Fondo con gradiente y efectos */}
      <Box
        position="absolute"
        inset={0}
        borderRadius="xl"
        background={style.background}
        border="1px solid"
        borderColor={isSelected ? style.color : borderColor}
        transition="all 0.3s cubic-bezier(0.4, 0, 0.2, 1)"
        opacity={isSelected ? 1 : 0.95}
        boxShadow={`0 4px 6px -1px ${shadowColor}, 0 2px 4px -1px ${shadowColor}`}
        _hover={{
          opacity: 1,
          borderColor: style.color,
        }}
      />

      {/* Título centrado */}
      <Text
        position="relative"
        fontSize={{ base: "md", sm: "lg", md: "xl", lg: "xl" }}
        fontWeight="600"
        color={textColor}
        textAlign="center"
        lineHeight="shorter"
        px={{ base: 2, sm: 3, md: 4 }}
        maxW="90%"
        noOfLines={2}
      >
        {toTitleCase(category.attributes.name)}
      </Text>

      {/* Texto para lectores de pantalla */}
      <VisuallyHidden>
        Categoría {category.attributes.name}
        {isSelected ? " - Seleccionada" : ""}
      </VisuallyHidden>
    </Box>
  );
}; 