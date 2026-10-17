import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Box, Button, Flex, HStack, Input, Text, VStack } from '@chakra-ui/react';
import { LuCheck, LuMenu } from 'react-icons/lu';
import {
  DndContext, KeyboardSensor, PointerSensor, closestCenter, useSensor, useSensors, type DragEndEvent,
} from '@dnd-kit/core';
import {
  SortableContext, arrayMove, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
} from '../../components/ui/drawer';
import { ApiError } from '../../api/client';
import { backofficeApi, type CountPlace } from '../../api/backoffice';

// Los lugares del recorrido del conteo (spec 040, maqueta C): se ordenan manteniendo presionada la
// agarradera y arrastrando. Es la única forma de reordenar (decisión del dueño). La espera de 500 ms
// y el `touch-action: none` SOLO en la agarradera dejan desplazar la lista con el dedo sin mover
// nada; en el resto del renglón el navegador conserva el desplazamiento vertical.

const articles = (n: number) => (n === 0 ? 'Sin artículos' : n === 1 ? '1 artículo' : `${n} artículos`);

export function CountPlacesSettings() {
  const qc = useQueryClient();
  const { data: places = [] } = useQuery({ queryKey: ['count-places'], queryFn: backofficeApi.countPlaces });
  // Orden optimista mientras el servidor confirma: sin él, el renglón soltado regresa a su lugar
  // un instante y parece que no se movió.
  const [order, setOrder] = useState<number[] | null>(null);
  const [editing, setEditing] = useState<CountPlace | null>(null);
  const [adding, setAdding] = useState(false);
  const [error, setError] = useState('');
  const refresh = () => qc.invalidateQueries({ queryKey: ['count-places'] });

  const shown = order ? order.map((id) => places.find((p) => p.id === id)).filter((p): p is CountPlace => !!p) : places;
  const move = useMutation({
    mutationFn: ({ id, to }: { id: number; to: number }) => backofficeApi.moveCountPlace(id, to),
    onSettled: async () => { await refresh(); setOrder(null); },
    onError: () => setError('No se pudo mover el lugar'),
  });
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { delay: 500, tolerance: 8 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );
  const onDragEnd = (e: DragEndEvent) => {
    const { active, over } = e;
    if (!over || active.id === over.id) return;
    const ids = shown.map((p) => p.id);
    const to = ids.indexOf(Number(over.id));
    setOrder(arrayMove(ids, ids.indexOf(Number(active.id)), to));
    setError('');
    move.mutate({ id: Number(active.id), to });
  };

  return (
    <Box mt={6} borderWidth="1px" borderRadius="lg" p={5}>
      <Flex justify="space-between" align="center" gap={3}>
        <Text fontWeight="700">Lugares</Text>
        <Button minH="44px" variant="outline" onClick={() => setAdding(true)}>Agregar lugar</Button>
      </Flex>
      <Text fontSize="sm" color="fg.muted" mb={2}>Mantén presionada ≡ para mover un lugar</Text>
      {shown.length === 0 && <Text color="fg.muted" py={2}>Todavía no hay lugares.</Text>}
      <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onDragEnd} autoScroll>
        <SortableContext items={shown.map((p) => p.id)} strategy={verticalListSortingStrategy}>
          {shown.map((p, i) => <PlaceRow key={p.id} place={p} n={i + 1} onEdit={() => setEditing(p)} />)}
        </SortableContext>
      </DndContext>
      {error && <Text fontSize="sm" color="red.600" mt={2}>{error}</Text>}
      {editing && <EditSheet place={editing} others={places.filter((p) => p.id !== editing.id)}
        onClose={() => setEditing(null)} onDone={() => { setEditing(null); void refresh(); }} />}
      {adding && <AddSheet onClose={() => setAdding(false)} onDone={() => { setAdding(false); void refresh(); }} />}
    </Box>
  );
}

function PlaceRow({ place, n, onEdit }: { place: CountPlace; n: number; onEdit: () => void }) {
  const { attributes, listeners, setNodeRef, setActivatorNodeRef, transform, transition, isDragging } = useSortable({ id: place.id });
  return (
    <Flex ref={setNodeRef} data-place={place.id} align="center" gap={3} minH="56px" borderBottomWidth="1px" bg="bg"
      style={{ transform: CSS.Transform.toString(transform), transition, touchAction: 'pan-y' }}
      boxShadow={isDragging ? 'lg' : undefined} position="relative" zIndex={isDragging ? 1 : undefined}>
      <Box as="button" ref={setActivatorNodeRef} {...attributes} {...listeners} aria-label={`Mover ${place.name}`}
        w="52px" h="52px" display="flex" alignItems="center" justifyContent="center" color={isDragging ? 'red.fg' : 'fg.muted'}
        borderRadius="lg" cursor="grab"
        style={{ touchAction: 'none', userSelect: 'none', WebkitTouchCallout: 'none' }}>
        {/* ≡, el mismo signo que dice la instrucción de arriba (T022: seis puntos no se reconocían). */}
        <LuMenu size={22} />
      </Box>
      <Text w="24px" color="fg.muted" textAlign="right" flexShrink={0}>{n}</Text>
      {/* Solo el nombre se recorta: cuántos artículos tiene decide si archivarlo duele. */}
      <Text flex="1" minW={0} truncate>{place.name}</Text>
      <Text color="fg.muted" flexShrink={0}>{articles(place.ingredients)}</Text>
      <Button minH="44px" variant="outline" onClick={onEdit}>Editar</Button>
    </Flex>
  );
}

