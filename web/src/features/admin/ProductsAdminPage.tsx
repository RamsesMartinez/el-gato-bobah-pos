import { useEffect, useMemo, useState } from 'react';
import {
  Box, Table, Center, Spinner, Input, Badge, Button, ButtonGroup,
  IconButton, HStack, VStack, Pagination, useDisclosure, Text,
} from '@chakra-ui/react';
import {
  LuStar, LuChevronLeft, LuChevronRight, LuSettings2, LuArrowUp, LuArrowDown, LuPlus,
  LuListFilter, LuPencil, LuLayers, LuArchive, LuRotateCcw, LuCopy,
} from 'react-icons/lu';
import { toaster } from '../../components/ui/toaster';
import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query';
import { adminApi, type AdminProduct, type Category, type ProductSort } from '../../api/admin';
import { money } from '../../utils/format';
import { Page } from '../../components/Page';
import { Picker, type PickerOption } from '../../components/Picker';
import { Switch } from '../../components/ui/switch';
import { ConfirmPopover } from '../../components/ui/confirm-popover';
import {
  MenuRoot, MenuTrigger, MenuContent, MenuItemGroup, MenuRadioItemGroup, MenuRadioItem, MenuSeparator,
} from '../../components/ui/menu';
import { ProductEditDialog } from './ProductEditDialog';
import { ProductGroupsDialog } from './ProductGroupsDialog';
import {
  DialogRoot, DialogBackdrop, DialogContent, DialogHeader, DialogBody, DialogFooter,
  DialogTitle, DialogCloseTrigger,
} from '../../components/ui/dialog';
import { Field } from '../../components/ui/field';

const PAGE_SIZE = 25;

type GroupsFilter = '' | 'some' | 'none';

// Etiqueta de categoría con prefijo del padre ("Bebidas › Calientes") para desambiguar subcategorías.
function categoryOptions(cats: Category[]): PickerOption[] {
  const nameById = new Map(cats.map((c) => [c.id, c.name]));
  return cats.map((c) => ({
    value: String(c.id),
    label: c.parentId ? `${nameById.get(c.parentId) ?? '—'} › ${c.name}` : c.name,
  }));
}

