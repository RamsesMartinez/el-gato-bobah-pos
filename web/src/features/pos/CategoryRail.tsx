import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { Box, HStack, Button, IconButton } from '@chakra-ui/react';
import { LuChevronLeft, LuChevronRight, LuFlame } from 'react-icons/lu';
import {
  DndContext, PointerSensor, KeyboardSensor, useSensor, useSensors, closestCenter,
  type DragEndEvent,
} from '@dnd-kit/core';
import { SortableContext, horizontalListSortingStrategy, useSortable, arrayMove } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import type { MenuCategory } from '../../types/pos';
import { categoryColor } from '../../utils/format';
import { RADIUS, BORDER_W, ACCENT_W } from '../../theme/ui';
import { useCatOrder } from '../../hooks/useCatOrder';

interface Props {
  categories: MenuCategory[];
  selection: Selection;
  onSelect: (s: Selection) => void;
}

export type Selection =
  | { kind: 'top' }
  | { kind: 'root'; rootId: number; subId: number | null; popular: boolean };

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

// aplica el orden guardado por el usuario; los ids no guardados quedan después en su orden original.
function applyOrder<T extends { id: number }>(items: T[], order?: number[]): T[] {
  if (!order || order.length === 0) return items;
  const pos = new Map(order.map((id, i) => [id, i]));
  return items
    .map((it, i) => ({ it, key: pos.get(it.id) ?? order.length + i }))
    .sort((a, b) => a.key - b.key)
    .map((x) => x.it);
}

export function CategoryRail({ categories, selection, onSelect }: Props) {
  const { order, setRootOrder, setSubOrder } = useCatOrder();

  const roots = useMemo(
    () => applyOrder(categories.filter((c) => c.parentId === null), order?.roots),
    [categories, order],
  );
  const activeRoot = selection.kind === 'root' ? selection.rootId : null;
  const subs = useMemo(
    () => applyOrder(categories.filter((c) => c.parentId === activeRoot), activeRoot != null ? order?.subs[activeRoot] : undefined),
    [categories, activeRoot, order],
  );

  // long-press (delay) para arrastrar; un swipe rápido (tolerancia) cancela y deja scrollear.
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { delay: 300, tolerance: 8 } }),
    useSensor(KeyboardSensor),
  );

  const onRootDragEnd = (e: DragEndEvent) => {
    const { active, over } = e;
    if (!over || active.id === over.id) return;
    const ids = roots.map((c) => c.id);
    setRootOrder(arrayMove(ids, ids.indexOf(Number(active.id)), ids.indexOf(Number(over.id))));
  };
  const onSubDragEnd = (e: DragEndEvent) => {
    const { active, over } = e;
    if (activeRoot == null || !over || active.id === over.id) return;
    const ids = subs.map((c) => c.id);
    setSubOrder(activeRoot, arrayMove(ids, ids.indexOf(Number(active.id)), ids.indexOf(Number(over.id))));
  };

  const cur = selection.kind === 'root' ? selection : null;

  // QUE SE NOTE QUE HAY MÁS. Con el ticket abierto, Desayunos, Ramen y Snacks quedaban fuera del
  // riel sin ninguna señal: un carrusel sin indicador es una categoría que nadie encuentra. En cada
  // orilla con algo escondido va un desvanecido con una flecha de 44 px que avanza.
  const riel = useRef<HTMLDivElement>(null);
  const [orillas, setOrillas] = useState({ antes: false, despues: false });
  const medir = useCallback(() => {
    const el = riel.current;
    if (!el) return;
    const antes = el.scrollLeft > 4;
    const despues = el.scrollLeft + el.clientWidth < el.scrollWidth - 4;
    setOrillas((o) => (o.antes === antes && o.despues === despues ? o : { antes, despues }));
  }, []);
  useEffect(() => {
    const el = riel.current;
    if (!el) return;
    const id = requestAnimationFrame(medir);
    const ro = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(medir);
    ro?.observe(el);
    return () => { cancelAnimationFrame(id); ro?.disconnect(); };
  }, [medir, roots.length]);
  const avanzar = (sentido: 1 | -1) => {
    const el = riel.current;
    if (el) el.scrollBy({ left: sentido * Math.round(el.clientWidth * 0.8), behavior: 'smooth' });
  };

  return (
    <Box>
      <Box position="relative">
      <HStack ref={riel} data-riel="categorias" data-testid="riel-categorias" onScroll={medir}
        gap={2} overflowX="auto" py={1.5} css={railScroll}>
        {/* Top global (más vendidos de todo) — fijo, no se reordena */}
        <Button {...chip(selection.kind === 'top', 'colorPalette.500')} onClick={() => onSelect({ kind: 'top' })}>
          <LuFlame /> Top
        </Button>
        <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onRootDragEnd}>
          <SortableContext items={roots.map((c) => c.id)} strategy={horizontalListSortingStrategy}>
            {roots.map((c) => (
              <SortableChip key={c.id} id={c.id} chipProps={chip(activeRoot === c.id, categoryColor(c.id, c.color))}
                onClick={() => onSelect({ kind: 'root', rootId: c.id, subId: null, popular: false })}>
                {c.name}
              </SortableChip>
            ))}
          </SortableContext>
        </DndContext>
      </HStack>
      {orillas.antes && (
        <Orilla lado="izquierda">
          <IconButton aria-label="Categorías anteriores" size="lg" minW="44px" minH="44px" variant="outline"
            colorPalette="gray" bg="bg.panel" borderRadius="full" onClick={() => avanzar(-1)}>
            <LuChevronLeft />
          </IconButton>
        </Orilla>
      )}
      {orillas.despues && (
        <Orilla lado="derecha">
          <IconButton aria-label="Más categorías" size="lg" minW="44px" minH="44px" variant="outline"
            colorPalette="gray" bg="bg.panel" borderRadius="full" onClick={() => avanzar(1)}>
            <LuChevronRight />
          </IconButton>
        </Orilla>
      )}
      </Box>

      {cur && (
        <Box mt={0.5} pl={3} borderLeftWidth={ACCENT_W} borderColor="colorPalette.400" bg="bg.muted" borderRadius={RADIUS}>
          <HStack gap={2} overflowX="auto" py={1} pr={2} css={railScroll}>
            {/* Todos + Populares: fijos (no se reordenan). Populares es un toggle que se combina con el scope. */}
            <Button {...subChip(cur.subId === null, 'colorPalette.500')}
              onClick={() => onSelect({ kind: 'root', rootId: cur.rootId, subId: null, popular: cur.popular })}>
              Todos
            </Button>
            <Button {...subChip(cur.popular, 'orange.500')}
              onClick={() => onSelect({ kind: 'root', rootId: cur.rootId, subId: cur.subId, popular: !cur.popular })}>
              <LuFlame /> Populares
            </Button>
            <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onSubDragEnd}>
              <SortableContext items={subs.map((c) => c.id)} strategy={horizontalListSortingStrategy}>
                {subs.map((c) => (
                  <SortableChip key={c.id} id={c.id} chipProps={subChip(cur.subId === c.id, categoryColor(c.id, c.color))}
                    onClick={() => onSelect({ kind: 'root', rootId: cur.rootId, subId: c.id, popular: cur.popular })}>
                    {c.name}
                  </SortableChip>
                ))}
              </SortableContext>
            </DndContext>
          </HStack>
        </Box>
      )}
    </Box>
  );
}

