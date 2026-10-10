import { useState } from 'react';
import { Flex, Box, HStack, Button, IconButton, Text, VStack } from '@chakra-ui/react';
import { LuTag, LuStar, LuClipboardList, LuCircleHelp } from 'react-icons/lu';
import { Outlet, useLocation, useNavigate } from 'react-router';
import { DialogRoot, DialogBackdrop, DialogContent, DialogBody } from '../../components/ui/dialog';

// Hub "Menú" (la ruta sigue siendo /catalogo): agrupa Productos, Extras y Recetas bajo un solo item
// del menú, con pestañas de 1 toque. Cada pestaña conserva su propia pantalla (Page fill).
//
// La ayuda de cada pestaña vive detrás de un icono y no como texto fijo: quien ya sabe no la
// necesita y el alto de la tableta es para la lista.
const TABS = [
  {
    to: '/catalogo/productos', label: 'Productos', icon: LuTag,
    what: 'Lo que vendes y su precio.',
    steps: [
      'Toca un producto para cambiar su nombre, precio, categoría o foto.',
      '«Nuevo producto» agrega uno.',
      'Archiva lo que ya no vendes: deja de salir en Vender y sus ventas pasadas se conservan.',
    ],
  },
  {
    to: '/catalogo/opciones', label: 'Extras', icon: LuStar,
    what: 'Lo que se le agrega o se le quita a un producto: toppings, tamaños, «sin hielo».',
    steps: [
      'Los extras van en grupos (Toppings, Leches…). Un grupo se liga a los productos que lo ofrecen.',
      'La estrella marca un extra como favorito para que aparezca arriba al vender.',
      'Un extra sin costo se cobra en $0; uno con precio suma al producto.',
    ],
  },
  {
    to: '/catalogo/recetas', label: 'Recetas', icon: LuClipboardList,
    what: 'Qué sale del almacén cada vez que vendes algo.',
    steps: [
      'Empieza por «Pendientes»: están ordenadas por lo que más vendes.',
      '«Por revisar» son recetas que se cargaron solas. Ábrelas y toca «Está bien» si son correctas.',
      'Un preparado es algo que hacen ustedes con otros insumos, como una salsa o una masa.',
      'Mientras falte la receta de un producto, venderlo no descuenta nada del almacén.',
    ],
  },
];

export function CatalogPage() {
  const nav = useNavigate();
  const { pathname } = useLocation();
  const [help, setHelp] = useState(false);
  const current = TABS.find((t) => pathname.startsWith(t.to)) ?? TABS[0];

  return (
    <Flex direction="column" h="100%">
      <Box flexShrink={0} borderBottomWidth="1px" bg="bg.panel">
        <HStack maxW="1150px" mx="auto" px={6} pt={4} gap={2}>
          {TABS.map((t) => {
            const Icon = t.icon;
            const active = t === current;
            return (
              <Button key={t.to} size="md" minH="44px" borderBottomRadius={0} onClick={() => nav(t.to)}
                variant={active ? 'solid' : 'ghost'} colorPalette={active ? undefined : 'gray'}>
                <Icon /> {t.label}
              </Button>
            );
          })}
          <Box flex="1" />
          <IconButton aria-label={`¿Para qué sirve ${current.label}?`} variant="ghost" colorPalette="gray" color="fg.muted"
            minH="44px" minW="44px" onClick={() => setHelp(true)}>
            <LuCircleHelp />
          </IconButton>
        </HStack>
      </Box>
      <Box flex="1" minH={0}>
        <Outlet />
      </Box>

      <DialogRoot open={help} onOpenChange={(e) => setHelp(e.open)} placement="center" size="sm">
        <DialogBackdrop />
        <DialogContent mx={4} borderRadius="2xl">
          <DialogBody py={6}>
            <Text fontWeight="700" fontSize="lg" mb={1}>{current.label}</Text>
            <Text color="fg.muted" mb={4}>{current.what}</Text>
            <VStack align="stretch" gap={3}>
              {current.steps.map((s, i) => (
                <HStack key={s} align="start" gap={3}>
                  <Box flexShrink={0} w="24px" h="24px" borderRadius="full" bg="colorPalette.100" color="colorPalette.800"
                    fontSize="sm" fontWeight="700" display="flex" alignItems="center" justifyContent="center">{i + 1}</Box>
                  <Text fontSize="sm">{s}</Text>
                </HStack>
              ))}
            </VStack>
            <Button mt={5} w="100%" minH="44px" onClick={() => setHelp(false)}>Entendido</Button>
          </DialogBody>
        </DialogContent>
      </DialogRoot>
    </Flex>
  );
}
