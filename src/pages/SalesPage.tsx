import React from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import { Box, Flex, Button, Text, Menu, MenuButton, MenuList, MenuItem } from '@chakra-ui/react';
import { ChevronDownIcon } from '@chakra-ui/icons';
import { SalesList } from '../components/SalesList';
import { useTheme } from '../hooks/useTheme';

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
        <Flex gap={8} align="center">
          <Box
            position="relative"
            cursor="pointer"
            onClick={() => navigate('/ventas')}
          >
            <Text color="white" fontSize="md">Ventas</Text>
            {isActive('/ventas') && (
              <Box
                position="absolute"
                bottom="-17px"
                left="0"
                right="0"
                h="3px"
                bg="blue.500"
                borderRadius="full"
              />
            )}
          </Box>
          <Box
            position="relative"
            cursor="pointer"
            onClick={() => navigate('/floor-plan')}
          >
            <Text color="gray.300" fontSize="md" _hover={{ color: 'white' }}>
              Plano de tu sucursal
            </Text>
            {isActive('/floor-plan') && (
              <Box
                position="absolute"
                bottom="-17px"
                left="0"
                right="0"
                h="3px"
                bg="blue.500"
                borderRadius="full"
              />
            )}
          </Box>
          <Box
            position="relative"
            cursor="pointer"
            onClick={() => navigate('/history')}
          >
            <Text color="gray.300" fontSize="md" _hover={{ color: 'white' }}>
              Historial de pedidos
            </Text>
            {isActive('/history') && (
              <Box
                position="absolute"
                bottom="-17px"
                left="0"
                right="0"
                h="3px"
                bg="blue.500"
                borderRadius="full"
              />
            )}
          </Box>
        </Flex>
        <Flex ml="auto" gap={4} align="center">
          <Flex align="center" gap={2}>
            <Text color="white">Ramses</Text>
            <Text fontSize="lg">🔒</Text>
          </Flex>
        </Flex>
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
        <SalesList />
      </Box>
    </Box>
  );
}; 