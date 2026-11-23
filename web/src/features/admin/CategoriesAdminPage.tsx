import { useState } from 'react';
import { Box, Button, Center, HStack, IconButton, SimpleGrid, Spinner, Table, Text, VStack } from '@chakra-ui/react';
import { LuChevronRight } from 'react-icons/lu';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useCategoryView, VIEWS } from './categoryView';
import { adminApi, type Category } from '../../api/admin';
import { Page } from '../../components/Page';
import { Picker } from '../../components/Picker';
import { Field } from '../../components/ui/field';
import { toaster } from '../../components/ui/toaster';
import {
  DialogRoot, DialogBackdrop, DialogContent, DialogHeader, DialogBody, DialogFooter, DialogTitle, DialogCloseTrigger,
} from '../../components/ui/dialog';
import { CatalogImage } from '../../components/CatalogImage';
import { PhotoField } from '../../shared/PhotoField';
import { FOOD_FAMILIES, familyLabel } from '../../media/foodFamilies';

// Pestaña Categorías del menú (spec 091, maqueta M3): la foto y la familia de comida de cada
// categoría, en tres vistas que el dueño eligió tener todas, con mosaico por omisión.

/** Selector de vista: vive en la barra de pestañas del menú, solo en Categorías. */
export function CategoryViewSwitch() {
  const { view, setView } = useCategoryView();
  return (
    <HStack gap={0} borderWidth="1px" borderRadius="lg" p="2px">
      {VIEWS.map((v) => {
        const I = v.icon;
        const on = v.id === view;
        return (
          <IconButton key={v.id} aria-label={`Ver como ${v.label}`} title={`Ver como ${v.label}`} aria-pressed={on}
            minW="48px" minH="44px" h="44px" variant={on ? 'solid' : 'ghost'} colorPalette={on ? undefined : 'gray'}
            onClick={() => setView(v.id)}><I /></IconButton>
        );
      })}
    </HStack>
  );
}

const famOptions = FOOD_FAMILIES.map((f) => ({ value: f.id, label: f.label }));

function useCategoryChanges() {
  const qc = useQueryClient();
  return () => {
    void qc.invalidateQueries({ queryKey: ['admin', 'categories'] });
    void qc.invalidateQueries({ queryKey: ['admin', 'products'] });
    void qc.invalidateQueries({ queryKey: ['menu'] });
  };
}

// CategoryEditor es la foto y la familia de una categoría. Todo se aplica al momento: en la vista
// de panel no hay pie con «Guardar», y nada queda pendiente al tocar otra categoría.
function CategoryEditor({ c }: { c: Category }) {
  const changed = useCategoryChanges();
  const [family, setFamily] = useState(c.foodFamily ?? '');
  const [imageUrl, setImageUrl] = useState(c.imageUrl ?? null);
  const saveFamily = useMutation({
    mutationFn: (f: string) => adminApi.setCategoryFamily(c.id, f),
    onSuccess: changed,
    onError: (e) => {
      setFamily(c.foodFamily ?? '');
      toaster.create({ title: 'No se guardó la familia', description: String(e), type: 'error' });
    },
  });
  return (
    <VStack align="stretch" gap={4}>
      <HStack align="start" gap={4}>
        <PhotoField kind="categories" id={c.id} name={c.name} imageUrl={imageUrl} family={family}
          onChanged={(u) => { setImageUrl(u); changed(); }} />
        <Text fontSize="sm" color="fg.muted" flex="1">Los productos de esta categoría que no tienen foto muestran ésta.</Text>
      </HStack>
      <Field label="Familia de comida" helperText="El dibujo que se ve cuando no hay foto.">
        <Picker value={family} options={famOptions} title="Familia de comida" clearable clearLabel="Platillo (genérica)"
          placeholder="Platillo (genérica)"
          onChange={(v) => { setFamily(v); saveFamily.mutate(v); }} />
      </Field>
    </VStack>
  );
}

function CategoryDialog({ c, onClose }: { c: Category; onClose: () => void }) {
  return (
    <DialogRoot open onOpenChange={(e) => { if (!e.open) onClose(); }} placement="center" size="md" scrollBehavior="inside">
      <DialogBackdrop />
      <DialogContent>
        <DialogHeader><DialogTitle>{c.name}</DialogTitle></DialogHeader>
        <DialogCloseTrigger />
        <DialogBody><CategoryEditor c={c} /></DialogBody>
        <DialogFooter><Button minH="44px" onClick={onClose}>Listo</Button></DialogFooter>
      </DialogContent>
    </DialogRoot>
  );
}