export function ProductsAdminPage() {
  const qc = useQueryClient();
  const [search, setSearch] = useState('');
  const [dsearch, setDsearch] = useState(''); // debounced: 1 request por pausa de tecleo, no por tecla
  const [status, setStatus] = useState<'act' | 'inact' | 'all'>('act');
  const [groupsFilter, setGroupsFilter] = useState<GroupsFilter>(''); // con/sin grupos
  const [categoryId, setCategoryId] = useState(''); // '' = todas
  const [sort, setSort] = useState<ProductSort>('name');
  const [dir, setDir] = useState<'asc' | 'desc'>('asc');
  const [page, setPage] = useState(1); // 1-based, como el paginador de Chakra
  const [edit, setEdit] = useState<AdminProduct | null>(null);
  const [groupsProduct, setGroupsProduct] = useState<AdminProduct | null>(null);
  const [duplicateSrc, setDuplicateSrc] = useState<AdminProduct | null>(null);
  const modal = useDisclosure();
  const newModal = useDisclosure();

  useEffect(() => {
    const t = setTimeout(() => { setDsearch(search); setPage(1); }, 300); // nueva búsqueda → página 1
    return () => clearTimeout(t);
  }, [search]);
  // los demás filtros resetean la página en su propio handler (sin efecto → sin renders en cascada).

  const { data: catData } = useQuery({ queryKey: ['admin', 'categories'], queryFn: adminApi.categories });
  const catOptions = useMemo(() => categoryOptions(catData?.items ?? []), [catData]);

  const { data, isLoading } = useQuery({
    queryKey: ['admin', 'products', { status, dsearch, groupsFilter, categoryId, sort, dir, page }],
    queryFn: () => adminApi.products({
      status, search: dsearch,
      groups: groupsFilter || undefined,
      categoryId: categoryId ? Number(categoryId) : undefined,
      sort, dir,
      limit: PAGE_SIZE, offset: (page - 1) * PAGE_SIZE,
    }),
    placeholderData: keepPreviousData, // no parpadea al cambiar de página
  });

  const items = data?.items ?? [];
  const total = data?.total ?? 0;
  const counts = { act: data?.counts.act ?? 0, inact: data?.counts.inact ?? 0, all: (data?.counts.act ?? 0) + (data?.counts.inact ?? 0) };

  // set explícito (no toggle) para que el «Deshacer» del toast revierta al estado anterior.
  const setActive = useMutation({
    mutationFn: ({ p, active }: { p: AdminProduct; active: boolean }) =>
      adminApi.updateProduct(p.id, {
        name: p.name, price: Number(p.price), favorite: p.is_favorite, active,
        availableFrom: p.availableFrom, availableUntil: p.availableUntil,
      }),
    onSuccess: (_d, { p, active }) => {
      qc.invalidateQueries({ queryKey: ['admin', 'products'] });
      qc.invalidateQueries({ queryKey: ['menu'] });
      toaster.create({
        title: active ? 'Producto reactivado' : 'Producto archivado',
        type: 'success',
        action: { label: 'Deshacer', onClick: () => setActive.mutate({ p, active: !active }) },
      });
    },
    onError: (e) => toaster.create({ title: 'Error', description: String(e), type: 'error' }),
  });

  // Filtros no-default activos (para el badge del botón «Filtros»): estado ≠ Activos y grupos ≠ todos.
  const activeFilters = (status !== 'act' ? 1 : 0) + (groupsFilter !== '' ? 1 : 0);

  // Orden por columna: 1er clic ordena (texto asc / número desc); reclics alternan asc/desc.
  const onSort = (col: ProductSort, numeric = false) => {
    if (sort === col) setDir((d) => (d === 'asc' ? 'desc' : 'asc'));
    else { setSort(col); setDir(numeric ? 'desc' : 'asc'); }
    setPage(1);
  };

  const openEdit = (p: AdminProduct) => { setEdit({ ...p }); modal.onOpen(); };

  return (
    <Page maxW="1150px" fill>
      {/* Toolbar compacta para tabletas de 7": búsqueda + categoría crecen; estado/grupos se
          colapsan en un menú «Filtros»; «Nuevo» es compacto (icono en pantallas chicas). */}
      <HStack mb={3} gap={2} wrap="wrap" flexShrink={0}>
        <Input placeholder="Buscar producto…" value={search} onChange={(e) => setSearch(e.target.value)}
          flex="1 1 150px" minW="130px" maxW={{ base: 'full', md: '260px' }} bg="bg.panel" size="sm" />
        <Box flex="1 1 150px" minW="130px" maxW={{ base: 'full', md: '240px' }}>
          <Picker size="sm" value={categoryId} onChange={(v) => { setCategoryId(v); setPage(1); }}
            placeholder="Todas las categorías" title="Filtrar por categoría"
            clearable clearLabel="Todas las categorías" options={catOptions} />
        </Box>
        <MenuRoot>
          <MenuTrigger asChild>
            <Button size="sm" variant="outline" colorPalette="gray" flexShrink={0}>
              <LuListFilter />
              <Box as="span" display={{ base: 'none', sm: 'inline' }}>Filtros</Box>
              {activeFilters > 0 && <Badge colorPalette="blue" borderRadius="full">{activeFilters}</Badge>}
            </Button>
          </MenuTrigger>
          <MenuContent minW="220px">
            <MenuItemGroup title="Estado">
              <MenuRadioItemGroup value={status} onValueChange={(e) => { setStatus(e.value as typeof status); setPage(1); }}>
                <MenuRadioItem value="act">Activos ({counts.act})</MenuRadioItem>
                <MenuRadioItem value="inact">Inactivos ({counts.inact})</MenuRadioItem>
                <MenuRadioItem value="all">Todos ({counts.all})</MenuRadioItem>
              </MenuRadioItemGroup>
            </MenuItemGroup>
            <MenuSeparator />
            <MenuItemGroup title="Grupos de modificadores">
              <MenuRadioItemGroup value={groupsFilter} onValueChange={(e) => { setGroupsFilter(e.value as GroupsFilter); setPage(1); }}>
                <MenuRadioItem value="">Todos</MenuRadioItem>
                <MenuRadioItem value="some">Con grupos</MenuRadioItem>
                <MenuRadioItem value="none">Sin grupos</MenuRadioItem>
              </MenuRadioItemGroup>
            </MenuItemGroup>
          </MenuContent>
        </MenuRoot>
        <Button size="sm" colorPalette="green" flexShrink={0} onClick={newModal.onOpen} title="Nuevo producto">
          <LuPlus /><Box as="span" display={{ base: 'none', sm: 'inline' }}>Nuevo</Box>
        </Button>
      </HStack>

      <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" overflow="auto" flex="1" minH={0}>
        <Table.Root size="sm" stickyHeader>
          <Table.Header>
            <Table.Row bg="bg.panel">
              <SortHead label="Producto" col="name" sort={sort} dir={dir} onSort={onSort} />
              <SortHead label="Categoría" col="category" sort={sort} dir={dir} onSort={onSort} />
              <SortHead label="Precio" col="price" sort={sort} dir={dir} onSort={onSort} numeric align="end" />
              <SortHead label="Costo" col="cost" sort={sort} dir={dir} onSort={onSort} numeric align="end" />
              <SortHead label="Margen" col="margin" sort={sort} dir={dir} onSort={onSort} numeric align="end" />
              <SortHead label="Grupos" col="groups" sort={sort} dir={dir} onSort={onSort} numeric align="center" />
              <Table.ColumnHeader></Table.ColumnHeader>
            </Table.Row>
          </Table.Header>
          <Table.Body>
            {items.map((p) => (
              <Table.Row key={p.id} opacity={p.is_active ? 1 : 0.5}>
                <Table.Cell>
                  {p.is_favorite && (
                    <Box as="span" color="yellow.500" mr={1} display="inline-flex" verticalAlign="middle"><LuStar size={12} /></Box>
                  )}
                  {p.name}
                  {p.overrideCount > 0 && (
                    <Badge ml={2} colorPalette="purple" title={`${p.overrideCount} grupo(s) con min/máx personalizado`}>
                      <LuSettings2 /> {p.overrideCount}
                    </Badge>
                  )}
                </Table.Cell>
                <Table.Cell>{p.category}</Table.Cell>
                <Table.Cell textAlign="end">{money(p.price)}</Table.Cell>
                <Table.Cell textAlign="end">{money(p.current_cost)}</Table.Cell>
                <Table.Cell textAlign="end">{money(Number(p.price) - Number(p.current_cost))}</Table.Cell>
                <Table.Cell textAlign="center">
                  <Text as="span" color={p.groupCount === 0 ? 'fg.subtle' : undefined}
                    fontWeight={p.groupCount === 0 ? undefined : '600'}>
                    {p.groupCount}
                  </Text>
                </Table.Cell>
                <Table.Cell>
                  {/* Acciones como iconos (no texto): ahorran ancho en tabletas de 7". title = tooltip. */}
                  <HStack justify="end" gap={1}>
                    {p.is_active ? (
                      <ConfirmPopover title="¿Archivar producto?"
                        description="Se ocultará del POS. Puedes deshacerlo enseguida o reactivarlo cuando quieras."
                        onConfirm={() => setActive.mutate({ p, active: false })}>
                        <IconButton size="sm" variant="ghost" colorPalette="gray" aria-label="Archivar" title="Archivar"><LuArchive /></IconButton>
                      </ConfirmPopover>
                    ) : (
                      <IconButton size="sm" variant="solid" colorPalette="green" aria-label="Reactivar" title="Reactivar"
                        loading={setActive.isPending && setActive.variables?.p.id === p.id}
                        onClick={() => setActive.mutate({ p, active: true })}><LuRotateCcw /></IconButton>
                    )}
                    <IconButton size="sm" variant="ghost" aria-label="Duplicar" title="Duplicar producto" onClick={() => setDuplicateSrc(p)}><LuCopy /></IconButton>
                    <IconButton size="sm" variant="ghost" aria-label="Grupos" title="Grupos modificadores" onClick={() => setGroupsProduct(p)}><LuLayers /></IconButton>
                    <IconButton size="sm" variant="outline" aria-label="Editar" title="Editar" onClick={() => openEdit(p)}><LuPencil /></IconButton>
                  </HStack>
                </Table.Cell>
              </Table.Row>
            ))}
            {!isLoading && items.length === 0 && (
              <Table.Row><Table.Cell colSpan={7}><Center py={12}><Text color="fg.muted">Sin resultados.</Text></Center></Table.Cell></Table.Row>
            )}
          </Table.Body>
        </Table.Root>
        {isLoading && <Center py={16}><Spinner size="xl" /></Center>}
      </Box>

      <HStack mt={3} pt={3} borderTopWidth="1px" justify="space-between" wrap="wrap" gap={2} flexShrink={0}>
        <Text color="fg.muted" fontSize="sm">
          {total === 0 ? 'Sin resultados' : `${total} producto${total === 1 ? '' : 's'}`}
        </Text>
        {total > PAGE_SIZE && (
          <Pagination.Root count={total} pageSize={PAGE_SIZE} page={page} onPageChange={(e) => setPage(e.page)}>
            <ButtonGroup variant="ghost" size="sm" attached>
              <Pagination.PrevTrigger asChild>
                <IconButton aria-label="Anterior"><LuChevronLeft /></IconButton>
              </Pagination.PrevTrigger>
              <Pagination.Items
                render={(pg) => (
                  <IconButton variant={pg.value === page ? 'outline' : 'ghost'}>{pg.value}</IconButton>
                )}
              />
              <Pagination.NextTrigger asChild>
                <IconButton aria-label="Siguiente"><LuChevronRight /></IconButton>
              </Pagination.NextTrigger>
            </ButtonGroup>
          </Pagination.Root>
        )}
      </HStack>

      <ProductEditDialog product={edit} isOpen={modal.open} onClose={modal.onClose} />
      <ProductGroupsDialog productId={groupsProduct?.id ?? null} productName={groupsProduct?.name ?? ''}
        isOpen={groupsProduct !== null} onClose={() => setGroupsProduct(null)} />
      <NewProductDialog isOpen={newModal.open} onClose={newModal.onClose} categoryOptions={catOptions} />
      {/* key → remonta por producto: el nombre pre-rellenado se inicializa sin useEffect+setState. */}
      {duplicateSrc && (
        <DuplicateProductDialog key={duplicateSrc.id} source={duplicateSrc} onClose={() => setDuplicateSrc(null)} />
      )}
    </Page>
  );
}

