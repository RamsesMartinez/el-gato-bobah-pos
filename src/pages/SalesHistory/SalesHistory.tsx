import React from 'react';
import { Box, Button, HStack, VStack, Text, Badge } from '@chakra-ui/react';
import { useNavigate } from 'react-router-dom';
import { MainNav } from '../../components/layout/MainNav';
import { useTheme } from '../../hooks/useTheme';

export const SalesHistory: React.FC = () => {
  const { theme } = useTheme();

  return (
    <Box bg="white" minH="100vh">
      <VStack spacing={0} align="stretch">
        <MainNav />
        {/* Contenido del historial de ventas */}
      </VStack>
    </Box>
  );
}; 