import React, { useState, useEffect } from 'react';
import { Box, Button, HStack, VStack, Text, Badge, Flex, Menu, MenuButton, MenuList, MenuItem } from '@chakra-ui/react';
import { useNavigate } from 'react-router-dom';
import { MainNav } from '../../components/layout/MainNav';

interface ActiveSale {
  id: string;
  ticketNumber: string;
  table: string;
  items: string[];
  total: number;
  status: 'NUEVO' | 'EN_PROCESO';
  createdAt: string;
  timeElapsed: string;
}

export const ActiveSales: React.FC = () => {
  const navigate = useNavigate();
  const [activeSales, setActiveSales] = useState<ActiveSale[]>([]);
  const [selectedDate, setSelectedDate] = useState(new Date());

  useEffect(() => {
    // Aquí cargaríamos las ventas activas del API
    const mockSales: ActiveSale[] = [
      {
        id: '14',
        ticketNumber: '#14',
        table: 'Mesa 1',
        items: ['Quesadilla potatoes with chorizo, Beer (Victory), Beer (Corona), Bottled water, Boing (Mango), Order of 2 sopes'],
        total: 230.00,
        status: 'NUEVO',
        createdAt: '2024-04-12T23:46:00',
        timeElapsed: '32 minutes ago'
      },
      // Más ventas mock...
    ];
    setActiveSales(mockSales);
  }, []);

  const handleNewSale = () => {
    navigate('/sales/new');
  };

  return (
    <Box bg="white" minH="100vh">
      <VStack spacing={0} align="stretch">
        <MainNav />

        {/* Header con botones de acción */}
        <Box borderBottom="1px" borderColor="gray.200">
          <Flex justify="space-between" align="center" p={4} maxW="1400px" mx="auto">
            <Button
              bg="#00C853"
              color="white"
              size="md"
              onClick={handleNewSale}
              _hover={{ bg: '#00B34A' }}
            >
              Nueva venta
            </Button>
            <Menu>
              <MenuButton as={Button} variant="outline" color="gray.600" borderColor="gray.300">
                Tipo de pedido
              </MenuButton>
              <MenuList bg="white" borderColor="gray.200">
                <MenuItem _hover={{ bg: 'gray.100' }}>Para llevar</MenuItem>
                <MenuItem _hover={{ bg: 'gray.100' }}>Mesa</MenuItem>
                <MenuItem _hover={{ bg: 'gray.100' }}>Delivery</MenuItem>
              </MenuList>
            </Menu>
          </Flex>
        </Box>

        {/* Headers de la tabla */}
        <Box borderBottom="1px" borderColor="gray.200">
          <Flex maxW="1400px" mx="auto" p={4} color="gray.600">
            <Text flex="1">Abierto</Text>
            <Text flex="1">Hora de entrega</Text>
            <Text flex="3">Orden</Text>
            <Text flex="1">Estatus</Text>
            <Text flex="1" textAlign="right">Importe</Text>
          </Flex>
        </Box>

        {/* Filtros de estado */}
        <Box p={4} borderBottom="1px" borderColor="gray.200">
          <HStack spacing={4} maxW="1400px" mx="auto">
            <Button 
              size="sm" 
              variant="ghost" 
              color="#2196F3"
              _hover={{ bg: 'transparent', color: '#1E88E5' }}
            >
              DÍAS ANTERIORES (8)
            </Button>
            <Button 
              size="sm" 
              variant="ghost" 
              color="#2196F3"
              fontWeight="bold"
              _hover={{ bg: 'transparent', color: '#1E88E5' }}
            >
              HOY (4)
            </Button>
          </HStack>
        </Box>

        {/* Lista de ventas activas */}
        <Box flex="1" overflowY="auto" bg="gray.50">
          <VStack spacing={2} p={4} align="stretch" maxW="1400px" mx="auto">
            {activeSales.map((sale) => (
              <Box
                key={sale.id}
                bg="white"
                p={4}
                cursor="pointer"
                borderRadius="md"
                boxShadow="sm"
                _hover={{ bg: 'gray.50' }}
                onClick={() => navigate(`/sales/${sale.id}`)}
              >
                <Flex>
                  <Box flex="1">
                    <Text color="#F44336" fontSize="lg">{sale.timeElapsed}</Text>
                    <Text color="gray.500" fontSize="sm">23:46</Text>
                  </Box>
                  <Box flex="1">
                    <Text color="gray.500">2 hours</Text>
                  </Box>
                  <Box flex="3">
                    <HStack>
                      <Text color="gray.900">{sale.ticketNumber}</Text>
                      <Text color="gray.500">• PARA LLEVAR</Text>
                    </HStack>
                    <Text color="gray.500" fontSize="sm" noOfLines={1}>
                      {sale.items.join(', ')}
                    </Text>
                  </Box>
                  <Box flex="1">
                    <Badge bg="#F44336" color="white">NUEVO</Badge>
                  </Box>
                  <Box flex="1" textAlign="right">
                    <Text color="gray.900" fontWeight="bold">${sale.total.toFixed(2)}</Text>
                    <Button 
                      bg="#2196F3" 
                      color="white" 
                      size="sm" 
                      mt={2}
                      _hover={{ bg: '#1E88E5' }}
                    >
                      Pagar
                    </Button>
                  </Box>
                </Flex>
              </Box>
            ))}
          </VStack>
        </Box>
      </VStack>
    </Box>
  );
}; 