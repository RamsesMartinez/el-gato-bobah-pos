import React from 'react';
import { SimpleGrid } from '@chakra-ui/react';
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
    <SimpleGrid
      columns={{ base: 2, sm: 3, md: 4, lg: 6 }}
      spacing={4}
      w="100%"
      p={4}
    >
      {categories.map((category) => (
        <CategoryCard
          key={category.id}
          category={category}
          isSelected={category.id === selectedCategory}
          onClick={() => onCategoryClick(category)}
        />
      ))}
    </SimpleGrid>
  );
}; 