import {
  Box, Flex, VStack, Text, Button, Image } from '@chakra-ui/react';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { LuChevronDown } from 'react-icons/lu';
import { NavLink, Outlet, useNavigate } from 'react-router';
import logo from '../assets/logo.webp';
import { posApi } from '../api/pos';
import { attendanceApi } from '../api/attendance';
import { useSessionStore } from '../stores/session';
import { useUiStore } from '../stores/ui';
import { canAccess } from './roles';
import { RADIUS } from '../theme/ui';
import { SystemInfo } from './SystemInfo';
import { NAV } from './nav';

export function AppShell() {
  const user = useSessionStore((s) => s.user);
  const clear = useSessionStore((s) => s.clear);
  const palette = useUiStore((s) => s.palette);
  const navigate = useNavigate();
  // No mostrar accesos que el rol no puede usar (evita el 403 sorpresa). El backend sigue
  // siendo la autoridad; esto es solo UX.
  // La asistencia es activable por empresa: apagada, su ítem no ocupa lugar en el riel.
  const { data: attendance } = useQuery({ queryKey: ['asistencia', 'roster'], queryFn: attendanceApi.roster, staleTime: 60_000 });
  const navList = useRef<HTMLDivElement>(null);
  const [moreBelow, setMoreBelow] = useState(false);
  const measureRail = useCallback(() => {
    const el = navList.current;
    setMoreBelow(!!el && el.scrollHeight - el.scrollTop - el.clientHeight > 4);
  }, []);
  const nav = NAV.filter((n) => canAccess(user?.role, n.to) && (n.to !== '/asistencia' || attendance?.enabled === true));
  useEffect(() => {
    measureRail();
    window.addEventListener('resize', measureRail);
    return () => window.removeEventListener('resize', measureRail);
  }, [measureRail, nav.length]);

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
    // `data-medible` marca QUÉ cuenta como pantalla para el mapa de toques (spec 019), y la marca
    // es fail-closed: lo que no cuelga de aquí no se cuenta. Eso deja fuera, por construcción, todo
    // lo que se pinta en un portal —hojas, diálogos y los avisos flotantes— y la pantalla de
    // bloqueo, que vive por encima del shell. La lista de exclusiones se invertió a propósito:
    // excluir capas conocidas hacía que un portal nuevo naciera CONTADO, y el aviso flotante cae
    // abajo a la derecha, justo sobre la zona del botón de cobrar.
    <Flex data-medible h="100dvh" overflow="hidden" colorPalette={palette}>
      <Flex direction="column" w="76px" bg="gray.900" color="white" py={3} flexShrink={0}>
        <Image src={logo} alt="El Gato Bobah" boxSize="44px" borderRadius="lg" mb={2} alignSelf="center" flexShrink={0} />
        {/* lista scrollable: en pantallas de poco alto (7") no se recortan los ítems. La barra de
            scroll va oculta, así que un ítem abajo del pliegue no se notaba (medido a 1024×600 con
            el rol admin: «Asistencia» desaparecía sin rastro). La flecha del pie dice que hay más. */}
        <Box position="relative" flex="1" minH={0} display="flex">
        <VStack
          ref={navList} onScroll={measureRail}
          flex="1" minH={0} overflowY="auto" gap={1} w="100%"
          css={{ scrollbarWidth: 'none', '&::-webkit-scrollbar': { display: 'none' } }}
        >
          {nav.map((n) => {
            const Icon = n.icon;
            return (
              <NavLink key={n.to} to={n.to} style={{ width: '100%' }}>
                {({ isActive }) => (
                  <VStack
                    gap={1} py={1.5} minH="44px" borderRadius={RADIUS} mx={1}
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
        </Box>
        {/* La flecha va en su propio renglón y no encima de la lista: superpuesta tapaba el rótulo del
            último ítem visible, y a 600 px «Reportes» se leía encimado con el nombre de quien entró. */}
        <Flex data-rail-more h="16px" flexShrink={0} justify="center" align="center" aria-hidden
          visibility={moreBelow ? 'visible' : 'hidden'}>
          <LuChevronDown size={14} />
        </Flex>
        <VStack gap={0} flexShrink={0} borderTopWidth="1px" borderColor="whiteAlpha.300" pt={1}>
          <Text fontSize="10px" color="gray.400" lineClamp={1} px={1}>{user?.name}</Text>
          {/* 44 px: es un control táctil como cualquier otro, aunque se toque poco. */}
          <Button size="xs" minH="44px" w="100%" variant="ghost" colorPalette="whiteAlpha" onClick={logout}>Salir</Button>
          <SystemInfo />
        </VStack>
      </Flex>
      <Box flex="1" minW={0} overflowY="auto">
        <Outlet />
      </Box>
    </Flex>
  );
}
