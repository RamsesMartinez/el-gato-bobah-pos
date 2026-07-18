import { useEffect, useState } from 'react';
import {
  Box, Text, Input, InputGroup, Button, ButtonGroup, IconButton, HStack, VStack,
  Center, Spinner, Badge, Pagination,
} from '@chakra-ui/react';
import {
  LuChevronRight, LuChevronDown, LuChevronLeft, LuPencil, LuArchive, LuArchiveRestore,
  LuPlus, LuArrowUp, LuArrowDown, LuChevronsDownUp, LuChevronsUpDown, LuX, LuSearch,
} from 'react-icons/lu';
import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query';
import { adminApi, type Group } from '../../api/admin';
import { normalize } from '../../utils/format';
import { NativeSelectRoot, NativeSelectField } from '../../components/ui/native-select';
import {
  DialogRoot, DialogBackdrop, DialogContent, DialogHeader, DialogBody, DialogFooter,
  DialogTitle, DialogCloseTrigger,
} from '../../components/ui/dialog';
import { Field } from '../../components/ui/field';
import { Switch } from '../../components/ui/switch';
import { toaster } from '../../components/ui/toaster';
import { useUiStore } from '../../stores/ui';
import { Page } from '../../components/Page';
import { ConfirmPopover } from '../../components/ui/confirm-popover';
import { GroupOptions } from './GroupOptions';

type Status = 'act' | 'inact' | 'all';
type Sort = 'name' | 'options' | 'products';
const PAGE_SIZE = 25;

