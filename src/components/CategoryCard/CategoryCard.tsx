import React from 'react';
import { Box, Text } from '@chakra-ui/react';
import { FudoCategory } from '../../types/fudo';
import { imageService } from '../../services/images';

interface CategoryCardProps {
  category: FudoCategory;
  isSelected: boolean;
  onClick: () => void;
}

export const CategoryCard: React.FC<CategoryCardProps> = ({
  category,
  isSelected,
  onClick,
}) => {
  const imageUrl = imageService.getCategoryImage(
    category.attributes.name,
    category.id
  );

  return (
    <Box
      as="button"
      display="flex"
      flexDirection="column"
      alignItems="stretch"
      p={{ base: 2, md: 3 }}
      borderRadius="xl"
      backgroundColor="white"
      cursor="pointer"
      width="full"
      minW="140px"
      maxW="200px"
      aspectRatio="4/5"
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
        flex="1"
        overflow="hidden"
        borderRadius="lg"
        mb={{ base: 2, md: 3 }}
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
      <Box
        position="relative"
        minH={{ base: "2em", md: "2.5em" }}
        display="flex"
        alignItems="center"
        justifyContent="center"
        px={2}
      >
        <Text
          fontSize={{ base: "xs", sm: "sm", md: "md" }}
          fontWeight="600"
          color="gray.800"
          textAlign="center"
          noOfLines={2}
          lineHeight="shorter"
        >
          {category.attributes.name}
        </Text>
      </Box>
    </Box>
  );
}; 