function AddSheet({ onClose, onDone }: { onClose: () => void; onDone: () => void }) {
  const [name, setName] = useState('');
  const [error, setError] = useState('');
  const save = useMutation({
    mutationFn: () => backofficeApi.createCountPlace(name),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError && e.status === 409 ? 'Ese lugar ya existe' : e instanceof ApiError && e.status === 400 ? 'El lugar lleva de 1 a 30 letras' : 'No se pudo guardar'),
  });
  return (
    <DrawerRoot open onOpenChange={(e) => { if (!e.open) onClose(); }} placement="bottom">
      <DrawerBackdrop />
      <DrawerContent>
        <DrawerHeader><Text fontWeight="700">Agregar lugar</Text></DrawerHeader>
        <DrawerBody>
          <Input aria-label="Nombre del lugar" minH="44px" maxLength={30} value={name} onChange={(e) => setName(e.target.value)} />
          {error && <Text fontSize="sm" color="red.600" mt={2}>{error}</Text>}
        </DrawerBody>
        <DrawerFooter>
          <Button minH="44px" disabled={!name.trim()} loading={save.isPending} onClick={() => save.mutate()}>Agregar</Button>
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}

function EditSheet({ place, others, onClose, onDone }: {
  place: CountPlace; others: CountPlace[]; onClose: () => void; onDone: () => void;
}) {
  const [name, setName] = useState(place.name);
  const [error, setError] = useState('');
  const [archiving, setArchiving] = useState(false);
  // undefined = todavía no elige; null = «Sin lugar».
  const [dest, setDest] = useState<number | null | undefined>(undefined);
  const fail = (e: unknown) => setError(e instanceof ApiError && e.status === 409 ? 'Ese lugar ya existe' : e instanceof ApiError && e.status === 400 ? 'El lugar lleva de 1 a 30 letras' : 'No se pudo guardar');
  const rename = useMutation({ mutationFn: () => backofficeApi.renameCountPlace(place.id, name), onSuccess: onDone, onError: fail });
  const archive = useMutation({
    mutationFn: () => backofficeApi.archiveCountPlace(place.id, place.ingredients > 0 ? dest : undefined),
    onSuccess: onDone, onError: fail,
  });

  return (
    <DrawerRoot open onOpenChange={(e) => { if (!e.open) onClose(); }} placement="bottom">
      <DrawerBackdrop />
      <DrawerContent>
        <DrawerHeader><Text fontWeight="700">{archiving ? 'Archivar lugar' : 'Editar lugar'}</Text></DrawerHeader>
        <DrawerBody>
          {!archiving && (
            <Input aria-label="Nombre del lugar" minH="44px" maxLength={30} value={name} onChange={(e) => setName(e.target.value)} />
          )}
          {archiving && place.ingredients === 0 && <Text>Se archiva «{place.name}». Ya no aparece en las listas.</Text>}
          {archiving && place.ingredients > 0 && (
            <VStack align="stretch" gap={0}>
              <Text mb={2}>{place.name} tiene {articles(place.ingredients)}. ¿A qué lugar pasan?</Text>
              {[...others.map((o) => ({ id: o.id as number | null, name: o.name })), { id: null, name: 'Sin lugar' }].map((o) => (
                <Button key={String(o.id)} minH="52px" justifyContent="space-between" variant={dest === o.id ? 'solid' : 'ghost'}
                  aria-pressed={dest === o.id} onClick={() => setDest(o.id)}>
                  {o.name}{dest === o.id && <LuCheck aria-hidden />}
                </Button>
              ))}
            </VStack>
          )}
          {error && <Text fontSize="sm" color="red.600" mt={2}>{error}</Text>}
        </DrawerBody>
        <DrawerFooter>
          {!archiving && (
            <HStack w="100%" justify="space-between" gap={12}>
              <Button minH="44px" disabled={!name.trim() || name.trim() === place.name} loading={rename.isPending}
                onClick={() => rename.mutate()}>Guardar</Button>
              <Button minH="44px" variant="ghost" colorPalette="red" onClick={() => { setError(''); setArchiving(true); }}>Archivar</Button>
            </HStack>
          )}
          {archiving && (
            <HStack w="100%" gap={3}>
              <Button minH="44px" colorPalette="red" loading={archive.isPending}
                disabled={place.ingredients > 0 && dest === undefined} onClick={() => archive.mutate()}>
                {place.ingredients > 0 ? 'Pasar y archivar' : 'Sí, archivar'}
              </Button>
              <Button minH="44px" variant="ghost" onClick={() => setArchiving(false)}>No</Button>
            </HStack>
          )}
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}
