import { useState } from 'react';
import { Box, HStack, VStack, Text, Button, IconButton, Center, Spinner } from '@chakra-ui/react';
import { LuStar, LuPencil, LuArchive, LuArchiveRestore, LuPlus, LuGripVertical } from 'react-icons/lu';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  DndContext, closestCenter, PointerSensor, KeyboardSensor, useSensor, useSensors,
  type DragEndEvent,
} from '@dnd-kit/core';
import { SortableContext, verticalListSortingStrategy, useSortable, arrayMove } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { adminApi, type GroupOption } from '../../api/admin';
import { money, normalize } from '../../utils/format';
import { toaster } from '../../components/ui/toaster';
import { ConfirmPopover } from '../../components/ui/confirm-popover';
import { OptionFormDialog } from './OptionFormDialog';
import type { ReactNode, CSSProperties, Ref } from 'react';

type FavMut = ReturnType<typeof useMutation<void, Error, GroupOption>>;
type ArchMut = ReturnType<typeof useMutation<void, Error, { o: GroupOption; active: boolean }>>;
const OPTS_KEY = (groupId: number) => ['admin', 'group-options', groupId] as const;

// Lista de opciones de un grupo (se monta al expandir → carga bajo demanda).
// Reordenable arrastrando el asa (⋮⋮); el orden se persiste en sort_key y se refleja en el POS.
// filter: si viene (búsqueda por nombre de opción), muestra solo las que coinciden y desactiva el
// arrastre (reordenar un subconjunto filtrado corrompería el orden real).
export function GroupOptions({ groupId, filter = '', hideInactive = false }: { groupId: number; filter?: string; hideInactive?: boolean }) {
  const qc = useQueryClient();
  const { data, isLoading } = useQuery({
    queryKey: OPTS_KEY(groupId),
    queryFn: () => adminApi.groupOptions(groupId),
  });
  const [edit, setEdit] = useState<GroupOption | null>(null);
  const [creating, setCreating] = useState(false);

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: OPTS_KEY(groupId) });
    qc.invalidateQueries({ queryKey: ['admin', 'groups'] });
    qc.invalidateQueries({ queryKey: ['admin', 'modifier-options'] });
    qc.invalidateQueries({ queryKey: ['menu'] });
  };
  const fav = useMutation({
    mutationFn: (o: GroupOption) => adminApi.setOptionFavorite(o.id, !o.favorite),
    onSuccess: invalidate,
    onError: (e) => toaster.create({ title: 'Error', description: String(e), type: 'error' }),
  });
  // set explícito (no toggle) para que «Deshacer» revierta al estado anterior.
  const arch = useMutation({
    mutationFn: ({ o, active }: { o: GroupOption; active: boolean }) => adminApi.setOptionActive(o.id, active),
    onSuccess: (_d, { o, active }) => {
      invalidate();
      toaster.create({
        title: active ? 'Opción reactivada' : 'Opción archivada',
        type: 'success',
        action: { label: 'Deshacer', onClick: () => arch.mutate({ o, active: !active }) },
      });
    },
    onError: (e) => toaster.create({ title: 'Error', description: String(e), type: 'error' }),
  });
  const reorder = useMutation({
    mutationFn: (ids: number[]) => adminApi.reorderOptions(groupId, ids),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['menu'] }); // el POS refleja el nuevo orden
      qc.invalidateQueries({ queryKey: ['admin', 'modifier-options'] });
    },
    onError: (e) => {
      qc.invalidateQueries({ queryKey: OPTS_KEY(groupId) }); // revierte al orden real
      toaster.create({ title: 'No se pudo reordenar', description: String(e), type: 'error' });
    },
  });

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }), // 6px evita drags accidentales al tocar
    useSensor(KeyboardSensor),
  );
  const onDragEnd = (e: DragEndEvent) => {
    const { active, over } = e;
    if (!over || active.id === over.id) return;
    const oldIndex = opts.findIndex((o) => o.id === active.id);
    const newIndex = opts.findIndex((o) => o.id === over.id);
    if (oldIndex < 0 || newIndex < 0) return;
    const next = arrayMove(opts, oldIndex, newIndex);
    qc.setQueryData(OPTS_KEY(groupId), { items: next }); // optimista
    reorder.mutate(next.map((o) => o.id));
  };

  if (isLoading) return <Center py={4}><Spinner /></Center>;
  const opts = data?.items ?? [];
  const q = normalize(filter);
  const searching = q !== '';
  const base = hideInactive ? opts.filter((o) => o.active) : opts;
  const shown = searching ? base.filter((o) => normalize(o.name).includes(q)) : base;
  const hiddenCount = opts.length - base.length; // archivadas ocultas por el toggle
  // arrastrar (reordenar) solo cuando se ven TODAS en su orden real: ni búsqueda ni ocultando inactivas
  const sortable = !searching && hiddenCount === 0;
  const plural = (n: number) => (n === 1 ? '' : 's');

  return (
    <VStack align="stretch" gap={1.5} pl={2}>
      {sortable ? (
        <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onDragEnd}>
          <SortableContext items={shown.map((o) => o.id)} strategy={verticalListSortingStrategy}>
            {shown.map((o) => <SortableOptionRow key={o.id} o={o} fav={fav} arch={arch} onEdit={setEdit} />)}
          </SortableContext>
        </DndContext>
      ) : (
        // vista sin arrastre (búsqueda u ocultando inactivas): reordenar un subconjunto corrompería el orden real
        shown.map((o) => <OptionRow key={o.id} o={o} fav={fav} arch={arch} onEdit={setEdit}
          style={{ opacity: o.active ? 1 : 0.55 }} />)
      )}
      {shown.length === 0 && (
        <Text fontSize="sm" color="fg.subtle" py={1}>
          {searching ? 'Sin coincidencias en este grupo.'
            : hiddenCount > 0 ? `${hiddenCount} archivada${plural(hiddenCount)} oculta${plural(hiddenCount)}.`
              : 'Sin opciones aún.'}
        </Text>
      )}
      {shown.length > 0 && hiddenCount > 0 && !searching && (
        <Text fontSize="xs" color="fg.subtle" py={0.5}>{hiddenCount} archivada{plural(hiddenCount)} oculta{plural(hiddenCount)}.</Text>
      )}
      {!searching && (
        <Button size="sm" variant="outline" alignSelf="start" onClick={() => setCreating(true)}>
          <LuPlus /> Opción
        </Button>
      )}

      <OptionFormDialog key={edit ? `e${edit.id}` : creating ? 'new' : 'closed'}
        groupId={groupId} option={edit} isOpen={edit !== null || creating}
        onClose={() => { setEdit(null); setCreating(false); }} onSaved={invalidate} />
    </VStack>
  );
}