// Cabecera de columna ordenable: clic alterna asc/desc y muestra la flecha en la columna activa.
function SortHead({ label, col, sort, dir, onSort, numeric, align }: {
  label: string;
  col: ProductSort;
  sort: ProductSort;
  dir: 'asc' | 'desc';
  onSort: (col: ProductSort, numeric?: boolean) => void;
  numeric?: boolean;
  align?: 'end' | 'center';
}) {
  const active = sort === col;
  const justify = align === 'end' ? 'end' : align === 'center' ? 'center' : 'start';
  return (
    <Table.ColumnHeader textAlign={align} cursor="pointer" userSelect="none" onClick={() => onSort(col, numeric)}
      title={`Ordenar por ${label.toLowerCase()}`}>
      <HStack gap={1} justify={justify}>
        <Text as="span">{label}</Text>
        {active && (dir === 'asc' ? <LuArrowUp size={12} /> : <LuArrowDown size={12} />)}
      </HStack>
    </Table.ColumnHeader>
  );
}

// Duplicar producto: copia el producto de origen con TODAS sus relaciones (categoría, precio,
// grupos de modificadores, canales y receta). Solo pide un nombre distinto (no se permiten
// nombres idénticos; el backend además lo valida como 409).
function DuplicateProductDialog({ source, onClose }: { source: AdminProduct; onClose: () => void }) {
  const qc = useQueryClient();
  const [name, setName] = useState(`Copia de ${source.name}`);
  const dup = useMutation({
    mutationFn: () => adminApi.duplicateProduct(source.id, name.trim()),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['admin', 'products'] });
      qc.invalidateQueries({ queryKey: ['menu'] });
      onClose();
      toaster.create({ title: 'Producto duplicado', description: 'Se copió con sus grupos, canales y receta. Edítalo para ajustarlo.', type: 'success' });
    },
    onError: (e) => toaster.create({ title: 'No se pudo duplicar', description: String(e), type: 'error' }),
  });
  const canDup = name.trim().length > 0 && name.trim().toLowerCase() !== source.name.toLowerCase();

  return (
    <DialogRoot open onOpenChange={(e) => { if (!e.open) onClose(); }}>
      <DialogBackdrop />
      <DialogContent>
        <DialogHeader><DialogTitle>Duplicar producto</DialogTitle></DialogHeader>
        <DialogCloseTrigger />
        <DialogBody>
          <VStack align="stretch" gap={3}>
            <Text fontSize="sm" color="fg.muted">
              Crea una copia de «{source.name}» con toda su configuración (categoría, precio, grupos de
              modificadores, canales y receta). Dale un nombre distinto para empezar.
            </Text>
            <Field label="Nombre del nuevo producto">
              <Input value={name} onChange={(e) => setName(e.target.value)} autoFocus />
            </Field>
          </VStack>
        </DialogBody>
        <DialogFooter>
          <Button variant="ghost" mr={3} onClick={onClose}>Cancelar</Button>
          <Button colorPalette="green" disabled={!canDup} loading={dup.isPending} onClick={() => dup.mutate()}>Duplicar</Button>
        </DialogFooter>
      </DialogContent>
    </DialogRoot>
  );
}

