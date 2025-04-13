import React, { useEffect, useState } from 'react';
import { 
  Box, 
  Flex, 
  Input, 
  IconButton, 
  useColorMode, 
  Spinner,
  Text,
  Table,
  Thead,
  Tbody,
  Tr,
  Th,
  Td,
  Button,
  HStack
} from '@chakra-ui/react';
import { SearchIcon } from '@chakra-ui/icons';
import { CategoryGrid } from '../../components/CategoryGrid/CategoryGrid';
import { FudoCategory } from '../../types/fudo';
import { categoryService } from '../../services/api/categories';
import { useNavigate } from 'react-router-dom';
import { Breadcrumb } from '../../components/Breadcrumb/Breadcrumb';
import { MainNav } from '../../components/layout/MainNav';

interface TicketItem {
  id: string;
  name: string;
  quantity: number;
  price: number;
  total: number;
}

const NewOrder: React.FC = () => {
  const { colorMode } = useColorMode();
  const bgColor = colorMode === 'light' ? 'gray.50' : 'gray.800';
  const [categories, setCategories] = useState<FudoCategory[]>([]);
  const [loading, setLoading] = useState(true);
  const [ticketItems, setTicketItems] = useState<TicketItem[]>([]);
  const [ticketTotal, setTicketTotal] = useState(0);
  const navigate = useNavigate();

  useEffect(() => {
    const fetchCategories = async () => {
      try {
        const response = await categoryService.getCategories();
        const activeCategories = response.data.filter(category => 
          category.relationships.products.data.length > 0
        );
        setCategories(activeCategories);
      } catch (error) {
        console.error('Error fetching categories:', error);
      } finally {
        setLoading(false);
      }
    };

    fetchCategories();
  }, []);

  useEffect(() => {
    const total = ticketItems.reduce((sum, item) => sum + item.total, 0);
    setTicketTotal(total);
  }, [ticketItems]);

  const handleCategoryClick = async (category: FudoCategory) => {
    navigate(`/sales/category/${category.id}`);
  };

  return (
    <Box bg={bgColor} minH="100vh">
      <MainNav />
      
      <Flex direction={{ base: 'column', md: 'row' }} h={{ base: 'auto', md: 'calc(100vh - 64px)' }}>
        {/* Panel izquierdo - Contenido principal */}
        <Box 
          flex={1} 
          overflow="auto"
          order={{ base: 2, md: 1 }}
        >
          {/* Barra superior con navegación y búsqueda */}
          <Box bg="white" borderBottom="1px" borderColor="gray.200">
            <Box maxW="1200px" mx="auto" px={4}>
              <Flex py={4} alignItems="center" justifyContent="space-between">
                <Breadcrumb
                  items={[
                    { label: 'Ventas', href: '/sales' },
                    { label: 'Nueva orden', href: '/sales/new' },
                    { label: 'Todos los productos' }
                  ]}
                />
                <HStack spacing={4}>
                  <IconButton
                    aria-label="Escanear"
                    icon={<SearchIcon />}
                    variant="outline"
                  />
                  <Button variant="outline">
                    Promociones
                  </Button>
                </HStack>
              </Flex>
            </Box>
          </Box>

          {/* Grid de categorías */}
          <Box maxW="1200px" mx="auto" px={4} py={6}>
            {loading ? (
              <Box display="flex" justifyContent="center" alignItems="center" minH="200px">
                <Spinner size="xl" />
              </Box>
            ) : (
              <CategoryGrid 
                categories={categories} 
                onCategoryClick={handleCategoryClick}
              />
            )}
          </Box>
        </Box>

        {/* Panel derecho - Ticket */}
        <Box 
          w={{ base: '100%', md: '40%', lg: '35%', xl: '700px' }}
          bg="white" 
          borderLeft={{ base: 'none', md: '1px solid' }}
          borderTop={{ base: '1px solid', md: 'none' }}
          borderColor="gray.200" 
          overflow="auto"
          order={{ base: 1, md: 2 }}
          position={{ base: 'sticky', md: 'relative' }}
          top={{ base: 0, md: 'auto' }}
          zIndex={{ base: 10, md: 1 }}
          minH={{ base: 'auto', md: '100%' }}
        >
          <Box p={6} borderBottom="1px solid" borderColor="gray.200">
            <Flex justify="space-between" align="center" mb={3}>
              <Text fontSize="2xl" fontWeight="bold">Ticket N°13</Text>
              <Text fontSize="lg" color="gray.600">Mesa 1</Text>
            </Flex>
            <Text color="gray.600" fontSize="md">Pulse sobre este cliente para añadir productos a su pedido</Text>
          </Box>

          <Table variant="simple" size="lg">
            <Thead>
              <Tr>
                <Th fontSize="md">Nombre</Th>
                <Th isNumeric fontSize="md">Cant.</Th>
                <Th isNumeric fontSize="md">Precio</Th>
                <Th isNumeric fontSize="md">Total</Th>
              </Tr>
            </Thead>
            <Tbody fontSize="md">
              {ticketItems.map(item => (
                <Tr key={item.id}>
                  <Td fontWeight="medium">{item.name}</Td>
                  <Td isNumeric>{item.quantity}</Td>
                  <Td isNumeric>${item.price.toFixed(2)}</Td>
                  <Td isNumeric fontWeight="semibold">${item.total.toFixed(2)}</Td>
                </Tr>
              ))}
            </Tbody>
          </Table>

          <Box 
            p={6} 
            borderTop="1px solid" 
            borderColor="gray.200" 
            position={{ base: 'sticky', md: 'relative' }}
            bottom={0}
            bg="white"
            mt="auto"
          >
            <Flex justify="space-between" mb={6} fontSize="lg">
              <Text fontWeight="medium">Precio total</Text>
              <Text fontSize="xl" fontWeight="bold">${ticketTotal.toFixed(2)}</Text>
            </Flex>
            <Button 
              colorScheme="green" 
              w="100%" 
              size="lg"
              fontSize="lg"
              py={7}
            >
              Pagar
            </Button>
          </Box>
        </Box>
      </Flex>
    </Box>
  );
};

export default NewOrder; 