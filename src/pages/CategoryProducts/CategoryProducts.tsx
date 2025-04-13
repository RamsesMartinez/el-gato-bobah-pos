import React, { useState, useEffect } from 'react';
import { 
  Box, 
  SimpleGrid, 
  Text, 
  Image, 
  Card, 
  CardBody, 
  Heading, 
  Stack, 
  Button, 
  HStack, 
  Flex, 
  Input,
  Table,
  Thead,
  Tbody,
  Tr,
  Th,
  Td,
  IconButton
} from '@chakra-ui/react';
import { useParams, useNavigate } from 'react-router-dom';
import { MainNav } from '../../components/layout/MainNav';
import { categoryService } from '../../services/api/categories';
import { FudoProduct, FudoCategory } from '../../types/fudo';
import { ChevronRightIcon, SearchIcon } from '@chakra-ui/icons';

interface Product {
  id: string;
  name: string;
  description: string;
  price: number;
  imageUrl: string | null;
}

interface TicketItem {
  id: string;
  name: string;
  quantity: number;
  price: number;
  total: number;
}

export const CategoryProducts: React.FC = () => {
  const { categoryId } = useParams();
  const navigate = useNavigate();
  const [products, setProducts] = useState<Product[]>([]);
  const [categoryName, setCategoryName] = useState('');
  const [loading, setLoading] = useState(true);
  const [ticketItems, setTicketItems] = useState<TicketItem[]>([]);
  const [ticketTotal, setTicketTotal] = useState(0);

  useEffect(() => {
    const loadProducts = async () => {
      try {
        if (!categoryId) return;
        console.log('🔍 DEBUG - Iniciando carga de productos para categoría:', categoryId);
        
        const response = await categoryService.getProductsByCategory(categoryId);
        console.log('🔍 DEBUG - Respuesta del servicio:', response);

        // El primer elemento es la categoría
        const category = response.data[0] as FudoCategory;
        console.log('🔍 DEBUG - Categoría encontrada:', {
          id: category.id,
          name: category.attributes?.name,
          type: category.type
        });

        if (category && category.type === 'ProductCategory') {
          setCategoryName(category.attributes.name);
        }

        // Los elementos restantes son productos
        const mappedProducts = response.data
          .slice(1) // Excluir la categoría
          .filter((item): item is FudoProduct => item.type === 'Product')
          .map(product => ({
            id: product.id,
            name: product.attributes.name,
            description: product.attributes.description || '',
            price: product.attributes.price,
            imageUrl: product.attributes.imageUrl
          }));

        console.log('🔍 DEBUG - Productos mapeados:', mappedProducts);
        setProducts(mappedProducts);
      } catch (error) {
        console.error('❌ ERROR - Error cargando productos:', error);
      } finally {
        setLoading(false);
      }
    };

    loadProducts();
  }, [categoryId]);

  const handleAddToTicket = (product: Product) => {
    setTicketItems(prevItems => {
      const existingItem = prevItems.find(item => item.id === product.id);
      if (existingItem) {
        return prevItems.map(item => 
          item.id === product.id 
            ? { ...item, quantity: item.quantity + 1, total: (item.quantity + 1) * item.price }
            : item
        );
      }
      return [...prevItems, {
        id: product.id,
        name: product.name,
        quantity: 1,
        price: product.price,
        total: product.price
      }];
    });
  };

  useEffect(() => {
    const total = ticketItems.reduce((sum, item) => sum + item.total, 0);
    setTicketTotal(total);
  }, [ticketItems]);

  return (
    <Box bg="#F7FAFC" minH="100vh">
      <MainNav />
      
      <Flex direction="row" h="calc(100vh - 64px)">
        {/* Panel izquierdo - Ticket */}
        <Box w="400px" bg="white" borderRight="1px solid" borderColor="gray.200" overflow="auto">
          <Box p={4} borderBottom="1px solid" borderColor="gray.200">
            <Text fontSize="lg" fontWeight="bold">COMENSAL 1</Text>
            <Text color="gray.500" fontSize="sm">Pulse sobre este cliente para añadir productos a su pedido</Text>
          </Box>

          <Table variant="simple">
            <Thead>
              <Tr>
                <Th>Nombre</Th>
                <Th isNumeric>Cantidad</Th>
                <Th isNumeric>Precio</Th>
                <Th isNumeric>Total</Th>
              </Tr>
            </Thead>
            <Tbody>
              {ticketItems.map(item => (
                <Tr key={item.id}>
                  <Td>{item.name}</Td>
                  <Td isNumeric>{item.quantity}</Td>
                  <Td isNumeric>${item.price.toFixed(2)}</Td>
                  <Td isNumeric>${item.total.toFixed(2)}</Td>
                </Tr>
              ))}
            </Tbody>
          </Table>

          <Box p={4} borderTop="1px solid" borderColor="gray.200" mt="auto">
            <Flex justify="space-between" mb={4}>
              <Text>Precio total</Text>
              <Text fontWeight="bold">${ticketTotal.toFixed(2)}</Text>
            </Flex>
            <Button colorScheme="green" w="100%">
              Pagar
            </Button>
          </Box>
        </Box>

        {/* Panel derecho - Productos */}
        <Box flex={1} overflow="auto">
          {/* Barra superior */}
          <Flex 
            bg="white" 
            p={4} 
            borderBottom="1px solid" 
            borderColor="gray.200"
            justify="space-between"
            align="center"
            position="sticky"
            top={0}
            zIndex={1}
          >
            <HStack spacing={2}>
              <Button 
                variant="link" 
                color="blue.500"
                fontWeight="normal" 
                onClick={() => navigate('/sales/new')}
              >
                Todos los productos
              </Button>
              <ChevronRightIcon color="gray.500" />
              <Text color="gray.700" fontWeight="medium">{categoryName}</Text>
            </HStack>

            <HStack spacing={4}>
              <IconButton
                aria-label="Escanear"
                icon={<SearchIcon />}
                variant="outline"
              />
              <Button variant="outline">
                Promociones
              </Button>
              <Text fontWeight="bold">Ticket #1</Text>
            </HStack>
          </Flex>

          {/* Productos */}
          <Box p={4}>
            {loading ? (
              <Text>Cargando productos...</Text>
            ) : (
              <SimpleGrid columns={{ base: 2, md: 3, lg: 4 }} spacing={6}>
                {products.map((product) => (
                  <Card 
                    key={product.id}
                    cursor="pointer"
                    overflow="hidden"
                    bg="white"
                    boxShadow="sm"
                    _hover={{ 
                      transform: 'translateY(-2px)',
                      boxShadow: 'md',
                      transition: 'all 0.2s'
                    }}
                    onClick={() => handleAddToTicket(product)}
                  >
                    <Image
                      src={product.imageUrl || 'https://placehold.co/400x300'}
                      alt={product.name}
                      objectFit="cover"
                      height="200px"
                      width="100%"
                    />
                    <CardBody p={4}>
                      <Stack spacing={2}>
                        <Heading size="md" noOfLines={1}>{product.name}</Heading>
                        <Text color="gray.600" fontSize="sm" noOfLines={2}>
                          {product.description}
                        </Text>
                        <Text color="blue.600" fontSize="xl" fontWeight="bold">
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
      </Flex>
    </Box>
  );
}; 