// Alta de producto (nombre, categoría, precio, favorito). Invalida catálogo + menú al crear.
function NewProductDialog({ isOpen, onClose, categoryOptions }: {
  isOpen: boolean; onClose: () => void; categoryOptions: PickerOption[];
}) {
  const qc = useQueryClient();
  const [name, setName] = useState('');
  const [categoryId, setCategoryId] = useState('');
  const [price, setPrice] = useState('');
  const [favorite, setFavorite] = useState(false);

  const reset = () => { setName(''); setCategoryId(''); setPrice(''); setFavorite(false); };
  const create = useMutation({
    mutationFn: () => adminApi.createProduct({
      name: name.trim(), categoryId: Number(categoryId), price: parseFloat(price) || 0, favorite,
    }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['admin', 'products'] });
      qc.invalidateQueries({ queryKey: ['menu'] });
      reset(); onClose();
      toaster.create({ title: 'Producto creado', type: 'success' });
    },
    onError: (e) => toaster.create({ title: 'No se pudo crear', description: String(e), type: 'error' }),
  });
  const canCreate = name.trim().length > 0 && !!categoryId && (parseFloat(price) || 0) >= 0 && price !== '';

  return (
    <DialogRoot open={isOpen} onOpenChange={(e) => { if (!e.open) { onClose(); reset(); } }}>
      <DialogBackdrop />
      <DialogContent>
        <DialogHeader><DialogTitle>Nuevo producto</DialogTitle></DialogHeader>
        <DialogCloseTrigger />
        <DialogBody>
          <VStack align="stretch" gap={4}>
            <Field label="Nombre">
              <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="Ej. Latte" />
            </Field>
            <Field label="Categoría">
              <Picker value={categoryId} onChange={setCategoryId} placeholder="Elegir categoría" title="Categoría"
                options={categoryOptions} />
            </Field>
            <Field label="Precio">
              <Input type="number" inputMode="decimal" value={price} onChange={(e) => setPrice(e.target.value)} placeholder="0" />
            </Field>
            <HStack justify="space-between">
              <Text>Favorito</Text>
              <Switch checked={favorite} onCheckedChange={(e) => setFavorite(e.checked)} />
            </HStack>
            <Text fontSize="xs" color="fg.muted">
              El costo, la receta y los grupos modificadores se configuran después, al editar el producto.
            </Text>
          </VStack>
        </DialogBody>
        <DialogFooter>
          <Button variant="ghost" mr={3} onClick={() => { onClose(); reset(); }}>Cancelar</Button>
          <Button colorPalette="green" disabled={!canCreate} loading={create.isPending} onClick={() => create.mutate()}>Crear</Button>
        </DialogFooter>
      </DialogContent>
    </DialogRoot>
  );
}
