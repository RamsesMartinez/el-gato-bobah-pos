import React, { useState } from 'react';
import { Box, Button, HStack, VStack, Text, Badge, Flex, Divider, ButtonGroup } from '@chakra-ui/react';
import { useNavigate } from 'react-router-dom';

interface SaleDetails {
  id: string;
  ticketNumber: string;
  table: string;
  items: string[];
  total: number;
  status: 'completed' | 'cancelled';
  openedAt: string;
  closedAt: string;
  server: string;
  guests: number;
  paymentMethod: 'cash' | 'card';
}

export const SalesHistory: React.FC = () => {
  const navigate = useNavigate();
  const [selectedDate, setSelectedDate] = useState(new Date());
  const [activeFilter, setActiveFilter] = useState<'all' | 'cash' | 'card'>('all');

  return (
    <Box bg="gray.900" minH="100vh">
      <VStack spacing={0} align="stretch">
        {/* Header con filtros */}
        <Box p={4} borderBottom="1px" borderColor="gray.700">
          <HStack justify="space-between">
            <ButtonGroup>
              <Button
                colorScheme={activeFilter === 'all' ? 'blue' : 'gray'}
                variant="ghost"
                onClick={() => setActiveFilter('all')}
              >
                Todas las órdenes
              </Button>
              <Button
                colorScheme={activeFilter === 'cash' ? 'blue' : 'gray'}
                variant="ghost"
                onClick={() => setActiveFilter('cash')}
              >
                En efectivo
              </Button>
              <Button
                colorScheme={activeFilter === 'card' ? 'blue' : 'gray'}
                variant="ghost"
                onClick={() => setActiveFilter('card')}
              >
                Con tarjeta
              </Button>
            </ButtonGroup>
            <Button
              variant="outline"
              colorScheme="gray"
              onClick={() => {/* Implementar selector de fecha */}}
            >
              12 de abril de 2024
            </Button>
          </HStack>
        </Box>

        {/* Tabla de ventas */}
        <Box p={4}>
          <VStack spacing={4} align="stretch">
            <HStack p={2} bg="gray.800" borderRadius="md">
              <Text flex="0.5" fontWeight="bold">Abierto</Text>
              <Text flex="1" fontWeight="bold">Hora de entrega</Text>
              <Text flex="2" fontWeight="bold">Orden</Text>
              <Text flex="1" fontWeight="bold">Estatus</Text>
              <Text flex="1" textAlign="right" fontWeight="bold">Importe</Text>
            </HStack>

            {/* Ejemplo de una venta */}
            <Box p={4} bg="gray.800" borderRadius="md" cursor="pointer" _hover={{ bg: 'gray.700' }}>
              <HStack>
                <Text flex="0.5">23:46</Text>
                <Text flex="1">32 minutes ago</Text>
                <VStack flex="2" align="start" spacing={1}>
                  <Text fontWeight="bold">Nº 2, Mesa 1</Text>
                  <Text color="gray.400" fontSize="sm" noOfLines={1}>
                    Quesadilla potatoes with chorizo, Order of 2 sopes
                  </Text>
                </VStack>
                <Badge flex="1" colorScheme="green">Completado</Badge>
                <Text flex="1" textAlign="right" fontWeight="bold">$113.00</Text>
              </HStack>
            </Box>
          </VStack>
        </Box>
      </VStack>
    </Box>
  );
}; 