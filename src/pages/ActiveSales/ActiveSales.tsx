import React, { useState, useEffect } from 'react';
import { Box, Button, HStack, VStack, Text, Badge, Flex, Menu, MenuButton, MenuList, MenuItem, Center, Spinner } from '@chakra-ui/react';
import { useNavigate } from 'react-router-dom';
import { MainNav } from '../../components/layout/MainNav';
import { useTheme } from '../../hooks/useTheme';
import { FudoSale } from '../../types/fudo';
import { saleService } from '../../services/api/sales';

export const ActiveSales: React.FC = () => {
  const navigate = useNavigate();
  const { theme } = useTheme();
  const [activeSales, setActiveSales] = useState<FudoSale[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const loadSales = async () => {
      try {
        const response = await saleService.getSales();
        console.log('Respuesta completa de ventas:', response); // Para debug

        // Filtramos solo las ventas activas (NEW o IN-COURSE)
        const activeOrders = response.data.filter(sale => {
          const rawState = (sale as any).raw_state;
          console.log('Estado original de la venta:', rawState); // Para debug
          return ['NEW', 'IN-COURSE'].includes(rawState);
        });
        
        console.log('Ventas activas filtradas:', activeOrders); // Para debug
        setActiveSales(activeOrders);
      } catch (error) {
        console.error('Error loading sales:', error);
      } finally {
        setLoading(false);
      }
    };

    loadSales();
  }, []);

  const handleNewSale = () => {
    navigate('/sales/new');
  };

  const handleSaleClick = (sale: FudoSale) => {
    console.log('Datos completos de la venta:', sale);
    navigate(`/sales/${sale.id}`);
  };

  return (
    <Box bg="white" minH="100vh">
      <VStack spacing={0} align="stretch">
        <MainNav />

        {/* Header con botones de acción */}
        <Box borderBottom="1px" borderColor="gray.200">
          <Flex justify="space-between" align="center" p={4}>
            <Button
              bg={theme.colors.success.main}
              color="white"
              size="md"
              onClick={() => {
                console.log('Creando nueva venta');
                handleNewSale();
              }}
              _hover={{ bg: theme.colors.success.dark }}
            >
              Nueva venta
            </Button>
            <Menu>
              <MenuButton as={Button} variant="outline" color="gray.600" borderColor="gray.300">
                Tipo de pedido
              </MenuButton>
              <MenuList bg="white" borderColor="gray.200">
                <MenuItem 
                  _hover={{ bg: 'gray.100' }}
                  onClick={() => console.log('Filtrar por: Para llevar')}
                >
                  Para llevar
                </MenuItem>
                <MenuItem 
                  _hover={{ bg: 'gray.100' }}
                  onClick={() => console.log('Filtrar por: Mesa')}
                >
                  Mesa
                </MenuItem>
                <MenuItem 
                  _hover={{ bg: 'gray.100' }}
                  onClick={() => console.log('Filtrar por: Delivery')}
                >
                  Delivery
                </MenuItem>
              </MenuList>
            </Menu>
          </Flex>
        </Box>

        {/* Headers de la tabla */}
        <Box borderBottom="1px" borderColor="gray.200">
          <Flex p={4} color="gray.600">
            <Text flex="1">Abierto</Text>
            <Text flex="1">Hora de entrega</Text>
            <Text flex="3">Orden</Text>
            <Text flex="1">Estatus</Text>
            <Text flex="1" textAlign="right">Importe</Text>
          </Flex>
        </Box>

        {/* Filtros de estado */}
        <Box p={4} borderBottom="1px" borderColor="gray.200">
          <HStack spacing={4}>
            <Button 
              size="sm" 
              variant="ghost" 
              color={theme.colors.primary.main}
              _hover={{ bg: 'transparent', color: theme.colors.primary.dark }}
            >
              DÍAS ANTERIORES (8)
            </Button>
            <Button 
              size="sm" 
              variant="ghost" 
              color={theme.colors.primary.main}
              fontWeight="bold"
              _hover={{ bg: 'transparent', color: theme.colors.primary.dark }}
            >
              HOY (4)
            </Button>
          </HStack>
        </Box>

        {/* Lista de ventas activas */}
        <Box flex="1" overflowY="auto" bg="gray.50">
          <VStack spacing={2} p={4} align="stretch">
            {loading ? (
              <Center py={8}>
                <Spinner size="xl" color="primary.500" />
              </Center>
            ) : activeSales.length === 0 ? (
              <Center py={8}>
                <Text>No hay ventas activas</Text>
              </Center>
            ) : (
              activeSales.map((sale) => (
                <Box
                  key={sale.id}
                  bg="white"
                  p={4}
                  cursor="pointer"
                  borderRadius="md"
                  boxShadow="sm"
                  _hover={{ bg: 'gray.50' }}
                  onClick={() => handleSaleClick(sale)}
                >
                  <Flex>
                    <Box flex="1">
                      <Text color={theme.colors.error.main} fontSize="lg">
                        {new Date(sale.attributes.openedAt).toLocaleTimeString()}
                      </Text>
                      <Text color="gray.500" fontSize="sm">
                        {new Date(sale.attributes.openedAt).toLocaleDateString()}
                      </Text>
                    </Box>
                    <Box flex="1">
                      <Text color="gray.500">2 hours</Text>
                    </Box>
                    <Box flex="3">
                      <HStack>
                        <Text color="gray.900">#{sale.attributes.number}</Text>
                        <Text color="gray.500">• {sale.attributes.type}</Text>
                      </HStack>
                      <Text color="gray.500" fontSize="sm" noOfLines={1}>
                        {sale.attributes.notes || 'Sin notas'}
                      </Text>
                    </Box>
                    <Box flex="1">
                      <Badge bg={theme.colors.error.main} color="white">
                        {sale.attributes.status}
                      </Badge>
                      <Text fontSize="xs" color="gray.500" mt={1}>
                        API: {(sale as any).raw_state || 'N/A'}
                      </Text>
                    </Box>
                    <Box flex="1" textAlign="right">
                      <Text color="gray.900" fontWeight="bold">
                        ${sale.attributes.totalAmount.toFixed(2)}
                      </Text>
                      <Button 
                        bg={theme.colors.primary.main} 
                        color="white" 
                        size="sm" 
                        mt={2}
                        onClick={(e) => {
                          e.stopPropagation();
                          console.log('Datos de pago:', {
                            id: sale.id,
                            total: sale.attributes.totalAmount,
                            status: sale.attributes.status,
                            raw_state: (sale as any).raw_state
                          });
                        }}
                        _hover={{ bg: theme.colors.primary.dark }}
                      >
                        Pagar
                      </Button>
                    </Box>
                  </Flex>
                </Box>
              ))
            )}
          </VStack>
        </Box>
      </VStack>
    </Box>
  );
}; 