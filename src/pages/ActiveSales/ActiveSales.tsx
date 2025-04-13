import React, { useState, useEffect } from 'react';
import { Box, Button, Text, Flex, Center, Spinner, Heading, Table, Thead, Tbody, Tr, Th, Td, Tabs, TabList, Tab, TabPanels, TabPanel } from '@chakra-ui/react';
import { useNavigate, useLocation } from 'react-router-dom';
import { MainNav } from '../../components/layout/MainNav';
import { useTheme } from '../../hooks/useTheme';
import { FudoSale } from '../../types/fudo';
import { saleService } from '../../services/api/sales';
import { ROUTES } from '../../constants/routes';

interface SalesGroups {
  pending: FudoSale[];
  inProgress: FudoSale[];
  toDeliver: FudoSale[];
}

type SaleType = 'counter' | 'delivery';

export const ActiveSales: React.FC = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const { theme } = useTheme();
  const [counterSales, setCounterSales] = useState<SalesGroups>({
    pending: [],
    inProgress: [],
    toDeliver: []
  });
  const [deliverySales, setDeliverySales] = useState<SalesGroups>({
    pending: [],
    inProgress: [],
    toDeliver: []
  });
  const [loading, setLoading] = useState(true);

  // Determinar la pestaña seleccionada basada en la URL
  const getSelectedTabFromPath = (path: string): SaleType => {
    if (path === ROUTES.SALES.ACTIVE.DELIVERY) return 'delivery';
    return 'counter'; // valor por defecto
  };

  const [selectedTab, setSelectedTab] = useState<SaleType>(getSelectedTabFromPath(location.pathname));

  // Efecto para sincronizar la pestaña con la URL
  useEffect(() => {
    const currentTab = getSelectedTabFromPath(location.pathname);
    setSelectedTab(currentTab);
  }, [location.pathname]);

  useEffect(() => {
    loadSalesByType(selectedTab);
  }, [selectedTab]);

  const handleTabChange = (index: number) => {
    const newTab = index === 0 ? 'counter' : 'delivery';
    const newPath = newTab === 'counter' ? ROUTES.SALES.ACTIVE.COUNTER : ROUTES.SALES.ACTIVE.DELIVERY;
    navigate(newPath);
  };

  const loadSalesByType = async (type: SaleType) => {
    try {
      setLoading(true);
      console.log("Tipo de venta:", type);
      const response = type === 'counter' 
        ? await saleService.getCounterSales()
        : await saleService.getDeliverySales();

      const groupedSales = {
        pending: response.data.filter(sale => sale.attributes.saleState === 'PENDING'),
        inProgress: response.data.filter(sale => sale.attributes.saleState === 'IN-COURSE'),
        toDeliver: response.data.filter(sale => 
          sale.attributes.saleState === 'READY_TO_DELIVER' || 
          sale.attributes.saleState === 'DELIVERY-SENT'
        )
      };

      if (type === 'counter') {
        setCounterSales(groupedSales);
      } else {
        setDeliverySales(groupedSales);
      }
    } catch (error) {
      console.error('Error loading sales:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleNewSale = () => {
    navigate('/sales/new');
  };

  const handleSaleClick = (sale: FudoSale) => {
    navigate(`/sales/${sale.id}`);
  };

  const SalesTable: React.FC<{ title: string; sales: FudoSale[] }> = ({ title, sales }) => (
    <Box mb={8}>
      <Heading size="md" mb={4} color={theme.colors.text.primary}>{title}</Heading>
      {sales.length === 0 ? (
        <Text color="gray.500" textAlign="center" py={4}>
          Sin ventas {title.toLowerCase()}.
        </Text>
      ) : (
        <Table variant="simple" bg="white" borderRadius="md" overflow="hidden">
          <Thead bg="gray.50">
            <Tr>
              <Th>ID</Th>
              <Th>Hora Inicio</Th>
              <Th>Origen</Th>
              <Th>Estado</Th>
              <Th>Cliente</Th>
              <Th isNumeric>Total</Th>
            </Tr>
          </Thead>
          <Tbody>
            {sales.map((sale) => (
              <Tr 
                key={sale.id}
                cursor="pointer"
                _hover={{ bg: 'gray.50' }}
                onClick={() => handleSaleClick(sale)}
              >
                <Td>{sale.id}</Td>
                <Td>{new Date(sale.attributes.createdAt).toLocaleString('es-ES', {
                  day: '2-digit',
                  month: '2-digit',
                  year: '2-digit',
                  hour: '2-digit',
                  minute: '2-digit'
                })}</Td>
                <Td>{sale.attributes.saleType}</Td>
                <Td>
                  <Text 
                    display="inline-block"
                    px={2}
                    py={1}
                    borderRadius="md"
                    fontSize="sm"
                    bg={sale.attributes.saleState === 'IN-COURSE' ? 'red.100' : 'yellow.100'}
                    color={sale.attributes.saleState === 'IN-COURSE' ? 'red.700' : 'yellow.700'}
                  >
                    {sale.attributes.saleState === 'IN-COURSE' ? 'En curso' : 
                     sale.attributes.saleState === 'READY_TO_DELIVER' ? 'A entregar' :
                     'Pendiente'}
                  </Text>
                </Td>
                <Td>{sale.attributes.customerName || '-'}</Td>
                <Td isNumeric>${sale.attributes.total.toFixed(2)}</Td>
              </Tr>
            ))}
          </Tbody>
        </Table>
      )}
    </Box>
  );

  if (loading) {
    return (
      <Box>
        <MainNav />
        <Center py={8}>
          <Spinner size="xl" color="primary.500" />
        </Center>
      </Box>
    );
  }

  return (
    <Box bg={theme.colors.background.default} minH="100vh">
      <MainNav />
      
      <Box p={6}>
        <Flex justify="space-between" align="center" mb={6}>
          <Heading size="lg" color={theme.colors.text.primary}>
            {selectedTab === 'counter' ? 'MOSTRADOR' : 'DOMICILIO'}
          </Heading>
          <Button
            onClick={handleNewSale}
            bg={theme.colors.success.main}
            color="white"
            size="lg"
            px={8}
            _hover={{ bg: theme.colors.success.dark }}
          >
            + Nuevo Pedido
          </Button>
        </Flex>

        <Tabs 
          onChange={handleTabChange}
          colorScheme="green"
          mb={6}
          index={selectedTab === 'counter' ? 0 : 1}
        >
          <TabList>
            <Tab>Mostrador</Tab>
            <Tab>Domicilio</Tab>
          </TabList>

          <TabPanels>
            <TabPanel px={0}>
              <Box>
                <SalesTable title="PENDIENTE" sales={counterSales.pending} />
                <SalesTable title="EN CURSO" sales={counterSales.inProgress} />
                <SalesTable title="A ENTREGAR" sales={counterSales.toDeliver} />
              </Box>
            </TabPanel>

            <TabPanel px={0}>
              <Box>
                <SalesTable title="PENDIENTE" sales={deliverySales.pending} />
                <SalesTable title="EN CURSO" sales={deliverySales.inProgress} />
                <SalesTable title="A ENTREGAR" sales={deliverySales.toDeliver} />
              </Box>
            </TabPanel>
          </TabPanels>
        </Tabs>
      </Box>
    </Box>
  );
}; 