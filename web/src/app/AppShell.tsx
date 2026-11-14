import { TemaRapido } from './TemaRapido';
import {
  Box, Flex, HStack, VStack, Text, Button, Image } from '@chakra-ui/react';
import { useCallback, useEffect, useRef, useState, useSyncExternalStore } from 'react';
import { useQuery } from '@tanstack/react-query';
import { LuChevronDown } from 'react-icons/lu';
import { Outlet, useLocation, useNavigate } from 'react-router';
import logo from '../assets/logo.webp';
import { posApi } from '../api/pos';
import { attendanceApi } from '../api/attendance';
import { useSessionStore } from '../stores/session';
import { useUiStore } from '../stores/ui';
import { can } from './permissions';
import { wasteApi } from '../api/waste';
import { SystemInfo } from './SystemInfo';
import { UpdateRailButton } from './UpdateRailButton';
import { useAppUpdate } from '../stores/appUpdate';
import { visibleModules } from './nav';
import { MenuRail, RAIL_WIDTH, RAIL_WIDTH_WIDE } from './MenuRail';
import { PendingWasteBar } from '../features/waste/PendingWasteBar';
import { railScrollFor } from './railScroll';

// Desde 1920×1080 el menú se muestra expandido (spec 085, FR-008). Con `matchMedia` y no leyendo el
// ancho una vez: la ventana se redimensiona y el menú la sigue.
const WIDE_QUERY = '(min-width: 1920px) and (min-height: 1080px)';
function subscribeWide(cb: () => void) {
  const mq = window.matchMedia(WIDE_QUERY);
  mq.addEventListener('change', cb);
  return () => mq.removeEventListener('change', cb);
}
function useWideMenu() {
  return useSyncExternalStore(subscribeWide, () => window.matchMedia(WIDE_QUERY).matches);
}

