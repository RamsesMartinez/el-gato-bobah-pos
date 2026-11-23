import { useEffect, useRef, useState } from 'react';
import { useLocation, useNavigate } from 'react-router';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Badge, Box, Button, Flex, HStack, Input, Text, VStack } from '@chakra-ui/react';
import { LuChevronRight, LuShoppingCart, LuStore, LuWarehouse } from 'react-icons/lu';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
} from '../../components/ui/drawer';
import { ApiError } from '../../api/client';
import { backofficeApi, type Warehouse } from '../../api/backoffice';

// Los almacenes de la sucursal (spec 094, maqueta A3): «Donde se vende» y «Donde se guarda», cada uno
// con su ícono y lo que tiene. Tocar un renglón abre su hoja (renombrar, archivar); es el mismo patrón
// que «Lugares para contar», que vive al lado.

const SUGGESTIONS = ['Bodega', 'Cuarto frío', 'Sótano'];
const stockLabel = (w: Warehouse) => (w.itemsWithStock === 0 ? 'Vacío'
  : w.itemsWithStock === 1 ? '1 artículo con existencia' : `${w.itemsWithStock} artículos con existencia`);

function errorMessage(e: unknown) {
  if (e instanceof ApiError) {
    if (e.code === 'WAREHOUSE_NAME_TAKEN') return 'Ya hay un almacén con ese nombre';
    if (e.status === 400) return 'El nombre lleva de 1 a 40 letras';
    if (e.code === 'WAREHOUSE_HAS_STOCK') return 'Tiene existencias: traspásalas o cuéntalas en cero antes de archivarlo';
    if (e.code === 'WAREHOUSE_COUNT_OPEN') return 'Tiene un conteo abierto: ciérralo o descártalo primero';
    if (e.code === 'WAREHOUSE_PENDING_PURCHASES') return 'Tiene compras por recibir: recíbelas o márcalas como que no llegan primero';
    if (e.code === 'WAREHOUSE_IS_SALES') return 'De aquí salen las ventas: este almacén no se archiva';
    if (e.code === 'WAREHOUSE_NOT_FOUND') return 'Este almacén ya no existe o lo archivó alguien más';
  }
  return 'No se pudo guardar';
}

export function WarehousesSettings() {
  const qc = useQueryClient();
  const { data } = useQuery({ queryKey: ['warehouses'], queryFn: () => backofficeApi.warehouses() });
  const items = data?.items ?? [];
  const [adding, setAdding] = useState(false);
  const [editing, setEditing] = useState<Warehouse | null>(null);
  // Los nombres van aparte (quien cuenta o registra merma no lee la lista completa): se refrescan juntos.
  const refresh = () => Promise.all([
    qc.invalidateQueries({ queryKey: ['warehouses'] }), qc.invalidateQueries({ queryKey: ['warehouse-names'] }),
  ]);
  const sales = items.filter((w) => w.isSales);
  const storage = items.filter((w) => !w.isSales);
  // Desde Almacén se llega con #almacenes: la sección vive dentro de Negocio y hay que ir a ella.
  const box = useRef<HTMLDivElement>(null);
  const location = useLocation();
  const navigate = useNavigate();
  const fromStock = location.hash === '#almacenes';
  useEffect(() => {
    if (fromStock && items.length > 0) box.current?.scrollIntoView?.({ block: 'start' });
  }, [fromStock, items.length]);

  return (
    <Box ref={box} id="almacenes" mt={6} borderWidth="1px" borderRadius="lg" p={5}>
      <Flex justify="space-between" align="center" gap={3} mb={1}>
        <Text fontWeight="700">Almacenes</Text>
        <HStack gap={2}>
          {fromStock && <Button minH="44px" variant="ghost" onClick={() => navigate('/almacen')}>‹ Volver al almacén</Button>}
          <Button minH="44px" variant="outline" onClick={() => setAdding(true)}>Agregar almacén</Button>
        </HStack>
      </Flex>
      <Text fontSize="sm" color="fg.muted" mb={3}>Dónde guardas mercancía: tu local y, si tienes, una bodega aparte.</Text>
      <StorageGroup title="Donde se vende" items={sales} emptyText="" onOpen={setEditing} />
      <StorageGroup title="Donde se guarda" items={storage} emptyText="Todavía no hay otro almacén." onOpen={setEditing} />
      {adding && <AddSheet onClose={() => setAdding(false)} onDone={() => { setAdding(false); void refresh(); }} />}
      {editing && <EditSheet w={editing} onClose={() => setEditing(null)} onDone={() => { setEditing(null); void refresh(); }} />}
    </Box>
  );
}

function StorageGroup({ title, items, emptyText, onOpen }: { title: string; items: Warehouse[]; emptyText: string; onOpen: (w: Warehouse) => void }) {
  if (items.length === 0 && !emptyText) return null;
  return (
    <VStack align="stretch" gap={2} mb={3} role="group" aria-label={title}>
      <Text fontSize="sm" fontWeight="600" color="fg.muted">{title}</Text>
      {items.length === 0 && <Text fontSize="sm" color="fg.muted">{emptyText}</Text>}
      {items.map((w) => (
        <Button key={w.id} variant="outline" minH="72px" h="auto" py={3} justifyContent="space-between" bg="bg.panel"
          whiteSpace="normal" textAlign="start" fontWeight="400" onClick={() => onOpen(w)}>
          <HStack gap={4} minW={0}>
            <Box boxSize="48px" borderRadius="lg" display="flex" alignItems="center" justifyContent="center" flexShrink={0}
              bg="colorPalette.subtle" color="colorPalette.fg" fontSize="24px" aria-hidden>
              {w.isSales ? <LuStore /> : <LuWarehouse />}
            </Box>
            <VStack align="start" gap={0} minW={0}>
              <HStack gap={2}>
                <Text fontWeight="700" fontSize="lg" truncate>{w.name}</Text>
                {w.isSales && <Badge variant="subtle"><LuShoppingCart aria-hidden /> De venta</Badge>}
              </HStack>
              <Text fontSize="sm" color="fg.muted">{stockLabel(w)}</Text>
            </VStack>
          </HStack>
          <LuChevronRight aria-hidden />
        </Button>
      ))}
    </VStack>
  );
}

