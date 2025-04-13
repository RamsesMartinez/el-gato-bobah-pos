import React from 'react';
import { Box, Flex, HStack, Text, IconButton, Menu, MenuButton, MenuList, MenuItem } from '@chakra-ui/react';
import { HamburgerIcon, LockIcon } from '@chakra-ui/icons';
import { useNavigate, useLocation } from 'react-router-dom';
import { useTheme } from '../../hooks/useTheme';

export const MainNav: React.FC = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const { theme } = useTheme();

  const isActive = (path: string) => location.pathname.startsWith(path);

  return (
    <Box bg={theme.colors.background.dark} borderBottom="1px" borderColor={theme.colors.border.dark}>
      <Flex justify="space-between" align="center" px={6} h="64px">
        <HStack spacing={8}>
          <Text
            color={isActive('/sales') && !isActive('/sales/history') ? theme.colors.text.light : theme.colors.text.disabled}
            fontWeight={isActive('/sales') && !isActive('/sales/history') ? "bold" : "normal"}
            cursor="pointer"
            onClick={() => navigate('/sales')}
            _hover={{ color: theme.colors.text.light }}
          >
            Ventas en proceso
          </Text>
          <Text
            color={isActive('/sales/history') ? theme.colors.text.light : theme.colors.text.disabled}
            fontWeight={isActive('/sales/history') ? "bold" : "normal"}
            cursor="pointer"
            onClick={() => navigate('/sales/history')}
            _hover={{ color: theme.colors.text.light }}
          >
            Historial de ventas
          </Text>
        </HStack>
        <Text color={theme.colors.text.light} fontWeight="bold">Bobah POS</Text>
        <HStack spacing={4}>
          <HStack spacing={2} color={theme.colors.text.disabled}>
            <Text>Ramses</Text>
            <LockIcon />
          </HStack>
          <Menu>
            <MenuButton
              as={IconButton}
              aria-label="Opciones"
              icon={<HamburgerIcon />}
              variant="ghost"
              color={theme.colors.text.disabled}
              _hover={{ bg: theme.colors.secondary.light }}
            />
            <MenuList bg={theme.colors.background.paper} borderColor={theme.colors.border.main}>
              <MenuItem _hover={{ bg: theme.colors.background.default }}>Configuración</MenuItem>
              <MenuItem _hover={{ bg: theme.colors.background.default }}>Cerrar sesión</MenuItem>
            </MenuList>
          </Menu>
        </HStack>
      </Flex>
    </Box>
  );
}; 