// El desvanecido de una orilla del riel: deja ver que la fila sigue y sostiene la flecha.
function Orilla({ lado, children }: { lado: 'izquierda' | 'derecha'; children: ReactNode }) {
  const derecha = lado === 'derecha';
  return (
    <Box position="absolute" top={0} bottom={0} {...(derecha ? { right: 0, pl: 8 } : { left: 0, pr: 8 })}
      display="flex" alignItems="center" pointerEvents="none"
      bgImage={`linear-gradient(to ${derecha ? 'right' : 'left'}, transparent, var(--chakra-colors-bg-subtle) 45%)`}>
      <Box pointerEvents="auto">{children}</Box>
    </Box>
  );
}

// Chip arrastrable con long-press. Un tap normal selecciona; tras un arrastre real se suprime el click.
function SortableChip({ id, chipProps, onClick, children }: {
  id: number;
  chipProps: Record<string, unknown>;
  onClick: () => void;
  children: ReactNode;
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id });
  const dragged = useRef(false);
  useEffect(() => {
    if (isDragging) dragged.current = true;
    else if (dragged.current) { const t = setTimeout(() => { dragged.current = false; }, 60); return () => clearTimeout(t); }
  }, [isDragging]);

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.6 : 1,
    zIndex: isDragging ? 2 : undefined,
  };
  return (
    <Button ref={setNodeRef} style={style} {...attributes} {...listeners} {...chipProps}
      onClick={() => { if (dragged.current) { dragged.current = false; return; } onClick(); }}>
      {children}
    </Button>
  );
}
