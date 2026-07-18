import { useEffect, useMemo, useState } from 'react';
import {
  Box, HStack, VStack, Text, Button, IconButton, Input, Center, Spinner, Badge,
} from '@chakra-ui/react';
import { LuPencil, LuTrash2, LuPlus, LuRotateCcw } from 'react-icons/lu';
import {
  DialogRoot, DialogBackdrop, DialogContent, DialogHeader, DialogBody, DialogFooter,
  DialogTitle, DialogCloseTrigger,
} from '../../components/ui/dialog';
import { NativeSelectRoot, NativeSelectField } from '../../components/ui/native-select';
import { Field } from '../../components/ui/field';
import { Switch } from '../../components/ui/switch';
import { toaster } from '../../components/ui/toaster';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { adminApi, type ProductGroup } from '../../api/admin';
import { useUiStore } from '../../stores/ui';

// Gestiona los grupos de modificadores de UN producto: asignar (reutilizando el catálogo),
// personalizar min/máx (o heredar el default del grupo), reordenar y quitar.
export function ProductGroupsDialog({ productId, productName, isOpen, onClose }: {
  productId: number | null;
  productName: string;
  isOpen: boolean;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const palette = useUiStore((s) => s.palette);
  const [linkEdit, setLinkEdit] = useState<ProductGroup | null>(null);
  const [attaching, setAttaching] = useState(false);

  const { data, isLoading } = useQuery({
    queryKey: ['admin', 'product-groups', productId],
    queryFn: () => adminApi.productGroups(productId!),
    enabled: isOpen && productId !== null,
  });
  const assigned = data?.items ?? [];

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ['admin', 'product-groups', productId] });
    qc.invalidateQueries({ queryKey: ['admin', 'products'] }); // cambia overrideCount del chip
    qc.invalidateQueries({ queryKey: ['admin', 'groups'] });   // cambia productCount/overrideCount
    qc.invalidateQueries({ queryKey: ['menu'] });
  };
  const detach = useMutation({
    mutationFn: (g: ProductGroup) => adminApi.detachProductGroup(productId!, g.groupId),
    onSuccess: (_d, g) => { invalidate(); toaster.create({ title: `«${g.groupName}» quitado`, type: 'success' }); },
    onError: (e) => toaster.create({ title: 'Error', description: String(e), type: 'error' }),
  });
  const reset = useMutation({
    mutationFn: (g: ProductGroup) => adminApi.attachProductGroup(productId!, {
      groupId: g.groupId, title: g.title, override: false, minSelect: 0, maxSelect: 1, position: g.position,
    }),
    onSuccess: () => { invalidate(); toaster.create({ title: 'Restablecido al default del grupo', type: 'success' }); },
    onError: (e) => toaster.create({ title: 'Error', description: String(e), type: 'error' }),
  });

  return (
    <>
    <DialogRoot open={isOpen} onOpenChange={(e) => { if (!e.open) onClose(); }} size="lg">
      <DialogBackdrop />
      <DialogContent colorPalette={palette}>
        <DialogHeader><DialogTitle>Grupos de «{productName}»</DialogTitle></DialogHeader>
        <DialogCloseTrigger />
        <DialogBody>
          {isLoading ? <Center py={6}><Spinner /></Center> : (
            <VStack align="stretch" gap={2}>
              {assigned.map((g) => (
                <HStack key={g.groupId} gap={2} p={3} borderWidth="1px" borderRadius="lg" bg="bg.subtle"
                  opacity={g.groupActive ? 1 : 0.55}>
                  <Box flex="1" minW={0}>
                    <HStack gap={2} wrap="wrap">
                      <Text fontWeight="600">{g.title}</Text>
                      <Badge colorPalette={g.overridden ? 'purple' : 'gray'}>{g.overridden ? 'personalizado' : 'por defecto'}</Badge>
                      <Badge colorPalette={g.minSelect > 0 ? 'orange' : 'gray'}>{g.minSelect > 0 ? 'obligatorio' : 'opcional'}</Badge>
                      {!g.groupActive && <Badge>grupo inactivo</Badge>}
                    </HStack>
                    <Text fontSize="xs" color="fg.muted">
                      elige {g.minSelect}–{g.maxSelect}
                      {g.overridden && <> · def. {g.defaultMin}–{g.defaultMax}</>}
                      {' '}· {g.optionCount} opciones · orden {g.position}
                    </Text>
                  </Box>
                  {g.overridden && (
                    <IconButton aria-label="Restablecer al default" size="sm" variant="ghost"
                      loading={reset.isPending && reset.variables?.groupId === g.groupId} onClick={() => reset.mutate(g)}>
                      <LuRotateCcw />
                    </IconButton>
                  )}
                  <IconButton aria-label="Editar" size="sm" variant="ghost" onClick={() => setLinkEdit(g)}><LuPencil /></IconButton>
                  <IconButton aria-label="Quitar" size="sm" variant="ghost" colorPalette="red"
                    loading={detach.isPending && detach.variables?.groupId === g.groupId} onClick={() => detach.mutate(g)}>
                    <LuTrash2 />
                  </IconButton>
                </HStack>
              ))}
              {assigned.length === 0 && <Text color="fg.subtle">Este producto no tiene grupos asignados.</Text>}
              <Button variant="outline" alignSelf="start" mt={1} onClick={() => setAttaching(true)}>
                <LuPlus /> Asignar grupo
              </Button>
            </VStack>
          )}
        </DialogBody>
        <DialogFooter>
          <Button onClick={onClose}>Cerrar</Button>
        </DialogFooter>
      </DialogContent>
    </DialogRoot>

    {productId !== null && (
      <LinkFormDialog
        productId={productId}
        link={linkEdit}
        isOpen={linkEdit !== null || attaching}
        assignedIds={assigned.map((g) => g.groupId)}
        defaultPosition={assigned.length}
        onClose={() => { setLinkEdit(null); setAttaching(false); }}
        onSaved={invalidate}
      />
    )}
    </>
  );
}

