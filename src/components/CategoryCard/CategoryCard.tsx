import React from 'react';
import { Box, Text } from '@chakra-ui/react';
import { FudoCategory } from '../../types/fudo';

interface CategoryCardProps {
  category: FudoCategory;
  isSelected: boolean;
  onClick: () => void;
}

const getDefaultImage = (categoryName: string): string => {
  const name = categoryName.toLowerCase();
  if (name.includes('quesadilla')) return 'https://images.unsplash.com/photo-1628191139360-4083564d03fd?w=300';
  if (name.includes('bebida') || name.includes('refresco')) return 'https://images.unsplash.com/photo-1570526427001-9d80d114054d?w=300';
  if (name.includes('postre') || name.includes('dessert')) return 'https://images.unsplash.com/photo-1488477181946-6428a0291777?w=300';
  if (name.includes('sope')) return 'https://images.unsplash.com/photo-1617424771170-d333ef3d2d9b?w=300';
  if (name.includes('kit') || name.includes('disposable')) return 'https://images.unsplash.com/photo-1610419241908-de94c55c14f9?w=300';
  return 'https://images.unsplash.com/photo-1555939594-58d7cb561ad1?w=300';
};

export const CategoryCard: React.FC<CategoryCardProps> = ({
  category,
  isSelected,
  onClick,
}) => {
  const imageUrl = getDefaultImage(category.attributes.name);

  return (
    <Box
      as="button"
      display="flex"
      flexDirection="column"
      alignItems="stretch"
      p={2}
      borderRadius="xl"
      backgroundColor="white"
      cursor="pointer"
      width="full"
      height="180px"
      position="relative"
      overflow="hidden"
      transition="all 0.2s"
      onClick={onClick}
      border={isSelected ? '2px solid' : '1px solid'}
      borderColor={isSelected ? 'blue.500' : 'gray.200'}
      _hover={{
        transform: 'translateY(-2px)',
        shadow: 'md',
      }}
    >
      <Box
        position="relative"
        height="130px"
        overflow="hidden"
        borderRadius="lg"
      >
        <Box
          as="img"
          src={imageUrl}
          alt={category.attributes.name}
          width="100%"
          height="100%"
          objectFit="cover"
          transition="transform 0.3s ease"
          _hover={{
            transform: 'scale(1.05)',
          }}
        />
      </Box>
      <Text
        mt={2}
        fontSize="sm"
        fontWeight="500"
        color="gray.800"
        textAlign="center"
        noOfLines={1}
      >
        {category.attributes.name}
      </Text>
    </Box>
  );
}; 