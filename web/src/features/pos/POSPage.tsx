import { useEffect, useMemo, useRef, useState, type PointerEvent } from 'react';
import {
  Box, Flex, VStack, HStack, Text, Button, Spinner, Center, IconButton, useDisclosure,
} from '@chakra-ui/react';
import {
  LuShoppingCart, LuChevronUp, LuCircleCheck, LuCircleAlert, LuPrinter, LuEye, LuEyeOff, LuPencil,
  LuPanelRightOpen, LuGripVertical, LuTriangleAlert, LuWallet, LuSearch, LuX,
} from 'react-icons/lu';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from 'react-router';
import { posApi } from '../../api/pos';
import { mensajeDeError } from '../../api/mensajes';
import { canAccess } from '../../app/roles';
import { can } from '../../app/permissions';
import { DrawerRoot, DrawerBackdrop, DrawerContent, DrawerGrabber } from '../../components/ui/drawer';
import { useSwipeDownToClose } from '../../hooks/useSwipeDownToClose';
import { DialogRoot, DialogBackdrop, DialogContent, DialogBody } from '../../components/ui/dialog';
import { ReasonSheet } from '../../components/ReasonSheet';
import { useMenu } from '../../hooks/useMenu';
import { useMenuEvents } from '../../hooks/useMenuEvents';
import { useOrderEvents } from '../../hooks/useOrderEvents';
import { usePopular } from '../../hooks/usePopular';
import { useModifierDefaults } from '../../hooks/useModifierDefaults';
import { useContainerWidth } from '../../hooks/useContainerWidth';
import { useUiStore } from '../../stores/ui';
import { useSessionStore } from '../../stores/session';
import { claveDeSeleccion, usePosStore } from '../../stores/pos';
import { accionPropia } from '../../stores/accionesPropias';
import { descartarCuenta } from './descartarCuenta';
import { adminApi, type AdminProduct } from '../../api/admin';
import { ProductEditDialog } from '../../shared/ProductEditDialog';
import { CancelarRenglonDialog } from '../../shared/CancelarRenglonDialog';
import type {
  AccountItem, CobroHecho, DraftView, MenuProduct, OrderView, PedidoParaCobrar, TicketModifier,
} from '../../types/pos';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';
import { money } from '../../utils/format';
import { TicketPreview } from '../../shared/tickets/TicketPreview';
import { AutoPrintTicket, KitchenTicket } from '../../shared/tickets/AutoPrintTicket';
import { buscarProductos } from './buscarProducto';
import { CategoryRail, type Selection } from './CategoryRail';
import { PlatformPicker } from './PlatformPicker';
import { AvisoDePlataforma } from './AvisoDePlataforma';
import { AvisoSinConexion } from './AvisoSinConexion';
import { PlatformPriceDialog } from './PlatformPriceDialog';
import { desglosePrecio, nombreDeLista, precioDeLista } from './precioPlataforma';
import { SearchBar } from './SearchBar';
import { FilaDeCuentas } from './FilaDeCuentas';
import { TodasLasCuentasSheet } from './TodasLasCuentasSheet';
import { DescartarCuentaSheet } from './DescartarCuentaSheet';
import { hayQuePreguntar } from './estadosDeCuenta';
import { ProductGrid } from './ProductGrid';
import { ModifierSheet } from './ModifierSheet';
import { Ticket } from './Ticket';
import { CobrarSheet } from '../../shared/CobrarSheet';
import { FolioPlataformaSheet } from './FolioPlataformaSheet';
import { hayQuePedirElFolio } from '../../domain/folioPlataforma';
import { toaster } from '../../components/ui/toaster';
import { useCuenta } from './useCuenta';
import { useCuentasVivas } from './useCuentasVivas';
import { useEnviarCuenta } from './useEnviarCuenta';
import { subirCuentasViejas } from './subirCuentasViejas';
import { useAbrirDesdeLaUrl } from './abrirDesdeLaUrl';
import type { RenglonNuevo, RenglonPedido } from './cuentaEnPantalla';
import { cuentaImpresa } from './cuentaImpresa';

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

