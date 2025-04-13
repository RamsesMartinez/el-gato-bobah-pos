import React from 'react';
import { Box, Text, VisuallyHidden, useColorModeValue } from '@chakra-ui/react';
import { FudoCategory } from '../../types/fudo';
import { imageService } from '../../services/images';

interface CategoryCardProps {
  category: FudoCategory;
  isSelected: boolean;
  onClick: () => void;
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
  isSelected,
  onClick,
}) => {
  const style = imageService.getCategoryStyle(category.id);
  const borderColor = useColorModeValue('gray.200', 'gray.700');
  const textColor = useColorModeValue('gray.800', 'gray.100');
  const shadowColor = useColorModeValue('rgba(0,0,0,0.1)', 'rgba(0,0,0,0.3)');

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
      minW="180px"
      height="auto"
      aspectRatio="1.2"
      position="relative"
      transition="all 0.3s cubic-bezier(0.4, 0, 0.2, 1)"
      onClick={onClick}
      role="button"
      aria-pressed={isSelected}
      transform={isSelected ? 'scale(0.98)' : 'scale(1)'}
      _focus={{
        outline: "none",
        boxShadow: `0 0 0 3px ${style.color}40`,
      }}
      _hover={{
        transform: isSelected ? 'scale(0.98)' : 'scale(1.03)',
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
          boxShadow: `0 10px 15px -3px ${shadowColor}, 0 4px 6px -2px ${shadowColor}`,
        }}
      />

      {/* Título centrado */}
      <Text
        position="relative"
        fontSize={{ base: "xl", md: "2xl" }}
        fontWeight="700"
        color={textColor}
        textAlign="center"
        lineHeight="shorter"
        letterSpacing="tight"
        px={6}
        py={4}
        maxW="95%"
        whiteSpace="pre-wrap"
        wordBreak="break-word"
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