import React from 'react';
import { Box, SimpleGrid } from '@chakra-ui/react';
import { FudoCategory } from '../../types/fudo';
import { CategoryCard } from '../CategoryCard/CategoryCard';

interface CategoryGridProps {
  categories: FudoCategory[];
  selectedCategory?: string;
  onCategoryClick: (category: FudoCategory) => void;
}

export const CategoryGrid: React.FC<CategoryGridProps> = ({
  categories,
  selectedCategory = '',
  onCategoryClick,
}) => {
  return (
    <Box
      w="100%"
      maxW="1200px"
      mx="auto"
      px={{ base: 3, sm: 4, md: 5 }}
      py={{ base: 3, sm: 4, md: 5 }}
    >
      <SimpleGrid
        columns={{ base: 2, sm: 3, md: 3, lg: 4, xl: 4 }}
        spacing={{ base: 3, sm: 4, md: 5 }}
        w="100%"
      >
        {categories.map((category, index) => (
          <CategoryCard
            key={category.id}
            category={category}
            isSelected={category.id === selectedCategory}
            onClick={() => onCategoryClick(category)}
            index={index}
          />
        ))}
      </SimpleGrid>
    </Box>
  );
}; 