import { Flex, Box, HStack, Button } from '@chakra-ui/react';
import { LuTag, LuStar } from 'react-icons/lu';
import { Outlet, useLocation, useNavigate } from 'react-router-dom';

// Hub "Catálogo": agrupa Productos y Opciones (relacionados) bajo un solo item del menú,
// con pestañas de 1 toque. Cada pestaña conserva su propia pantalla (Page fill).
const TABS = [
  { to: '/catalogo/productos', label: 'Productos', icon: LuTag },
  { to: '/catalogo/opciones', label: 'Opciones', icon: LuStar },
];

export function CatalogPage() {
  const nav = useNavigate();
  const { pathname } = useLocation();

  return (
    <Flex direction="column" h="100%">
      <Box flexShrink={0} borderBottomWidth="1px" bg="bg.panel">
        <HStack maxW="1150px" mx="auto" px={6} pt={4} gap={2}>
          {TABS.map((t) => {
            const Icon = t.icon;
            const active = pathname.startsWith(t.to);
            return (
              <Button key={t.to} size="md" borderBottomRadius={0} onClick={() => nav(t.to)}
                variant={active ? 'solid' : 'ghost'} colorPalette={active ? undefined : 'gray'}>
                <Icon /> {t.label}
              </Button>
            );
          })}
        </HStack>
      </Box>
      <Box flex="1" minH={0}>
        <Outlet />
      </Box>
    </Flex>
  );
}
