import React, { useState, useEffect } from 'react';
import { Box, SimpleGrid, Text, Image, Card, CardBody, Heading, Stack, Button, HStack } from '@chakra-ui/react';
import { useParams, useNavigate } from 'react-router-dom';
import { MainNav } from '../../components/layout/MainNav';
import { categoryService } from '../../services/api/categories';
import { FudoProduct, FudoCategory } from '../../types/fudo';

interface Product {
  id: string;
  name: string;
  description: string;
  price: number;
  imageUrl: string | null;
}

export const CategoryProducts: React.FC = () => {
  const { categoryId } = useParams();
  const navigate = useNavigate();
  const [products, setProducts] = useState<Product[]>([]);
  const [categoryName, setCategoryName] = useState('');
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const loadProducts = async () => {
      try {
        if (!categoryId) return;
        const response = await categoryService.getProductsByCategory(categoryId);

        // Extraer el nombre de la categoría
        const category = response.data.find((item): item is FudoCategory => 
          item.type === 'ProductCategory' && item.id === categoryId
        );
        
        if (category) {
          setCategoryName(category.attributes.name);
        }

        // Mapear los productos
        const mappedProducts = response.data
          .filter((item): item is FudoProduct => item.type === 'Product')
          .map(product => ({
            id: product.id,
            name: product.attributes.name,
            description: product.attributes.description || '',
            price: product.attributes.price,
            imageUrl: product.attributes.imageUrl
          }));

        setProducts(mappedProducts);
      } catch (error) {
        console.error('Error cargando productos:', error);
      } finally {
        setLoading(false);
      }
    };

    loadProducts();
  }, [categoryId]);

  return (
    <Box bg="white" minH="100vh">
      <MainNav />
      
      <Box p={4} maxW="1400px" mx="auto">
        {/* Breadcrumb y título */}
        <HStack mb={6} spacing={2}>
          <Button 
            variant="link" 
            color="blue.500" 
            onClick={() => navigate('/sales/new')}
          >
            Todos los productos
          </Button>
          <Text color="gray.500">›</Text>
          <Text color="gray.700" fontWeight="bold">{categoryName}</Text>
        </HStack>

        {loading ? (
          <Text>Cargando productos...</Text>
        ) : (
          <SimpleGrid columns={{ base: 2, md: 3, lg: 4 }} spacing={6}>
            {products.map((product) => (
              <Card 
                key={product.id}
                cursor="pointer"
                _hover={{ transform: 'scale(1.02)', transition: 'all 0.2s' }}
                onClick={() => {
                  console.log('Producto seleccionado:', product);
                  // Aquí implementaremos la lógica para agregar al ticket
                }}
              >
                <CardBody>
                  <Image
                    src={product.imageUrl || 'https://via.placeholder.com/200'}
                    alt={product.name}
                    borderRadius="lg"
                    mb={4}
                  />
                  <Stack>
                    <Heading size="md">{product.name}</Heading>
                    <Text color="gray.600" noOfLines={2}>
                      {product.description}
                    </Text>
                    <Text color="blue.600" fontSize="2xl">
                      ${product.price.toFixed(2)}
                    </Text>
                  </Stack>
                </CardBody>
              </Card>
            ))}
          </SimpleGrid>
        )}
      </Box>
    </Box>
  );
}; 