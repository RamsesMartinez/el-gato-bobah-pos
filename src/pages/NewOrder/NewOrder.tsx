import React, { useEffect, useState, useCallback } from 'react';
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
import { SearchIcon } from '@chakra-ui/icons';
import { CategoryGrid } from '../../components/CategoryGrid/CategoryGrid';
import { FudoCategory, FudoProduct } from '../../types/fudo';
import { categoryService } from '../../services/api/categories';
import { useNavigate, useParams, useLocation } from 'react-router-dom';
import { Breadcrumb } from '../../components/Breadcrumb/Breadcrumb';
import { BreadcrumbItem } from '../../types/breadcrumb';
import { MainNav } from '../../components/layout/MainNav';
import { ProductCard } from '../../components/ProductCard/ProductCard';
import { ROUTES, generateCategoryRoute } from '../../constants/routes';

interface TicketItem {
  id: string;
  name: string;
  quantity: number;
  price: number;
  total: number;
}

export const NewOrder: React.FC = () => {
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
  const { categoryId } = useParams();
  const location = useLocation();

  const loadInitialCategories = useCallback(async () => {
    try {
      setLoading(true);
      const response = await categoryService.getCategories();
      setCategories(response.data);
      setProducts([]);
      setCurrentCategory(null);
      setNavigationStack([]);
    } catch (error) {
      console.error('Error fetching categories:', error);
    } finally {
      setLoading(false);
    }
  }, []);

  const loadCategoryData = useCallback(async (id: string) => {
    try {
      setLoading(true);
      
      // Obtener la información de la categoría actual
      const categoryResponse = await categoryService.getCategoryById(id);
      const currentCat = categoryResponse.data[0];
      setCurrentCategory(currentCat);

      // Primero verificamos si tiene subcategorías
      const subCategoriesResponse = await categoryService.getSubCategories(id);
      
      if (subCategoriesResponse.data && subCategoriesResponse.data.length > 0) {
        // Si tiene subcategorías, las mostramos
        setCategories(subCategoriesResponse.data);
        setProducts([]);
      } else {
        // Si no tiene subcategorías, buscamos productos
        const productsResponse = await categoryService.getProductsByCategory(id);
        setProducts(productsResponse.data);
        setCategories([]);
      }

      // Actualizar el stack de navegación
      if (currentCat) {
        const newStack = [];
        let parent = currentCat;
        
        while (parent && parent.relationships.parentCategory.data) {
          newStack.unshift(parent);
          const parentResponse = await categoryService.getCategoryById(parent.relationships.parentCategory.data.id);
          parent = parentResponse.data[0];
        }
        if (parent) {
          newStack.unshift(parent);
        }
        setNavigationStack(newStack);
      }
    } catch (error) {
      console.error('Error al cargar categoría:', error);
      navigate(ROUTES.SALES.NEW, { replace: true });
    } finally {
      setLoading(false);
    }
  }, [navigate]);

  const getBreadcrumbItems = () => {
    const items: BreadcrumbItem[] = [
      { label: 'Ventas', href: ROUTES.SALES.ROOT },
      { label: 'Nueva venta', href: ROUTES.SALES.NEW }
    ];

    navigationStack.forEach(category => {
      items.push({
        label: category.attributes.name,
        href: generateCategoryRoute(category.id)
      });
    });

    return items;
  };

  // Efecto para cargar datos basados en la URL
  useEffect(() => {
    let isSubscribed = true;

    const loadData = async () => {
      if (location.pathname === ROUTES.SALES.NEW) {
        if (isSubscribed) {
          await loadInitialCategories();
        }
      } else if (categoryId && isSubscribed) {
        await loadCategoryData(categoryId);
      }
    };

    loadData();

    return () => {
      isSubscribed = false;
    };
  }, [location.pathname, categoryId, loadInitialCategories, loadCategoryData]);

  useEffect(() => {
    const total = ticketItems.reduce((sum, item) => sum + item.total, 0);
    setTicketTotal(total);
  }, [ticketItems]);

  const handleCategoryClick = async (category: FudoCategory) => {
    navigate(generateCategoryRoute(category.id));
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

  return (
    <Box bg={bgColor} minH="100vh">
      <MainNav />
      
      <Flex direction={{ base: 'column', md: 'row' }} h={{ base: 'auto', md: 'calc(100vh - 64px)' }}>
        {/* Panel izquierdo - Contenido principal */}
        <Box 
          flex={1} 
          overflow="hidden"
          order={{ base: 2, md: 1 }}
          display="flex"
          flexDirection="column"
        >
          {/* Barra superior */}
          <Box 
            bg="white" 
            borderBottom="1px solid" 
            borderColor="gray.200"
            position="sticky"
            top={0}
            zIndex={10}
          >
            <Flex py={4} px={4} alignItems="center" justifyContent="space-between">
              <Breadcrumb items={getBreadcrumbItems()} />
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
          </Box>

          {/* Contenido principal */}
          <Box flex={1} overflow="auto" p={4}>
            {loading ? (
              <Flex justify="center" align="center" minH="200px">
                <Spinner size="xl" />
              </Flex>
            ) : categories.length > 0 ? (
              <CategoryGrid 
                categories={categories} 
                onCategoryClick={handleCategoryClick}
                selectedCategory={currentCategory?.id}
              />
            ) : products.length > 0 ? (
              <SimpleGrid 
                columns={{ base: 2, sm: 3, md: 4, lg: 5, xl: 6 }} 
                spacing={{ base: 3, sm: 4 }}
              >
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
          w={{ base: '100%', md: '500px' }}
          bg="white"
          borderLeft={{ base: 'none', md: '1px solid' }}
          borderTop={{ base: '1px solid', md: 'none' }}
          borderColor="gray.200"
          order={{ base: 1, md: 2 }}
          h={{ base: 'auto', md: '100%' }}
          overflow="auto"
        >
          <Box p={4} borderBottom="1px solid" borderColor="gray.200">
            <Text fontSize="lg" fontWeight="bold">COMENSAL 1</Text>
            <Text color="gray.500" fontSize="sm">Pulse sobre este cliente para añadir productos a su pedido</Text>
          </Box>

          <Table variant="simple">
            <Thead>
              <Tr>
                <Th>Nombre</Th>
                <Th isNumeric>Cant.</Th>
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
              <Text>Total</Text>
              <Text fontWeight="bold">${ticketTotal.toFixed(2)}</Text>
            </Flex>
            <Button colorScheme="green" w="100%">
              Pagar
            </Button>
          </Box>
        </Box>
      </Flex>
    </Box>
  );
}; 