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
      height="100%"
      display="flex"
      flexDirection="column"
    >
      <SimpleGrid
        columns={{ base: 2, sm: 3, md: 4, lg: 5, xl: 6 }}
        spacing={{ base: 3, sm: 4 }}
        w="100%"
      >
        {categories.map((category, index) => (
          <CategoryCard
            key={category.id}
            category={category}
            isSelected={category.id === selectedCategory}
            onClick={() => onCategoryClick(category)}
            index={index}
            totalCategories={categories.length}
          />
        ))}
      </SimpleGrid>
    </Box>
  );
}; 