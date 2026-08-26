import { Box, Flex, VStack, Text, Button, Image } from '@chakra-ui/react';
import { NavLink, Outlet, useNavigate } from 'react-router';
import {
  LuShoppingCart, LuClipboardList, LuWallet, LuHandCoins,
  LuPackage, LuChartColumn, LuTag, LuUsers, LuPalette, LuStore, LuUserCog,
} from 'react-icons/lu';
import logo from '../assets/logo.webp';
import { posApi } from '../api/pos';
import { useSessionStore } from '../stores/session';
import { useUiStore } from '../stores/ui';
import { canAccess } from './roles';
import { RADIUS } from '../theme/ui';
import { SystemInfo } from './SystemInfo';

const NAV = [
  { to: '/pos', icon: LuShoppingCart, label: 'Vender' },
  { to: '/pedidos', icon: LuClipboardList, label: 'Pedidos' },
  { to: '/caja', icon: LuWallet, label: 'Caja' },
  { to: '/gastos', icon: LuHandCoins, label: 'Gastos' },
  { to: '/almacen', icon: LuPackage, label: 'Almacén' },
  { to: '/reportes', icon: LuChartColumn, label: 'Reportes' },
  { to: '/catalogo', icon: LuTag, label: 'Catálogo' },
  { to: '/empleados', icon: LuUsers, label: 'Empleados' },
  { to: '/negocio', icon: LuStore, label: 'Negocio' },
  { to: '/apariencia', icon: LuPalette, label: 'Interfaz' },
  { to: '/cuenta', icon: LuUserCog, label: 'Mi cuenta' },
];

export function AppShell() {
  const user = useSessionStore((s) => s.user);
  const clear = useSessionStore((s) => s.clear);
  const palette = useUiStore((s) => s.palette);
  const navigate = useNavigate();
  // No mostrar accesos que el rol no puede usar (evita el 403 sorpresa). El backend sigue
  // siendo la autoridad; esto es solo UX.
  const nav = NAV.filter((n) => canAccess(user?.role, n.to));

  const logout = async () => {
    // Revoca la cookie de refresh en el server ANTES de limpiar; si no, la sesión revive
    // tras un reload (la cookie sobrevive y el arranque la canjea). Best-effort: si la red
    // se cae, igual cerramos localmente.
    try {
      await posApi.logout();
    } catch {
      /* red caída: cerramos localmente de todos modos */
    }
    clear();
    navigate('/login');
  };

  return (
    <Flex h="100dvh" overflow="hidden" colorPalette={palette}>
      <Flex direction="column" w="76px" bg="gray.900" color="white" py={3} flexShrink={0}>
        <Image src={logo} alt="El Gato Bobah" boxSize="44px" borderRadius="lg" mb={2} alignSelf="center" flexShrink={0} />
        {/* lista scrollable: en pantallas de poco alto (7") no se recortan los ítems */}
        <VStack
          flex="1" minH={0} overflowY="auto" gap={2} w="100%"
          css={{ scrollbarWidth: 'none', '&::-webkit-scrollbar': { display: 'none' } }}
        >
          {nav.map((n) => {
            const Icon = n.icon;
            return (
              <NavLink key={n.to} to={n.to} style={{ width: '100%' }}>
                {({ isActive }) => (
                  <VStack
                    gap={1} py={2} borderRadius={RADIUS} mx={1}
                    bg={isActive ? 'colorPalette.600' : 'transparent'}
                    _hover={{ bg: isActive ? 'colorPalette.600' : 'whiteAlpha.200' }}
                  >
                    <Icon size={22} />
                    <Text fontSize="10px">{n.label}</Text>
                  </VStack>
                )}
              </NavLink>
            );
          })}
        </VStack>
        <VStack gap={0} flexShrink={0} pt={2}>
          <Text fontSize="10px" color="gray.400" lineClamp={1} px={1}>{user?.name}</Text>
          <Button size="xs" variant="ghost" colorPalette="whiteAlpha" onClick={logout}>Salir</Button>
          <SystemInfo />
        </VStack>
      </Flex>
      <Box flex="1" minW={0} overflowY="auto">
        <Outlet />
      </Box>
    </Flex>
  );
}
