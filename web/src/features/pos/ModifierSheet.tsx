/* eslint-disable react-hooks/set-state-in-effect */
// Patrón legítimo: reinicia el estado del sheet cuando se abre para otro producto/edición.
import { useEffect, useRef, useState } from 'react';
import {
  Box, Button, HStack, VStack, Text, Wrap, WrapItem, Input, Flex,
} from '@chakra-ui/react';
import { LuSearch, LuStar, LuSplit } from 'react-icons/lu';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
  DrawerCloseTrigger,
} from '../../components/ui/drawer';
import type { MenuGroup, MenuOption, MenuProduct, TicketModifier } from '../../types/pos';
import { money, normalize } from '../../utils/format';
import { useUiStore } from '../../stores/ui';

interface Props {
  product: MenuProduct | null;
  isOpen: boolean;
  initialModifiers?: TicketModifier[];
  initialNotes?: string;
  optionRanks?: Record<string, number[]>; // grupo(id) → opciones rankeadas por contexto
  onClose: () => void;
  onConfirm: (modifiers: TicketModifier[], notes: string, qty: number) => void;
}

// selección: porción ('' entero | 'A' | 'B') -> groupId -> optionId -> cantidad
type Sel = Record<string, Record<number, Record<number, number>>>;

const SEARCH_THRESHOLD = 12; // muestra el buscador de opciones si hay más de esto

