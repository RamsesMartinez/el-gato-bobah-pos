// Reinicia el estado del sheet cuando se abre para otro producto/edición.
import { useEffect, useRef, useState, type MouseEvent } from 'react';
import {
  Box, Button, HStack, VStack, Text, Wrap, WrapItem, Input, Textarea, Flex,
} from '@chakra-ui/react';
import { LuSearch, LuArchiveRestore, LuPlus } from 'react-icons/lu';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
  DrawerCloseTrigger, DrawerGrabber,
} from '../../components/ui/drawer';
import { useSwipeDownToClose } from '../../hooks/useSwipeDownToClose';
import { DialogRoot, DialogBackdrop, DialogContent, DialogBody } from '../../components/ui/dialog';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { MenuGroup, MenuOption, MenuProduct, RankedOption, TicketModifier } from '../../types/pos';
import { money, normalize } from '../../utils/format';
import { useUiStore } from '../../stores/ui';
import { useSessionStore } from '../../stores/session';
import { adminApi, type AdminModifierOption } from '../../api/admin';
import { toaster } from '../../components/ui/toaster';
import { deltaDeLista, desgloseDelta, nombreDeLista, precioDeLista } from './precioPlataforma';
import { cabeOtra, cantidadDe, sumarUna } from './seleccionModificadores';
import { OptionPriceFields } from './OptionPriceFields';
import { useMenu } from '../../hooks/useMenu';
import { useActiveTicket } from '../../stores/ticket';

interface Props {
  product: MenuProduct | null;
  isOpen: boolean;
  initialModifiers?: TicketModifier[];
  initialNotes?: string;
  optionRanks?: Record<string, RankedOption[]>; // grupo(id) → opciones rankeadas (con %) por contexto
  onClose: () => void;
  onConfirm: (modifiers: TicketModifier[], notes: string, qty: number) => void;
}

// selección: groupId -> optionId -> cantidad
type Sel = Record<number, Record<number, number>>;

const SEARCH_THRESHOLD = 12; // muestra el buscador de opciones si hay más de esto

