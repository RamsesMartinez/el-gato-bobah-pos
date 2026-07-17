import { useMemo, useState } from 'react';
import {
  Box, Flex, VStack, HStack, Text, Button, Spinner, Center, IconButton, useDisclosure,
} from '@chakra-ui/react';
import { LuShoppingCart, LuChevronUp, LuCircleCheck, LuPrinter, LuEye, LuEyeOff, LuPencil } from 'react-icons/lu';
import { useQuery } from '@tanstack/react-query';
import { DrawerRoot, DrawerBackdrop, DrawerContent } from '../../components/ui/drawer';
import { DialogRoot, DialogBackdrop, DialogContent, DialogBody } from '../../components/ui/dialog';
import { useMenu } from '../../hooks/useMenu';
import { usePopular } from '../../hooks/usePopular';
import { useModifierDefaults } from '../../hooks/useModifierDefaults';
import { useContainerWidth } from '../../hooks/useContainerWidth';
import { useTicketStore, useActiveTicket, ticketTotal, ticketCount } from '../../stores/ticket';
import { useUiStore } from '../../stores/ui';
import { useSessionStore } from '../../stores/session';
import { adminApi, type AdminProduct } from '../../api/admin';
import { ProductEditDialog } from '../admin/ProductEditDialog';
import type { MenuProduct, OrderView, TicketLine, TicketModifier } from '../../types/pos';
import { money, normalize } from '../../utils/format';
import { printReceipt } from '../../utils/printReceipt';
import { CategoryRail, type Selection } from './CategoryRail';
import { TicketTabs } from './TicketTabs';
import { SearchBar } from './SearchBar';
import { ProductGrid } from './ProductGrid';
import { ModifierSheet } from './ModifierSheet';
import { Ticket } from './Ticket';
import { CheckoutSheet } from './CheckoutSheet';

