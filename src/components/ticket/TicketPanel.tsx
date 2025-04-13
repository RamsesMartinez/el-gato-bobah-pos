import React from 'react';
import { Box, Text, Button, VStack, HStack, Divider } from '@chakra-ui/react';
import { useTheme } from '../../hooks/useTheme';

interface CurrentSale {
  items: Array<{
    product_id: string;
    quantity: number;
    unit_price: number;
    modifiers?: Array<{
      id: string;
      quantity: number;
    }>;
  }>;
  total_amount: number;
  status: 'pending' | 'completed' | 'cancelled';
}

interface TicketPanelProps {
  currentSale: CurrentSale;
  onPay: () => void;
}

export const TicketPanel: React.FC<TicketPanelProps> = ({
  currentSale,
  onPay,
}) => {
  const { theme } = useTheme();

  return (
    <Box
      width="400px"
      height="100%"
      borderLeft={`1px solid ${theme.colors.border.main}`}
      backgroundColor={theme.colors.background.paper}
      display="flex"
      flexDirection="column"
    >
      <VStack spacing={4} p={4} align="stretch">
        <Text fontSize="xl" fontWeight="bold">
          Ticket #1
        </Text>
        <Divider />
        <Box flex="1" overflowY="auto">
          {currentSale.items.map((item, index) => (
            <HStack key={index} spacing={4} p={2}>
              <Text flex="1">{item.product_id}</Text>
              <Text>{item.quantity}x</Text>
              <Text>${item.unit_price.toFixed(2)}</Text>
            </HStack>
          ))}
        </Box>
        <Divider />
        <HStack justify="space-between">
          <Text fontWeight="bold">Total:</Text>
          <Text fontWeight="bold">${currentSale.total_amount.toFixed(2)}</Text>
        </HStack>
        <Button
          colorScheme="green"
          size="lg"
          onClick={onPay}
          isDisabled={currentSale.items.length === 0}
        >
          Pagar
        </Button>
      </VStack>
    </Box>
  );
}; 