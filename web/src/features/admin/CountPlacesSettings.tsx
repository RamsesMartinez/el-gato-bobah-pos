import { useEffect, useRef, useState } from 'react';
import { useLocation, useNavigate } from 'react-router';
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
import { backofficeApi, type CountPlace, type Warehouse } from '../../api/backoffice';
import { Segmented } from '../../components/Segmented';
import { placeAnnouncements } from './placeAnnouncements';

// Los lugares del recorrido del conteo (spec 040, maqueta C): se ordenan manteniendo presionada la
// agarradera y arrastrando. Es la única forma de reordenar (decisión del dueño). La espera de 500 ms
// y el `touch-action: none` SOLO en la agarradera dejan desplazar la lista con el dedo sin mover
// nada; en el resto del renglón el navegador conserva el desplazamiento vertical.

const articles = (n: number) => (n === 0 ? 'Sin artículos' : n === 1 ? '1 artículo' : `${n} artículos`);

// placesKey: el de venta comparte la llave de siempre con la ficha del artículo y el conteo.
const placesKey = (warehouseId?: number) => (warehouseId === undefined ? ['count-places'] : ['count-places', warehouseId]);

export function CountPlacesSettings() {
  const qc = useQueryClient();
  // Con varios almacenes (spec 094) cada uno tiene sus lugares: la lista va por almacén y el alta
  // pregunta en cuál. El de venta va sin id. Con uno, todo queda como antes.
  const { data: whData } = useQuery({ queryKey: ['warehouses'], queryFn: () => backofficeApi.warehouses() });
  const whList = whData?.items ?? [];
  const multi = whList.length > 1;
  const [adding, setAdding] = useState(false);
  // Desde el conteo se llega con #lugares: la sección vive al fondo de Negocio.
  const box = useRef<HTMLDivElement>(null);
  const location = useLocation();
  const navigate = useNavigate();
  const fromCount = location.hash === '#lugares';
  const { data: salesPlaces = [] } = useQuery({ queryKey: placesKey(), queryFn: () => backofficeApi.countPlaces() });
  useEffect(() => {
    if (fromCount && salesPlaces.length > 0) box.current?.scrollIntoView?.({ block: 'start' });
  }, [fromCount, salesPlaces.length]);

  return (
    <Box ref={box} id="lugares" mt={6} borderWidth="1px" borderRadius="lg" p={5}>
      <Flex justify="space-between" align="center" gap={3}>
        <Text fontWeight="700">Lugares para contar</Text>
        <HStack gap={2}>
          {/* Quien llegó desde el conteo no sabía cómo volver: el menú lateral ya no lo lleva ahí. */}
          {fromCount && <Button minH="44px" variant="ghost" onClick={() => navigate('/almacen?tab=conteo')}>‹ Volver al conteo</Button>}
          <Button minH="44px" variant="outline" onClick={() => setAdding(true)}>Agregar lugar</Button>
        </HStack>
      </Flex>
      <Text fontSize="sm" color="fg.muted" mb={2}>Mantén presionada ≡ para mover un lugar</Text>
      {multi
        ? whList.map((w) => <PlaceList key={w.id} warehouseId={w.isSales ? undefined : w.id} title={w.name} />)
        : <PlaceList />}
      {adding && <AddSheet warehouses={multi ? whList : []} onClose={() => setAdding(false)}
        onDone={() => { setAdding(false); void qc.invalidateQueries({ queryKey: ['count-places'] }); }} />}
    </Box>
  );
}