// Fila visual de una opción; `handle` (asa de arrastre) es opcional según haya DnD o no.
function OptionRow({ o, fav, arch, onEdit, handle, innerRef, style }: {
  o: GroupOption;
  fav: FavMut;
  arch: ArchMut;
  onEdit: (o: GroupOption) => void;
  handle?: ReactNode;
  innerRef?: Ref<HTMLDivElement>;
  style?: CSSProperties;
}) {
  return (
    <HStack ref={innerRef} style={style} gap={2} p={2} borderWidth="1px" borderColor="border.muted"
      borderRadius="lg" bg="bg.subtle">
      {handle}
      <IconButton aria-label="Favorito" size="md" minW="40px"
        variant={o.favorite ? 'solid' : 'ghost'} colorPalette={o.favorite ? 'yellow' : 'gray'}
        loading={fav.isPending && fav.variables?.id === o.id} onClick={() => fav.mutate(o)}>
        <LuStar fill={o.favorite ? 'currentColor' : 'none'} />
      </IconButton>
      <Box flex="1" minW={0}>
        <Text fontWeight="500">{o.name}</Text>
        <Text fontSize="xs" color="fg.muted">
          {Number(o.priceDelta) === 0 ? 'sin costo extra' : money(o.priceDelta)} · máx {o.maxPerLine}/línea
        </Text>
      </Box>
      <IconButton aria-label="Editar opción" size="md" variant="ghost" onClick={() => onEdit(o)}><LuPencil /></IconButton>
      {o.active ? (
        <ConfirmPopover title="¿Archivar opción?"
          description="Se ocultará del POS. Puedes deshacerlo enseguida."
          onConfirm={() => arch.mutate({ o, active: false })}>
          <IconButton aria-label="Archivar opción" size="md" variant="ghost" colorPalette="gray">
            <LuArchive />
          </IconButton>
        </ConfirmPopover>
      ) : (
        <IconButton aria-label="Reactivar opción" size="md" variant="ghost" colorPalette="green"
          loading={arch.isPending && arch.variables?.o.id === o.id} onClick={() => arch.mutate({ o, active: true })}>
          <LuArchiveRestore />
        </IconButton>
      )}
    </HStack>
  );
}

function SortableOptionRow(props: { o: GroupOption; fav: FavMut; arch: ArchMut; onEdit: (o: GroupOption) => void }) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: props.o.id });
  const style: CSSProperties = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.6 : props.o.active ? 1 : 0.55,
    zIndex: isDragging ? 1 : undefined,
  };
  const handle = (
    <IconButton aria-label="Reordenar" size="md" variant="ghost" cursor="grab" touchAction="none"
      color="fg.muted" {...attributes} {...listeners}>
      <LuGripVertical />
    </IconButton>
  );
  return <OptionRow {...props} handle={handle} innerRef={setNodeRef} style={style} />;
}