function aPedidoParaCobrar(o: OrderView): PedidoParaCobrar {
  return {
    id: o.id, number: o.number, folioName: o.folioName, total: o.total, outstanding: o.outstanding,
    currency: o.currency, deliveryPlatformId: o.deliveryPlatformId,
  };
}

export function POSPage() {
  const qc = useQueryClient();
  const { data: menu, isLoading, error } = useMenu();
  const { data: popular } = usePopular();
  const { data: modifierDefaults } = useModifierDefaults();
  const { data: settings } = useQuery({ queryKey: ['business-settings'], queryFn: posApi.businessSettings });
  const envioPorDefecto = settings ? Number(settings.deliveryFee) : 20;
  const { ref, width } = useContainerWidth<HTMLDivElement>();
  const wide = width >= 900;

  // Un precio que corrigieron en otra tablet tiene que llegar a esta ANTES de cobrar.
  useMenuEvents();
  // Las cuentas y los pedidos de las otras tabletas, al instante (spec 030).
  useOrderEvents();
  // Las cuentas que la versión anterior guardaba en esta tableta suben una sola vez (D-12).
  useEffect(() => { void subirCuentasViejas(); }, []);

  const palette = useUiStore((s) => s.palette);
  const topCount = useUiStore((s) => s.topCount);
  const user = useSessionStore((s) => s.user);
  const role = user?.role;
  const canEdit = role === 'admin' || role === 'gerente';
  const navigate = useNavigate();
  const horaNegocio = useHoraDelNegocio();
  const cashStatus = useQuery({ queryKey: ['cash', 'status'], queryFn: posApi.cashStatus, refetchInterval: 30000 });
  const canOpenCash = canAccess(role, '/caja');

  // El pedido que se está cobrando. La hoja se abre SIEMPRE sobre un pedido que ya existe.
  const [cobrando, setCobrando] = useState<PedidoParaCobrar | null>(null);
  const cuenta = useCuenta({ envioPorDefecto, cobrando: cobrando !== null });
  const { vista } = cuenta;
  const lista = vista.platformId;
  const { data: vivas } = useCuentasVivas();
  const seleccion = usePosStore((s) => s.selected);
  const seleccionar = usePosStore((s) => s.seleccionar);
  const cuentaNueva = usePosStore((s) => s.cuentaNueva);
  const claveSeleccionada = seleccion ? claveDeSeleccion(seleccion) : null;
  const recientes = usePosStore((s) => s.recientes);


  const [selection, setSelection] = useState<Selection>({ kind: 'top' });
  const [search, setSearch] = useState('');
  const [buscando, setBuscando] = useState(false);
  const [showPrices, setShowPrices] = useState(() => localStorage.getItem('pos.showPrices') !== '0');
  const togglePrices = () =>
    setShowPrices((v) => { localStorage.setItem('pos.showPrices', v ? '0' : '1'); return !v; });
  const [modProduct, setModProduct] = useState<MenuProduct | null>(null);
  const [editing, setEditing] = useState<RenglonNuevo | null>(null);
  // La comanda: el pedido y los renglones que salen a cocina.
  const [comanda, setComanda] = useState<{ order: OrderView; ids: number[] } | null>(null);
  const [lastOrder, setLastOrder] = useState<OrderView | null>(null);
  const [printOrder, setPrintOrder] = useState<OrderView | null>(null);
  const [pidiendoFolio, setPidiendoFolio] = useState<{ hacer: () => void } | null>(null);
  const [sesionDeCobro, setSesionDeCobro] = useState(0);
  const [ticketOpen, setTicketOpen] = useState(false);
  // El papel de la cuenta abierta (spec 012), armado al tocar «Imprimir cuenta».
  const [papel, setPapel] = useState<ReturnType<typeof cuentaImpresa>>(null);
  const [todasAbierta, setTodasAbierta] = useState(false);
  const [descartando, setDescartando] = useState(false);
  const [cancelando, setCancelando] = useState(false);
  const [perdiendo, setPerdiendo] = useState(false);
  const [quitando, setQuitando] = useState<RenglonPedido | null>(null);
  // El folio de plataforma que se está tecleando en la barra, antes de guardarse al salir del campo.
  const [folioTecleado, setFolioTecleado] = useState<{ cuenta: string; texto: string } | null>(null);
  const [editMode, setEditMode] = useState(false);
  const [editProduct, setEditProduct] = useState<AdminProduct | null>(null);
  const { data: adminProducts } = useQuery({
    queryKey: ['admin', 'products', 'all'],
    queryFn: () => adminApi.products({ status: 'all', limit: 0 }),
    enabled: editMode && canEdit,
  });

  const ticketDrawer = useDisclosure();
  const ticketSwipe = useSwipeDownToClose(ticketDrawer.onClose);
  const { enviar, enviando, motivo, limpiarMotivo } = useEnviarCuenta({
    onComanda: (order, ids) => setComanda({ order, ids }),
  });

  const claveCuenta = vista.borradorId ?? '';
  const folioActual = folioTecleado && folioTecleado.cuenta === claveCuenta ? folioTecleado.texto : vista.platformOrderRef;

  // conFolio es la ÚNICA puerta por la que se manda un pedido de plataforma capturado a mano. Con
  // el campo de la barra lleno no se interpone nada; vacío, se pide con una salida explícita.
  const conFolio = (hacer: () => void) => {
    if (vista.tipo === 'pedido' || !hayQuePedirElFolio(vista.platformId, folioActual)) {
      hacer();
      return;
    }
    setPidiendoFolio({ hacer });
  };

  const enviarYa = async (): Promise<OrderView | null> => {
    await cuenta.esperar();
    const id = vista.borradorId;
    if (!id) return null;
    return enviar(id);
  };
  const enviarACocina = () => conFolio(() => { ticketDrawer.onClose(); void enviarYa(); });

  // COBRAR, LA PUERTA ÚNICA (US4). Con algo en «Nuevo» lo manda primero —el botón lo dice: «Enviar y
  // cobrar»— y abre la hoja sobre el pedido que regresa. Si el envío falla, la hoja no se abre y el
  // motivo queda en el pie.
  const cobrarLaCuenta = () => conFolio(() => {
    void (async () => {
      let pedido = vista.pedido;
      if (vista.nuevos.length > 0) {
        pedido = await enviarYa();
        if (!pedido) return;
      }
      if (!pedido) return;
      ticketDrawer.onClose();
      setSesionDeCobro((n) => n + 1);
      setCobrando(aPedidoParaCobrar(pedido));
    })();
  });

  // La venta termina cuando el pedido queda saldado. Se RELEE para imprimir: el ticket estampa
  // PAGADO o POR COBRAR y solo el servidor lo sabe.
  const terminarElCobro = async (res: CobroHecho, orderId: number) => {
    const order = await posApi.order(orderId);
    qc.setQueryData(['orders', orderId], order);
    qc.invalidateQueries({ queryKey: ['pos', 'accounts'] });
    setPrintOrder(order);
    if (res.paid) setLastOrder(order);
  };

  const descartar = async () => {
    const id = vista.borradorId;
    setDescartando(false);
    if (!id) { cuentaNueva(); return; }
    try {
      await cuenta.esperar();
      // La versión de la cuenta como la tiene esta pantalla —con lo propio ya guardado—: si otra
      // tableta le agregó algo que aquí todavía no se ve, el servidor no la descarta.
      const enPantalla = qc.getQueryData<DraftView>(['pos', 'draft', id]);
      if (!enPantalla) {
        qc.invalidateQueries({ queryKey: ['pos'] });
        return;
      }
      if (!(await descartarCuenta(qc, id, enPantalla.version))) return;
      cuentaNueva();
      qc.invalidateQueries({ queryKey: ['pos', 'accounts'] });
      qc.invalidateQueries({ queryKey: ['pos', 'folio-names'] });
    } catch (e) {
      toaster.create({ title: 'No se pudo descartar', description: mensajeDeError(e), type: 'error' });
      qc.invalidateQueries({ queryKey: ['pos'] });
    }
  };
  // Una cuenta vacía se descarta sin preguntar; con productos, en una hoja de la app (D-7).
  const pedirDescartar = () => {
    if (hayQuePreguntar(vista.nuevos.length)) setDescartando(true);
    else void descartar();
  };

  // «Cancelar lo que falta» (dueño, 2026-10-09): lo pagado se queda como venta, el resto se pierde.
  const cancelarResto = async (motivoResto: string | null) => {
    setPerdiendo(false);
    if (motivoResto === null || vista.pedidoId === null) return;
    try {
      const id = vista.pedidoId;
      await accionPropia(() => posApi.writeOffOrder(id, motivoResto));
      toaster.create({ title: `Se canceló lo que faltaba de ${vista.nombre || 'el pedido'}`, type: 'success' });
      cuentaNueva();
      qc.invalidateQueries({ queryKey: ['orders'] });
      qc.invalidateQueries({ queryKey: ['pos', 'accounts'] });
    } catch (e) {
      toaster.create({ title: 'No se pudo cancelar lo que falta', description: mensajeDeError(e), type: 'error' });
    }
  };

  const cancelarPedido = async (motivoCancelar: string | null) => {
    setCancelando(false);
    if (motivoCancelar === null || vista.pedidoId === null) return;
    try {
      const id = vista.pedidoId;
      await accionPropia(() => posApi.cancelOrder(id, motivoCancelar));
      toaster.create({ title: `${vista.nombre || 'El pedido'} se canceló`, type: 'success' });
      cuentaNueva();
      qc.invalidateQueries({ queryKey: ['orders'] });
      qc.invalidateQueries({ queryKey: ['pos', 'accounts'] });
    } catch (e) {
      toaster.create({ title: 'No se pudo cancelar', description: mensajeDeError(e), type: 'error' });
    }
  };

  const quitarDeCocina = async (motivoQuitar: string, qty: number) => {
    const r = quitando;
    setQuitando(null);
    if (!r || vista.pedidoId === null) return;
    try {
      const id = vista.pedidoId;
      const res = await accionPropia(() => posApi.cancelOrderLine(id, r.id, motivoQuitar, qty));
      toaster.create({
        title: 'Producto quitado',
        description: res.repusoInventario
          ? 'El ingrediente volvió al almacén.'
          : 'Ya se estaba preparando: el ingrediente no vuelve al almacén.',
        type: 'success',
      });
    } catch (e) {
      toaster.create({ title: 'No se pudo quitar', description: mensajeDeError(e), type: 'error' });
    }
    qc.invalidateQueries({ queryKey: ['orders'] });
    qc.invalidateQueries({ queryKey: ['pos', 'accounts'] });
  };

  const elegirCuenta = (c: AccountItem) => {
    limpiarMotivo();
    if (c.kind === 'draft' && c.draftId) seleccionar({ kind: 'draft', id: c.draftId });
    else if (c.orderId) seleccionar({ kind: 'order', id: c.orderId });
  };

  const modSheet = useDisclosure();
  // En pantallas bajas (7" landscape) el panel lateral roba ~31% del ancho: arranca colapsado.
  const [panelHidden, setPanelHidden] = useState(
    () => window.matchMedia?.('(max-height: 720px)')?.matches ?? false,
  );

  // «Abrir cuenta» del tablero y del cierre de caja llegan por la URL (FR-019), y abren el ticket.
  useAbrirDesdeLaUrl(() => {
    if (wide) setPanelHidden(false);
    else ticketDrawer.onOpen();
  });

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
    const m = 8;
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
      const byId = new Map(allProducts.map((p) => [p.id, p]));
      const ranked = (popular ?? [])
        .map((id) => byId.get(id))
        .filter((p): p is MenuProduct => p !== undefined);
      const base = ranked.length ? ranked : allProducts.filter((p) => p.favorite);
      return (base.length ? base : allProducts).slice(0, topCount);
    }
    const { rootId, subId, popular: showPopular } = selection;
    const scope = subId !== null
      ? allProducts.filter((p) => p.categoryId === subId)
      : allProducts.filter((p) => {
          const cats = new Set<number>([rootId, ...(childrenByRoot[rootId] ?? [])]);
          return cats.has(p.categoryId);
        });
    if (!showPopular) return scope;
    const rankById = new Map((popular ?? []).map((id, i) => [id, i] as const));
    const ranked = scope.filter((p) => rankById.has(p.id)).sort((a, b) => rankById.get(a.id)! - rankById.get(b.id)!);
    const base = ranked.length ? ranked : scope.filter((p) => p.favorite);
    return (base.length ? base : scope).slice(0, topCount);
  }, [allProducts, search, selection, childrenByRoot, popular, topCount]);

  const counts = useMemo(() => {
    const c: Record<number, number> = {};
    vista.nuevos.forEach((l) => (c[l.productId] = (c[l.productId] ?? 0) + l.qty));
    return c;
  }, [vista.nuevos]);

  const count = vista.nuevos.reduce((s, l) => s + l.qty, 0);
  const hayAlgo = count > 0 || vista.tipo === 'pedido';
  const cobrarApagado = vista.falta <= 0 || vista.guardando || cuenta.sinConexion || vista.noDisponibles.length > 0;
  const etiquetaCobrar = vista.nuevos.length > 0 ? 'Enviar y cobrar' : 'Cobrar';
  const resumen = vista.tipo === 'pedido'
    ? `${vista.nombre} · falta ${money(vista.falta)}`
    : `${count} art · ${money(vista.falta)}`;

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
    if (vista.cerrada) {
      toaster.create({
        title: 'Esta cuenta ya está pagada y entregada',
        description: 'Lo que pidan ahora va en una cuenta nueva.',
        type: 'info',
        action: { label: 'Cuenta nueva', onClick: cuentaNueva },
      });
      return;
    }
    if (p.groups.length > 0) {
      setEditing(null);
      setModProduct(p);
      modSheet.onOpen();
    } else {
      cuenta.agregar({
        productId: p.id, name: p.name, qty: 1, modifiers: [],
        unitPrice: precioDeLista(menu, lista, p.id, Number(p.price)),
      });
    }
  };

  // Los modificadores del renglón como los pide la hoja: con su grupo, que el servidor no guarda.
  const modificadoresDe = (r: RenglonNuevo, p: MenuProduct): TicketModifier[] =>
    r.modifiers.map((m) => ({
      optionId: m.optionId, name: m.name, qty: m.qty, priceDelta: Number(m.priceDelta),
      groupId: p.groups.find((g) => g.options.some((o) => o.id === m.optionId))?.id ?? 0,
    }));

  const editLine = (r: RenglonNuevo) => {
    const p = allProducts.find((x) => x.id === r.productId);
    if (!p || p.groups.length === 0) return;
    setEditing(r);
    setModProduct(p);
    modSheet.onOpen();
  };

  const confirmModifiers = (modifiers: TicketModifier[], notes: string, qty: number) => {
    if (!modProduct) return;
    if (editing) {
      cuenta.cambiar(editing, modifiers, notes);
    } else {
      for (let i = 0; i < qty; i++) {
        cuenta.agregar({
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

  // Sin turno abierto la pantalla de venta no se muestra.
  if (cashStatus.data && !cashStatus.data.open) {
    return (
      <>
        {/* El aviso de plataforma también aquí: aceptar un pedido de plataforma no exige turno. */}
        <AvisoDePlataforma />
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
      </>
    );
  }

  // Con el panel abierto la fila tiene ~612 px: el buscador se pliega a un botón y despliega el
  // campo en el lugar de la fila mientras se busca.
  const buscadorPlegado = wide && !panelHidden;
  const campoDeBusqueda = buscadorPlegado && (buscando || search !== '');

  const ticketProps = {
    vista,
    hora: vista.abiertaEn ? horaNegocio.soloHora(vista.abiertaEn) : undefined,
    canal: vista.platformId !== null ? nombreDeLista(menu, vista.platformId) : undefined,
    sinConexion: cuenta.sinConexion,
    envioPorDefecto,
    enviando,
    motivo,
    puedeCancelar: can('orders.cancel', user),
    onMas: cuenta.mas,
    onMenos: cuenta.menos,
    onQuitar: cuenta.quitar,
    onEditLine: editLine,
    onCabecera: cuenta.cabecera,
    onQuitarNoDisponibles: () => vista.noDisponibles.forEach((l) => cuenta.quitar(l)),
    onEnviar: enviarACocina,
    onCobrar: cobrarLaCuenta,
    onDescartar: pedirDescartar,
    onCancelarPedido: () => setCancelando(true),
    onCancelarResto: () => setPerdiendo(true),
    onQuitarDeCocina: setQuitando,
    onImprimir: () => setPapel(cuentaImpresa(vista, new Date())),
  };

  const catalog = (
    <VStack align="stretch" gap={2} h="100%" overflow="hidden">
      <Box px={{ base: 3, md: 4 }} pt={3}>
        <PlatformPicker
          platformId={vista.platformId}
          platformOrderRef={folioActual}
          bloqueado={vista.tipo === 'pedido'}
          onCambiar={(id) => { setFolioTecleado(null); cuenta.cabecera({ platformId: id }); }}
          onFolio={(texto) => {
            if (vista.tipo === 'nueva') cuenta.cabecera({ platformOrderRef: texto });
            else setFolioTecleado({ cuenta: claveCuenta, texto });
          }}
          onFolioListo={() => {
            if (vista.tipo !== 'captura' || !folioTecleado || folioTecleado.cuenta !== claveCuenta) return;
            if (folioTecleado.texto.trim() !== vista.platformOrderRef) {
              cuenta.cabecera({ platformOrderRef: folioTecleado.texto.trim() || null });
            }
          }}
        />
      </Box>
      <Box px={{ base: 3, md: 4 }} pt={2}>
        {/* Fila 2: las cuentas vivas · buscar · precios · editar. Las cuentas son lo único elástico. */}
        <HStack gap={2} align="center">
          {campoDeBusqueda ? (
            <HStack flex="1" minW={0} gap={1}>
              <Box flex="1" minW={0}><SearchBar value={search} onChange={setSearch} /></Box>
              <IconButton aria-label="Cerrar búsqueda" size="lg" minW="44px" variant="ghost" colorPalette="gray"
                onClick={() => { setSearch(''); setBuscando(false); }}><LuX /></IconButton>
            </HStack>
          ) : (
            <Box flex="1" minW={0}>
              <FilaDeCuentas cuentas={vivas?.items ?? []} seleccionada={claveSeleccionada}
                recientes={recientes} nueva={seleccion === null}
                onElegir={elegirCuenta} onNueva={() => { limpiarMotivo(); cuentaNueva(); }}
                onVerTodas={() => setTodasAbierta(true)} />
            </Box>
          )}
          {buscadorPlegado ? (
            !campoDeBusqueda && (
              <IconButton aria-label="Buscar producto" size="lg" minW="44px" variant="outline" colorPalette="gray"
                onClick={() => setBuscando(true)}><LuSearch /></IconButton>
            )
          ) : (
            <Box w="clamp(120px, 20%, 200px)" flexShrink={0}><SearchBar value={search} onChange={setSearch} /></Box>
          )}
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
      <Box flex="1" minH={0} position="relative" data-testid="zona-de-productos">
      {/* Sobre los productos y no sobre el encabezado: arriba tapaba la fila de canales, y sin red
          los productos no se pueden agregar de todos modos. */}
      <AvisoSinConexion />
      <Box h="100%" overflowY="auto" px={{ base: 3, md: 4 }} css={{ overscrollBehavior: 'contain' }}>
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
      </Box>
    </VStack>
  );

  return (
    <Box ref={ref} h="100%" bg="bg.subtle" position="relative">
      <AvisoDePlataforma />
      <AvisoDeTurnoViejo estado={cashStatus.data} />
      {wide ? (
        <Flex h="100%">
          <Box flex="1" minW={0}>{catalog}</Box>
          {!panelHidden && (
            <Box w="clamp(300px, 32%, 380px)" borderLeftWidth="1px" borderColor="border">
              <Ticket {...ticketProps} onHide={() => setPanelHidden(true)} />
            </Box>
          )}
        </Flex>
      ) : (
        <Flex direction="column" h="100%">
          <Box flex="1" minH={0}>{catalog}</Box>
          {hayAlgo && (
          <HStack h="64px" px={3} bg="colorPalette.600" color="white" gap={2}>
            <HStack as="button" onClick={ticketDrawer.onOpen} flex="1" minW={0} gap={2}>
              <LuShoppingCart />
              <Text fontWeight="700" truncate>{resumen}</Text>
              <LuChevronUp />
            </HStack>
            <Button size="md" colorPalette="green" fontWeight="800" px={6}
              disabled={cobrarApagado} onClick={cobrarLaCuenta}>
              {etiquetaCobrar}
            </Button>
          </HStack>
          )}
        </Flex>
      )}

      {/* Panel oculto (modo ancho): píldora flotante para reabrir + atajo Cobrar */}
      {wide && panelHidden && (
        <HStack ref={pillRef} position="absolute" bottom={4} right={4} zIndex={20}
          transform={`translate(${pillOffset.x}px, ${pillOffset.y}px)`}
          bg="colorPalette.600" color="white" borderRadius="full" boxShadow="lg" pl={2} pr={2} py={2} gap={2}>
          <Box aria-label="Mover" cursor="grab"
            onPointerDown={onPillDragStart} onPointerMove={onPillDragMove}
            onPointerUp={onPillDragEnd} onPointerCancel={onPillDragEnd}
            display="flex" alignItems="center" justifyContent="center" minW="36px" minH="44px"
            color="whiteAlpha.800" css={{ touchAction: 'none' }}>
            <LuGripVertical size={20} />
          </Box>
          <HStack as="button" onClick={() => setPanelHidden(false)} gap={2} minH="44px" px={1}>
            <LuPanelRightOpen />
            <Text fontWeight="700">{hayAlgo ? resumen : 'Ver pedido'}</Text>
          </HStack>
          {hayAlgo && (
            <Button size="md" colorPalette="green" borderRadius="full" fontWeight="800" px={6}
              disabled={cobrarApagado} onClick={cobrarLaCuenta}>
              {etiquetaCobrar}
            </Button>
          )}
        </HStack>
      )}

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
            <Box flex="1" minH={0}>
              <Ticket {...ticketProps} onHide={ticketDrawer.onClose} swipeHandlers={ticketSwipe.handlers} />
            </Box>
          </Flex>
        </DrawerContent>
      </DrawerRoot>

      <ModifierSheet
        product={modProduct}
        lista={lista}
        isOpen={modSheet.open}
        optionRanks={modProduct ? modifierDefaults?.[String(modProduct.id)] : undefined}
        initialModifiers={editing && modProduct ? modificadoresDe(editing, modProduct) : undefined}
        initialNotes={editing?.notes || undefined}
        onClose={() => { modSheet.onClose(); setEditing(null); setModProduct(null); }}
        onConfirm={confirmModifiers}
      />

      <TodasLasCuentasSheet isOpen={todasAbierta} seleccionada={claveSeleccionada}
        onElegir={elegirCuenta} onClose={() => setTodasAbierta(false)} />

      <DescartarCuentaSheet isOpen={descartando} nombre={vista.nombre}
        productos={vista.nuevos.length} total={vista.totalNuevo}
        onSeguir={() => setDescartando(false)} onDescartar={() => void descartar()} />

      <ReasonSheet isOpen={cancelando} required destructive title={`¿Cancelar el pedido de ${vista.nombre || 'esta cuenta'}?`}
        label="Motivo" confirmLabel="Cancelar pedido" onDone={(r) => void cancelarPedido(r)} />

      <ReasonSheet isOpen={perdiendo} required destructive title={`¿Cancelar lo que falta de ${vista.nombre || 'esta cuenta'}?`}
        label="Motivo" placeholder="Ej. se fue sin pagar" confirmLabel="Cancelar lo que falta"
        onDone={(r) => void cancelarResto(r)} />

      {quitando && (
        <CancelarRenglonDialog nombre={quitando.name} pendientes={Math.max(1, quitando.qty - quitando.delivered)}
          yaSalioACocina enviando={false} onCerrar={() => setQuitando(null)}
          onConfirmar={(m, qty) => void quitarDeCocina(m, qty)} />
      )}

      <FolioPlataformaSheet
        isOpen={pidiendoFolio !== null}
        plataforma={nombreDeLista(menu, vista.platformId)}
        valorInicial={folioActual}
        onGuardarYMandar={(folio) => {
          // El folio se guarda en la cuenta ANTES de mandar: la fila de escrituras de la cuenta
          // garantiza que el envío sale después de este cambio.
          cuenta.cabecera({ platformOrderRef: folio });
          setFolioTecleado(null);
          const seguir = pidiendoFolio?.hacer;
          setPidiendoFolio(null);
          seguir?.();
        }}
        onMandarSinFolio={() => {
          const seguir = pidiendoFolio?.hacer;
          setPidiendoFolio(null);
          seguir?.();
        }}
        onCancelar={() => setPidiendoFolio(null)}
      />

      <CobrarSheet
        key={sesionDeCobro}
        pantalla="pos"
        order={cobrando}
        onClose={() => setCobrando(null)}
        onCobrado={terminarElCobro}
      />

      <ProductEditDialog
        product={editProduct}
        isOpen={editProduct !== null}
        onClose={() => setEditProduct(null)}
      />

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

      {/* Confirmación — modal compacto centrado. Cobrado o pendiente se distinguen por color y texto. */}
      <DialogRoot open={lastOrder !== null} onOpenChange={(e) => { if (!e.open) setLastOrder(null); }} placement="center" size="xs">
        <DialogBackdrop />
        <DialogContent colorPalette={palette} mx={4} borderRadius="2xl">
          <DialogBody py={6} textAlign="center">
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
                Falta cobrar {money(Number(lastOrder?.outstanding ?? 0))} · #{lastOrder?.number}
              </Text>
            )}
            <VStack gap={2}>
              <Button size="lg" w="100%" onClick={() => { setLastOrder(null); cuentaNueva(); }}>
                Nueva cuenta
              </Button>
              <Button size="md" variant="outline" w="100%" onClick={() => setTicketOpen(true)}>
                <LuPrinter /> Ver ticket
              </Button>
            </VStack>
          </DialogBody>
        </DialogContent>
      </DialogRoot>

      <TicketPreview order={lastOrder} isOpen={ticketOpen} onClose={() => setTicketOpen(false)} />
      <TicketPreview order={papel?.order ?? null} preCuenta={papel?.preCuenta ?? false}
        isOpen={papel !== null} onClose={() => setPapel(null)} />
      <AutoPrintTicket order={printOrder} />
      {/* La comanda sale en cuanto el servidor confirma el envío, solo con los renglones que dijo. */}
      <KitchenTicket order={comanda?.order ?? null} soloLineas={comanda?.ids} />
    </Box>
  );
}

// Aviso de que la caja abierta ya no es de hoy. No bloquea el cobro, a propósito: un negocio en
// operación prefiere una fecha corrida a una caja parada. `deOtroDia` lo decide el servidor.
function AvisoDeTurnoViejo({ estado }: { estado?: { deOtroDia?: boolean; openedAt?: string } }) {
  const [oculto, setOculto] = useState(false);
  const horaNegocio = useHoraDelNegocio();
  const navigate = useNavigate();
  if (!estado?.deOtroDia || oculto) return null;
  return (
    <HStack bg="orange.subtle" borderBottomWidth="1px" borderColor="orange.emphasized"
      px={3} py={1} gap={3} justify="space-between">
      <HStack gap={2} minW={0}>
        <Box color="orange.fg" flexShrink={0}><LuTriangleAlert size={18} /></Box>
        <Text fontSize="sm" truncate>
          La caja lleva abierta desde el {horaNegocio.fechaYHora(estado.openedAt)}.
          Ciérrala para que el corte cuadre por día.
        </Text>
      </HStack>
      <HStack gap={1} flexShrink={0}>
        <Button size="xs" variant="outline" minH="44px" px={3} onClick={() => navigate('/caja')}>
          Ir a caja
        </Button>
        <Button size="xs" variant="ghost" minH="44px" px={3} onClick={() => setOculto(true)}>
          Ahora no
        </Button>
      </HStack>
    </HStack>
  );
}
