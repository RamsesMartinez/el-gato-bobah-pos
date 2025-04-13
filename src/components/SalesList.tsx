import React, { useEffect, useState } from 'react';
import { Box, Grid, Text, Spinner, Alert, AlertIcon, AlertTitle, AlertDescription, Button, Flex, Badge, Icon } from '@chakra-ui/react';
import { TimeIcon, CheckIcon, WarningIcon, InfoIcon } from '@chakra-ui/icons';
import { FudoSale } from '../types/fudo';
import { saleService } from '../services/api/sales';
import { formatDateTime } from '../utils/dateUtils';
import { useTheme } from '../hooks/useTheme';

type OrderStatus = 'nuevo' | 'en_proceso' | 'completado' | 'cancelado';

// Función para obtener el color del estado
const getStatusColor = (status: OrderStatus, theme: any): string => {
  const statusColors = {
    'nuevo': theme.colors.status.new,
    'en_proceso': theme.colors.status.inProgress,
    'completado': theme.colors.status.completed,
    'cancelado': theme.colors.text.disabled
  };
  return statusColors[status];
};

// Función para obtener el icono del tipo de orden
const getOrderTypeIcon = (type: string, theme: any) => {
  switch (type) {
    case 'para_llevar':
      return <Icon as={InfoIcon} color={theme.colors.primary.main} />;
    case 'delivery':
      return <Icon as={InfoIcon} color={theme.colors.warning.main} />;
    case 'mesa':
      return <Icon as={InfoIcon} color={theme.colors.success.main} />;
    default:
      return null;
  }
};

export const SalesList: React.FC = () => {
  const [sales, setSales] = useState<FudoSale[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const { theme } = useTheme();

  useEffect(() => {
    const fetchSales = async () => {
      try {
        const response = await saleService.getSales();
        setSales(response.data);
        setError(null);
      } catch (err) {
        setError('Error al cargar las ventas');
        console.error('Error fetching sales:', err);
      } finally {
        setLoading(false);
      }
    };

    fetchSales();
  }, []);

  if (loading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minH="200px">
        <Spinner size="xl" color={theme.colors.primary.main} />
      </Box>
    );
  }

  if (error) {
    return (
      <Alert status="error" m={2} bg={theme.colors.error.main} color={theme.colors.text.light}>
        <AlertIcon />
        <AlertTitle>Error</AlertTitle>
        <AlertDescription>{error}</AlertDescription>
      </Alert>
    );
  }

  if (sales.length === 0) {
    return (
      <Box m={2}>
        <Text fontSize="xl" color={theme.colors.text.primary}>No hay ventas disponibles</Text>
      </Box>
    );
  }

  return (
    <Box>
      {/* Header de la tabla */}
      <Grid
        templateColumns="120px 120px 1fr 120px 150px 80px"
        gap={4}
        p={3}
        borderBottomWidth="1px"
        borderColor={theme.colors.border.main}
        bg="transparent"
      >
        <Flex align="center" gap={2}>
          <TimeIcon boxSize={4} color={theme.colors.text.secondary} />
          <Text color={theme.colors.text.secondary} fontSize="sm">Abierto</Text>
        </Flex>
        <Flex align="center" gap={2}>
          <TimeIcon boxSize={4} color={theme.colors.text.secondary} />
          <Text color={theme.colors.text.secondary} fontSize="sm">Hora de entrega</Text>
        </Flex>
        <Text color={theme.colors.text.secondary} fontSize="sm">Orden</Text>
        <Text color={theme.colors.text.secondary} fontSize="sm">Estatus</Text>
        <Text color={theme.colors.text.secondary} fontSize="sm" textAlign="right">Importe</Text>
        <Box />
      </Grid>

      {/* Lista de ventas */}
      {sales.map((sale) => (
        <Grid
          key={sale.id}
          templateColumns="120px 120px 1fr 120px 150px 80px"
          gap={4}
          p={3}
          borderBottomWidth="1px"
          borderColor={theme.colors.border.main}
          alignItems="center"
          bg="transparent"
          _hover={{ bg: `${theme.colors.primary.main}15` }}
          transition="all 0.2s"
        >
          <Text fontSize="sm" color={theme.colors.text.primary}>
            {formatDateTime(sale.attributes.openedAt)}
          </Text>
          <Text color={theme.colors.error.main} fontSize="sm">
            {sale.attributes.closedAt ? formatDateTime(sale.attributes.closedAt) : '--:--'}
            {!sale.attributes.closedAt && (
              <WarningIcon ml={2} color={theme.colors.warning.main} boxSize={3} />
            )}
          </Text>
          <Flex align="center" gap={2}>
            {getOrderTypeIcon(sale.attributes.type, theme)}
            <Text fontSize="sm" color={theme.colors.text.primary}>
              #{sale.attributes.number}
            </Text>
            <Text color={theme.colors.text.secondary} fontSize="sm">•</Text>
            <Badge 
              bg={`${theme.colors.primary.main}15`}
              color={theme.colors.primary.main}
              fontSize="xs"
              px={2}
              py={1}
              borderRadius={theme.borderRadius.sm}
            >
              {sale.attributes.type}
            </Badge>
            {sale.attributes.customerName && (
              <>
                <Text color={theme.colors.text.secondary} fontSize="sm">•</Text>
                <Text color={theme.colors.text.primary} fontSize="sm">
                  {sale.attributes.customerName}
                </Text>
              </>
            )}
          </Flex>
          <Badge
            bg={theme.colors.status.new}
            color={theme.colors.text.light}
            fontSize="xs"
            px={2}
            py={1}
            borderRadius={theme.borderRadius.sm}
          >
            NUEVO
          </Badge>
          <Text
            textAlign="right"
            fontSize="sm"
            color={theme.colors.text.primary}
          >
            ${sale.attributes.totalAmount.toFixed(2)}
          </Text>
          <Button
            bg={theme.colors.primary.main}
            color={theme.colors.text.light}
            size="sm"
            width="full"
            _hover={{ bg: theme.colors.primary.dark }}
          >
            Pagar
          </Button>
        </Grid>
      ))}
    </Box>
  );
}; 