// Asignar (elige del catálogo) o editar el enlace. "Personalizar" on = override; off = hereda el default del grupo.
function LinkFormDialog({ productId, link, isOpen, assignedIds, defaultPosition, onClose, onSaved }: {
  productId: number;
  link: ProductGroup | null;
  isOpen: boolean;
  assignedIds: number[];
  defaultPosition: number;
  onClose: () => void;
  onSaved: () => void;
}) {
  const palette = useUiStore((s) => s.palette);
  const [groupId, setGroupId] = useState('');
  const [title, setTitle] = useState('');
  const [override, setOverride] = useState(false);
  const [min, setMin] = useState('0');
  const [max, setMax] = useState('1');
  const [position, setPosition] = useState('0');

  const { data: catalog } = useQuery({
    queryKey: ['admin', 'groups', 'all'],
    queryFn: () => adminApi.groups({ status: 'act', limit: 0 }),
    enabled: isOpen && link === null,
  });
  const available = useMemo(
    () => (catalog?.items ?? []).filter((g) => !assignedIds.includes(g.id)),
    [catalog, assignedIds],
  );
  // default del grupo elegido (para mostrar "hereda: elige X–Y" y sembrar los inputs)
  const selectedDefault = useMemo(() => {
    if (link) return { min: link.defaultMin, max: link.defaultMax };
    const g = (catalog?.items ?? []).find((x) => String(x.id) === groupId);
    return { min: g?.defaultMin ?? 0, max: g?.defaultMax ?? 1 };
  }, [link, catalog, groupId]);

  useEffect(() => {
    setGroupId(link ? String(link.groupId) : '');
    setTitle(link?.title ?? '');
    setOverride(link?.overridden ?? false);
    setMin(String(link?.minSelect ?? 0));
    setMax(String(link?.maxSelect ?? 1));
    setPosition(String(link?.position ?? defaultPosition));
  }, [link, isOpen, defaultPosition]);

  // al elegir un grupo nuevo, siembra los inputs con su default
  useEffect(() => {
    if (!link && !override) { setMin(String(selectedDefault.min)); setMax(String(selectedDefault.max)); }
  }, [groupId]); // eslint-disable-line react-hooks/exhaustive-deps

  const minN = parseInt(min, 10) || 0;
  const maxN = parseInt(max, 10) || 0;
  const overrideValid = !override || (maxN >= 1 && minN >= 0 && minN <= maxN);
  const valid = (link !== null || groupId !== '') && overrideValid;

  const save = useMutation({
    mutationFn: () => adminApi.attachProductGroup(productId, {
      groupId: link ? link.groupId : Number(groupId),
      title: title.trim(),
      override,
      minSelect: minN,
      maxSelect: maxN,
      position: parseInt(position, 10) || 0,
    }),
    onSuccess: () => { onSaved(); onClose(); toaster.create({ title: link ? 'Grupo actualizado' : 'Grupo asignado', type: 'success' }); },
    onError: (e) => toaster.create({ title: 'Error', description: String(e), type: 'error' }),
  });

  return (
    <DialogRoot open={isOpen} onOpenChange={(e) => { if (!e.open) onClose(); }}>
      <DialogBackdrop />
      <DialogContent colorPalette={palette}>
        <DialogHeader><DialogTitle>{link ? `Editar «${link.groupName}»` : 'Asignar grupo'}</DialogTitle></DialogHeader>
        <DialogCloseTrigger />
        <DialogBody>
          <VStack align="stretch" gap={4}>
            {!link && (
              <Field label="Grupo del catálogo">
                <NativeSelectRoot>
                  <NativeSelectField placeholder="Elige un grupo…" value={groupId} onChange={(e) => setGroupId(e.target.value)}>
                    {available.map((g) => <option key={g.id} value={g.id}>{g.name} ({g.optionCount} opc. · def. {g.defaultMin}–{g.defaultMax})</option>)}
                  </NativeSelectField>
                </NativeSelectRoot>
                {available.length === 0 && <Text fontSize="xs" color="fg.muted" mt={1}>No hay más grupos activos por asignar. Crea uno en «Opciones».</Text>}
              </Field>
            )}
            <Field label="Título (opcional; vacío usa el nombre del grupo)">
              <Input value={title} onChange={(e) => setTitle(e.target.value)} placeholder={link?.groupName} />
            </Field>
            <HStack justify="space-between">
              <Box>
                <Text fontWeight="600">Personalizar min/máx</Text>
                <Text fontSize="xs" color="fg.muted">
                  {override ? 'Sobrescribe el default del grupo.' : `Hereda del grupo: elige ${selectedDefault.min}–${selectedDefault.max}.`}
                </Text>
              </Box>
              <Switch checked={override} onCheckedChange={(e) => setOverride(e.checked)} />
            </HStack>
            {override && (
              <HStack>
                <Field label="Mínimo (≥1 = obligatorio)">
                  <Input type="number" min={0} value={min} onChange={(e) => setMin(e.target.value)} />
                </Field>
                <Field label="Máximo a elegir">
                  <Input type="number" min={1} value={max} onChange={(e) => setMax(e.target.value)} />
                </Field>
              </HStack>
            )}
            <Field label="Orden">
              <Input type="number" min={0} value={position} onChange={(e) => setPosition(e.target.value)} maxW="120px" />
            </Field>
            {override && !overrideValid && <Text fontSize="xs" color="red.500">Revisa min/máx: el máximo debe ser ≥ 1 y el mínimo no puede superarlo.</Text>}
          </VStack>
        </DialogBody>
        <DialogFooter>
          <Button variant="ghost" mr={3} onClick={onClose}>Cancelar</Button>
          <Button loading={save.isPending} disabled={!valid} onClick={() => save.mutate()}>Guardar</Button>
        </DialogFooter>
      </DialogContent>
    </DialogRoot>
  );
}