// Catálogo central de grupos de modificadores. Cada grupo es reutilizable entre productos y trae
// un default de min/max de selección; cada producto puede sobrescribirlo.
export function ModifierOptionsPage() {
  const [search, setSearch] = useState('');
  const [dsearch, setDsearch] = useState('');
  const [status, setStatus] = useState<Status>('act');
  const [sort, setSort] = useState<Sort>('name');
  const [dir, setDir] = useState<'asc' | 'desc'>('asc');
  const [page, setPage] = useState(1);
  const [expanded, setExpanded] = useState<Set<number>>(new Set());
  const [editGroup, setEditGroup] = useState<Group | null>(null);
  const [creating, setCreating] = useState(false);
  const [productsFor, setProductsFor] = useState<Group | null>(null);

  useEffect(() => {
    const t = setTimeout(() => setDsearch(search), 300);
    return () => clearTimeout(t);
  }, [search]);
  useEffect(() => { setPage(1); }, [dsearch, status, sort, dir]);

  const { data, isLoading } = useQuery({
    queryKey: ['admin', 'groups', { status, dsearch, sort, dir, page }],
    queryFn: () => adminApi.groups({ status, search: dsearch, sort, dir, limit: PAGE_SIZE, offset: (page - 1) * PAGE_SIZE }),
    placeholderData: keepPreviousData,
  });

  const items = data?.items ?? [];
  const total = data?.total ?? 0;
  const counts = { act: data?.counts.act ?? 0, inact: data?.counts.inact ?? 0, all: (data?.counts.act ?? 0) + (data?.counts.inact ?? 0) };
  const searching = dsearch.trim() !== ''; // al buscar: grupos auto-desplegados y opciones filtradas

  const allOpen = items.length > 0 && items.every((g) => expanded.has(g.id));
  const toggleAll = () => setExpanded(allOpen ? new Set() : new Set(items.map((g) => g.id)));
  const toggleOne = (id: number) => setExpanded((prev) => {
    const n = new Set(prev);
    if (n.has(id)) n.delete(id); else n.add(id);
    return n;
  });

  if (isLoading) return <Center h="60vh"><Spinner size="xl" /></Center>;

  const TABS: Array<{ k: Status; label: string; n: number }> = [
    { k: 'act', label: 'Activos', n: counts.act },
    { k: 'inact', label: 'Inactivos', n: counts.inact },
    { k: 'all', label: 'Todos', n: counts.all },
  ];

  return (
    <Page maxW="1150px" fill>
      <Text color="fg.muted" mb={3} flexShrink={0}>
        Catálogo de grupos de modificadores (reutilizables entre productos). El default de min/máx
        se puede sobrescribir al asignar el grupo a cada <b>producto</b>.
      </Text>

      <HStack mb={2} gap={3} wrap="wrap" justify="space-between" flexShrink={0}>
        <HStack gap={3} wrap="wrap">
          {/* ✕ dentro del input (endElement): no recalcula el layout al aparecer y queda
              separado de las pestañas → sin misclick. */}
          <InputGroup
            maxW="300px"
            startElement={<LuSearch />}
            endElement={search !== '' ? (
              <IconButton aria-label="Limpiar búsqueda" size="sm" variant="ghost" onClick={() => setSearch('')}>
                <LuX />
              </IconButton>
            ) : undefined}
          >
            <Input placeholder="Buscar grupo u opción…" value={search} onChange={(e) => setSearch(e.target.value)} bg="bg.panel" />
          </InputGroup>
          <HStack gap={1} bg="bg.muted" p={1} borderRadius="lg">
            {TABS.map((t) => (
              <Button key={t.k} size="sm" variant={status === t.k ? 'solid' : 'ghost'}
                colorPalette={status === t.k ? undefined : 'gray'} onClick={() => setStatus(t.k)}>
                {t.label} <Text as="span" opacity={0.7} ml={1}>{t.n}</Text>
              </Button>
            ))}
          </HStack>
        </HStack>
        <HStack gap={2} wrap="wrap">
          <HStack gap={1}>
            <NativeSelectRoot size="sm" w="auto">
              <NativeSelectField value={sort} onChange={(e) => setSort(e.target.value as Sort)}>
                <option value="name">Nombre</option>
                <option value="options"># opciones</option>
                <option value="products"># productos</option>
              </NativeSelectField>
            </NativeSelectRoot>
            <IconButton aria-label="Dirección" size="sm" variant="outline"
              onClick={() => setDir((d) => (d === 'asc' ? 'desc' : 'asc'))}>
              {dir === 'asc' ? <LuArrowUp /> : <LuArrowDown />}
            </IconButton>
          </HStack>
          <Button size="sm" variant="ghost" onClick={toggleAll}>
            {allOpen ? <LuChevronsDownUp /> : <LuChevronsUpDown />} {allOpen ? 'Colapsar' : 'Desplegar'} todo
          </Button>
          <Button size="sm" onClick={() => setCreating(true)}><LuPlus /> Nuevo grupo</Button>
        </HStack>
      </HStack>

      <Box flex="1" minH={0} overflowY="auto" mt={2}>
        {items.map((g) => (
          <GroupCard key={g.id} group={g} search={dsearch} open={searching || expanded.has(g.id)}
            onToggle={() => toggleOne(g.id)} onEdit={setEditGroup} onShowProducts={setProductsFor} />
        ))}
        {items.length === 0 && <Text color="fg.subtle">Sin coincidencias.</Text>}
      </Box>

      <HStack mt={3} pt={3} borderTopWidth="1px" justify="space-between" wrap="wrap" gap={2} flexShrink={0}>
        <Text color="fg.muted" fontSize="sm">
          {total === 0 ? 'Sin resultados' : `${total} grupo${total === 1 ? '' : 's'}`}
        </Text>
        {total > PAGE_SIZE && (
          <Pagination.Root count={total} pageSize={PAGE_SIZE} page={page} onPageChange={(e) => setPage(e.page)}>
            <ButtonGroup variant="ghost" size="sm" attached>
              <Pagination.PrevTrigger asChild><IconButton aria-label="Anterior"><LuChevronLeft /></IconButton></Pagination.PrevTrigger>
              <Pagination.Items render={(pg) => (
                <IconButton variant={pg.value === page ? 'outline' : 'ghost'}>{pg.value}</IconButton>
              )} />
              <Pagination.NextTrigger asChild><IconButton aria-label="Siguiente"><LuChevronRight /></IconButton></Pagination.NextTrigger>
            </ButtonGroup>
          </Pagination.Root>
        )}
      </HStack>

      <GroupFormDialog group={editGroup} isOpen={editGroup !== null || creating}
        onClose={() => { setEditGroup(null); setCreating(false); }} />
      <GroupProductsDialog group={productsFor} isOpen={productsFor !== null} onClose={() => setProductsFor(null)} />
    </Page>
  );
}

