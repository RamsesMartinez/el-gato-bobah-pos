import React, { useEffect, useState } from 'react';
import { 
  Box, 
  Flex, 
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
  HStack,
  SimpleGrid
} from '@chakra-ui/react';
import { SearchIcon, ChevronLeftIcon } from '@chakra-ui/icons';
import { CategoryGrid } from '../../components/CategoryGrid/CategoryGrid';
import { FudoCategory, FudoProduct } from '../../types/fudo';
import { categoryService } from '../../services/api/categories';
import { useNavigate } from 'react-router-dom';
import { Breadcrumb } from '../../components/Breadcrumb/Breadcrumb';
import { MainNav } from '../../components/layout/MainNav';
import { ProductCard } from '../../components/ProductCard/ProductCard';

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
  const [products, setProducts] = useState<FudoProduct[]>([]);
  const [loading, setLoading] = useState(true);
  const [ticketItems, setTicketItems] = useState<TicketItem[]>([]);
  const [ticketTotal, setTicketTotal] = useState(0);
  const [currentCategory, setCurrentCategory] = useState<FudoCategory | null>(null);
  const [navigationStack, setNavigationStack] = useState<FudoCategory[]>([]);
  const navigate = useNavigate();

  const loadInitialCategories = async () => {
    try {
      setLoading(true);
      const response = await categoryService.getCategories();
      setCategories(response.data);
      setProducts([]);
      setCurrentCategory(null);
      setNavigationStack([]);
      navigate('/sales/new', { replace: true });
    } catch (error) {
      console.error('Error fetching categories:', error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadInitialCategories();
  // Este efecto solo debe ejecutarse una vez al montar el componente
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    const total = ticketItems.reduce((sum, item) => sum + item.total, 0);
    setTicketTotal(total);
  }, [ticketItems]);

  const handleCategoryClick = async (category: FudoCategory) => {
    try {
      setLoading(true);
      
      // Primero verificamos si tiene subcategorías
      const subCategoriesResponse = await categoryService.getSubCategories(category.id);
      
      if (subCategoriesResponse.data && subCategoriesResponse.data.length > 0) {
        // Si tiene subcategorías, las mostramos
        setCategories(subCategoriesResponse.data);
        setProducts([]);
        setNavigationStack([...navigationStack, category]);
        setCurrentCategory(category);
        navigate(`/sales/category/${category.id}`);
      } else {
        // Si no tiene subcategorías, buscamos productos
        const productsResponse = await categoryService.getProductsByCategory(category.id);
        
        if (productsResponse.data && productsResponse.data.length > 0) {
          // Si tiene productos, los mostramos
          setProducts(productsResponse.data);
          setCategories([]);
          setNavigationStack([...navigationStack, category]);
          setCurrentCategory(category);
          navigate(`/sales/category/${category.id}`);
        } else {
          // Si no tiene ni subcategorías ni productos, mostramos mensaje
          setCategories([]);
          setProducts([]);
          setCurrentCategory(category);
          console.log('Categoría sin productos ni subcategorías:', category.attributes.name);
        }
      }
    } catch (error) {
      console.error('Error al cargar categoría:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleBackClick = () => {
    if (navigationStack.length === 0) {
      loadInitialCategories();
      return;
    }

    const newStack = [...navigationStack];
    newStack.pop(); // Remover la última categoría
    
    if (newStack.length === 0) {
      loadInitialCategories();
    } else {
      const previousCategory = newStack[newStack.length - 1];
      handleCategoryClick(previousCategory);
    }
    setNavigationStack(newStack);
  };

  const handleAddProduct = (product: FudoProduct) => {
    const existingItem = ticketItems.find(item => item.id === product.id);
    
    if (existingItem) {
      setTicketItems(ticketItems.map(item =>
        item.id === product.id
          ? { ...item, quantity: item.quantity + 1, total: (item.quantity + 1) * item.price }
          : item
      ));
    } else {
      const newItem: TicketItem = {
        id: product.id,
        name: product.attributes.name,
        quantity: 1,
        price: product.attributes.price,
        total: product.attributes.price
      };
      setTicketItems([...ticketItems, newItem]);
    }
  };

  const getBreadcrumbItems = () => {
    const items = [
      { label: 'Ventas', href: '/sales' },
      { label: 'Nueva orden', href: '/sales/new' }
    ];

    navigationStack.forEach(category => {
      items.push({ label: category.attributes.name, href: `/sales/category/${category.id}` });
    });

    return items;
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
                <HStack spacing={4}>
                  {navigationStack.length > 0 && (
                    <IconButton
                      aria-label="Volver"
                      icon={<ChevronLeftIcon />}
                      onClick={handleBackClick}
                      variant="ghost"
                    />
                  )}
                  <Breadcrumb items={getBreadcrumbItems()} />
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
                </HStack>
              </Flex>
            </Box>
          </Box>

          {/* Contenido principal */}
          <Box maxW="1200px" mx="auto" px={4} py={6}>
            {loading ? (
              <Box display="flex" justifyContent="center" alignItems="center" minH="200px">
                <Spinner size="xl" />
              </Box>
            ) : categories.length > 0 ? (
              <CategoryGrid 
                categories={categories} 
                onCategoryClick={handleCategoryClick}
                selectedCategory={currentCategory?.id}
              />
            ) : products.length > 0 ? (
              <SimpleGrid columns={{ base: 2, sm: 3, md: 4 }} spacing={4}>
                {products.map(product => (
                  <ProductCard
                    key={product.id}
                    product={product}
                    onClick={() => handleAddProduct(product)}
                  />
                ))}
              </SimpleGrid>
            ) : (
              <Box textAlign="center" py={10}>
                <Text fontSize="lg" color="gray.600">
                  No hay elementos disponibles en esta categoría
                </Text>
              </Box>
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
            <Text color="gray.600" fontSize="md">
              {ticketItems.length === 0 
                ? 'Seleccione productos para añadir al pedido'
                : `${ticketItems.length} productos en el pedido`}
            </Text>
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
              isDisabled={ticketItems.length === 0}
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