export function AppShell() {
  const user = useSessionStore((s) => s.user);
  const clear = useSessionStore((s) => s.clear);
  const palette = useUiStore((s) => s.palette);
  const needRefresh = useAppUpdate((s) => s.needRefresh);
  const navigate = useNavigate();
  const { pathname } = useLocation();
  // No mostrar accesos que los permisos no abren (evita el 403 sorpresa). El backend sigue
  // siendo la autoridad; esto es solo UX.
  // La asistencia es activable por empresa: apagada, su ítem no ocupa lugar en el riel.
  const { data: attendance } = useQuery({ queryKey: ['asistencia', 'roster'], queryFn: attendanceApi.roster, staleTime: 60_000 });
  const navList = useRef<HTMLDivElement>(null);
  const [moreBelow, setMoreBelow] = useState(false);
  const [overflowing, setOverflowing] = useState(false);
  const measureRail = useCallback(() => {
    const el = navList.current;
    setMoreBelow(!!el && el.scrollHeight - el.scrollTop - el.clientHeight > 4);
    // Se mide sin el relleno del final, que solo existe cuando la lista desborda.
    const spacer = el?.querySelector<HTMLElement>('[data-rail-spacer]');
    setOverflowing(!!el && el.scrollHeight - (spacer ? spacer.offsetHeight + 4 : 0) > el.clientHeight + 1);
  }, []);
  const modules = visibleModules(user, { attendanceEnabled: attendance?.enabled === true });
  const wide = useWideMenu();
  // Las mermas por aprobar, para el número del riel: en Vender y Pedidos es el único aviso.
  const approver = can('inventory.write_off_approve', user);
  const { data: pendingWaste } = useQuery({
    queryKey: ['merma', 'por-aprobar'], queryFn: wasteApi.pending, enabled: approver, staleTime: 30_000, refetchInterval: 60_000,
  });
  const pendingCount = approver ? (pendingWaste?.length ?? 0) : 0;
  // El módulo de la pantalla abierta siempre a la vista. A 1280×854 todo cabe; debajo de ese alto
  // (1280×768, la barra del navegador) la lista desplaza y el módulo abierto quedaba partido.
  useEffect(() => {
    const list = navList.current;
    const active = list?.querySelector<HTMLElement>('[data-active="true"], [aria-current="page"]');
    if (list && active) list.scrollTop = railScrollFor(list, active);
    // needRefresh: el botón de versión nueva toma el lugar del logo; mide lo mismo, pero si un día
    // el alto del riel cambia con él, la opción abierta no queda partida.
  }, [pathname, modules.length, needRefresh, wide, overflowing]);
  useEffect(() => {
    measureRail();
    window.addEventListener('resize', measureRail);
    return () => window.removeEventListener('resize', measureRail);
  }, [measureRail, modules.length, needRefresh, wide]);

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
      <Flex direction="column" w={wide ? RAIL_WIDTH_WIDE : RAIL_WIDTH} bg="gray.900" color="white" py={3} px={wide ? 2 : 1} flexShrink={0}>
        {/* El aviso de versión nueva va en el lugar del logo (spec 064): fuera de la pantalla y sin
            quitarle alto a la lista. */}
        {needRefresh
          ? <UpdateRailButton />
          : <Image src={logo} alt="El Gato Bobah" boxSize="44px" borderRadius="lg" mb={2} alignSelf="center" flexShrink={0} />}
        {/* A 1280×854 los módulos caben sin desplazar (spec 085). La lista sigue siendo desplazable
            como red para ventanas más bajas: la barra de scroll va oculta, así que un módulo abajo
            del pliegue no se notaría sin la flecha del pie, que dice que hay más. */}
        <Box position="relative" flex="1" minH={0} display="flex">
        <VStack
          ref={navList} onScroll={measureRail} data-rail-list
          flex="1" minH={0} overflowY="auto" gap={1} w="100%"
          // Se detiene siempre al inicio de una opción (spec 045): al llevar a la vista la opción de
          // la pantalla abierta, o al soltar el dedo, la de arriba quedaba partida a la mitad.
          css={{ scrollbarWidth: 'none', '&::-webkit-scrollbar': { display: 'none' }, scrollSnapType: 'y mandatory' }}
        >
          <MenuRail modules={modules} wide={wide} pendingWaste={pendingCount} />
          {/* Con la lista desplazada hasta el fondo, el desplazamiento máximo casi nunca coincide con
              el inicio de un módulo y el de arriba quedaba partido (B10, a 600 px). Un módulo de
              relleno deja que siempre se detenga en un inicio. Solo existe si la lista desborda: a
              1280×854 no se pinta y el riel no desplaza. */}
          {overflowing && <Box data-rail-spacer aria-hidden h="72px" flexShrink={0} />}
        </VStack>
        </Box>
        {/* La flecha va en su propio renglón y no encima de la lista: superpuesta tapaba el rótulo del
            último ítem visible, y a 600 px «Reportes» se leía encimado con el nombre de quien entró. */}
        <Flex data-rail-more h="16px" flexShrink={0} justify="center" align="center" aria-hidden
          visibility={moreBelow ? 'visible' : 'hidden'}>
          <LuChevronDown size={14} />
        </Flex>
        <VStack gap={0} flexShrink={0} borderTopWidth="1px" borderColor="whiteAlpha.300" pt={1}>
          {/* El tema y la campana de notificaciones (#291, otra sesión) comparten renglón: así la
              campana no le quita alto a los módulos. El hueco existe aunque todavía no haya campana. */}
          <HStack gap={0} w="100%">
            <Box flex="1"><TemaRapido /></Box>
            <Box data-bell-slot flex="1" minH="44px" />
          </HStack>
          <Text fontSize="10px" color="gray.400" lineClamp={1} px={1}>{user?.name}</Text>
          {/* 44 px: es un control táctil como cualquier otro, aunque se toque poco. */}
          <Button size="xs" minH="44px" w="100%" variant="ghost" colorPalette="whiteAlpha" color="gray.100" onClick={logout}>Salir</Button>
          <SystemInfo />
        </VStack>
      </Flex>
      {/* La franja de mermas por aprobar es una FILA de esta columna y no algo encima: así le quita
          su alto a la pantalla en vez de empujarla 44 px fuera de los 600 (spec 039). */}
      <Flex flex="1" minW={0} direction="column">
        <PendingWasteBar />
        <Box data-work-area flex="1" minH={0} overflowY="auto">
          <Outlet />
        </Box>
      </Flex>
    </Flex>
  );
}
