import React from 'react';
import { Flex, Text, Box } from '@chakra-ui/react';
import { useNavigate, useLocation } from 'react-router-dom';

export const MainTabs: React.FC = () => {
  const navigate = useNavigate();
  const location = useLocation();

  const tabs = [
    { path: '/sales', label: 'Ventas en proceso' },
    { path: '/sales/history', label: 'Historial de pedidos' }
  ];

  return (
    <Flex 
      bg="gray.900" 
      color="gray.300" 
      p={4} 
      gap={8}
      borderBottom="1px"
      borderColor="gray.700"
    >
      {tabs.map((tab) => (
        <Box
          key={tab.path}
          cursor="pointer"
          onClick={() => navigate(tab.path)}
          position="relative"
          _after={{
            content: '""',
            position: 'absolute',
            bottom: '-17px',
            left: 0,
            right: 0,
            height: '2px',
            bg: location.pathname === tab.path ? 'green.500' : 'transparent',
          }}
        >
          <Text
            fontSize="xl"
            fontWeight={location.pathname === tab.path ? 'semibold' : 'normal'}
            color={location.pathname === tab.path ? 'white' : 'inherit'}
          >
            {tab.label}
          </Text>
        </Box>
      ))}
    </Flex>
  );
};

// Asegurarnos de que el archivo sea tratado como un módulo
export {}; 