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
      columns={{ base: 2, sm: 3, md: 3, lg: 4, xl: 5 }}
      spacing={{ base: 3, sm: 4, md: 5 }}
      w="100%"
      p={{ base: 3, sm: 4, md: 5 }}
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