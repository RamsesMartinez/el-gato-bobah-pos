import React, { useEffect, useState } from 'react';
import { Box, Flex, Input, IconButton, useColorModeValue, Spinner } from '@chakra-ui/react';
import { SearchIcon } from '@chakra-ui/icons';
import { CategoryGrid } from '../../components/CategoryGrid/CategoryGrid';
import { FudoCategory } from '../../types/fudo';
import { categoryService } from '../../services/api/categories';
import { useNavigate } from 'react-router-dom';

const NewOrder: React.FC = () => {
  const bgColor = useColorModeValue('gray.50', 'gray.800');
  const [categories, setCategories] = useState<FudoCategory[]>([]);
  const [loading, setLoading] = useState(true);
  const navigate = useNavigate();

  useEffect(() => {
    const fetchCategories = async () => {
      try {
        const response = await categoryService.getCategories();
        setCategories(response.data);
      } catch (error) {
        console.error('Error fetching categories:', error);
      } finally {
        setLoading(false);
      }
    };

    fetchCategories();
  }, []);

  const handleCategoryClick = async (category: FudoCategory) => {
    try {
      console.log('Navegando a categoría:', category);
      navigate(`/sales/category/${category.id}`);
    } catch (error) {
      console.error('Error al seleccionar categoría:', error);
    }
  };

  return (
    <Box bg={bgColor} minH="100vh">
      <Flex 
        justify="space-between" 
        align="center" 
        p={4} 
        borderBottomWidth="1px"
        borderColor="gray.200"
        bg="white"
      >
        <Flex gap={4} flex={1} maxW="container.xl" mx="auto">
          <Input
            placeholder="Buscar producto..."
            size="md"
            maxW="300px"
            borderRadius="md"
          />
          <IconButton
            aria-label="Buscar"
            icon={<SearchIcon />}
            size="md"
          />
        </Flex>
      </Flex>

      {loading ? (
        <Box display="flex" justifyContent="center" alignItems="center" minH="200px">
          <Spinner size="xl" />
        </Box>
      ) : (
        <Box maxW="container.xl" mx="auto" p={4}>
          <CategoryGrid 
            categories={categories} 
            onCategoryClick={handleCategoryClick}
          />
        </Box>
      )}
    </Box>
  );
};

export default NewOrder; 