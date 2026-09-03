import { useMemo, useState } from 'react';
import {
  Box, HStack, VStack, Text, Button, IconButton, Input, InputGroup, Center, Spinner, Badge, Wrap, WrapItem,
} from '@chakra-ui/react';
import { LuPencil, LuTrash2, LuPlus, LuRotateCcw, LuSearch, LuCheck } from 'react-icons/lu';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { adminApi, type ProductGroup } from '../api/admin';
import { normalize } from '../utils/format';
import { Field } from '../components/ui/field';
import { Switch } from '../components/ui/switch';
import { toaster } from '../components/ui/toaster';

interface AttachBody {
  groupId: number; title: string; override: boolean; minSelect: number; maxSelect: number; position: number;
}

// Gestión inline (sin diálogos anidados → se puede embeber en cualquier lado) de los grupos
// de UN producto: asignar con un tap desde el catálogo, personalizar min/máx, quitar. Rápido y táctil.
export function ProductGroupsManager({ productId }: { productId: number }) {
  const qc = useQueryClient();
  const [picking, setPicking] = useState(false);
  const [pq, setPq] = useState('');
  const [editingId, setEditingId] = useState<number | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: ['admin', 'product-groups', productId],
    queryFn: () => adminApi.productGroups(productId),
  });
  const assigned = useMemo(() => data?.items ?? [], [data]);
  const assignedIds = useMemo(() => new Set(assigned.map((g) => g.groupId)), [assigned]);

  const { data: catalog, isLoading: catLoading } = useQuery({
    queryKey: ['admin', 'groups', 'all'],
    queryFn: () => adminApi.groups({ status: 'act', limit: 0 }),
    enabled: picking,
  });
  const available = useMemo(() => {
    const nq = normalize(pq);
    return (catalog?.items ?? [])
      .filter((g) => !assignedIds.has(g.id))
      .filter((g) => !nq || normalize(g.name).includes(nq));
  }, [catalog, assignedIds, pq]);

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ['admin', 'product-groups', productId] });
    qc.invalidateQueries({ queryKey: ['admin', 'products'] }); // overrideCount/groupCount de los chips
    qc.invalidateQueries({ queryKey: ['admin', 'groups'] });   // productCount del catálogo
    qc.invalidateQueries({ queryKey: ['menu'] });
  };

  const attach = useMutation({
    mutationFn: (b: AttachBody) => adminApi.attachProductGroup(productId, b),
    onSuccess: invalidate,
    onError: (e) => toaster.create({ title: 'Error', description: String(e), type: 'error' }),
  });
  const detach = useMutation({
    mutationFn: (g: ProductGroup) => adminApi.detachProductGroup(productId, g.groupId),
    onSuccess: (_d, g) => {
      invalidate();
      toaster.create({
        title: `«${g.title}» quitado`, type: 'success',
        action: {
          label: 'Deshacer',
          onClick: () => attach.mutate({
            groupId: g.groupId, title: g.title, override: g.overridden,
            minSelect: g.minSelect, maxSelect: g.maxSelect, position: g.position,
          }),
        },
      });
    },
    onError: (e) => toaster.create({ title: 'Error', description: String(e), type: 'error' }),
  });

  // tap-para-agregar: hereda el default del grupo (override=false). El picker sigue abierto
  // para añadir varios seguidos; el grupo elegido salta a la lista de arriba al instante.
  const quickAdd = (g: { id: number }) =>
    attach.mutate({ groupId: g.id, title: '', override: false, minSelect: 0, maxSelect: 1, position: assigned.length });

  if (isLoading) return <Center py={6}><Spinner /></Center>;

  return (
    <VStack align="stretch" gap={2}>
      {assigned.map((g) => (
        <Box key={g.groupId} borderWidth="1px" borderRadius="lg" bg="bg.subtle" opacity={g.groupActive ? 1 : 0.55}>
          <HStack gap={2} p={3}>
            <Box flex="1" minW={0}>
              <HStack gap={2} wrap="wrap">
                <Text fontWeight="600">{g.title}</Text>
                <Badge colorPalette={g.minSelect > 0 ? 'orange' : 'gray'}>{g.minSelect > 0 ? 'obligatorio' : 'opcional'}</Badge>
                {g.overridden && <Badge colorPalette="purple">personalizado</Badge>}
                {!g.groupActive && <Badge>grupo inactivo</Badge>}
              </HStack>
              <Text fontSize="xs" color="fg.muted">
                elige {g.minSelect}–{g.maxSelect}
                {g.overridden && <> · def. {g.defaultMin}–{g.defaultMax}</>}
                {' '}· {g.optionCount} opciones
              </Text>
            </Box>
            <IconButton aria-label="Personalizar" size="sm" variant={editingId === g.groupId ? 'solid' : 'ghost'}
              onClick={() => setEditingId(editingId === g.groupId ? null : g.groupId)}>
              <LuPencil />
            </IconButton>
            {/* quitar al instante + «Deshacer» en el toast (dentro de un diálogo, evita popover anidado) */}
            <IconButton aria-label="Quitar" size="sm" variant="ghost" colorPalette="red"
              loading={detach.isPending && detach.variables?.groupId === g.groupId} onClick={() => detach.mutate(g)}>
              <LuTrash2 />
            </IconButton>
          </HStack>
          {editingId === g.groupId && (
            <RowEditor g={g}
              saving={attach.isPending}
              onSave={(b) => attach.mutate(b, { onSuccess: () => setEditingId(null) })}
              onReset={() => attach.mutate(
                { groupId: g.groupId, title: g.title, override: false, minSelect: 0, maxSelect: 1, position: g.position },
                { onSuccess: () => setEditingId(null) },
              )}
              onCancel={() => setEditingId(null)}
            />
          )}
        </Box>
      ))}
      {assigned.length === 0 && !picking && <Text color="fg.subtle" fontSize="sm">Sin grupos asignados.</Text>}

      {/* Picker táctil: reemplaza el viejo <select>. Un tap agrega el grupo (hereda su default). */}
      {picking ? (
        <Box borderWidth="1px" borderRadius="lg" p={3} bg="bg.panel">
          <HStack mb={2}>
            <InputGroup flex="1" startElement={<LuSearch />}>
              <Input autoFocus placeholder="Buscar grupo del catálogo…" value={pq} onChange={(e) => setPq(e.target.value)} bg="bg.subtle" />
            </InputGroup>
            <Button size="sm" variant="ghost" onClick={() => { setPicking(false); setPq(''); }}>
              <LuCheck /> Listo
            </Button>
          </HStack>
          {catLoading ? <Center py={4}><Spinner size="sm" /></Center> : (
            <Wrap gap={2}>
              {available.map((g) => (
                <WrapItem key={g.id}>
                  <Button size="md" minH="44px" variant="outline" colorPalette="gray"
                    loading={attach.isPending && attach.variables?.groupId === g.id}
                    onClick={() => quickAdd(g)}>
                    <LuPlus /> {g.name}
                    <Text as="span" ml={1} fontSize="xs" color="fg.muted">
                      {g.optionCount} opc · {g.defaultMin}–{g.defaultMax}
                    </Text>
                  </Button>
                </WrapItem>
              ))}
              {available.length === 0 && (
                <Text fontSize="sm" color="fg.muted">
                  {pq ? 'Sin coincidencias.' : 'No hay más grupos por asignar. Crea uno en «Opciones».'}
                </Text>
              )}
            </Wrap>
          )}
        </Box>
      ) : (
        <Button variant="outline" alignSelf="start" mt={1} onClick={() => setPicking(true)}>
          <LuPlus /> Asignar grupo
        </Button>
      )}
    </VStack>
  );
}

