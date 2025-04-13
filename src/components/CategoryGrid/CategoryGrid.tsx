import React, { useEffect, useState } from 'react';
import { Grid, Box, Image, Text, Skeleton } from '@chakra-ui/react';
import { categoryImagesService } from '../../services/categoryImages';
import { CategoryImage } from '../../types/api';
import { FudoCategory } from '../../types/fudo';

interface CategoryGridProps {
  categories: FudoCategory[];
  onCategoryClick: (category: FudoCategory) => void;
}

const CategoryGrid: React.FC<CategoryGridProps> = ({ categories, onCategoryClick }) => {
  const [categoryImages, setCategoryImages] = useState<Record<string, CategoryImage>>({});
  const [loadingImages, setLoadingImages] = useState<Record<string, boolean>>({});

  useEffect(() => {
    const fetchCategoryImages = async () => {
      const newLoadingState: Record<string, boolean> = {};
      categories.forEach(category => {
        newLoadingState[category.id] = true;
      });
      setLoadingImages(newLoadingState);

      const imagePromises = categories.map(async (category) => {
        try {
          const response = await categoryImagesService.getCategoryImage(category.id);
          return response.data;
        } catch (error) {
          console.error(`Error fetching image for category ${category.id}:`, error);
          return null;
        }
      });

      const images = await Promise.all(imagePromises);
      const newImagesState: Record<string, CategoryImage> = {};
      const finalLoadingState: Record<string, boolean> = {};

      images.forEach((image, index) => {
        if (image) {
          newImagesState[categories[index].id] = image;
        }
        finalLoadingState[categories[index].id] = false;
      });

      setCategoryImages(newImagesState);
      setLoadingImages(finalLoadingState);
    };

    fetchCategoryImages();
  }, [categories]);

  return (
    <Grid
      templateColumns="repeat(auto-fill, minmax(280px, 1fr))"
      gap={6}
      p={4}
    >
      {categories.map((category) => (
        <Box
          key={category.id}
          onClick={() => onCategoryClick(category)}
          cursor="pointer"
          borderRadius="lg"
          overflow="hidden"
          bg="white"
          boxShadow="sm"
          transition="all 0.2s"
          _hover={{
            transform: 'translateY(-4px)',
            boxShadow: 'md'
          }}
        >
          <Box position="relative" paddingTop="66.67%"> {/* Aspect ratio 3:2 */}
            <Skeleton
              isLoaded={!loadingImages[category.id]}
              position="absolute"
              top={0}
              left={0}
              w="100%"
              h="100%"
            >
              <Image
                src={categoryImages[category.id]?.imageUrl || 'https://via.placeholder.com/300x200/EDEDED/999999?text=Categoria'}
                alt={category.attributes.name}
                position="absolute"
                top={0}
                left={0}
                w="100%"
                h="100%"
                objectFit="cover"
              />
            </Skeleton>
            <Box
              position="absolute"
              bottom={0}
              left={0}
              right={0}
              p={4}
              bg="rgba(255, 255, 255, 0.9)"
              borderTop="1px"
              borderColor="gray.100"
            >
              <Text
                fontSize="xl"
                fontWeight="semibold"
                color="gray.800"
              >
                {category.attributes.name}
              </Text>
            </Box>
          </Box>
        </Box>
      ))}
    </Grid>
  );
};

export default CategoryGrid; 