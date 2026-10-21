import { useState } from 'react';
import { medirAccion } from '../../../api/uso';
import { Box, Button, HStack, Input, SimpleGrid, Stack, Text } from '@chakra-ui/react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader,
} from '../../../components/ui/drawer';
import { backofficeApi } from '../../../api/backoffice';
import { ApiError } from '../../../api/client';
import { Picker } from '../../../components/Picker';
import { toaster } from '../../../components/ui/toaster';
import { QtyWithUnit } from './qty';
import { useLocations } from '../useLocations';
import { baseQty, KINDS, bigUnitOf, type Kind } from './articleUnits';

// «Nuevo artículo» (spec 042, FR-002, decisión del dueño): una hoja corta con lo obligatorio marcado.
// Nada se guarda hasta tocar «Guardar»; al guardar se abre la ficha con «Agregar empaque» a la vista.
// Categoría y empaques se capturan ahí.

interface Props {
  word?: string;
  onClose: () => void;
  onCreated: (id: number) => void;
}

export function NewArticleSheet({ word = 'artículo', onClose, onCreated }: Props) {
  const qc = useQueryClient();
  // Se piden al abrir para que al guardar ya estén casi siempre.
  useQuery({ queryKey: ['units'], queryFn: backofficeApi.units });
  const locations = useLocations();
  const [name, setName] = useState('');
  const [kind, setKind] = useState<Kind | null>(null);
  const [stock, setStock] = useState({ qty: '', big: true });
  const [min, setMin] = useState({ qty: '', big: true });
  const [cost, setCost] = useState('');
  const [placeId, setPlaceId] = useState('');
  const [taken, setTaken] = useState<{ id: number; archived: boolean } | null>(null);

  const big = kind ? bigUnitOf(kind) : null;
  const save = useMutation({
    // Las unidades pueden no haber llegado al tocar «Guardar»: se esperan aquí en vez de dejar el
    // botón apagado sin decir por qué.
    mutationFn: async () => {
      const all = await qc.ensureQueryData({ queryKey: ['units'], queryFn: backofficeApi.units });
      const unit = all.items.find((u) => u.kind === kind && Number(u.toBase) === 1);
      if (!unit) throw new Error('unit');
      return backofficeApi.createArticle({
        name, baseUnitId: unit.id,
        placeId: placeId ? Number(placeId) : null,
        initialStock: baseQty(stock.qty, kind!, stock.big),
        minStock: baseQty(min.qty, kind!, min.big),
        // El costo se captura por la unidad grande («$ por kg») y viaja por la base.
        initialCost: cost.trim() === '' ? undefined : String(Number(cost) / (big?.factor ?? 1)),
      });
    },
    onSuccess: async ({ id }) => {
      medirAccion('inventario', 'crear-articulo');
      await qc.invalidateQueries({ queryKey: ['articles'] });
      toaster.create({ title: `«${name.trim()}» quedó creado`, type: 'success' });
      onCreated(id);
    },
    onError: (e) => {
      if (e instanceof ApiError && e.code === 'NAME_TAKEN' && e.details?.existingId) {
        setTaken({ id: e.details.existingId, archived: !!e.details.archived });
        return;
      }
      toaster.create({ title: e instanceof ApiError && e.status === 400 ? e.message : 'No se pudo crear el artículo', type: 'error' });
    },
  });
  const reactivate = useMutation({
    mutationFn: (id: number) => backofficeApi.reactivateArticle(id),
    onSuccess: async (_, id) => {
      await qc.invalidateQueries({ queryKey: ['articles'] });
      toaster.create({ title: 'Volvió a estar activo', type: 'success' });
      onCreated(id);
    },
  });
  const canSave = name.trim() !== '' && kind !== null && !save.isPending;

  return (
    <DrawerRoot open placement="bottom" onOpenChange={(e) => { if (!e.open) onClose(); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="l3" maxH="92dvh" display="flex" flexDirection="column">
        <DrawerHeader pb={2}>
          <HStack justify="space-between">
            <Text fontSize="lg" fontWeight="700">Nuevo {word}</Text>
            <HStack gap={2}>
              <Button minH="44px" variant="outline" onClick={onClose}>Cancelar</Button>
              <Button minH="44px" colorPalette="brand" disabled={!canSave} loading={save.isPending} onClick={() => save.mutate()}>Guardar</Button>
            </HStack>
          </HStack>
        </DrawerHeader>
        <DrawerBody flex="1" minH={0} overflowY="auto" pb={4}>
          <Stack gap={3}>
            <SimpleGrid columns={2} gap={3}>
              <Stack gap={1}>
                <Text asChild><label htmlFor="new-article-name">Nombre <Text as="span" color="red.600">*</Text></label></Text>
                <Input id="new-article-name" minH="48px" maxLength={60} placeholder="Por ejemplo: Leche deslactosada"
                  value={name} onChange={(e) => { setName(e.target.value); setTaken(null); }} />
              </Stack>
              <Stack gap={1}>
                <Text>En qué se mide <Text as="span" color="red.600">*</Text></Text>
                <HStack gap={2} flexWrap="wrap">
                  {KINDS.map((k) => (
                    <Button key={k.kind} minH="44px" variant={kind === k.kind ? 'solid' : 'outline'} colorPalette="gray"
                      aria-pressed={kind === k.kind} onClick={() => setKind(k.kind)}>{k.label}</Button>
                  ))}
                </HStack>
              </Stack>
            </SimpleGrid>
            {taken && (
              <HStack gap={3} bg="orange.subtle" borderRadius="lg" p={3}>
                <Text flex="1">{taken.archived ? `Ya existe «${name.trim()}», archivado.` : `Ya existe «${name.trim()}».`}</Text>
                {taken.archived
                  ? <Button minH="44px" colorPalette="brand" loading={reactivate.isPending} onClick={() => reactivate.mutate(taken.id)}>Reactivar</Button>
                  : <Button minH="44px" variant="outline" onClick={() => onCreated(taken.id)}>Abrir ese</Button>}
              </HStack>
            )}
            {kind && (
              <SimpleGrid columns={2} gap={3}>
                <QtyWithUnit label="Existencia inicial" optional kind={kind} value={stock} onChange={setStock} />
                <Stack gap={1}>
                  <Text asChild><label htmlFor="new-article-cost">Costo <Text as="span" color="fg.muted">(opcional)</Text></label></Text>
                  <HStack gap={2}>
                    <Input id="new-article-cost" minH="48px" inputMode="decimal" placeholder="0.00"
                      value={cost} onChange={(e) => setCost(e.target.value.replace(',', '.'))} />
                    <Text flexShrink={0}>$ por {big?.label ?? 'pieza'}</Text>
                  </HStack>
                </Stack>
                <Stack gap={1}>
                  <Text>Ubicación <Text as="span" color="fg.muted">(opcional)</Text></Text>
                  <Box>
                    <Picker {...locations.pickerProps} value={placeId} options={locations.options} onChange={setPlaceId} />
                  </Box>
                </Stack>
                <QtyWithUnit label="Mínimo" optional kind={kind} value={min} onChange={setMin} />
              </SimpleGrid>
            )}
            <Text fontSize="sm" color="fg.muted"><Text as="span" color="red.600">*</Text> Obligatorio. Nada se guarda hasta tocar «Guardar».</Text>
          </Stack>
        </DrawerBody>
      </DrawerContent>
    </DrawerRoot>
  );
}
