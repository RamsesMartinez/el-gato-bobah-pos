import React from 'react';
import { Box } from '@chakra-ui/react';
import { MainNav } from '../../components/layout/MainNav';

export const SalesHistory: React.FC = () => {
  return (
    <Box>
      <MainNav />
      <Box p={6}>
        Historial de Ventas
      </Box>
    </Box>
  );
}; 