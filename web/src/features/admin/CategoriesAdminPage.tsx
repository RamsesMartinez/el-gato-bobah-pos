import { useState } from 'react';
import { Box, Button, Center, HStack, IconButton, SimpleGrid, Spinner, Table, Text, VStack } from '@chakra-ui/react';
import { LuChevronRight } from 'react-icons/lu';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useCategoryView, VIEWS } from './categoryView';
import { useUiStore } from '../../stores/ui';
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
    <HStack gap={0} borderWidth="1px" borderRadius="lg" p="2px" role="group" aria-label="Vista de categorías">
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

// «Platillo» ya es la opción genérica del selector: ofrecerla dos veces obligaba a adivinar cuál.
const famOptions = FOOD_FAMILIES.filter((f) => f.id !== 'dish').map((f) => ({ value: f.id, label: f.label }));
const productos = (n: number) => (n === 1 ? '1 producto' : `${n} productos`);

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
    // Lo que había antes de ESTE cambio, no lo de la última consulta: con dos cambios seguidos, el
    // segundo que falla regresa al primero, que sí se guardó.
    onMutate: (f: string) => {
      const prev = family;
      setFamily(f);
      return { prev };
    },
    onSuccess: changed,
    onError: (_e, _f, ctx) => {
      setFamily(ctx?.prev ?? '');
      toaster.create({ title: 'No se guardó la familia', description: 'Revisa la conexión e intenta de nuevo.', type: 'error' });
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
          onChange={(v) => saveFamily.mutate(v)} />
      </Field>
    </VStack>
  );
}

function CategoryDialog({ c, onClose }: { c: Category; onClose: () => void }) {
  const palette = useUiStore((st) => st.palette);
  return (
    <DialogRoot open onOpenChange={(e) => { if (!e.open) onClose(); }} placement="center" size="md" scrollBehavior="inside">
      <DialogBackdrop />
      {/* La paleta del negocio, como el editor de producto: el botón de la foto es del mismo color
          en las dos pantallas. */}
      <DialogContent colorPalette={palette}>
        <DialogHeader><DialogTitle>{c.name}</DialogTitle></DialogHeader>
        <DialogCloseTrigger />
        <DialogBody><CategoryEditor c={c} /></DialogBody>
        <DialogFooter><Button minH="44px" onClick={onClose}>Listo</Button></DialogFooter>
      </DialogContent>
    </DialogRoot>
  );
}

// Una subcategoría sin foto o sin familia propias muestra las de su categoría padre, igual que sus
// productos (el servidor hereda lo mismo en la lista de productos).
function withParent(c: Category, byId: Map<number, Category>): Category {
  const p = c.parentId ? byId.get(c.parentId) : undefined;
  if (!p) return c;
  return { ...c, imageUrl: c.imageUrl ?? p.imageUrl, foodFamily: c.foodFamily || p.foodFamily };
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
  const byId = new Map(cats.map((c) => [c.id, c]));
  const nameById = new Map(cats.map((c) => [c.id, c.name]));
  const eff = (c: Category) => withParent(c, byId);
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
                aria-current={c.id === sel?.id ? 'true' : undefined}
                // La elegida se marca con fondo Y con una barra: el color solo no basta para saber cuál
                // se está editando a la derecha.
                bg={c.id === sel?.id ? 'bg.emphasized' : undefined} borderLeftWidth="4px"
                borderLeftColor={c.id === sel?.id ? 'brand.solid' : 'transparent'} onClick={() => setSelectedId(c.id)}>
                {thumb(eff(c), 40)}
                <Box flex="1" minW={0}>
                  <Text fontWeight="600" lineClamp={1}>{label(c)}</Text>
                  <Text fontSize="xs" color="fg.muted">{familyLabel(eff(c).foodFamily)} · {productos(c.productCount ?? 0)}</Text>
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
                <Table.Row key={c.id} cursor="pointer" h="52px" tabIndex={0} aria-label={`Abrir ${label(c)}`}
                  onClick={() => setOpenId(c.id)}
                  onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); setOpenId(c.id); } }}>
                  <Table.Cell py={1}>{thumb(eff(c), 40)}</Table.Cell>
                  <Table.Cell fontWeight="600">{label(c)}</Table.Cell>
                  <Table.Cell color={c.foodFamily ? undefined : 'fg.muted'}>
                    {c.foodFamily || !eff(c).foodFamily ? familyLabel(c.foodFamily) : `${familyLabel(eff(c).foodFamily)} (de ${nameById.get(c.parentId!)})`}
                  </Table.Cell>
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
                {thumb(eff(c), 200, true)}
                {/* El nombre propio arriba y el padre abajo: «Bebidas frías › …» cortado no deja
                    distinguir una subcategoría de otra. */}
                <Box p={2}>
                  <Text fontWeight="600" lineClamp={1}>{c.name}</Text>
                  <Text fontSize="xs" color="fg.muted" lineClamp={1}>
                    {c.parentId ? `${nameById.get(c.parentId) ?? ''} · ` : ''}{productos(c.productCount ?? 0)}
                  </Text>
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