export function POSPage() {
  const { data: menu, isLoading, error } = useMenu();
  const { data: popular } = usePopular();
  const { data: modifierDefaults } = useModifierDefaults();
  const { ref, width } = useContainerWidth<HTMLDivElement>();
  const wide = width >= 900;

  const palette = useUiStore((s) => s.palette);
  const topCount = useUiStore((s) => s.topCount);
  const role = useSessionStore((s) => s.user?.role);
  const canEdit = role === 'admin' || role === 'gerente';
  const lines = useActiveTicket().lines;
  const addLine = useTicketStore((s) => s.addLine);
  const updateLineModifiers = useTicketStore((s) => s.updateLineModifiers);

  const [selection, setSelection] = useState<Selection>({ kind: 'top' });
  const [search, setSearch] = useState('');
  // preferencia por dispositivo: mostrar/ocultar precios en las cards (menos ruido visual).
  const [showPrices, setShowPrices] = useState(() => localStorage.getItem('pos.showPrices') !== '0');
  const togglePrices = () =>
    setShowPrices((v) => { localStorage.setItem('pos.showPrices', v ? '0' : '1'); return !v; });
  const [modProduct, setModProduct] = useState<MenuProduct | null>(null);
  const [editing, setEditing] = useState<TicketLine | null>(null);
  const [lastOrder, setLastOrder] = useState<OrderView | null>(null);
  // modo editar: reutiliza el grid del POS para editar productos (admin/gerente).
  const [editMode, setEditMode] = useState(false);
  const [editProduct, setEditProduct] = useState<AdminProduct | null>(null);
  // catálogo admin (con todos los campos editables); solo se carga en modo editar.
  const { data: adminProducts } = useQuery({
    queryKey: ['admin', 'products'], queryFn: adminApi.products, enabled: editMode && canEdit,
  });

  const ticketDrawer = useDisclosure();
  const checkout = useDisclosure();
  const modSheet = useDisclosure();

  // defensivo: nunca asumir que vienen arreglos (catálogo vacío o respuesta parcial)
  const allCategories = menu?.categories ?? [];
  const allProducts = menu?.products ?? [];

  const childrenByRoot = useMemo(() => {
    const m: Record<number, number[]> = {};
    allCategories.forEach((c) => {
      if (c.parentId !== null) (m[c.parentId] ??= []).push(c.id);
    });
    return m;
  }, [allCategories]);

  const products = useMemo(() => {
    if (search.trim()) {
      const q = normalize(search);
      return allProducts.filter((p) => normalize(p.name).includes(q));
    }
    if (selection.kind === 'top') {
      // más vendidos (orden del backend). Fallback: favoritos, luego los primeros del catálogo,
      // para que un negocio recién estrenado no vea la pestaña vacía.
      const byId = new Map(allProducts.map((p) => [p.id, p]));
      const ranked = (popular ?? [])
        .map((id) => byId.get(id))
        .filter((p): p is MenuProduct => p !== undefined);
      const base = ranked.length ? ranked : allProducts.filter((p) => p.favorite);
      return (base.length ? base : allProducts).slice(0, topCount);
    }
    const { rootId, subId } = selection;
    if (subId !== null) return allProducts.filter((p) => p.categoryId === subId);
    const cats = new Set<number>([rootId, ...(childrenByRoot[rootId] ?? [])]);
    return allProducts.filter((p) => cats.has(p.categoryId));
  }, [allProducts, search, selection, childrenByRoot, popular, topCount]);

  const counts = useMemo(() => {
    const c: Record<number, number> = {};
    lines.forEach((l) => (c[l.productId] = (c[l.productId] ?? 0) + l.qty));
    return c;
  }, [lines]);

  const total = ticketTotal(lines);
  const count = ticketCount(lines);

  const tapProduct = (p: MenuProduct) => {
    if (editMode && canEdit) {
      const ap = adminProducts?.items.find((x) => x.id === p.id);
      if (ap) setEditProduct(ap);
      return;
    }
    if (p.groups.length > 0) {
      setEditing(null);
      setModProduct(p);
      modSheet.onOpen();
    } else {
      addLine({ productId: p.id, name: p.name, unitPrice: p.price, qty: 1, modifiers: [] });
    }
  };

  const editLine = (line: TicketLine) => {
    const p = allProducts.find((x) => x.id === line.productId);
    if (!p || p.groups.length === 0) return; // sin modificadores no hay nada que editar
    setEditing(line);
    setModProduct(p);
    modSheet.onOpen();
  };

  const confirmModifiers = (modifiers: TicketModifier[], notes: string, qty: number) => {
    if (!modProduct) return;
    if (editing) {
      updateLineModifiers(editing.lineId, modifiers, notes || undefined);
    } else {
      for (let i = 0; i < qty; i++) {
        addLine({ productId: modProduct.id, name: modProduct.name, unitPrice: modProduct.price, qty: 1, modifiers, notes: notes || undefined });
      }
    }
    setEditing(null);
    setModProduct(null);
  };

  if (isLoading) return <Center h="80vh"><Spinner size="xl" /></Center>;
  if (error) return <Center h="80vh"><Text color="red.500">Error cargando el menú</Text></Center>;

  const catalog = (
    <VStack align="stretch" gap={2} h="100%" overflow="hidden">
      <Box px={{ base: 3, md: 4 }} pt={3}>
        <TicketTabs />
      </Box>
      <Box px={{ base: 3, md: 4 }}>
        <HStack gap={2}>
          <Box flex="1"><SearchBar value={search} onChange={setSearch} /></Box>
          <IconButton
            aria-label={showPrices ? 'Ocultar precios' : 'Mostrar precios'}
            size="lg" variant={showPrices ? 'outline' : 'solid'}
            colorPalette={showPrices ? 'gray' : undefined}
            onClick={togglePrices}
          >
            {showPrices ? <LuEye /> : <LuEyeOff />}
          </IconButton>
          {canEdit && (
            <IconButton
              aria-label={editMode ? 'Salir de edición' : 'Editar productos'}
              size="lg" variant={editMode ? 'solid' : 'outline'}
              colorPalette={editMode ? 'orange' : 'gray'}
              onClick={() => setEditMode((v) => !v)}
            >
              <LuPencil />
            </IconButton>
          )}
        </HStack>
      </Box>
      {editMode && (
        <Box mx={{ base: 3, md: 4 }} px={3} py={2} borderRadius="md" bg="orange.500" color="white" fontWeight="600" fontSize="sm">
          Modo edición — toca un producto para editarlo (los cambios se ven al instante)
        </Box>
      )}
      <Box px={{ base: 3, md: 4 }}>
        {!search && <CategoryRail categories={allCategories} selection={selection} onSelect={setSelection} />}
      </Box>
      <Box flex="1" overflowY="auto" px={{ base: 3, md: 4 }} css={{ overscrollBehavior: 'contain' }}>
        <ProductGrid products={products} counts={counts} onTap={tapProduct} showPrice={showPrices} />
      </Box>
    </VStack>
  );

  return (
    <Box ref={ref} h="100%" bg="bg.subtle">
      {wide ? (
        <Flex h="100%">
          <Box flex="1" minW={0}>{catalog}</Box>
          <Box w="clamp(300px, 32%, 380px)" borderLeftWidth="1px" borderColor="border">
            <Ticket onCheckout={checkout.onOpen} onEditLine={editLine} />
          </Box>
        </Flex>
      ) : (
        <Flex direction="column" h="100%">
          <Box flex="1" minH={0}>{catalog}</Box>
          <HStack
            as="button" onClick={ticketDrawer.onOpen}
            h="64px" px={4} bg="colorPalette.600" color="white" justify="space-between"
            display={count > 0 ? 'flex' : 'none'}
          >
            <HStack gap={2}>
              <LuShoppingCart />
              <Text fontWeight="700">{count} art · {money(total)}</Text>
            </HStack>
            <HStack gap={1}>
              <Text fontWeight="600">Ver pedido</Text>
              <LuChevronUp />
            </HStack>
          </HStack>
        </Flex>
      )}

      {/* Ticket como bottom sheet en modo angosto */}
      <DrawerRoot open={ticketDrawer.open} placement="bottom" onOpenChange={(e) => { if (!e.open) ticketDrawer.onClose(); }} size="full">
        <DrawerBackdrop />
        <DrawerContent colorPalette={palette} borderTopRadius={{ base: 0, md: '2xl' }} maxH={{ base: '100dvh', md: '92vh' }}>
          <Box h={{ base: '100dvh', md: '92vh' }}>
            <Ticket onCheckout={() => { ticketDrawer.onClose(); checkout.onOpen(); }} onEditLine={editLine} />
          </Box>
        </DrawerContent>
      </DrawerRoot>

      <ModifierSheet
        product={modProduct}
        isOpen={modSheet.open}
        optionRanks={modProduct ? modifierDefaults?.[String(modProduct.id)] : undefined}
        initialModifiers={editing?.modifiers}
        initialNotes={editing?.notes}
        onClose={() => { modSheet.onClose(); setEditing(null); setModProduct(null); }}
        onConfirm={confirmModifiers}
      />

      <CheckoutSheet
        isOpen={checkout.open}
        onClose={checkout.onClose}
        onDone={(order) => { checkout.onClose(); ticketDrawer.onClose(); setLastOrder(order); }}
      />

      {/* Modo editar: editor de producto reutilizando el grid del POS */}
      <ProductEditDialog
        product={editProduct}
        isOpen={editProduct !== null}
        onClose={() => setEditProduct(null)}
      />

      {/* Confirmación */}
      <DialogRoot open={lastOrder !== null} onOpenChange={(e) => { if (!e.open) setLastOrder(null); }} placement="center" size={{ base: 'full', md: 'sm' }}>
        <DialogBackdrop />
        <DialogContent colorPalette={palette} mx={{ base: 0, md: 4 }}>
          <DialogBody py={10} textAlign="center">
            <Center color="green.500" mb={2}><LuCircleCheck size={72} /></Center>
            <Text fontSize="2xl" fontWeight="800">Pedido #{lastOrder?.number}</Text>
            <Text color="fg.muted" mb={6}>Registrado correctamente</Text>
            <VStack gap={2}>
              <Button size="lg" w="100%" onClick={() => setLastOrder(null)}>
                Nuevo pedido
              </Button>
              <Button size="md" variant="outline" w="100%" onClick={() => lastOrder && printReceipt(lastOrder)}>
                <LuPrinter /> Imprimir ticket
              </Button>
            </VStack>
          </DialogBody>
        </DialogContent>
      </DialogRoot>
    </Box>
  );
}
