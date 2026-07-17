import { useMemo, useState } from 'react';
import {
  Box, Heading, Table, Center, Spinner, Input, Badge, Button,
  HStack, useDisclosure, Text,
} from '@chakra-ui/react';
import { LuStar } from 'react-icons/lu';
import { toaster } from '../../components/ui/toaster';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { adminApi, type AdminProduct } from '../../api/admin';
import { money, normalize } from '../../utils/format';
import { ProductEditDialog } from './ProductEditDialog';

export function ProductsAdminPage() {
  const qc = useQueryClient();
  const { data, isLoading } = useQuery({ queryKey: ['admin', 'products'], queryFn: adminApi.products });
  const [search, setSearch] = useState('');
  const [status, setStatus] = useState<'act' | 'inact' | 'all'>('act');
  const [edit, setEdit] = useState<AdminProduct | null>(null);
  const modal = useDisclosure();

  const counts = useMemo(() => {
    const list = data?.items ?? [];
    const act = list.filter((p) => p.is_active).length;
    return { act, inact: list.length - act, all: list.length };
  }, [data]);

  const filtered = useMemo(() => {
    let list = data?.items ?? [];
    if (status !== 'all') list = list.filter((p) => (status === 'act' ? p.is_active : !p.is_active));
    if (search.trim()) {
      const q = normalize(search);
      list = list.filter((p) => normalize(p.name).includes(q));
    }
    return list.slice(0, 200);
  }, [data, search, status]);

  const toggleActive = useMutation({
    mutationFn: (p: AdminProduct) =>
      adminApi.updateProduct(p.id, {
        name: p.name, price: p.price, favorite: p.is_favorite, active: !p.is_active,
        availableFrom: p.availableFrom, availableUntil: p.availableUntil,
      }),
    onSuccess: (_d, p) => {
      qc.invalidateQueries({ queryKey: ['admin', 'products'] });
      qc.invalidateQueries({ queryKey: ['menu'] });
      toaster.create({ title: p.is_active ? 'Producto archivado' : 'Producto reactivado', type: 'success' });
    },
    onError: (e) => toaster.create({ title: 'Error', description: String(e), type: 'error' }),
  });

  if (isLoading) return <Center h="60vh"><Spinner size="xl" /></Center>;

  const TABS: Array<{ k: typeof status; label: string; n: number }> = [
    { k: 'act', label: 'Activos', n: counts.act },
    { k: 'inact', label: 'Inactivos', n: counts.inact },
    { k: 'all', label: 'Todos', n: counts.all },
  ];

  const openEdit = (p: AdminProduct) => { setEdit({ ...p }); modal.onOpen(); };

  return (
    <Box p={6} maxW="960px">
      <Heading size="lg" mb={4}>Productos</Heading>
      <HStack mb={4} gap={3} wrap="wrap">
        <Input placeholder="Buscar…" value={search} onChange={(e) => setSearch(e.target.value)} maxW="320px" bg="bg.panel" />
        <HStack gap={1} bg="bg.muted" p={1} borderRadius="lg">
          {TABS.map((t) => (
            <Button key={t.k} size="sm" variant={status === t.k ? 'solid' : 'ghost'}
              colorPalette={status === t.k ? undefined : 'gray'} onClick={() => setStatus(t.k)}>
              {t.label} <Text as="span" opacity={0.7} ml={1}>{t.n}</Text>
            </Button>
          ))}
        </HStack>
      </HStack>

      <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" overflowX="auto">
        <Table.Root size="sm">
          <Table.Header><Table.Row><Table.ColumnHeader>Producto</Table.ColumnHeader><Table.ColumnHeader>Categoría</Table.ColumnHeader><Table.ColumnHeader textAlign="end">Precio</Table.ColumnHeader><Table.ColumnHeader textAlign="end">Costo</Table.ColumnHeader><Table.ColumnHeader textAlign="end">Margen</Table.ColumnHeader><Table.ColumnHeader></Table.ColumnHeader></Table.Row></Table.Header>
          <Table.Body>
            {filtered.map((p) => (
              <Table.Row key={p.id} opacity={p.is_active ? 1 : 0.5}>
                <Table.Cell>
                  {p.is_favorite && (
                    <Box as="span" color="yellow.500" mr={1} display="inline-flex" verticalAlign="middle"><LuStar size={12} /></Box>
                  )}
                  {p.name}
                </Table.Cell>
                <Table.Cell>{p.category}</Table.Cell>
                <Table.Cell textAlign="end">{money(p.price)}</Table.Cell>
                <Table.Cell textAlign="end">{money(p.current_cost)}</Table.Cell>
                <Table.Cell textAlign="end">{money(p.price - p.current_cost)}</Table.Cell>
                <Table.Cell>
                  <HStack justify="end" gap={2}>
                    {!p.is_active && <Badge>inactivo</Badge>}
                    <Button size="xs" variant={p.is_active ? 'outline' : 'solid'}
                      colorPalette={p.is_active ? 'gray' : 'green'}
                      loading={toggleActive.isPending && toggleActive.variables?.id === p.id}
                      onClick={() => toggleActive.mutate(p)}>
                      {p.is_active ? 'Archivar' : 'Reactivar'}
                    </Button>
                    <Button size="xs" onClick={() => openEdit(p)}>Editar</Button>
                  </HStack>
                </Table.Cell>
              </Table.Row>
            ))}
          </Table.Body>
        </Table.Root>
      </Box>
      {(data?.items.length ?? 0) > 200 && <Text mt={2} color="fg.muted" fontSize="sm">Mostrando 200; usa la búsqueda para filtrar.</Text>}

      <ProductEditDialog product={edit} isOpen={modal.open} onClose={modal.onClose} />
    </Box>
  );
}