// --- Tarjeta de grupo (expandible → opciones) ---
function GroupCard({ group, search, open, onToggle, onEdit, onShowProducts }: {
  group: Group;
  search: string;
  open: boolean;
  onToggle: () => void;
  onEdit: (g: Group) => void;
  onShowProducts: (g: Group) => void;
}) {
  const qc = useQueryClient();
  // Si el match fue por el NOMBRE del grupo, mostramos todas sus opciones; si fue por una opción,
  // filtramos a las coincidentes (así "brava" en «Salsa extra» deja ver solo «Salsa Brava»).
  const nameMatches = search.trim() !== '' && normalize(group.name).includes(normalize(search));
  const optionFilter = search.trim() !== '' && !nameMatches ? search : '';
  // set explícito (no toggle) para que «Deshacer» revierta al estado anterior.
  const arch = useMutation({
    mutationFn: (active: boolean) => adminApi.updateGroup(group.id, {
      name: group.name, active, defaultMin: group.defaultMin, defaultMax: group.defaultMax,
    }),
    onSuccess: (_d, active) => {
      qc.invalidateQueries({ queryKey: ['admin', 'groups'] });
      qc.invalidateQueries({ queryKey: ['menu'] });
      toaster.create({
        title: active ? 'Grupo reactivado' : 'Grupo archivado',
        type: 'success',
        action: { label: 'Deshacer', onClick: () => arch.mutate(!active) },
      });
    },
    onError: (e) => toaster.create({ title: 'Error', description: String(e), type: 'error' }),
  });

  return (
    <Box borderWidth="1px" borderRadius="lg" bg="bg.panel" mb={3} opacity={group.isActive ? 1 : 0.6} overflow="hidden">
      <HStack gap={0} align="stretch">
        {/* Zona grande tappable: toda la franja izquierda despliega/colapsa (target amplio para dedo) */}
        <HStack flex="1" minW={0} gap={3} px={4} py={3} cursor="pointer" onClick={onToggle}
          _hover={{ bg: 'bg.muted' }} role="button" aria-expanded={open}>
          <Box color="fg.muted" flexShrink={0} fontSize="lg">{open ? <LuChevronDown /> : <LuChevronRight />}</Box>
          <Box flex="1" minW={0}>
            <HStack gap={2} wrap="wrap">
              <Text fontWeight="700">{group.name}</Text>
              {!group.isActive && <Badge>inactivo</Badge>}
              {group.overrideCount > 0 && <Badge colorPalette="purple">{group.overrideCount} personaliz.</Badge>}
            </HStack>
            <Text fontSize="xs" color="fg.muted" mt={0.5}>
              {group.optionCount} opciones · por defecto elige {group.defaultMin}–{group.defaultMax}
            </Text>
          </Box>
        </HStack>
        {/* Zona de acciones separada: no dispara el desplegar */}
        <HStack gap={1} pr={2} pl={1} flexShrink={0} align="center" borderLeftWidth="1px" borderColor="border.muted">
          <Button size="sm" variant="ghost" colorPalette="gray" onClick={() => onShowProducts(group)}
            aria-label={`Usado en ${group.productCount} productos, ver cuáles`}>
            {group.productCount} prod.
          </Button>
          <IconButton aria-label="Editar grupo" size="md" variant="ghost" onClick={() => onEdit(group)}><LuPencil /></IconButton>
          {group.isActive ? (
            <ConfirmPopover title="¿Archivar grupo?"
              description="Se quita del POS en todos los productos que lo usan. Puedes deshacerlo enseguida."
              onConfirm={() => arch.mutate(false)}>
              <IconButton aria-label="Archivar grupo" size="md" variant="ghost" colorPalette="gray">
                <LuArchive />
              </IconButton>
            </ConfirmPopover>
          ) : (
            <IconButton aria-label="Reactivar grupo" size="md" variant="ghost" colorPalette="green"
              loading={arch.isPending} onClick={() => arch.mutate(true)}>
              <LuArchiveRestore />
            </IconButton>
          )}
        </HStack>
      </HStack>
      {open && <Box px={4} pb={3} pt={1}><GroupOptions groupId={group.id} filter={optionFilter} /></Box>}
    </Box>
  );
}

