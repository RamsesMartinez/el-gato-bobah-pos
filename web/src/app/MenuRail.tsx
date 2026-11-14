import { Box, Flex, HStack, Portal, Text, VStack } from '@chakra-ui/react';
import { useCallback, useEffect, useRef, useState } from 'react';
import { LuChevronRight } from 'react-icons/lu';
import { NavLink, useLocation } from 'react-router';
import { BORDER_W, RADIUS, TAP_LG } from '../theme/ui';
import { useUiStore } from '../stores/ui';
import { moduleOf, PRIMARY_MODULES, type MenuModule, type ModuleId } from './nav';

// Ancho del riel angosto: el panel se abre pegado a él.
export const RAIL_WIDTH = '96px';
export const RAIL_WIDTH_WIDE = '260px';
const PANEL_WIDTH = 240;
const ROW_H = parseInt(TAP_LG, 10);

const WASTE_ITEM = '/merma';

// Cuántos avisos de «Se echó a perder» esperan aprobación. Con `role="img"` el lector dice qué es; el
// número solo no dice nada.
function WasteBadge({ n }: { n: number }) {
  return (
    <Box as="span" role="img" aria-label={`${n} por aprobar en Se echó a perder`} position="absolute" top="-6px" right="-12px"
      minW="18px" h="18px" px="4px" borderRadius="full" bg="orange.500" color="white" fontSize="11px" fontWeight="700"
      lineHeight="18px" textAlign="center">
      {n}
    </Box>
  );
}

type Placement = { top: number } | { bottom: number };

// MenuRail es el menú lateral en dos niveles (spec 085, maqueta C ajustada). En el equipo del
// mostrador cada módulo es un botón de 72 px; el que tiene varios destinos abre un panel junto al
// riel y el que tiene uno solo abre directo. En pantalla ancha se ve todo, sin panel.
export function MenuRail({ modules, wide, pendingWaste }: { modules: MenuModule[]; wide: boolean; pendingWaste: number }) {
  return wide
    ? <WideRail modules={modules} pendingWaste={pendingWaste} />
    : <NarrowRail modules={modules} pendingWaste={pendingWaste} />;
}

