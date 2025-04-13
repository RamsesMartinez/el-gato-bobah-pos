import React from 'react';
import { Box, Text } from '@chakra-ui/react';
import { FudoCategory } from '../../types/fudo';

interface CategoryCardProps {
  category: FudoCategory;
  isSelected: boolean;
  onClick: () => void;
}

const getCategoryIcon = (categoryName: string): string => {
  const name = categoryName.toLowerCase();
  if (name === 'todos los productos') return '/images/categories/todos.svg';
  if (name.includes('frappé')) return '/images/categories/frappes.svg';
  if (name.includes('soda')) return '/images/categories/sodas.svg';
  if (name.includes('té') || name.includes('tea')) return '/images/categories/tes.svg';
  if (name.includes('café')) return '/images/categories/cafes.svg';
  if (name.includes('crepa')) return '/images/categories/crepas.svg';
  if (name.includes('granizado')) return '/images/categories/granizados.svg';
  if (name.includes('licuado')) return '/images/categories/licuados.svg';
  if (name.includes('bobah')) return '/images/categories/bobah.svg';
  return '/images/categories/default.svg';
};

const getCategoryColor = (categoryName: string): string => {
  const name = categoryName.toLowerCase();
  if (name === 'todos los productos') return '#B5FFB9'; // Verde pastel
  if (name.includes('frappé')) return '#FFB5E8'; // Rosa pastel
  if (name.includes('soda')) return '#B5B9FF'; // Azul pastel
  if (name.includes('té') || name.includes('tea')) return '#BFFCC6'; // Menta pastel
  if (name.includes('café')) return '#C4FAF8'; // Turquesa pastel
  if (name.includes('crepa')) return '#FFC9DE'; // Rosa claro
  if (name.includes('granizado')) return '#DBCDF0'; // Lavanda pastel
  return '#FFE0B2'; // Naranja pastel (default)
};

export const CategoryCard: React.FC<CategoryCardProps> = ({
  category,
  isSelected,
  onClick,
}) => {
  const backgroundColor = getCategoryColor(category.attributes.name);
  const icon = getCategoryIcon(category.attributes.name);

  return (
    <Box
      as="button"
      display="flex"
      flexDirection="column"
      alignItems="center"
      justifyContent="center"
      p={4}
      borderRadius="lg"
      border={isSelected ? '2px solid' : 'none'}
      borderColor="primary.500"
      backgroundColor={backgroundColor}
      cursor="pointer"
      minH="100px"
      transition="all 0.2s ease-in-out"
      transform={isSelected ? 'scale(0.98)' : 'scale(1)'}
      boxShadow={isSelected ? 'inset 0 2px 4px rgba(0,0,0,0.1)' : '0 2px 4px rgba(0,0,0,0.05)'}
      onClick={onClick}
      _hover={{
        transform: 'scale(1.02)',
      }}
    >
      <Box
        as="img"
        src={icon}
        alt={category.attributes.name}
        width="48px"
        height="48px"
        mb={2}
      />
      <Text
        fontSize="sm"
        fontWeight="medium"
        textAlign="center"
        wordBreak="break-word"
      >
        {category.attributes.name}
      </Text>
    </Box>
  );
}; 