// --- Crear / editar grupo (con default min/max) ---
function GroupFormDialog({ group, isOpen, onClose }: { group: Group | null; isOpen: boolean; onClose: () => void }) {
  const qc = useQueryClient();
  const palette = useUiStore((s) => s.palette);
  const [name, setName] = useState('');
  const [active, setActive] = useState(true);
  const [min, setMin] = useState('0');
  const [max, setMax] = useState('1');

  useEffect(() => {
    setName(group?.name ?? '');
    setActive(group?.isActive ?? true);
    setMin(String(group?.defaultMin ?? 0));
    setMax(String(group?.defaultMax ?? 1));
  }, [group, isOpen]);

  const minN = parseInt(min, 10) || 0;
  const maxN = parseInt(max, 10) || 0;
  const valid = name.trim() !== '' && maxN >= 1 && minN >= 0 && minN <= maxN;

  const save = useMutation({
    mutationFn: () => group
      ? adminApi.updateGroup(group.id, { name: name.trim(), active, defaultMin: minN, defaultMax: maxN })
      : adminApi.createGroup(name.trim(), minN, maxN).then(() => {}),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['admin', 'groups'] });
      qc.invalidateQueries({ queryKey: ['menu'] });
      onClose();
      toaster.create({ title: group ? 'Grupo actualizado' : 'Grupo creado', type: 'success' });
    },
    onError: (e) => toaster.create({ title: 'Error', description: String(e), type: 'error' }),
  });

  return (
    <DialogRoot open={isOpen} onOpenChange={(e) => { if (!e.open) onClose(); }}>
      <DialogBackdrop />
      <DialogContent colorPalette={palette}>
        <DialogHeader><DialogTitle>{group ? 'Editar grupo' : 'Nuevo grupo'}</DialogTitle></DialogHeader>
        <DialogCloseTrigger />
        <DialogBody>
          <VStack align="stretch" gap={4}>
            <Field label="Nombre">
              <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="p. ej. Perlas explosivas" />
            </Field>
            <Box>
              <Text fontSize="sm" fontWeight="600" mb={1}>Selección por defecto</Text>
              <Text fontSize="xs" color="fg.muted" mb={2}>Cuántas opciones se piden por defecto. Mín ≥ 1 = obligatorio. Cada producto puede sobrescribirlo.</Text>
              <HStack>
                <Field label="Mínimo"><Input type="number" min={0} value={min} onChange={(e) => setMin(e.target.value)} /></Field>
                <Field label="Máximo"><Input type="number" min={1} value={max} onChange={(e) => setMax(e.target.value)} /></Field>
              </HStack>
            </Box>
            {group && (
              <HStack justify="space-between">
                <Text>Activo</Text>
                <Switch checked={active} onCheckedChange={(e) => setActive(e.checked)} />
              </HStack>
            )}
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

// --- "Usado en N productos" (read-only), marcando los personalizados ---
function GroupProductsDialog({ group, isOpen, onClose }: { group: Group | null; isOpen: boolean; onClose: () => void }) {
  const palette = useUiStore((s) => s.palette);
  const { data, isLoading } = useQuery({
    queryKey: ['admin', 'group-products', group?.id],
    queryFn: () => adminApi.groupProducts(group!.id),
    enabled: group !== null,
  });
  const products = data?.items ?? [];

  return (
    <DialogRoot open={isOpen} onOpenChange={(e) => { if (!e.open) onClose(); }}>
      <DialogBackdrop />
      <DialogContent colorPalette={palette}>
        <DialogHeader><DialogTitle>{group?.name} — productos</DialogTitle></DialogHeader>
        <DialogCloseTrigger />
        <DialogBody>
          {isLoading ? <Center py={4}><Spinner /></Center> : (
            <VStack align="stretch" gap={2}>
              {products.map((p) => (
                <HStack key={p.id} justify="space-between" p={2} borderWidth="1px" borderRadius="lg" bg="bg.subtle">
                  <Text>{p.name}</Text>
                  <HStack gap={2}>
                    {p.overridden && <Badge colorPalette="purple">personalizado</Badge>}
                    <Badge colorPalette={p.required ? 'orange' : 'gray'}>{p.required ? 'obligatorio' : 'opcional'}</Badge>
                    <Text fontSize="xs" color="fg.muted">elige {p.minSelect}–{p.maxSelect}</Text>
                  </HStack>
                </HStack>
              ))}
              {products.length === 0 && <Text color="fg.subtle">No está asignado a ningún producto.</Text>}
            </VStack>
          )}
        </DialogBody>
        <DialogFooter>
          <Button onClick={onClose}>Cerrar</Button>
        </DialogFooter>
      </DialogContent>
    </DialogRoot>
  );
}
