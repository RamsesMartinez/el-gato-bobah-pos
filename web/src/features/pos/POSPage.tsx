import { useEffect, useMemo, useRef, useState, type PointerEvent } from 'react';
import {
  Box, Flex, VStack, HStack, Text, Button, Spinner, Center, IconButton, useDisclosure,
} from '@chakra-ui/react';
import { LuShoppingCart, LuChevronUp, LuCircleCheck, LuCircleAlert, LuPrinter, LuEye, LuEyeOff, LuPencil, LuPanelRightOpen, LuGripVertical, LuTriangleAlert, LuWallet } from 'react-icons/lu';
import { useQuery } from '@tanstack/react-query';
import { useNavigate } from 'react-router';
import { posApi } from '../../api/pos';
import { canAccess } from '../../app/roles';
import { DrawerRoot, DrawerBackdrop, DrawerContent, DrawerGrabber } from '../../components/ui/drawer';
import { useSwipeDownToClose } from '../../hooks/useSwipeDownToClose';
import { DialogRoot, DialogBackdrop, DialogContent, DialogBody } from '../../components/ui/dialog';
import { useMenu } from '../../hooks/useMenu';
import { useMenuEvents } from '../../hooks/useMenuEvents';
import { usePopular } from '../../hooks/usePopular';
import { useModifierDefaults } from '../../hooks/useModifierDefaults';
import { useContainerWidth } from '../../hooks/useContainerWidth';
import { useTicketStore, useActiveTicket, ticketTotal, ticketCount } from '../../stores/ticket';
import { useMandarPedido } from './useMandarPedido';
import { useAgregarAPedido } from './useAgregarAPedido';
import { useUiStore } from '../../stores/ui';
import { useSessionStore } from '../../stores/session';
import { adminApi, type AdminProduct } from '../../api/admin';
import { ProductEditDialog } from '../admin/ProductEditDialog';
import type { BoardOrder, MenuProduct, OrderView, TicketLine, TicketModifier } from '../../types/pos';
import { money } from '../../utils/format';
import { TicketPreview } from '../tickets/TicketPreview';
import { AutoPrintTicket, KitchenTicket } from '../tickets/AutoPrintTicket';
import { buscarProductos } from './buscarProducto';
import { CategoryRail, type Selection } from './CategoryRail';
import { PlatformPicker } from './PlatformPicker';
import { PlatformPriceDialog } from './PlatformPriceDialog';
import { desglosePrecio, nombreDeLista, precioDeLista } from './precioPlataforma';
import { TicketTabs } from './TicketTabs';
import { SearchBar } from './SearchBar';
import { PedidosEnCurso } from './PedidosEnCurso';
import { ProductGrid } from './ProductGrid';
import { ModifierSheet } from './ModifierSheet';
import { Ticket } from './Ticket';
import { CheckoutSheet } from './CheckoutSheet';

// Posición de la píldora flotante (carrito/cobrar) como offset desde su esquina inferior-derecha.
// Clamp aproximado al cargar por si el viewport cambió de tamaño entre sesiones (no dejarla fuera).
function loadPillOffset(): { x: number; y: number } {
  try {
    const s = JSON.parse(localStorage.getItem('pos.pillOffset') || 'null');
    if (s && typeof s.x === 'number' && typeof s.y === 'number') {
      return {
        x: Math.min(0, Math.max(s.x, -(window.innerWidth - 96))),
        y: Math.min(0, Math.max(s.y, -(window.innerHeight - 72))),
      };
    }
  } catch { /* corrupto → default */ }
  return { x: 0, y: 0 };
}

