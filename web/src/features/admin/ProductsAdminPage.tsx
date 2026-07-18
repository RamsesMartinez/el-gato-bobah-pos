import { useEffect, useState } from 'react';
import {
  Box, Table, Center, Spinner, Input, Badge, Button, ButtonGroup,
  IconButton, HStack, Pagination, useDisclosure, Text,
} from '@chakra-ui/react';
import { LuStar, LuChevronLeft, LuChevronRight, LuSettings2, LuArrowUp, LuArrowDown } from 'react-icons/lu';
import { toaster } from '../../components/ui/toaster';
import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query';
import { adminApi, type AdminProduct } from '../../api/admin';
import { money } from '../../utils/format';
import { Page } from '../../components/Page';
import { ConfirmPopover } from '../../components/ui/confirm-popover';
import { ProductEditDialog } from './ProductEditDialog';
import { ProductGroupsDialog } from './ProductGroupsDialog';

const PAGE_SIZE = 25;

type GroupsFilter = '' | 'some' | 'none';

export function ProductsAdminPage() {
  const qc = useQueryClient();
  const [search, setSearch] = useState('');
  const [dsearch, setDsearch] = useState(''); // debounced: 1 request por pausa de tecleo, no por tecla
  const [status, setStatus] = useState<'act' | 'inact' | 'all'>('act');
  const [groupsFilter, setGroupsFilter] = useState<GroupsFilter>(''); // con/sin grupos
  const [sort, setSort] = useState<'name' | 'groups'>('name');
  const [dir, setDir] = useState<'asc' | 'desc'>('desc');
  const [page, setPage] = useState(1); // 1-based, como el paginador de Chakra
  const [edit, setEdit] = useState<AdminProduct | null>(null);
  const [groupsProduct, setGroupsProduct] = useState<AdminProduct | null>(null);
  const modal = useDisclosure();

  useEffect(() => {
    const t = setTimeout(() => { setDsearch(search); setPage(1); }, 300); // nueva búsqueda → página 1
    return () => clearTimeout(t);
  }, [search]);
  // los demás filtros resetean la página en su propio handler (sin efecto → sin renders en cascada).

  const { data, isLoading } = useQuery({
    queryKey: ['admin', 'products', { status, dsearch, groupsFilter, sort, dir, page }],
    queryFn: () => adminApi.products({
      status, search: dsearch,
      groups: groupsFilter || undefined,
      sort: sort === 'groups' ? 'groups' : undefined,
      dir: sort === 'groups' ? dir : undefined,
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
        name: p.name, price: p.price, favorite: p.is_favorite, active,
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

  if (isLoading) return <Center h="60vh"><Spinner size="xl" /></Center>;

  const TABS: Array<{ k: typeof status; label: string; n: number }> = [
    { k: 'act', label: 'Activos', n: counts.act },
    { k: 'inact', label: 'Inactivos', n: counts.inact },
    { k: 'all', label: 'Todos', n: counts.all },
  ];
  const GROUP_FILTERS: Array<{ k: GroupsFilter; label: string }> = [
    { k: '', label: 'Todos' },
    { k: 'some', label: 'Con grupos' },
    { k: 'none', label: 'Sin grupos' },
  ];

  // 1er clic en «Grupos» → orden desc; 2º → asc; 3º → vuelve a nombre.
  const toggleGroupSort = () => {
    if (sort !== 'groups') { setSort('groups'); setDir('desc'); }
    else if (dir === 'desc') setDir('asc');
    else setSort('name');
    setPage(1);
  };

  const openEdit = (p: AdminProduct) => { setEdit({ ...p }); modal.onOpen(); };

  return (
    <Page maxW="1150px" fill>
      <HStack mb={4} gap={3} wrap="wrap" flexShrink={0}>
        <Input placeholder="Buscar…" value={search} onChange={(e) => setSearch(e.target.value)} maxW="280px" bg="bg.panel" />
        <HStack gap={1} bg="bg.muted" p={1} borderRadius="lg">
          {TABS.map((t) => (
            <Button key={t.k} size="sm" variant={status === t.k ? 'solid' : 'ghost'}
              colorPalette={status === t.k ? undefined : 'gray'} onClick={() => { setStatus(t.k); setPage(1); }}>
              {t.label} <Text as="span" opacity={0.7} ml={1}>{t.n}</Text>
            </Button>
          ))}
        </HStack>
        <HStack gap={1} bg="bg.muted" p={1} borderRadius="lg">
          {GROUP_FILTERS.map((f) => (
            <Button key={f.k} size="sm" variant={groupsFilter === f.k ? 'solid' : 'ghost'}
              colorPalette={groupsFilter === f.k ? undefined : 'gray'} onClick={() => { setGroupsFilter(f.k); setPage(1); }}>
              {f.label}
            </Button>
          ))}
        </HStack>
      </HStack>

      <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" overflow="auto" flex="1" minH={0}>
        <Table.Root size="sm" stickyHeader>
          <Table.Header>
            <Table.Row bg="bg.panel">
              <Table.ColumnHeader>Producto</Table.ColumnHeader>
              <Table.ColumnHeader>Categoría</Table.ColumnHeader>
              <Table.ColumnHeader textAlign="end">Precio</Table.ColumnHeader>
              <Table.ColumnHeader textAlign="end">Costo</Table.ColumnHeader>
              <Table.ColumnHeader textAlign="end">Margen</Table.ColumnHeader>
              <Table.ColumnHeader textAlign="center" cursor="pointer" userSelect="none" onClick={toggleGroupSort}
                title="Ordenar por cantidad de grupos">
                <HStack gap={1} justify="center">
                  <Text as="span">Grupos</Text>
                  {sort === 'groups' && (dir === 'asc' ? <LuArrowUp size={12} /> : <LuArrowDown size={12} />)}
                </HStack>
              </Table.ColumnHeader>
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
                <Table.Cell textAlign="end">{money(p.price - p.current_cost)}</Table.Cell>
                <Table.Cell textAlign="center">
                  <Text as="span" color={p.groupCount === 0 ? 'fg.subtle' : undefined}
                    fontWeight={p.groupCount === 0 ? undefined : '600'}>
                    {p.groupCount}
                  </Text>
                </Table.Cell>
                <Table.Cell>
                  <HStack justify="end" gap={2}>
                    {!p.is_active && <Badge>inactivo</Badge>}
                    {p.is_active ? (
                      <ConfirmPopover title="¿Archivar producto?"
                        description="Se ocultará del POS. Puedes deshacerlo enseguida o reactivarlo cuando quieras."
                        onConfirm={() => setActive.mutate({ p, active: false })}>
                        <Button size="xs" variant="outline" colorPalette="gray">Archivar</Button>
                      </ConfirmPopover>
                    ) : (
                      <Button size="xs" variant="solid" colorPalette="green"
                        loading={setActive.isPending && setActive.variables?.p.id === p.id}
                        onClick={() => setActive.mutate({ p, active: true })}>
                        Reactivar
                      </Button>
                    )}
                    <Button size="xs" variant="outline" onClick={() => setGroupsProduct(p)}>Grupos</Button>
                    <Button size="xs" onClick={() => openEdit(p)}>Editar</Button>
                  </HStack>
                </Table.Cell>
              </Table.Row>
            ))}
          </Table.Body>
        </Table.Root>
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
    </Page>
  );
}
