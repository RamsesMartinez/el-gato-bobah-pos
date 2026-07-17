import { Box, HStack, Button } from '@chakra-ui/react';
import { LuFlame } from 'react-icons/lu';
import type { MenuCategory } from '../../types/pos';
import { categoryColor } from '../../utils/format';
import { RADIUS, BORDER_W, ACCENT_W } from '../../theme/ui';

interface Props {
  categories: MenuCategory[];
  selection: Selection;
  onSelect: (s: Selection) => void;
}

export type Selection =
  | { kind: 'top' }
  | { kind: 'root'; rootId: number; subId: number | null };

// Categorías: nivel principal, prominente por tamaño (el radio lo hereda del tema).
function chip(active: boolean, color?: string) {
  return {
    flexShrink: 0,
    minH: '52px',
    px: 5,
    size: 'lg' as const,
    fontSize: 'lg' as const,
    borderWidth: BORDER_W,
    scrollSnapAlign: 'start' as const,
    variant: active ? ('solid' as const) : ('outline' as const),
    bg: active ? color : undefined,
    borderColor: active ? color : undefined,
    color: active ? 'white' : undefined,
    _hover: active ? { opacity: 0.9 } : {},
  };
}

// Subcategorías: mismo lenguaje que las categorías pero un escalón menor (más chico),
// sobre superficie elevada dentro de la banda para que se lean como filtro anidado.
function subChip(active: boolean, color?: string) {
  return {
    flexShrink: 0,
    minH: '48px',
    px: 5,
    size: 'md' as const,
    fontSize: 'md' as const,
    fontWeight: '700' as const,
    borderWidth: BORDER_W,
    scrollSnapAlign: 'start' as const,
    variant: active ? ('solid' as const) : ('outline' as const),
    bg: active ? color : 'bg.panel',
    borderColor: active ? color : 'border.emphasized',
    color: active ? 'white' : 'fg',
    _hover: active ? { opacity: 0.9 } : { bg: 'bg.muted' },
  };
}

// scroll horizontal táctil nativo (momentum + snap), sin librería de drag.
const railScroll = {
  scrollbarWidth: 'none' as const,
  '&::-webkit-scrollbar': { display: 'none' },
  WebkitOverflowScrolling: 'touch' as const,
  touchAction: 'pan-x' as const,
  overscrollBehaviorX: 'contain' as const,
  scrollSnapType: 'x proximity' as const,
};

export function CategoryRail({ categories, selection, onSelect }: Props) {
  const roots = categories.filter((c) => c.parentId === null);
  const activeRoot = selection.kind === 'root' ? selection.rootId : null;
  const subs = categories.filter((c) => c.parentId === activeRoot);

  return (
    <Box>
      <HStack gap={2} overflowX="auto" py={2} css={railScroll}>
        <Button {...chip(selection.kind === 'top', 'colorPalette.500')} onClick={() => onSelect({ kind: 'top' })}>
          <LuFlame /> Top
        </Button>
        {roots.map((c) => (
          <Button
            key={c.id}
            {...chip(activeRoot === c.id, categoryColor(c.id, c.color))}
            onClick={() => onSelect({ kind: 'root', rootId: c.id, subId: null })}
          >
            {c.name}
          </Button>
        ))}
      </HStack>

      {selection.kind === 'root' && subs.length > 0 && (
        <Box mt={1} pl={3} borderLeftWidth={ACCENT_W} borderColor="colorPalette.400" bg="bg.muted" borderRadius={RADIUS}>
          <HStack gap={2} overflowX="auto" py={2} pr={2} css={railScroll}>
            <Button
              {...subChip(selection.subId === null, 'colorPalette.500')}
              onClick={() => onSelect({ kind: 'root', rootId: selection.rootId, subId: null })}
            >
              Todos
            </Button>
            {subs.map((c) => (
              <Button
                key={c.id}
                {...subChip(selection.subId === c.id, categoryColor(c.id, c.color))}
                onClick={() => onSelect({ kind: 'root', rootId: selection.rootId, subId: c.id })}
              >
                {c.name}
              </Button>
            ))}
          </HStack>
        </Box>
      )}
    </Box>
  );
}