function NarrowRail({ modules, pendingWaste }: { modules: MenuModule[]; pendingWaste: number }) {
  const { pathname } = useLocation();
  const current = moduleOf(pathname);
  const palette = useUiStore((s) => s.palette);
  // El panel abierto recuerda en qué ruta se abrió. Al cambiar de ruta por cualquier lado se olvida
  // (no solo se esconde: volver con «atrás» lo reabría), y con un solo destino ya no hay panel que
  // mostrar porque el botón se volvió enlace. Se ajusta durante el render, sin efecto de por medio.
  const [open, setOpen] = useState<{ id: ModuleId; path: string; at: Placement } | null>(null);
  const stale = open && (open.path !== pathname || (modules.find((m) => m.id === open.id)?.items.length ?? 0) < 2);
  if (stale) setOpen(null);
  const openId = stale ? null : open?.id ?? null;
  const buttons = useRef(new Map<ModuleId, HTMLButtonElement>());
  const panelRef = useRef<HTMLElement>(null);

  // Al abrir, el foco va al primer destino: con teclado se elige sin recorrer el riel otra vez.
  useEffect(() => {
    if (openId) panelRef.current?.querySelector<HTMLElement>('a')?.focus();
  }, [openId]);

  const close = useCallback((returnFocus: boolean) => {
    setOpen((cur) => {
      if (returnFocus && cur) buttons.current.get(cur.id)?.focus();
      return null;
    });
  }, []);

  // Mientras el panel está abierto: Escape lo cierra esté donde esté el foco (abrir con el dedo deja
  // el foco en el botón, no en el panel), y redimensionar o desplazar el riel lo cierran porque su
  // lugar se calculó al abrir y quedaría despegado de su botón.
  useEffect(() => {
    if (!openId) return;
    const onKey = (e: globalThis.KeyboardEvent) => { if (e.key === 'Escape') close(true); };
    const onMove = () => close(false);
    const list = buttons.current.get(openId)?.closest('[data-rail-list]');
    document.addEventListener('keydown', onKey);
    window.addEventListener('resize', onMove);
    list?.addEventListener('scroll', onMove);
    return () => {
      document.removeEventListener('keydown', onKey);
      window.removeEventListener('resize', onMove);
      list?.removeEventListener('scroll', onMove);
    };
  }, [openId, close]);

  // Al elegir, el panel se desmonta con el foco dentro y el foco caería en <body>: va al área de
  // trabajo, donde está la pantalla que se acaba de abrir.
  const pick = () => {
    setOpen(null);
    document.querySelector<HTMLElement>('[data-work-area]')?.focus();
  };

  const toggle = (m: MenuModule) => {
    if (openId === m.id) return close(true);
    const btn = buttons.current.get(m.id);
    const rect = btn?.getBoundingClientRect();
    // Alineado con su botón; si no cabe hacia abajo (Ajustes, al fondo), se apoya en el borde de abajo
    // del botón. Un cálculo y no una medición: el panel todavía no existe.
    const estimate = 52 + m.items.length * (ROW_H + 4) + 16;
    const at: Placement = !rect || rect.top + estimate <= window.innerHeight - 8
      // Nunca arriba del borde: un botón medio recortado por el desplazamiento daba un `top` negativo.
      ? { top: Math.max(8, rect?.top ?? 8) }
      : { bottom: Math.max(8, window.innerHeight - rect.bottom) };
    setOpen({ id: m.id, path: pathname, at });
  };

  const opened = openId ? modules.find((m) => m.id === openId) : undefined;

  return (
    <>
      {modules.map((m, i) => {
        const Icon = m.icon;
        const active = current?.moduleId === m.id;
        const highlighted = openId ? openId === m.id : active;
        const badge = m.id === 'inventory' && pendingWaste > 0 ? <WasteBadge n={pendingWaste} /> : null;
        const content = (
          <>
            <Box position="relative"><Icon size={26} />{badge}</Box>
            <Text fontSize="12px" fontWeight="600" lineHeight="1.1" textAlign="center">{m.label}</Text>
          </>
        );
        const look = {
          w: '100%', minH: '72px', px: 1, gap: 1.5, borderRadius: RADIUS, position: 'relative' as const,
          flexDirection: 'column' as const, alignItems: 'center', justifyContent: 'center', color: 'white',
          bg: highlighted ? 'colorPalette.600' : 'transparent',
          _hover: { bg: highlighted ? 'colorPalette.600' : 'whiteAlpha.200' },
          _focusVisible: { outline: '2px solid', outlineColor: 'colorPalette.300', outlineOffset: '-2px' },
        };
        const divider = i > 0 && PRIMARY_MODULES.includes(modules[i - 1].id) && !PRIMARY_MODULES.includes(m.id)
          ? <Box data-rail-divider aria-hidden h="1px" bg="whiteAlpha.300" mx={2} my={1} flexShrink={0} />
          : null;
        return (
          <Box key={m.id} w="100%" style={{ scrollSnapAlign: 'start' }}>
            {divider}
            {m.items.length === 1 ? (
              <Flex asChild {...look} data-active={active}>
                <NavLink to={m.items[0].to} onClick={() => setOpen(null)}>{content}</NavLink>
              </Flex>
            ) : (
              <Flex
                asChild {...look} data-active={active}
                aria-expanded={openId === m.id} aria-controls={`rail-panel-${m.id}`}
                aria-current={active ? 'true' : undefined}
              >
                <button
                  type="button" onClick={() => toggle(m)}
                  ref={(el) => { if (el) buttons.current.set(m.id, el); else buttons.current.delete(m.id); }}
                >
                  {content}
                  <Box position="absolute" right="4px" top="50%" transform="translateY(-50%)" opacity={0.6} aria-hidden>
                    <LuChevronRight size={12} />
                  </Box>
                </button>
              </Flex>
            )}
          </Box>
        );
      })}
      {opened && open && !stale && (
        // En un portal: así no lo recorta la lista desplazable del riel, y el mapa de toques no lo
        // cuenta como parte de la pantalla de abajo (AppShell, `data-medible`).
        <Portal>
          <Box data-rail-veil position="fixed" inset={0} left={RAIL_WIDTH} bg="blackAlpha.400" zIndex="modal"
            onClick={() => close(false)} />
          <Box
            ref={panelRef} asChild colorPalette={palette}
            position="fixed" left={RAIL_WIDTH} zIndex="modal" w={`${PANEL_WIDTH}px`} maxH="calc(100dvh - 16px)"
            display="flex" flexDirection="column" bg="bg.panel" color="fg" borderWidth={BORDER_W}
            borderRadius={RADIUS} boxShadow="lg" p={2}
            style={'top' in open.at ? { top: open.at.top } : { bottom: open.at.bottom }}
          >
            <nav id={`rail-panel-${opened.id}`} aria-label={opened.label}>
              <Text fontSize="xs" fontWeight="700" color="fg.muted" textTransform="uppercase" letterSpacing="wide"
                px={3} pt={1} pb={1} flexShrink={0}>
                {opened.label}
              </Text>
              <VStack as="ul" listStyleType="none" m={0} p={0} gap={1} align="stretch" overflowY="auto" minH={0}>
                {opened.items.map((it) => (
                  <li key={it.to}><PanelLink to={it.to} label={it.label} icon={it.icon}
                    badge={it.to === WASTE_ITEM ? pendingWaste : 0} onPick={pick} /></li>
                ))}
              </VStack>
            </nav>
          </Box>
        </Portal>
      )}
    </>
  );
}

