import React, { useState, useEffect } from 'react';
import { Box, Button, Text, Flex, Center, Spinner, Heading, Table, Thead, Tbody, Tr, Th, Td, Tabs, TabList, Tab, TabPanels, TabPanel } from '@chakra-ui/react';
import { useNavigate, useLocation } from 'react-router-dom';
import { MainNav } from '../../components/layout/MainNav';
import { useTheme } from '../../hooks/useTheme';
import { FudoSale } from '../../types/fudo';
import { saleService } from '../../services/api/sales';
import { ROUTES } from '../../constants/routes';
import { SALE_STATES, isStateInGroup } from '../../constants/saleStates';
import { SaleState } from '../../types/filters';

interface SalesGroups {
  pending: FudoSale[];
  inProgress: FudoSale[];
  toDeliver: FudoSale[];
}

type SaleType = 'takeaway' | 'eat-in' | 'delivery';

// Agregar estas funciones auxiliares al inicio del componente
const getStateStyle = (state: SaleState): { bg: string; color: string } => {
  switch (state) {
    case SALE_STATES.PENDING:
      return { bg: 'purple.50', color: 'purple.700' };
    case SALE_STATES['IN-COURSE']:
      return { bg: 'pink.50', color: 'pink.700' };
    case SALE_STATES['PAYMENT-PROCESS']:
      return { bg: 'blue.50', color: 'blue.700' };
    case SALE_STATES['READY_TO_DELIVER']:
      return { bg: 'green.50', color: 'green.700' };
    case SALE_STATES['DELIVERY-SENT']:
      return { bg: 'orange.50', color: 'orange.700' };
    case SALE_STATES.CANCELED:
      return { bg: 'gray.50', color: 'gray.700' };
    case SALE_STATES.CLOSED:
      return { bg: 'gray.50', color: 'gray.700' };
    default:
      return { bg: 'purple.50', color: 'purple.700' };
  }
};

const getStateText = (state: SaleState): string => {
  switch (state) {
    case SALE_STATES.PENDING:
      return 'Pendiente';
    case SALE_STATES['IN-COURSE']:
      return 'En curso';
    case SALE_STATES['PAYMENT-PROCESS']:
      return 'En pago';
    case SALE_STATES['READY_TO_DELIVER']:
      return 'Listo para entregar';
    case SALE_STATES['DELIVERY-SENT']:
      return 'En camino';
    case SALE_STATES.CANCELED:
      return 'Cancelado';
    case SALE_STATES.CLOSED:
      return 'Cerrado';
    default:
      return 'Pendiente';
  }
};

export const ActiveSales: React.FC = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const { theme } = useTheme();
  const [takeawaySales, setTakeawaySales] = useState<SalesGroups>({
    pending: [],
    inProgress: [],
    toDeliver: []
  });
  const [eatInSales, setEatInSales] = useState<SalesGroups>({
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
    switch (path) {
      case ROUTES.SALES.ACTIVE.DELIVERY:
        return 'delivery';
      case ROUTES.SALES.ACTIVE.EAT_IN:
        return 'eat-in';
      default:
        return 'takeaway';
    }
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
    let newPath: string;
    switch (index) {
      case 0:
        newPath = ROUTES.SALES.ACTIVE.TAKEAWAY;
        break;
      case 1:
        newPath = ROUTES.SALES.ACTIVE.EAT_IN;
        break;
      case 2:
        newPath = ROUTES.SALES.ACTIVE.DELIVERY;
        break;
      default:
        newPath = ROUTES.SALES.ACTIVE.TAKEAWAY;
    }
    navigate(newPath);
  };

  const loadSalesByType = async (type: SaleType) => {
    try {
      setLoading(true);
      let response;
      
      switch(type) {
        case 'takeaway':
          response = await saleService.getTakeawaySales();
          break;
        case 'eat-in':
          response = await saleService.getEatInSales();
          break;
        case 'delivery':
          response = await saleService.getDeliverySales();
          break;
      }

      // Asegurarnos de que response.data es un array
      const sales = Array.isArray(response.data) ? response.data : [];

      const groupedSales = {
        pending: sales.filter(sale => 
          isStateInGroup(sale.attributes.saleState as SaleState, 'PENDING')
        ),
        inProgress: sales.filter(sale => 
          isStateInGroup(sale.attributes.saleState as SaleState, 'IN_PROGRESS')
        ),
        toDeliver: sales.filter(sale => 
          isStateInGroup(sale.attributes.saleState as SaleState, 'TO_DELIVER')
        )
      };

      switch(type) {
        case 'takeaway':
          setTakeawaySales(groupedSales);
          break;
        case 'eat-in':
          setEatInSales(groupedSales);
          break;
        case 'delivery':
          setDeliverySales(groupedSales);
          break;
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
                    bg={getStateStyle(sale.attributes.saleState).bg}
                    color={getStateStyle(sale.attributes.saleState).color}
                  >
                    {getStateText(sale.attributes.saleState)}
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
            {selectedTab === 'takeaway' ? 'PARA LLEVAR' :
             selectedTab === 'eat-in' ? 'COMER AQUÍ' :
             'DOMICILIO'}
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
          index={selectedTab === 'takeaway' ? 0 : selectedTab === 'eat-in' ? 1 : 2}
        >
          <TabList>
            <Tab>Para Llevar</Tab>
            <Tab>Comer Aquí</Tab>
            <Tab>Domicilio</Tab>
          </TabList>

          <TabPanels>
            <TabPanel px={0}>
              <SalesTable title="Pendientes" sales={takeawaySales.pending} />
              <SalesTable title="En Curso" sales={takeawaySales.inProgress} />
              <SalesTable title="Listos para Entregar" sales={takeawaySales.toDeliver} />
            </TabPanel>
            <TabPanel px={0}>
              <SalesTable title="Pendientes" sales={eatInSales.pending} />
              <SalesTable title="En Curso" sales={eatInSales.inProgress} />
              <SalesTable title="Listos para Entregar" sales={eatInSales.toDeliver} />
            </TabPanel>
            <TabPanel px={0}>
              <SalesTable title="Pendientes" sales={deliverySales.pending} />
              <SalesTable title="En Curso" sales={deliverySales.inProgress} />
              <SalesTable title="Listos para Entregar" sales={deliverySales.toDeliver} />
            </TabPanel>
          </TabPanels>
        </Tabs>
      </Box>
    </Box>
  );
}; 