export function ModifierSheet({ product, isOpen, initialModifiers, initialNotes, optionRanks, onClose, onConfirm }: Props) {
  const [sel, setSel] = useState<Sel>({});
  const [notes, setNotes] = useState('');
  const [qty, setQty] = useState(1);
  const [optQuery, setOptQuery] = useState('');
  const palette = useUiStore((s) => s.palette);
  const recStrategy = useUiStore((s) => s.recStrategy);
  // Los cargos de los extras siguen la lista de precios de la cuenta, igual que el producto: si
  // aquí se mostrara el delta base, el total de pantalla no cuadraría con el cobrado.
  const { data: menu } = useMenu();
  const lista = useActiveTicket().platformId;
  const role = useSessionStore((s) => s.user?.role);
  const canManage = role === 'admin' || role === 'gerente';
  const qc = useQueryClient();
  const swipe = useSwipeDownToClose(onClose);

  // Gestionar una opción (mantener presionado / clic derecho): desactivar para quitar basura.
  const [manageOpt, setManageOpt] = useState<{ id: number; name: string; priceDelta: number } | null>(null);
  const [hiddenIds, setHiddenIds] = useState<Set<number>>(new Set());
  const suppressClick = useRef(false);
  const pressTimer = useRef<number | undefined>(undefined);

  const deactivate = useMutation({
    mutationFn: (id: number) => adminApi.setOptionActive(id, false),
    onSuccess: (_d, id) => {
      setHiddenIds((s) => new Set(s).add(id));
      qc.invalidateQueries({ queryKey: ['menu'] });
      qc.invalidateQueries({ queryKey: ['admin', 'modifier-options'] });
      setManageOpt(null);
      toaster.create({
        title: 'Opción archivada',
        type: 'success',
        action: {
          label: 'Deshacer',
          onClick: () => {
            adminApi.setOptionActive(id, true)
              .then(() => {
                setHiddenIds((s) => { const n = new Set(s); n.delete(id); return n; });
                qc.invalidateQueries({ queryKey: ['menu'] });
                qc.invalidateQueries({ queryKey: ['admin', 'modifier-options'] });
              })
              .catch((e) => toaster.create({ title: 'Error', description: String(e), type: 'error' }));
          },
        },
      });
    },
    onError: (e) => toaster.create({ title: 'Error', description: String(e), type: 'error' }),
  });

  // Ver archivadas dentro del sheet + reactivar al elegirlas (para venderlas de nuevo).
  const [showInactive, setShowInactive] = useState(false);
  const [extraOptions, setExtraOptions] = useState<Map<number, MenuOption[]>>(new Map());
  const { data: allOptions } = useQuery({
    queryKey: ['admin', 'modifier-options', 'all'],
    queryFn: () => adminApi.modifierOptions({ status: 'all', limit: 0 }), // 0 = todas (incluye inactivas para reactivar)
    enabled: canManage && showInactive,
  });
  const reactivate = useMutation({
    mutationFn: (v: { g: MenuGroup; ao: AdminModifierOption }) => adminApi.setOptionActive(v.ao.id, true),
    onSuccess: (_d, { g, ao }) => {
      const mo: MenuOption = { id: ao.id, name: ao.name, priceDelta: ao.priceDelta, maxPerLine: 1, favorite: false };
      setExtraOptions((prev) => { const m = new Map(prev); m.set(g.id, [...(m.get(g.id) ?? []), mo]); return m; });
      setSel((s) => (g.max === 1
        ? { ...s, [g.id]: { [ao.id]: 1 } }
        : { ...s, [g.id]: { ...(s[g.id] ?? {}), [ao.id]: 1 } }));
      qc.invalidateQueries({ queryKey: ['menu'] });
      qc.invalidateQueries({ queryKey: ['admin', 'modifier-options'] });
      toaster.create({ title: `«${ao.name}» reactivada`, description: 'Queda disponible para la venta.', type: 'success' });
    },
    onError: (e) => toaster.create({ title: 'Error', description: String(e), type: 'error' }),
  });

  // El diálogo se abre para archivar (admin/gerente) O para corregir el cargo en la plataforma
  // activa (cualquiera que pueda vender). Son dos cosas distintas detrás del mismo gesto porque el
  // gesto ya existía y agregar un segundo sería una cosa más que aprender.
  const puedeAbrirDialogo = canManage || lista !== null;
  const openManage = (o: MenuOption) => {
    if (puedeAbrirDialogo) setManageOpt({ id: o.id, name: o.name, priceDelta: Number(o.priceDelta) });
  };
  const pressStart = (o: MenuOption) => {
    if (!puedeAbrirDialogo) return;
    suppressClick.current = false;
    pressTimer.current = window.setTimeout(() => { suppressClick.current = true; openManage(o); }, 500);
  };
  const pressCancel = () => { if (pressTimer.current) window.clearTimeout(pressTimer.current); };
  const manageHandlers = (o: MenuOption) => (puedeAbrirDialogo ? {
    onContextMenu: (e: MouseEvent) => { e.preventDefault(); openManage(o); },
    onPointerDown: () => pressStart(o),
    onPointerUp: pressCancel,
    onPointerLeave: pressCancel,
    onPointerCancel: pressCancel,
  } : {});

  const desgloseEnEdicion = manageOpt
    ? desgloseDelta(menu, lista, manageOpt.id, manageOpt.priceDelta)
    : null;

  // orden de inserción de picks multi-select (las claves numéricas de sel no lo conservan),
  // para expulsar la más antigua al llegar al tope (FIFO).
  const pickSeq = useRef(0);
  const pickOrder = useRef<Record<string, number>>({});
  const okey = (gid: number, oid: number) => `${gid}:${oid}`;

  const ranksFor = (gid: number): RankedOption[] => optionRanks?.[String(gid)] ?? [];

  // Patrón Strategy: cada modo decide qué opciones se recomiendan (para pre-marcar y
  // mostrar arriba). 'smart' trae % contextual; 'favorites' las marcadas; 'alphabetical' nada.
  const byName = (a: MenuOption, b: MenuOption) => a.name.localeCompare(b.name, 'es');
  const favRanked = (g: MenuGroup) => g.options.filter((o) => o.favorite).sort(byName).map((o) => ({ id: o.id }));
  const strategyRanked = (g: MenuGroup): { id: number; pct?: number }[] => {
    // 'smart' cae a favoritas cuando aún no hay señal contextual (sin registro/probabilidad).
    if (recStrategy === 'smart') { const r = ranksFor(g.id); return r.length > 0 ? r : favRanked(g); }
    if (recStrategy === 'favorites') return favRanked(g);
    return [];
  };

  // inicializa al abrir: restaura edición, o preselecciona el default contextual
  // (top rankeado, o el primero) en grupos de elección única obligatoria.
  useEffect(() => {
    if (!isOpen || !product) return;
    const s: Sel = {};
    pickOrder.current = {};
    pickSeq.current = 0;
    const put = (gid: number, oid: number, q: number) => {
      (s[gid] ??= {});
      s[gid][oid] = q;
      pickOrder.current[okey(gid, oid)] = pickSeq.current++;
    };
    if (initialModifiers?.length) {
      for (const m of initialModifiers) {
        put(m.groupId, m.optionId, m.qty);
      }
    } else {
      // pre-marcado inteligente por contexto (optionRanks). Regla anti-"deseleccionar":
      // - elección única requerida → un default (rankeado, o el más barato como fallback).
      // - multi-select requerido → SOLO opciones con señal real (rankeadas), hasta el mínimo.
      //   sin datos NO adivina: pre-marcar un fallback alfabético obliga a deseleccionarlo.
      for (const g of product.groups) {
        if (g.options.length === 0) continue;
        const ranked = strategyRanked(g);
        if (g.max === 1) {
          if (g.min <= 0) continue;
          const rank = new Map(ranked.map((r, i) => [r.id, i]));
          const best = [...g.options].sort((a, b) => {
            const ra = rank.get(a.id) ?? Infinity, rb = rank.get(b.id) ?? Infinity;
            if (ra !== rb) return ra - rb;          // 1º lo más probable por contexto
            return Number(a.priceDelta) - Number(b.priceDelta); // sin datos: default = más barato
          })[0];
          put(g.id, best.id, 1);
        } else if (g.min > 0 && ranked.length > 0) {
          const valid = new Set(g.options.map((o) => o.id));
          for (const r of ranked.slice(0, g.min)) if (valid.has(r.id)) put(g.id, r.id, 1);
        }
      }
    }
    setSel(s);
    setNotes(initialNotes ?? '');
    setQty(1);
    setOptQuery('');
    setHiddenIds(new Set());
    setManageOpt(null);
    setShowInactive(false);
    setExtraOptions(new Map());
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOpen, product?.id]);

  if (!product) return null;

  // obligatorios primero (sort estable → conserva el orden del menú dentro de cada grupo).
  const orderedGroups = [...product.groups].sort((a, b) => (b.min > 0 ? 1 : 0) - (a.min > 0 ? 1 : 0));

  const countIn = (gid: number) =>
    Object.values(sel[gid] ?? {}).reduce((a, b) => a + b, 0);

  const setSingle = (gid: number, oid: number) =>
    setSel((s) => ({ ...s, [gid]: { [oid]: 1 } }));

  const toggleMulti = (gid: number, oid: number, max: number) =>
    setSel((s) => {
      const grp = { ...(s[gid] ?? {}) };
      if (grp[oid]) {
        delete grp[oid];
        delete pickOrder.current[okey(gid, oid)];
      } else {
        // al tope: FIFO — expulsa la más antigua para que el nuevo pick entre en un solo toque.
        if (Object.values(grp).reduce((a, b) => a + b, 0) >= max) {
          let oldest: number | null = null, min = Infinity;
          for (const k of Object.keys(grp)) {
            const seq = pickOrder.current[okey(gid, Number(k))] ?? 0;
            if (seq < min) { min = seq; oldest = Number(k); }
          }
          if (oldest !== null) { delete grp[oldest]; delete pickOrder.current[okey(gid, oldest)]; }
        }
        grp[oid] = 1;
        pickOrder.current[okey(gid, oid)] = pickSeq.current++;
      }
      return { ...s, [gid]: grp };
    });

  // opciones de un grupo = las del menú + las reactivadas en esta sesión del sheet.
  const optsOf = (g: MenuGroup): MenuOption[] => {
    const ex = extraOptions.get(g.id);
    return ex && ex.length ? [...g.options, ...ex] : g.options;
  };
  const extraIds = new Set([...extraOptions.values()].flat().map((o) => o.id));
  const archivedFor = (g: MenuGroup): AdminModifierOption[] =>
    (allOptions?.items ?? []).filter((o) => !o.active && o.groupId === g.id && !extraIds.has(o.id));

  const optById = new Map<number, MenuOption>();
  product.groups.forEach((g) => optsOf(g).forEach((o) => optById.set(o.id, o)));

  let perUnitDelta = 0;
  for (const picks of Object.values(sel)) {
    for (const [oid, q] of Object.entries(picks)) {
      const o = optById.get(Number(oid));
      if (o) perUnitDelta += deltaDeLista(menu, lista, o.id, Number(o.priceDelta)) * q;
    }
  }
  const unit = Number(product.price) + perUnitDelta;
  const total = Math.round(unit * qty * 100) / 100;

  const unmet = orderedGroups.filter((g) => countIn(g.id) < g.min);
  const canConfirm = unmet.length === 0;
  const firstUnmetId = unmet[0]?.id ?? null;

  const q = normalize(optQuery);
  const showSearch = product.groups.reduce((n, g) => n + g.options.length, 0) > SEARCH_THRESHOLD;
  const visibleOptions = (g: MenuGroup) =>
    optsOf(g).filter((o) => !hiddenIds.has(o.id) && (!q || normalize(o.name).includes(q)));

  const confirm = () => {
    const modifiers: TicketModifier[] = [];
    for (const g of product.groups) {
      const picks = sel[g.id] ?? {};
      for (const o of optsOf(g)) {
        if (picks[o.id]) {
          modifiers.push({
            optionId: o.id, groupId: g.id, name: o.name,
            priceDelta: deltaDeLista(menu, lista, o.id, Number(o.priceDelta)),
            qty: picks[o.id],
          });
        }
      }
    }
    onConfirm(modifiers, notes.trim(), qty);
    onClose();
  };

  // renderiza las opciones de un grupo: primero las "top" (rankeadas, con su %),
  // luego el resto en orden alfabético. Al buscar, lista plana filtrada.
  const optionButtons = (g: MenuGroup) => {
    const single = g.max === 1;
    const picks = sel[g.id] ?? {};
    const ranked = strategyRanked(g);
    const pctById = new Map(ranked.flatMap((r) => (r.pct === undefined ? [] : [[r.id, r.pct] as const])));
    const rankedIds = new Set(ranked.map((r) => r.id));

    const btn = (o: MenuOption) => {
      const veces = cantidadDe(picks, o.id);
      const on = veces > 0;
      const pct = pctById.get(o.id);
      // El "+" solo aparece cuando de verdad cabe otra de ESTA opción, así que presionarlo nunca
      // le quita nada a otra. Es lo que faltaba para pedir dos del mismo sabor: el grupo pide dos
      // salsas y el cliente quiere las dos de mango habanero.
      const repetible = !single && cabeOtra(picks, o, g.max);
      return (
        <WrapItem key={o.id}>
          {/* Envoltura, no botón: el "+" es un control aparte y un botón dentro de otro no es HTML
              válido — el navegador lo desanida y el toque deja de caer donde se ve. */}
          <HStack gap={0}>
          <Button
            size="lg" minH="48px"
            variant={on ? 'solid' : 'outline'}
            colorPalette={on ? undefined : 'gray'}
            borderRightRadius={repetible ? 0 : undefined}
            onClick={() => {
              if (suppressClick.current) { suppressClick.current = false; return; } // fue long-press
              if (single) setSingle(g.id, o.id); else toggleMulti(g.id, o.id, g.max);
            }}
            {...manageHandlers(o)}
          >
            {o.name}
            {veces > 1 && (
              <Text as="span" ml={1.5} fontSize="sm" fontWeight="800">
                ×{veces}
              </Text>
            )}
            {pct !== undefined && (
              <Text as="span" ml={1.5} fontSize="xs" fontWeight="700" color={on ? 'whiteAlpha.800' : 'colorPalette.500'}>
                {pct}%
              </Text>
            )}
            {deltaDeLista(menu, lista, o.id, Number(o.priceDelta)) !== 0 && (
              <Text as="span" ml={1} fontSize="xs" opacity={0.8}>
                {deltaDeLista(menu, lista, o.id, Number(o.priceDelta)) > 0 ? '+' : ''}{money(deltaDeLista(menu, lista, o.id, Number(o.priceDelta)))}
              </Text>
            )}
          </Button>
          {repetible && (
            <Button
              aria-label={`Otra vez ${o.name}`}
              size="lg" minH="48px" minW="48px" px={0}
              borderLeftRadius={0} borderLeftWidth="1px" borderLeftColor="bg.panel"
              onClick={() => setSel((st) => ({ ...st, [g.id]: sumarUna(st[g.id] ?? {}, o.id) }))}
            >
              <LuPlus />
            </Button>
          )}
          </HStack>
        </WrapItem>
      );
    };

    // archivadas (con el toggle "ver archivadas"): al elegirlas se reactivan para la venta.
    const archived = showInactive
      ? archivedFor(g).filter((o) => !q || normalize(o.name).includes(q))
      : [];
    const archBlock = archived.length > 0 ? (
      <Box>
        <Text fontSize="2xs" color="orange.500" fontWeight="700" letterSpacing="wide" textTransform="uppercase" mb={1}>
          Archivadas · se reactivan al elegirlas
        </Text>
        <Wrap gap={2}>
          {archived.map((ao) => (
            <WrapItem key={`arch-${ao.id}`}>
              <Button size="lg" minH="48px" variant="outline" colorPalette="orange"
                loading={reactivate.isPending && reactivate.variables?.ao.id === ao.id}
                onClick={() => reactivate.mutate({ g, ao })}>
                <LuArchiveRestore size={14} />
                <Text as="span" ml={1}>{ao.name}</Text>
                {Number(ao.priceDelta) !== 0 && (
                  <Text as="span" ml={1} fontSize="xs" opacity={0.8}>
                    {Number(ao.priceDelta) > 0 ? '+' : ''}{money(ao.priceDelta)}
                  </Text>
                )}
              </Button>
            </WrapItem>
          ))}
        </Wrap>
      </Box>
    ) : null;

    if (q) {
      return (
        <VStack align="stretch" gap={2}>
          <Wrap gap={2}>{visibleOptions(g).map(btn)}</Wrap>
          {archBlock}
        </VStack>
      );
    }

    const opts = optsOf(g).filter((o) => !hiddenIds.has(o.id)); // + reactivadas, − recién archivadas
    const top = ranked.map((r) => opts.find((o) => o.id === r.id)).filter((o): o is MenuOption => !!o);
    // "Todas" respeta el orden manual del catálogo (sort_key); el menú ya llega ordenado así.
    const rest = opts.filter((o) => !rankedIds.has(o.id));
    return (
      <VStack align="stretch" gap={2}>
        {top.length > 0 && <Wrap gap={2}>{top.map(btn)}</Wrap>}
        {top.length > 0 && rest.length > 0 && (
          <Text fontSize="2xs" color="fg.subtle" fontWeight="700" letterSpacing="wide" textTransform="uppercase">
            Todas
          </Text>
        )}
        {rest.length > 0 && <Wrap gap={2}>{rest.map(btn)}</Wrap>}
        {archBlock}
      </VStack>
    );
  };

  return (
    <>
    <DrawerRoot open={isOpen} placement="bottom" onOpenChange={(e) => { if (!e.open) onClose(); }} size="full">
      <DrawerBackdrop />
      <DrawerContent
        colorPalette={palette}
        borderRadius={0}
        h="100dvh"
        maxH="100dvh"
        w="100vw"
        maxW="100vw"
        // POS táctil: al tocar/mantener una opción no debe salir el menú de selección de texto de
        // Android sobre nombres/precios. Los inputs (buscar opción) sí quedan editables.
        css={{
          userSelect: 'none',
          WebkitUserSelect: 'none',
          '& input, & textarea': { userSelect: 'text', WebkitUserSelect: 'text' },
        }}
        style={{
          transform: swipe.offset ? `translateY(${swipe.offset}px)` : undefined,
          transition: swipe.dragging ? 'none' : 'transform 0.2s ease',
        }}
      >
        <DrawerGrabber {...swipe.handlers} />
        <DrawerCloseTrigger />
        {/* El gesto de cerrar cubre todo el header (más fácil de agarrar en tablet que solo
            el grip), no el body: ahí abajo hay scroll y hay que dejarlo libre. */}
        <DrawerHeader pb={2} style={{ touchAction: 'none' }} {...swipe.handlers}>
          <Text fontSize="lg" fontWeight="700">{product.name}</Text>
          {/* Con una plataforma activa se cobra su lista, así que mostrar el precio base aquí
              contradecía al ticket. El operador tiene que ver el número que va a cobrar. */}
          {lista === null ? (
            <Text fontSize="sm" color="fg.muted">{money(product.price)} base</Text>
          ) : (
            <Text fontSize="sm" fontWeight="600" color="orange.fg">
              {money(precioDeLista(menu, lista, product.id, Number(product.price)))} en {nombreDeLista(menu, lista)}
            </Text>
          )}
          {showSearch && (
            <HStack mt={2} px={3} borderWidth="1px" borderRadius="lg" bg="bg.subtle">
              <LuSearch />
              <Input
                variant="flushed" border="none" placeholder="Buscar opción…"
                value={optQuery} onChange={(e) => setOptQuery(e.target.value)}
              />
            </HStack>
          )}
          {canManage && (
            <Button mt={2} size="sm" variant={showInactive ? 'solid' : 'outline'}
              colorPalette={showInactive ? 'orange' : 'gray'} onClick={() => setShowInactive((v) => !v)}>
              <LuArchiveRestore /> {showInactive ? 'Ocultar archivadas' : 'Ver archivadas'}
            </Button>
          )}
          {/* Misma pista que en el catálogo: el gesto es el mismo y uno que nadie ve no existe. */}
          {lista !== null && (
            <Text fontSize="xs" color="fg.muted" mt={1}>
              Mantén presionado un extra para corregir su cargo.
            </Text>
          )}
        </DrawerHeader>
        <DrawerBody>
          {/* flujo en columnas (masonry): los grupos cortos empacan juntos y uno
              largo baja por su propia columna sin dejar huecos al lado. */}
          <Box
            css={{
              columnGap: '1rem',
              '@media(min-width:48em)': { columnCount: 2 },
              '@media(min-width:80em)': { columnCount: 3 },
            }}
          >
            {orderedGroups.map((g) => {
              const hasArchived = showInactive && archivedFor(g).length > 0;
              if (visibleOptions(g).length === 0 && !hasArchived) return null; // sin coincidencias
              const single = g.max === 1;
              const isFirstUnmet = g.id === firstUnmetId;
              const totalCount = countIn(g.id);
              const need = totalCount < g.min;
              return (
                <Box key={g.id} display="inline-block" w="100%" mb={4} p={4} borderWidth="1px" borderColor="border.muted" borderRadius="lg" bg="bg.panel" css={{ breakInside: 'avoid' }}>
                  <HStack justify="space-between" mb={2} align="baseline">
                    <Text fontWeight="700" color={isFirstUnmet ? 'colorPalette.600' : undefined}>
                      {g.title}
                      {g.min > 0 && <Text as="span" color="colorPalette.500" ml={1}>*</Text>}
                    </Text>
                    <Text fontSize="xs" color={need ? 'colorPalette.600' : 'fg.muted'} fontWeight="600" whiteSpace="nowrap">
                      {isFirstUnmet ? '← empieza aquí'
                        : single ? (g.min >= 1 ? 'elige 1' : 'opcional')
                          : `${totalCount}/${g.max}${g.min > 0 ? ` · mín ${g.min}` : ''}`}
                    </Text>
                  </HStack>

                  {optionButtons(g)}
                </Box>
              );
            })}
          </Box>

          {/* pb generoso: en 7" el textarea de 2 líneas no debe pegarse al footer (qty/Agregar) */}
          <Box mt={2} pb={6}>
            <Text fontWeight="600" mb={2}>Nota de cocina</Text>
            <Textarea rows={2} resize="none" placeholder="Ej. sin cebolla" value={notes} onChange={(e) => setNotes(e.target.value)} />
          </Box>
        </DrawerBody>
        <DrawerFooter borderTopWidth="1px">
          <Flex w="100%" align="center" gap={3}>
            <HStack>
              <Button size="lg" onClick={() => setQty((n) => Math.max(1, n - 1))}>−</Button>
              <Text minW="24px" textAlign="center" fontWeight="700">{qty}</Text>
              <Button size="lg" onClick={() => setQty((n) => n + 1)}>+</Button>
            </HStack>
            <Button flex="1" size="lg" disabled={!canConfirm} onClick={confirm}>
              {canConfirm ? `Agregar ${qty} · ${money(total)}` : `Falta: ${unmet[0].title}`}
            </Button>
          </Flex>
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>

    {/* Gestionar opción (long-press / clic derecho) — desactivar para quitar basura del POS */}
    <DialogRoot open={manageOpt !== null} onOpenChange={(e) => { if (!e.open) setManageOpt(null); }} placement="center" size="xs">
      <DialogBackdrop />
      <DialogContent colorPalette={palette} mx={4} borderRadius="2xl">
        <DialogBody py={5}>
          <Text fontWeight="700" fontSize="lg" mb={desgloseEnEdicion ? 3 : 1}>{manageOpt?.name}</Text>
          {desgloseEnEdicion && manageOpt && lista !== null && (
            <OptionPriceFields
              key={manageOpt.id}
              optionId={manageOpt.id}
              optionName={manageOpt.name}
              plataforma={nombreDeLista(menu, lista)}
              plataformaId={lista}
              desglose={desgloseEnEdicion}
              onDone={() => setManageOpt(null)}
            />
          )}
          <VStack align="stretch" gap={2} mt={desgloseEnEdicion ? 5 : 0}>
            {canManage && (
              <>
                <Text fontSize="sm" color="fg.muted">
                  Archivarla la oculta de esta lista. Puedes reactivarla en Admin → Opciones.
                </Text>
                <Button size="lg" colorPalette="red" loading={deactivate.isPending}
                  onClick={() => manageOpt && deactivate.mutate(manageOpt.id)}>
                  Archivar opción
                </Button>
              </>
            )}
            <Button size="lg" variant="ghost" onClick={() => setManageOpt(null)}>Cancelar</Button>
          </VStack>
        </DialogBody>
      </DialogContent>
    </DialogRoot>
    </>
  );
}