const thumb = (c: Category, size: number, fluid?: boolean) => (
  <CatalogImage imageUrl={c.imageUrl} family={c.foodFamily} size={size} alt="" fluid={fluid} />
);

export function CategoriesAdminPage() {
  const view = useCategoryView((s) => s.view);
  const [openId, setOpenId] = useState<number | null>(null);
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const { data, isLoading } = useQuery({ queryKey: ['admin', 'categories'], queryFn: adminApi.categories });
  const cats = data?.items ?? [];
  const nameById = new Map(cats.map((c) => [c.id, c.name]));
  const label = (c: Category) => (c.parentId ? `${nameById.get(c.parentId) ?? '—'} › ${c.name}` : c.name);
  const open = cats.find((c) => c.id === openId);

  if (isLoading) return <Center h="100%"><Spinner /></Center>;

  if (view === 'panel') {
    const sel = cats.find((c) => c.id === selectedId) ?? cats[0];
    return (
      <Page fill>
        <HStack flex="1" minH={0} align="stretch" gap={4}>
          <Box w="44%" overflowY="auto" borderWidth="1px" borderRadius="lg" bg="bg.panel">
            {cats.map((c) => (
              <HStack key={c.id} as="button" w="100%" textAlign="left" px={3} minH="56px" gap={3} borderBottomWidth="1px"
                bg={c.id === sel?.id ? 'colorPalette.subtle' : undefined} onClick={() => setSelectedId(c.id)}>
                {thumb(c, 40)}
                <Box flex="1" minW={0}>
                  <Text fontWeight="600" lineClamp={1}>{label(c)}</Text>
                  <Text fontSize="xs" color="fg.muted">{familyLabel(c.foodFamily)} · {c.productCount ?? 0}</Text>
                </Box>
              </HStack>
            ))}
          </Box>
          <Box flex="1" minW={0} overflowY="auto">
            {sel && (
              <Box p={5} borderWidth="1px" borderRadius="xl" bg="bg.panel">
                <Text fontWeight="700" fontSize="lg" mb={4}>{label(sel)}</Text>
                <CategoryEditor key={sel.id} c={sel} />
              </Box>
            )}
          </Box>
        </HStack>
      </Page>
    );
  }

  return (
    <Page fill>
      <Box flex="1" minH={0} overflowY="auto" borderWidth={view === 'tabla' ? '1px' : undefined} borderRadius="lg">
        {view === 'tabla' ? (
          <Table.Root size="sm" interactive stickyHeader>
            <Table.Header><Table.Row bg="bg.panel">
              <Table.ColumnHeader w="56px" /><Table.ColumnHeader>Categoría</Table.ColumnHeader>
              <Table.ColumnHeader>Familia</Table.ColumnHeader>
              <Table.ColumnHeader textAlign="end">Productos</Table.ColumnHeader><Table.ColumnHeader w="40px" />
            </Table.Row></Table.Header>
            <Table.Body>
              {cats.map((c) => (
                <Table.Row key={c.id} cursor="pointer" h="52px" onClick={() => setOpenId(c.id)}>
                  <Table.Cell py={1}>{thumb(c, 40)}</Table.Cell>
                  <Table.Cell fontWeight="600">{label(c)}</Table.Cell>
                  <Table.Cell color={c.foodFamily ? undefined : 'fg.muted'}>{familyLabel(c.foodFamily)}</Table.Cell>
                  <Table.Cell textAlign="end">{c.productCount ?? 0}</Table.Cell>
                  <Table.Cell><LuChevronRight /></Table.Cell>
                </Table.Row>
              ))}
            </Table.Body>
          </Table.Root>
        ) : (
          <SimpleGrid gap={3} css={{ gridTemplateColumns: 'repeat(auto-fill, minmax(150px, 1fr))' }}>
            {cats.map((c) => (
              <Box key={c.id} as="button" textAlign="left" borderWidth="1px" borderRadius="xl" overflow="hidden"
                bg="bg.panel" onClick={() => setOpenId(c.id)}>
                {thumb(c, 200, true)}
                <Box p={2}>
                  <Text fontWeight="600" lineClamp={1}>{label(c)}</Text>
                  <Text fontSize="xs" color="fg.muted">{c.productCount ?? 0} productos</Text>
                </Box>
              </Box>
            ))}
          </SimpleGrid>
        )}
      </Box>
      {open && <CategoryDialog c={open} onClose={() => setOpenId(null)} />}
    </Page>
  );
}