function AddSheet({ onClose, onDone }: { onClose: () => void; onDone: () => void }) {
  const [name, setName] = useState('');
  const [error, setError] = useState('');
  const save = useMutation({ mutationFn: () => backofficeApi.createWarehouse(name.trim()), onSuccess: onDone, onError: (e) => setError(errorMessage(e)) });
  return (
    <DrawerRoot open onOpenChange={(e) => { if (!e.open) onClose(); }} placement="bottom">
      <DrawerBackdrop />
      <DrawerContent>
        <DrawerHeader><Text fontWeight="700">Agregar almacén</Text></DrawerHeader>
        <DrawerBody>
          <VStack align="stretch" gap={3}>
            <Input aria-label="Nombre del almacén" placeholder="Nombre del almacén" minH="44px" maxLength={40}
              value={name} onChange={(e) => setName(e.target.value)} />
            <HStack gap={2} flexWrap="wrap">
              <Text fontSize="sm" color="fg.muted">Sugerencias:</Text>
              {SUGGESTIONS.map((s) => <Button key={s} size="sm" minH="44px" variant="outline" onClick={() => setName(s)}>{s}</Button>)}
            </HStack>
            <Text fontSize="sm" color="fg.muted">Nace vacío. Lo que llegue ahí se registra en la compra o con un traspaso.</Text>
            {error && <Text fontSize="sm" color="fg.error">{error}</Text>}
          </VStack>
        </DrawerBody>
        <DrawerFooter>
          <Button minH="44px" disabled={!name.trim()} loading={save.isPending} onClick={() => save.mutate()}>Agregar</Button>
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}

function EditSheet({ w, onClose, onDone }: { w: Warehouse; onClose: () => void; onDone: () => void }) {
  const [name, setName] = useState(w.name);
  const [error, setError] = useState('');
  const [archiving, setArchiving] = useState(false);
  const [makingSales, setMakingSales] = useState(false);
  const makeSales = useMutation({ mutationFn: () => backofficeApi.setSalesWarehouse(w.id), onSuccess: onDone, onError: (e) => setError(errorMessage(e)) });
  const rename = useMutation({ mutationFn: () => backofficeApi.renameWarehouse(w.id, name.trim()), onSuccess: onDone, onError: (e) => setError(errorMessage(e)) });
  const archive = useMutation({ mutationFn: () => backofficeApi.archiveWarehouse(w.id), onSuccess: onDone, onError: (e) => setError(errorMessage(e)) });
  const canArchive = !w.isSales && w.itemsWithStock === 0;
  return (
    <DrawerRoot open onOpenChange={(e) => { if (!e.open) onClose(); }} placement="bottom">
      <DrawerBackdrop />
      <DrawerContent>
        <DrawerHeader><Text fontWeight="700">{archiving ? 'Archivar almacén' : w.name}</Text></DrawerHeader>
        <DrawerBody>
          {!archiving && (
            <VStack align="stretch" gap={3}>
              <Input aria-label="Nombre del almacén" minH="44px" maxLength={40} value={name} onChange={(e) => setName(e.target.value)} />
              {w.isSales && <Text fontSize="sm" color="fg.muted">De aquí salen las ventas. Este almacén no se archiva.</Text>}
              {!w.isSales && w.itemsWithStock > 0 && (
                <Text fontSize="sm" color="fg.muted">{stockLabel(w)}. Para archivarlo, traspasa o cuenta en cero lo que tiene.</Text>
              )}
            </VStack>
          )}
          {archiving && <Text>Se archiva «{w.name}». Ya no aparece en las listas.</Text>}
          {makingSales && <Text>Desde ahora las ventas descontarán de «{w.name}». Lo que ya se vendió no cambia.</Text>}
          {!archiving && !makingSales && !w.isSales && (
            <Button mt={4} minH="44px" variant="outline" onClick={() => { setError(''); setMakingSales(true); }}>Vender desde aquí</Button>
          )}
          {error && <Text fontSize="sm" color="fg.error" mt={2}>{error}</Text>}
        </DrawerBody>
        <DrawerFooter>
          {makingSales && (
            <HStack w="100%" gap={3}>
              <Button minH="44px" loading={makeSales.isPending} onClick={() => makeSales.mutate()}>Sí, vender desde {w.name}</Button>
              <Button minH="44px" variant="ghost" onClick={() => setMakingSales(false)}>No</Button>
            </HStack>
          )}
          {!archiving && !makingSales && (
            <HStack w="100%" justify="space-between" gap={12}>
              <Button minH="44px" disabled={!name.trim() || name.trim() === w.name} loading={rename.isPending}
                onClick={() => rename.mutate()}>Guardar</Button>
              {canArchive && <Button minH="44px" variant="ghost" colorPalette="red" onClick={() => { setError(''); setArchiving(true); }}>Archivar</Button>}
            </HStack>
          )}
          {archiving && (
            <HStack w="100%" gap={3}>
              <Button minH="44px" colorPalette="red" loading={archive.isPending} onClick={() => archive.mutate()}>Sí, archivar</Button>
              <Button minH="44px" variant="ghost" onClick={() => setArchiving(false)}>No</Button>
            </HStack>
          )}
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}