export function ModifierSheet({ product, isOpen, initialModifiers, initialNotes, optionRanks, onClose, onConfirm }: Props) {
  const [sel, setSel] = useState<Sel>({});
  const [notes, setNotes] = useState('');
  const [qty, setQty] = useState(1);
  const [splitOn, setSplitOn] = useState(false);
  const [optQuery, setOptQuery] = useState('');
  const palette = useUiStore((s) => s.palette);

  // orden de inserción de picks multi-select (las claves numéricas de sel no lo conservan),
  // para expulsar la más antigua al llegar al tope (FIFO).
  const pickSeq = useRef(0);
  const pickOrder = useRef<Record<string, number>>({});
  const okey = (portion: string, gid: number, oid: number) => `${portion}:${gid}:${oid}`;

  const ranksFor = (gid: number): number[] => optionRanks?.[String(gid)] ?? [];

  // inicializa al abrir: restaura edición, o preselecciona el default contextual
  // (top rankeado, o el primero) en grupos de elección única obligatoria.
  useEffect(() => {
    if (!isOpen || !product) return;
    const s: Sel = {};
    pickOrder.current = {};
    pickSeq.current = 0;
    const put = (portion: string, gid: number, oid: number, q: number) => {
      (s[portion] ??= {});
      (s[portion][gid] ??= {});
      s[portion][gid][oid] = q;
      pickOrder.current[okey(portion, gid, oid)] = pickSeq.current++;
    };
    let split = false;
    if (initialModifiers?.length) {
      for (const m of initialModifiers) {
        if (m.portion) split = true;
        put(m.portion ?? '', m.groupId, m.optionId, m.qty);
      }
    } else {
      // pre-marcado inteligente: en TODO grupo con variantes, pre-elige lo que el
      // sistema de análisis (optionRanks por contexto) marca como más probable.
      // single-select → 1 opción; multi-select → su mínimo requerido (opcional multi = 0).
      for (const g of product.groups) {
        const count = g.max === 1 ? 1 : g.min;
        if (count <= 0 || g.options.length === 0) continue;
        const rank = new Map(ranksFor(g.id).map((id, i) => [id, i]));
        const ordered = [...g.options].sort((a, b) => {
          const ra = rank.get(a.id) ?? Infinity, rb = rank.get(b.id) ?? Infinity;
          if (ra !== rb) return ra - rb;          // 1º lo más probable por contexto
          return a.priceDelta - b.priceDelta;      // sin datos: default = más barato
        });
        for (const o of ordered.slice(0, count)) put('', g.id, o.id, 1);
      }
    }
    setSel(s);
    setSplitOn(split);
    setNotes(initialNotes ?? '');
    setQty(1);
    setOptQuery('');
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOpen, product?.id]);

  if (!product) return null;

  // grupos single-select se dividen A/B cuando el modo dividir está activo; el resto es compartido ('').
  const groupPortions = (g: MenuGroup): string[] => (splitOn && g.max === 1 ? ['A', 'B'] : ['']);
  const splittable = product.groups.some((g) => g.max === 1);
  // obligatorios primero (sort estable → conserva el orden del menú dentro de cada grupo).
  const orderedGroups = [...product.groups].sort((a, b) => (b.min > 0 ? 1 : 0) - (a.min > 0 ? 1 : 0));

  const countIn = (portion: string, gid: number) =>
    Object.values(sel[portion]?.[gid] ?? {}).reduce((a, b) => a + b, 0);

  const setSingle = (portion: string, gid: number, oid: number) =>
    setSel((s) => ({ ...s, [portion]: { ...(s[portion] ?? {}), [gid]: { [oid]: 1 } } }));

  const toggleMulti = (gid: number, oid: number, max: number) =>
    setSel((s) => {
      const grp = { ...((s[''] ?? {})[gid] ?? {}) };
      if (grp[oid]) {
        delete grp[oid];
        delete pickOrder.current[okey('', gid, oid)];
      } else {
        // al tope: FIFO — expulsa la más antigua para que el nuevo pick entre en un solo toque.
        if (Object.values(grp).reduce((a, b) => a + b, 0) >= max) {
          let oldest: number | null = null, min = Infinity;
          for (const k of Object.keys(grp)) {
            const seq = pickOrder.current[okey('', gid, Number(k))] ?? 0;
            if (seq < min) { min = seq; oldest = Number(k); }
          }
          if (oldest !== null) { delete grp[oldest]; delete pickOrder.current[okey('', gid, oldest)]; }
        }
        grp[oid] = 1;
        pickOrder.current[okey('', gid, oid)] = pickSeq.current++;
      }
      return { ...s, ['']: { ...(s[''] ?? {}), [gid]: grp } };
    });

  // al activar dividir, duplica la elección única a ambas mitades; al desactivar, colapsa a la mitad A.
  const toggleSplit = () => {
    setSel((s) => {
      const next: Sel = JSON.parse(JSON.stringify(s)); // ponytail: clon simple, estado chico
      for (const g of product.groups) {
        if (g.max !== 1) continue;
        if (!splitOn) {
          const cur = next['']?.[g.id];
          if (cur) {
            (next['A'] ??= {})[g.id] = { ...cur };
            (next['B'] ??= {})[g.id] = { ...cur };
            delete next[''][g.id];
          }
        } else {
          const a = next['A']?.[g.id];
          if (a) (next[''] ??= {})[g.id] = { ...a };
          if (next['A']) delete next['A'][g.id];
          if (next['B']) delete next['B'][g.id];
        }
      }
      return next;
    });
    setSplitOn((v) => !v);
  };

  const optById = new Map<number, MenuOption>();
  product.groups.forEach((g) => g.options.forEach((o) => optById.set(o.id, o)));

  let perUnitDelta = 0;
  for (const groups of Object.values(sel)) {
    for (const picks of Object.values(groups)) {
      for (const [oid, q] of Object.entries(picks)) {
        const o = optById.get(Number(oid));
        if (o) perUnitDelta += o.priceDelta * q;
      }
    }
  }
  const unit = product.price + perUnitDelta;
  const total = Math.round(unit * qty * 100) / 100;

  const unmet = orderedGroups.filter((g) => groupPortions(g).some((p) => countIn(p, g.id) < g.min));
  const canConfirm = unmet.length === 0;
  const firstUnmetId = unmet[0]?.id ?? null;

  // ordena las opciones: rankeadas por contexto primero, luego el resto (sort estable).
  const orderedOptions = (g: MenuGroup): MenuOption[] => {
    const r = ranksFor(g.id);
    if (r.length === 0) return g.options;
    const rank = new Map(r.map((id, i) => [id, i]));
    return [...g.options].sort((a, b) => (rank.get(a.id) ?? Infinity) - (rank.get(b.id) ?? Infinity));
  };

  const q = normalize(optQuery);
  const showSearch = product.groups.reduce((n, g) => n + g.options.length, 0) > SEARCH_THRESHOLD;
  const visibleOptions = (g: MenuGroup) =>
    orderedOptions(g).filter((o) => !q || normalize(o.name).includes(q));

  const confirm = () => {
    const modifiers: TicketModifier[] = [];
    for (const g of product.groups) {
      for (const portion of groupPortions(g)) {
        const picks = sel[portion]?.[g.id] ?? {};
        for (const o of g.options) {
          if (picks[o.id]) {
            modifiers.push({
              optionId: o.id, groupId: g.id, name: o.name, priceDelta: o.priceDelta,
              qty: picks[o.id], portion: portion === '' ? undefined : (portion as 'A' | 'B'),
            });
          }
        }
      }
    }
    onConfirm(modifiers, notes.trim(), qty);
    onClose();
  };

  // renderiza los botones de opción de un grupo para una porción dada.
  const optionButtons = (g: MenuGroup, portion: string) => {
    const single = g.max === 1;
    const picks = sel[portion]?.[g.id] ?? {};
    const suggested = ranksFor(g.id)[0];
    return (
      <Wrap gap={2}>
        {visibleOptions(g).map((o) => {
          const on = !!picks[o.id];
          // ⭐ solo como pista en grupos que NO se auto-seleccionan (opcionales/múltiples).
          const star = o.id === suggested && !(g.min >= 1 && single) && !on;
          return (
            <WrapItem key={o.id}>
              <Button
                size="lg" minH="48px"
                variant={on ? 'solid' : 'outline'}
                colorPalette={on ? undefined : 'gray'}
                onClick={() => (single ? setSingle(portion, g.id, o.id) : toggleMulti(g.id, o.id, g.max))}
              >
                {star && <Box as="span" color="yellow.500" mr={1}><LuStar size={14} /></Box>}
                {o.name}
                {o.priceDelta !== 0 && (
                  <Text as="span" ml={1} fontSize="xs" opacity={0.8}>
                    {o.priceDelta > 0 ? '+' : ''}{money(o.priceDelta)}
                  </Text>
                )}
              </Button>
            </WrapItem>
          );
        })}
      </Wrap>
    );
  };

  return (
    <DrawerRoot open={isOpen} placement="bottom" onOpenChange={(e) => { if (!e.open) onClose(); }} size="full">
      <DrawerBackdrop />
      <DrawerContent
        colorPalette={palette}
        borderRadius={0}
        h="100dvh"
        maxH="100dvh"
        w="100vw"
        maxW="100vw"
      >
        <DrawerCloseTrigger />
        <DrawerHeader pb={2}>
          <Text fontSize="lg" fontWeight="700">{product.name}</Text>
          <Text fontSize="sm" color="fg.muted">{money(product.price)} base</Text>
          {splittable && (
            <Button
              mt={2} size="sm"
              variant={splitOn ? 'solid' : 'outline'}
              colorPalette={splitOn ? 'purple' : 'gray'}
              onClick={toggleSplit}
            >
              <LuSplit /> {splitOn ? 'Dividido en mitades' : 'Dividir (mitad y mitad)'}
            </Button>
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
              if (visibleOptions(g).length === 0) return null; // oculto al filtrar sin coincidencias
              const single = g.max === 1;
              const isFirstUnmet = g.id === firstUnmetId;
              const totalCount = groupPortions(g).reduce((n, p) => n + countIn(p, g.id), 0);
              const need = groupPortions(g).some((p) => countIn(p, g.id) < g.min);
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

                  {groupPortions(g).length === 1
                    ? optionButtons(g, '')
                    : (
                      <VStack align="stretch" gap={2}>
                        {(['A', 'B'] as const).map((p) => (
                          <Box key={p}>
                            <Text fontSize="sm" fontWeight="700" color="purple.600" mb={1}>Mitad {p}</Text>
                            {optionButtons(g, p)}
                          </Box>
                        ))}
                      </VStack>
                    )}
                </Box>
              );
            })}
          </Box>

          <Box mt={2}>
            <Text fontWeight="600" mb={2}>Nota de cocina</Text>
            <Input placeholder="Ej. sin cebolla" value={notes} onChange={(e) => setNotes(e.target.value)} />
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
  );
}