export function POSPage() {
  const { data: menu, isLoading, error } = useMenu();
  const { data: popular } = usePopular();
  // La lista de animales es estática dentro de un despliegue: se pide una vez y se guarda por lo
  // que dure la sesión, no por cada cuenta que se abre.
  const { data: folios } = useQuery({
    queryKey: ['pos', 'folio-names'],
    queryFn: posApi.folioNames,
    staleTime: Infinity,
    gcTime: Infinity,
  });
  const bautizarCuentas = useTicketStore((s) => s.bautizarCuentas);
  const cuentasSinNombre = useTicketStore((s) => s.tabs.some((t) => !t.folioName));
  // Corre al llegar la lista y cada vez que se abre una cuenta. Bautizar es idempotente: no
  // renombra lo que ya tiene animal, porque el operador pudo habérselo dicho ya al cliente.
  useEffect(() => {
    if (folios?.items?.length && cuentasSinNombre) bautizarCuentas(folios.items);
  }, [folios, cuentasSinNombre, bautizarCuentas]);
  const { data: modifierDefaults } = useModifierDefaults();
  const { ref, width } = useContainerWidth<HTMLDivElement>();
  const wide = width >= 900;

  // Un precio que corrigieron en otra tablet tiene que llegar a esta ANTES de cobrar, no cinco
  // minutos después: el servidor cobra por la lista, no por lo que muestra la pantalla.
  useMenuEvents();

  const palette = useUiStore((s) => s.palette);
  const topCount = useUiStore((s) => s.topCount);
  const role = useSessionStore((s) => s.user?.role);
  const canEdit = role === 'admin' || role === 'gerente';
  const navigate = useNavigate();
  // Estado de caja: sin turno abierto la pantalla de venta no se muestra. Poll suave por si otra
  // tablet abre/cierra; al abrir caja desde /caja se invalida ['cash'] y refresca al instante.
  const cashStatus = useQuery({ queryKey: ['cash', 'status'], queryFn: posApi.cashStatus, refetchInterval: 30000 });
  const canOpenCash = canAccess(role, '/caja');
  const cuenta = useActiveTicket();
  const lines = cuenta.lines;
  // La lista de precios de ESTA cuenta. El servidor recalcula todo al cobrar; esto es para que la
  // pantalla muestre el mismo número, o el operador entrega un ticket con un total que no es el
  // cobrado.
  const lista = cuenta.platformId;
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
  const [ticketOpen, setTicketOpen] = useState(false);
  // modo editar: reutiliza el grid del POS para editar productos (admin/gerente).
  const [editMode, setEditMode] = useState(false);
  const [editProduct, setEditProduct] = useState<AdminProduct | null>(null);
  // catálogo admin (con todos los campos editables); solo se carga en modo editar.
  const { data: adminProducts } = useQuery({
    queryKey: ['admin', 'products', 'all'],
    queryFn: () => adminApi.products({ status: 'all', limit: 0 }), // 0 = todo el catálogo (para mapear el producto tocado)
    enabled: editMode && canEdit,
  });

  const ticketDrawer = useDisclosure();
  const ticketSwipe = useSwipeDownToClose(ticketDrawer.onClose);
  const checkout = useDisclosure();
  // Mandar a cocina sin cobrar vive aquí y no en la hoja de cobro: es una decisión sobre el
  // pedido, no sobre el dinero. Tenerlo dentro del cobro hacía que la pantalla pidiera método de
  // pago y propina para algo que después se descartaba.
  // Qué renglones acaban de entrar: decide si la comanda sale con el pedido completo (confirmar) o
  // solo con lo agregado.
  const [agregados, setAgregados] = useState<number[] | undefined>(undefined);
  const { mandar, enviando } = useMandarPedido((order) => {
    ticketDrawer.onClose();
    // Sin lista: sale la comanda del pedido COMPLETO, que es lo que confirmar significa.
    setAgregados(undefined);
    setLastOrder(order);
  });
  const enviarACocina = () => mandar({});

  // Agregarle a un pedido que ya está en cocina, desde el chip de la barra. Es el camino que la
  // feature 005 viene a abrir: antes existía enterrado en la hoja de cobro —armar el carrito,
  // abrir Cobrar, bajar hasta un selector, elegir el pedido— y en producción no se usó nunca.
  const { agregar } = useAgregarAPedido((order, nuevos) => {
    ticketDrawer.onClose();
    setAgregados(nuevos);
    setLastOrder(order);
  });
  // Con la cuenta vacía, tocar el chip no tiene nada que agregar: se abre el pedido para verlo.
  // Con productos capturados, se los lleva.
  const abrirPedidoEnCurso = (pedido: BoardOrder) => {
    if (cuenta.lines.length === 0) return;
    agregar(pedido);
  };
  const modSheet = useDisclosure();
  // En pantallas bajas (7" landscape) el panel lateral roba ~31% del ancho: arranca colapsado
  // y el grid ocupa todo. La píldora flotante lo reabre y mantiene el total visible. En tablets
  // altas se ve por defecto. matchMedia puede faltar en jsdom (tests) → default false.
  const [panelHidden, setPanelHidden] = useState(
    () => window.matchMedia?.('(max-height: 720px)')?.matches ?? false,
  );

  // Píldora arrastrable: a veces tapa las cards de abajo-derecha; el operador la mueve a discreción.
  // Offset relativo a la esquina inferior-derecha; se persiste. Handle dedicado → no choca con los taps.
  const pillRef = useRef<HTMLDivElement>(null);
  const pillDrag = useRef<{ px: number; py: number; ox: number; oy: number; rect: DOMRect } | null>(null);
  const [pillOffset, setPillOffset] = useState(loadPillOffset);
  const pillOffsetRef = useRef(pillOffset);

  const onPillDragStart = (e: PointerEvent<HTMLDivElement>) => {
    const el = pillRef.current;
    if (!el) return;
    e.currentTarget.setPointerCapture(e.pointerId);
    pillDrag.current = { px: e.clientX, py: e.clientY, ox: pillOffset.x, oy: pillOffset.y, rect: el.getBoundingClientRect() };
  };
  const onPillDragMove = (e: PointerEvent<HTMLDivElement>) => {
    const d = pillDrag.current;
    if (!d) return;
    const m = 8; // margen mínimo al borde de la pantalla
    const left = Math.min(Math.max(d.rect.left + (e.clientX - d.px), m), window.innerWidth - d.rect.width - m);
    const top = Math.min(Math.max(d.rect.top + (e.clientY - d.py), m), window.innerHeight - d.rect.height - m);
    const next = { x: d.ox + (left - d.rect.left), y: d.oy + (top - d.rect.top) };
    pillOffsetRef.current = next;
    setPillOffset(next);
  };
  const onPillDragEnd = () => {
    if (!pillDrag.current) return;
    pillDrag.current = null;
    localStorage.setItem('pos.pillOffset', JSON.stringify(pillOffsetRef.current));
  };

  // defensivo: nunca asumir que vienen arreglos (catálogo vacío o respuesta parcial).
  // useMemo para estabilizar la referencia: sin él, `?? []` crea un arreglo nuevo cada
  // render y rompería la memoización de los useMemo de abajo.
  const allCategories = useMemo(() => menu?.categories ?? [], [menu]);
  const allProducts = useMemo(() => menu?.products ?? [], [menu]);

  const childrenByRoot = useMemo(() => {
    const m: Record<number, number[]> = {};
    allCategories.forEach((c) => {
      if (c.parentId !== null) (m[c.parentId] ??= []).push(c.id);
    });
    return m;
  }, [allCategories]);

  const products = useMemo(() => {
    if (search.trim()) return buscarProductos(allProducts, search);
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
    // scope = subcategoría, o categoría raíz + sus hijos
    const { rootId, subId, popular: showPopular } = selection;
    const scope = subId !== null
      ? allProducts.filter((p) => p.categoryId === subId)
      : allProducts.filter((p) => {
          const cats = new Set<number>([rootId, ...(childrenByRoot[rootId] ?? [])]);
          return cats.has(p.categoryId);
        });
    if (!showPopular) return scope;
    // Populares del scope: mismo ranking global de ventas, filtrado a este scope y cortado a topCount.
    const rankById = new Map((popular ?? []).map((id, i) => [id, i] as const));
    const ranked = scope.filter((p) => rankById.has(p.id)).sort((a, b) => rankById.get(a.id)! - rankById.get(b.id)!);
    const base = ranked.length ? ranked : scope.filter((p) => p.favorite);
    return (base.length ? base : scope).slice(0, topCount);
  }, [allProducts, search, selection, childrenByRoot, popular, topCount]);

  const counts = useMemo(() => {
    const c: Record<number, number> = {};
    lines.forEach((l) => (c[l.productId] = (c[l.productId] ?? 0) + l.qty));
    return c;
  }, [lines]);

  const total = ticketTotal(lines);
  const count = ticketCount(lines);

  // Producto cuyo precio de plataforma se está corrigiendo. Solo con una lista activa: en
  // mostrador el precio se edita en el catálogo, y confundir las dos listas es el error que esta
  // pantalla no puede permitir.
  const [editandoPrecio, setEditandoPrecio] = useState<MenuProduct | null>(null);
  const desgloseEnEdicion = editandoPrecio
    ? desglosePrecio(menu, lista, editandoPrecio.id, Number(editandoPrecio.price))
    : null;

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
      addLine({
        productId: p.id, name: p.name, qty: 1, modifiers: [],
        unitPrice: precioDeLista(menu, lista, p.id, Number(p.price)),
      });
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
        addLine({
          productId: modProduct.id, name: modProduct.name, qty: 1, modifiers, notes: notes || undefined,
          unitPrice: precioDeLista(menu, lista, modProduct.id, Number(modProduct.price)),
        });
      }
    }
    setEditing(null);
    setModProduct(null);
  };

  if (isLoading) return <Center h="80vh"><Spinner size="xl" /></Center>;
  if (error) return <Center h="80vh"><Text color="red.500">Error cargando el menú</Text></Center>;

  // Sin turno abierto la pantalla de venta no se muestra. El backend rechaza el cobro
  // (NO_OPEN_REGISTER) y antes esto era solo un aviso: se podía armar el ticket completo y toparse
  // con el error hasta el momento de cobrar, con el cliente enfrente. `open` viene del backend y
  // ya significa "la caja principal tiene turno"; aquí no se decide nada, solo se pinta.
  if (cashStatus.data && !cashStatus.data.open) {
    return (
      <Center h="80vh" px={6}>
        <VStack gap={4} maxW="420px" textAlign="center">
          <Box color="orange.500"><LuTriangleAlert size={44} /></Box>
          <Text fontSize="xl" fontWeight="700">No hay caja abierta</Text>
          <Text color="fg.muted">Abre el turno para empezar a vender.</Text>
          {canOpenCash ? (
            <Button size="lg" minH="52px" w="100%" onClick={() => navigate('/caja')}>
              <LuWallet /> Abrir caja
            </Button>
          ) : (
            <Text color="fg.muted" fontSize="sm">Pídele a un gerente que la abra.</Text>
          )}
        </VStack>
      </Center>
    );
  }

  const catalog = (
    <VStack align="stretch" gap={2} h="100%" overflow="hidden">
      {/* Una sola fila (cuentas · buscador · toggles): recupera ~56px de alto en 7" landscape.
          Las cuentas scrollean solas; el buscador queda con ancho cómodo y fijo a la derecha. */}
      <Box px={{ base: 3, md: 4 }} pt={3}>
        <PlatformPicker />
      </Box>
      <Box px={{ base: 3, md: 4 }} pt={2}>
        <HStack gap={2} align="center">
          <Box flex="1" minW={0}><TicketTabs /></Box>
          <Box w="clamp(150px, 28%, 280px)" flexShrink={0}><SearchBar value={search} onChange={setSearch} /></Box>
          {/* Los pedidos que ya se mandaron a cocina, en la MISMA fila que las cuentas sin mandar.
              No cuesta alto nuevo —la fila ya existía— y absorbe la píldora de "Por cobrar", que
              mostraba esto mismo en otro lugar. Un toque en un chip vuelve a abrir el pedido; antes
              recuperarlo costaba cinco por un camino enterrado en la hoja de cobro. */}
          <PedidosEnCurso onAbrir={abrirPedidoEnCurso} />
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
        <ProductGrid
          products={products}
          counts={counts}
          onTap={tapProduct}
          showPrice={showPrices}
          menu={menu}
          lista={lista}
          onEditPrice={lista !== null && !editMode ? setEditandoPrecio : undefined}
        />
      </Box>
    </VStack>
  );

  return (
    <Box ref={ref} h="100%" bg="bg.subtle" position="relative">
      {wide ? (
        <Flex h="100%">
          <Box flex="1" minW={0}>{catalog}</Box>
          {!panelHidden && (
            <Box w="clamp(300px, 32%, 380px)" borderLeftWidth="1px" borderColor="border">
              <Ticket onCheckout={checkout.onOpen} onEnviar={enviarACocina} enviando={enviando}
                onEditLine={editLine} onHide={() => setPanelHidden(true)} />
            </Box>
          )}
        </Flex>
      ) : (
        <Flex direction="column" h="100%">
          <Box flex="1" minH={0}>{catalog}</Box>
          <HStack
            h="64px" px={3} bg="colorPalette.600" color="white" gap={2}
            display={count > 0 ? 'flex' : 'none'}
          >
            <HStack as="button" onClick={ticketDrawer.onOpen} flex="1" minW={0} gap={2}>
              <LuShoppingCart />
              <Text fontWeight="700" truncate>{count} art · {money(total)}</Text>
              <LuChevronUp />
            </HStack>
            <Button size="md" colorPalette="green" fontWeight="800" px={6} onClick={checkout.onOpen}>
              Cobrar
            </Button>
          </HStack>
        </Flex>
      )}

      {/* Panel oculto (modo ancho): píldora flotante para reabrir + atajo Cobrar */}
      {wide && panelHidden && (
        <HStack ref={pillRef} position="absolute" bottom={4} right={4} zIndex={20}
          transform={`translate(${pillOffset.x}px, ${pillOffset.y}px)`}
          bg="colorPalette.600" color="white" borderRadius="full" boxShadow="lg" pl={2} pr={2} py={2} gap={2}>
          {/* Handle de arrastre. touch-action:none → mover no scrollea la página; los toques de
              "Ver pedido"/"Cobrar" siguen siendo taps normales (no compiten con el drag). */}
          <Box aria-label="Mover" cursor="grab"
            onPointerDown={onPillDragStart} onPointerMove={onPillDragMove}
            onPointerUp={onPillDragEnd} onPointerCancel={onPillDragEnd}
            display="flex" alignItems="center" justifyContent="center" minW="36px" minH="44px"
            color="whiteAlpha.800" css={{ touchAction: 'none' }}>
            <LuGripVertical size={20} />
          </Box>
          <HStack as="button" onClick={() => setPanelHidden(false)} gap={2} minH="44px" px={1}>
            <LuPanelRightOpen />
            <Text fontWeight="700">{count > 0 ? `${count} art · ${money(total)}` : 'Ver pedido'}</Text>
          </HStack>
          {count > 0 && (
            <Button size="md" colorPalette="green" borderRadius="full" fontWeight="800" px={6} onClick={checkout.onOpen}>
              Cobrar
            </Button>
          )}
        </HStack>
      )}

      {/* Ticket como bottom sheet en modo angosto */}
      <DrawerRoot open={ticketDrawer.open} placement="bottom" onOpenChange={(e) => { if (!e.open) ticketDrawer.onClose(); }} size="full">
        <DrawerBackdrop />
        <DrawerContent
          colorPalette={palette} borderTopRadius={{ base: 0, md: '2xl' }} maxH={{ base: '100dvh', md: '92vh' }}
          style={{
            transform: ticketSwipe.offset ? `translateY(${ticketSwipe.offset}px)` : undefined,
            transition: ticketSwipe.dragging ? 'none' : 'transform 0.2s ease',
          }}
        >
          <Flex direction="column" h={{ base: '100dvh', md: '92vh' }}>
            <DrawerGrabber {...ticketSwipe.handlers} />
            {/* onHide = cerrar el sheet: a size=full el backdrop queda tapado, sin esto no hay cómo cerrarlo */}
            <Box flex="1" minH={0}>
              <Ticket
                onCheckout={() => { ticketDrawer.onClose(); checkout.onOpen(); }}
                onEnviar={enviarACocina} enviando={enviando} onEditLine={editLine}
                onHide={ticketDrawer.onClose} swipeHandlers={ticketSwipe.handlers}
              />
            </Box>
          </Flex>
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

      {/* Corregir el precio de un producto en la lista activa. Se remonta por producto (`key`) para
          que el campo arranque con el precio de ESE producto en cada apertura. */}
      {editandoPrecio && desgloseEnEdicion && lista !== null && (
        <PlatformPriceDialog
          key={editandoPrecio.id}
          productId={editandoPrecio.id}
          productName={editandoPrecio.name}
          plataforma={nombreDeLista(menu, lista)}
          plataformaId={lista}
          desglose={desgloseEnEdicion}
          isOpen
          onClose={() => setEditandoPrecio(null)}
        />
      )}

      {/* Confirmación — modal compacto centrado (no full-screen; ocupa lo mínimo) */}
      <DialogRoot open={lastOrder !== null} onOpenChange={(e) => { if (!e.open) setLastOrder(null); }} placement="center" size="xs">
        <DialogBackdrop />
        <DialogContent colorPalette={palette} mx={4} borderRadius="2xl">
          <DialogBody py={6} textAlign="center">
            {/* El color y el texto distinguen cobrado de pendiente. Antes los dos casos decían
                "Registrado correctamente": quien mandaba a cocina sin cobrar cerraba la cuenta,
                el pedido desaparecía de la pantalla y nada volvía a recordarle que faltaba el
                dinero hasta el corte. */}
            <Center color={lastOrder?.paid ? 'green.500' : 'orange.500'} mb={2}>
              {lastOrder?.paid ? <LuCircleCheck size={56} /> : <LuCircleAlert size={56} />}
            </Center>
            <Text fontSize="xl" fontWeight="800">
              {lastOrder?.folioName || `Pedido #${lastOrder?.number}`}
            </Text>
            {lastOrder?.paid ? (
              <Text color="fg.muted" mb={5}>Cobrado · #{lastOrder?.number}</Text>
            ) : (
              <Text color="orange.600" fontWeight="700" mb={5}>
                Falta cobrar {money(Number(lastOrder?.total ?? 0))} · #{lastOrder?.number}
              </Text>
            )}
            <VStack gap={2}>
              <Button size="lg" w="100%" onClick={() => setLastOrder(null)}>
                Nuevo pedido
              </Button>
              {/* Antes esto imprimía a ciegas: el operador no sabía qué había salido hasta tener
                  el papel en la mano, y un ticket equivocado ya costó papel y tiempo del cliente. */}
              <Button size="md" variant="outline" w="100%" onClick={() => setTicketOpen(true)}>
                <LuPrinter /> Ver ticket
              </Button>
            </VStack>
          </DialogBody>
        </DialogContent>
      </DialogRoot>

      {/* Encima de la confirmación, no en lugar de ella: al cerrar el ticket el operador sigue
          teniendo "Nuevo pedido" a un toque. reprint queda en false — la marca de reimpresión es
          para los tickets que se sacan después, desde el tablero. */}
      <TicketPreview order={lastOrder} isOpen={ticketOpen} onClose={() => setTicketOpen(false)} />

      {/* Sin UI: si el negocio activó la impresión automática, el ticket sale al cerrar el pedido.
          El botón de arriba se queda igual — ver el ticket y reimprimirlo siguen disponibles. */}
      <AutoPrintTicket order={lastOrder} />

      {/* La comanda de cocina, si el negocio la activó. Sale del MISMO pedido recién mandado: son
          dos papeles distintos para dos personas distintas, y cada uno con su propio ajuste. */}
      <KitchenTicket order={lastOrder} soloLineas={agregados} />
    </Box>
  );
}
