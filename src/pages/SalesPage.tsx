import React from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import { Box, Flex, Button, Text, Menu, MenuButton, MenuList, MenuItem } from '@chakra-ui/react';
import { ChevronDownIcon } from '@chakra-ui/icons';
import { useTheme } from '../hooks/useTheme';
import { ActiveSales } from './ActiveSales/ActiveSales';

export const SalesPage: React.FC = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const { theme } = useTheme();

  const handleNewSale = () => {
    navigate('/dashboard');
  };

  const isActive = (path: string) => location.pathname === path;

  return (
    <Box h="100vh" bg={theme.colors.background.default}>
      {/* Header/Navbar */}
      <Flex
        bg="gray.900"
        p={4}
        align="center"
        borderBottomWidth="1px"
        borderColor="gray.700"
      >
        <Text color="white" fontSize="xl" fontWeight="bold">
          Bobah POS
        </Text>
      </Flex>

      {/* Subheader con botón de nueva venta */}
      <Flex
        p={4}
        bg="gray.900"
        borderBottomWidth="1px"
        borderColor="gray.700"
        justify="space-between"
        align="center"
      >
        <Button
          onClick={handleNewSale}
          bg="#00C853"
          color="white"
          _hover={{ bg: '#00B848' }}
          size="md"
          borderRadius="md"
          px={6}
        >
          Nuevo pedido
        </Button>
        <Menu>
          <MenuButton
            as={Button}
            rightIcon={<ChevronDownIcon />}
            bg="transparent"
            color="white"
            _hover={{ bg: 'whiteAlpha.200' }}
            _active={{ bg: 'whiteAlpha.300' }}
          >
            Tipo de pedido
          </MenuButton>
          <MenuList>
            <MenuItem>Para llevar</MenuItem>
            <MenuItem>Delivery</MenuItem>
            <MenuItem>Mesa</MenuItem>
          </MenuList>
        </Menu>
      </Flex>

      {/* Contenido principal */}
      <Box flex={1} overflow="auto" bg={theme.colors.background.default}>
        <ActiveSales />
      </Box>
    </Box>
  );
}; 