// Editor inline de un enlace ya asignado: título + personalizar min/máx + orden. Sin diálogo.
function RowEditor({ g, saving, onSave, onReset, onCancel }: {
  g: ProductGroup;
  saving: boolean;
  onSave: (b: AttachBody) => void;
  onReset: () => void;
  onCancel: () => void;
}) {
  const [title, setTitle] = useState(g.title);
  const [override, setOverride] = useState(g.overridden);
  const [min, setMin] = useState(String(g.minSelect));
  const [max, setMax] = useState(String(g.maxSelect));
  const [position, setPosition] = useState(String(g.position));

  const minN = parseInt(min, 10) || 0;
  const maxN = parseInt(max, 10) || 0;
  const valid = !override || (maxN >= 1 && minN >= 0 && minN <= maxN);

  return (
    <Box px={3} pb={3} pt={0}>
      <VStack align="stretch" gap={3} borderTopWidth="1px" pt={3}>
        <Field label="Título (vacío = nombre del grupo)">
          <Input size="sm" value={title} onChange={(e) => setTitle(e.target.value)} placeholder={g.groupName} />
        </Field>
        <HStack justify="space-between">
          <Box>
            <Text fontSize="sm" fontWeight="600">Personalizar min/máx</Text>
            <Text fontSize="xs" color="fg.muted">
              {override ? 'Sobrescribe el default del grupo.' : `Hereda del grupo: elige ${g.defaultMin}–${g.defaultMax}.`}
            </Text>
          </Box>
          <Switch checked={override} onCheckedChange={(e) => setOverride(e.checked)} />
        </HStack>
        {override && (
          <HStack>
            <Field label="Mín (≥1 = obligatorio)"><Input size="sm" type="number" min={0} value={min} onChange={(e) => setMin(e.target.value)} /></Field>
            <Field label="Máx a elegir"><Input size="sm" type="number" min={1} value={max} onChange={(e) => setMax(e.target.value)} /></Field>
            <Field label="Orden"><Input size="sm" type="number" min={0} value={position} onChange={(e) => setPosition(e.target.value)} maxW="90px" /></Field>
          </HStack>
        )}
        {!valid && <Text fontSize="xs" color="red.500">El máximo debe ser ≥ 1 y el mínimo no puede superarlo.</Text>}
        <HStack justify="end" gap={2}>
          {g.overridden && (
            <Button size="sm" variant="ghost" onClick={onReset}><LuRotateCcw /> Usar default</Button>
          )}
          <Button size="sm" variant="ghost" onClick={onCancel}>Cancelar</Button>
          <Button size="sm" loading={saving} disabled={!valid}
            onClick={() => onSave({
              groupId: g.groupId, title: title.trim(), override,
              minSelect: minN, maxSelect: maxN, position: parseInt(position, 10) || 0,
            })}>
            Guardar
          </Button>
        </HStack>
      </VStack>
    </Box>
  );
}