function PanelLink({ to, label, icon: Icon, badge, onPick }: {
  to: string; label: string; icon: MenuModule['icon']; badge: number; onPick?: () => void;
}) {
  return (
    <HStack
      asChild w="100%" minH={TAP_LG} px={3} gap={3} borderRadius={RADIUS} color="fg"
      _hover={{ bg: 'bg.muted' }}
      css={{ '&[aria-current=page]': { bg: 'colorPalette.600', color: 'white', fontWeight: '700' } }}
    >
      <NavLink to={to} onClick={onPick}>
        <Box position="relative"><Icon size={18} />{badge > 0 && <WasteBadge n={badge} />}</Box>
        <Text fontSize="sm" fontWeight="inherit">{label}</Text>
      </NavLink>
    </HStack>
  );
}

// En pantalla ancha sobra lugar: los módulos con sus destinos, todos a la vista.
function WideRail({ modules, pendingWaste }: { modules: MenuModule[]; pendingWaste: number }) {
  const { pathname } = useLocation();
  const current = moduleOf(pathname);
  const row = {
    w: '100%', px: 3, gap: 3, borderRadius: RADIUS, color: 'white',
    _hover: { bg: 'whiteAlpha.200' },
    css: { '&[aria-current=page]': { bg: 'colorPalette.600', fontWeight: '700' } },
  };
  return (
    <>
      {modules.map((m, i) => {
        const Icon = m.icon;
        const divider = i > 0 && PRIMARY_MODULES.includes(modules[i - 1].id) && !PRIMARY_MODULES.includes(m.id)
          ? <Box data-rail-divider aria-hidden h="1px" bg="whiteAlpha.300" mx={2} my={1} />
          : null;
        if (m.items.length === 1) {
          return (
            <Box key={m.id} w="100%">
              {divider}
              <HStack asChild minH="56px" {...row}>
                <NavLink to={m.items[0].to}><Icon size={22} /><Text fontWeight="700">{m.label}</Text></NavLink>
              </HStack>
            </Box>
          );
        }
        return (
          <Box key={m.id} w="100%" data-active={current?.moduleId === m.id}>
            {divider}
            <HStack minH="44px" px={3} gap={3} color="gray.300" aria-hidden>
              <Icon size={20} /><Text fontWeight="700" fontSize="sm">{m.label}</Text>
            </HStack>
            <VStack as="ul" aria-label={m.label} listStyleType="none" m={0} p={0} gap={0.5} align="stretch">
              {m.items.map((it) => {
                const I = it.icon;
                return (
                  <li key={it.to}>
                    <HStack asChild minH="44px" pl={10} {...row}>
                      <NavLink to={it.to}>
                        <Box position="relative"><I size={16} />
                          {it.to === WASTE_ITEM && pendingWaste > 0 && <WasteBadge n={pendingWaste} />}
                        </Box>
                        <Text fontSize="sm" fontWeight="inherit">{it.label}</Text>
                      </NavLink>
                    </HStack>
                  </li>
                );
              })}
            </VStack>
          </Box>
        );
      })}
    </>
  );
}