// PlaceList son los lugares de un almacén, en el orden del recorrido. Con title es una sección con
// el nombre del almacén.
function PlaceList({ warehouseId, title }: { warehouseId?: number; title?: string }) {
  const qc = useQueryClient();
  const { data: places = [] } = useQuery({ queryKey: placesKey(warehouseId), queryFn: () => backofficeApi.countPlaces(warehouseId) });
  // Orden optimista mientras el servidor confirma: sin él, el renglón soltado regresa a su lugar
  // un instante y parece que no se movió.
  const [order, setOrder] = useState<number[] | null>(null);
  const [editing, setEditing] = useState<CountPlace | null>(null);
  const [error, setError] = useState('');
  const refresh = () => qc.invalidateQueries({ queryKey: placesKey(warehouseId) });

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
    <Box role={title ? 'region' : undefined} aria-label={title} mt={title ? 3 : 0}>
      {title && <Text fontWeight="700" bg="bg.subtle" px={3} py={2} borderRadius="md">{title}</Text>}
      {shown.length === 0 && <Text color="fg.muted" py={2}>Todavía no hay lugares.</Text>}
      <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onDragEnd} autoScroll
        accessibility={{
          announcements: placeAnnouncements((id) => shown.find((p) => p.id === Number(id))?.name ?? '',
            (id) => shown.findIndex((p) => p.id === Number(id)), shown.length),
          screenReaderInstructions: { draggable: 'Para mover un lugar, presiona espacio, muévelo con las flechas y vuelve a presionar espacio.' },
        }}>
        <SortableContext items={shown.map((p) => p.id)} strategy={verticalListSortingStrategy}>
          {shown.map((p, i) => <PlaceRow key={p.id} place={p} n={i + 1} onEdit={() => setEditing(p)} />)}
        </SortableContext>
      </DndContext>
      {error && <Text fontSize="sm" color="fg.error" mt={2}>{error}</Text>}
      {editing && <EditSheet place={editing} others={places.filter((p) => p.id !== editing.id)}
        onClose={() => setEditing(null)} onDone={() => { setEditing(null); void refresh(); }} />}
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
        w="52px" h="52px" display="flex" alignItems="center" justifyContent="center" color={isDragging ? 'fg.error' : 'fg.muted'}
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

// AddSheet pregunta en qué almacén solo si hay más de uno (warehouses no vacío); por omisión el de venta.
function AddSheet({ warehouses, onClose, onDone }: { warehouses: Warehouse[]; onClose: () => void; onDone: () => void }) {
  const [name, setName] = useState('');
  const [error, setError] = useState('');
  const [whId, setWhId] = useState(() => warehouses.find((w) => w.isSales)?.id);
  const target = warehouses.find((w) => w.id === whId && !w.isSales)?.id;
  const save = useMutation({
    mutationFn: () => backofficeApi.createCountPlace(name, target),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError && e.status === 409 ? 'Ese lugar ya existe' : e instanceof ApiError && e.status === 400 ? 'El lugar lleva de 1 a 30 letras' : 'No se pudo guardar'),
  });
  return (
    <DrawerRoot open onOpenChange={(e) => { if (!e.open) onClose(); }} placement="bottom">
      <DrawerBackdrop />
      <DrawerContent>
        <DrawerHeader><Text fontWeight="700">Agregar lugar</Text></DrawerHeader>
        <DrawerBody>
          {warehouses.length > 1 && (
            <HStack mb={3} gap={3} flexWrap="wrap">
              <Text fontWeight="600">¿En qué almacén?</Text>
              <Segmented label="¿En qué almacén?" value={String(whId ?? '')}
                options={warehouses.map((w) => ({ value: String(w.id), label: w.name }))} onChange={(v) => setWhId(Number(v))} />
            </HStack>
          )}
          <Input aria-label="Nombre del lugar" placeholder="Ej. Refri, Anaquel, Barra" minH="44px" maxLength={30} value={name} onChange={(e) => setName(e.target.value)} />
          {error && <Text fontSize="sm" color="fg.error" mt={2}>{error}</Text>}
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
                <Button key={String(o.id)} minH="52px" justifyContent="space-between" variant={dest === o.id ? 'solid' : 'outline'}
                  aria-pressed={dest === o.id} onClick={() => setDest(o.id)}>
                  {o.name}{dest === o.id && <LuCheck aria-hidden />}
                </Button>
              ))}
            </VStack>
          )}
          {error && <Text fontSize="sm" color="fg.error" mt={2}>{error}</Text>}
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
