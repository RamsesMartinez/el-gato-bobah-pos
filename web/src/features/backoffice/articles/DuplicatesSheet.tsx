import { useEffect, useRef, useState } from 'react';
import { Box, Button, Center, HStack, Spinner, Stack, Text } from '@chakra-ui/react';
import { useQuery, useQueryClient } from '@tanstack/react-query';

import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
} from '../../../components/ui/drawer';
import { backofficeApi, type DuplicateGroup, type DuplicateItem } from '../../../api/backoffice';
import { medirAccion } from '../../../api/uso';
import { formatBaseQty } from '../../../shared/units';
import { MergeSheet } from './MergeSheet';

// Posibles duplicados (spec 056, maqueta «Unir insumos», opción C): los grupos de insumos con nombres
// parecidos. Tocar un grupo lo abre en su lugar (revisión de tableta: nunca tres hojas): cuál se queda,
// «No son lo mismo» por par y «Revisar unión», que abre la misma hoja de unir que la ficha.

interface Props {
  onClose: () => void;
  onMerged: () => void;
}

// UNDO_MS: «No son lo mismo» esconde el par para siempre; mientras dura el aviso se puede deshacer, y
// solo al terminar se guarda (el par es un hecho sin borrar en la base).
const UNDO_MS = 5000;

type Pair = [number, number];
const pairKey = ([a, b]: Pair) => `${Math.min(a, b)}-${Math.max(a, b)}`;

function pairsOf(items: DuplicateItem[]): [DuplicateItem, DuplicateItem][] {
  const out: [DuplicateItem, DuplicateItem][] = [];
  for (let i = 0; i < items.length; i++) for (let j = i + 1; j < items.length; j++) out.push([items[i], items[j]]);
  return out;
}

export function DuplicatesSheet({ onClose, onMerged }: Props) {
  const qc = useQueryClient();
  const groups = useQuery({ queryKey: ['duplicate-groups'], queryFn: backofficeApi.duplicateGroups });
  const [open, setOpen] = useState<number | null>(null);
  const [keep, setKeep] = useState<Record<number, number>>({});
  const [hidden, setHidden] = useState<Set<string>>(new Set());
  const [pending, setPending] = useState<{ pair: Pair; label: string } | null>(null);
  const [merging, setMerging] = useState<{ target: DuplicateItem; sources: number[] } | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const pendingRef = useRef<Pair | null>(null);

  const save = (pair: Pair) => {
    pendingRef.current = null;
    void backofficeApi.markDistinct(pair[0], pair[1]).then(() => qc.invalidateQueries({ queryKey: ['duplicate-groups'] }));
    medirAccion('inventario', 'no-son-lo-mismo');
  };
  // Al cerrar la hoja con un par por guardar, se guarda: «Deshacer» es solo mientras se ve el aviso.
  useEffect(() => () => {
    if (timer.current) clearTimeout(timer.current);
    if (pendingRef.current) save(pendingRef.current);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const distinct = (a: DuplicateItem, b: DuplicateItem) => {
    if (timer.current) clearTimeout(timer.current);
    if (pendingRef.current) save(pendingRef.current);
    const pair: Pair = [a.id, b.id];
    pendingRef.current = pair;
    setHidden((h) => new Set(h).add(pairKey(pair)));
    setPending({ pair, label: `${a.name} y ${b.name}` });
    timer.current = setTimeout(() => { save(pair); setPending(null); }, UNDO_MS);
  };
  const undo = () => {
    if (timer.current) clearTimeout(timer.current);
    if (pending) setHidden((h) => { const n = new Set(h); n.delete(pairKey(pending.pair)); return n; });
    pendingRef.current = null;
    setPending(null);
  };

  const list = (groups.data?.groups ?? []).map((g: DuplicateGroup) => g.items ?? []).filter((items) => items.length > 1);

  return (
    <>
      <DrawerRoot open placement="bottom" onOpenChange={(e) => { if (!e.open) onClose(); }}>
        <DrawerBackdrop />
        <DrawerContent borderTopRadius="2xl" maxH="92dvh" display="flex" flexDirection="column">
          <DrawerHeader py={3}>
            <Text fontWeight="700" fontSize="lg">Posibles duplicados</Text>
            <Text fontSize="sm" color="fg.muted">Insumos con nombres parecidos. Revisa si son lo mismo con otra marca.</Text>
          </DrawerHeader>
          <DrawerBody flex="1" minH={0} overflowY="auto">
            {groups.isLoading ? <Center py={8}><Spinner /></Center> : groups.isError ? (
              // Sin esto, una caída de red se leía como «no hay duplicados» y se dejaba de revisar.
              <HStack gap={3} bg="red.subtle" borderRadius="lg" p={3}>
                <Text flex="1" role="alert">No se pudieron cargar los posibles duplicados.</Text>
                <Button minH="44px" variant="outline" bg="bg" onClick={() => void groups.refetch()}>Reintentar</Button>
              </HStack>
            ) : list.length === 0 ? (
              <Text color="fg.muted" py={4}>No hay insumos con nombres parecidos.</Text>
            ) : (
              <Stack gap={2}>
                {list.map((items, gi) => {
                  const expanded = open === gi;
                  const kept = keep[gi] ?? items[0].id;
                  const pairs = pairsOf(items).filter(([a, b]) => !hidden.has(pairKey([a.id, b.id])));
                  return (
                    <Box key={items.map((i) => i.id).join('-')} borderWidth="1px" borderRadius="lg" overflow="hidden">
                      <Button variant="ghost" w="100%" minH="52px" justifyContent="space-between" borderRadius="0"
                        aria-expanded={expanded} onClick={() => setOpen(expanded ? null : gi)}>
                        <Text truncate textAlign="start">{items.map((i) => i.name).join(' · ')}</Text>
                        <Text color="fg.muted" flexShrink={0}>{expanded ? 'Cerrar' : 'Revisar ›'}</Text>
                      </Button>
                      {expanded && (
                        <Stack role="group" aria-label={items[0].name} gap={0} px={3} pb={3} borderTopWidth="1px">
                          <Text fontWeight="600" pt={2}>¿Cuál se queda?</Text>
                          {items.map((it) => (
                            <HStack key={it.id} as="button" role="radio" aria-checked={kept === it.id} aria-label={`Se queda ${it.name}`}
                              minH="52px" w="100%" gap={3} borderBottomWidth="1px" textAlign="start" cursor="pointer"
                              onClick={() => setKeep((k) => ({ ...k, [gi]: it.id }))}>
                              <Box w="22px" h="22px" borderRadius="full" borderWidth="2px" flexShrink={0}
                                borderColor={kept === it.id ? 'brand.solid' : 'border'} bg={kept === it.id ? 'brand.solid' : undefined} />
                              <Text flex="1" minW={0}>{it.name}</Text>
                              <Text color="fg.muted">{formatBaseQty(it.onHand, it.baseUnitCode)}</Text>
                            </HStack>
                          ))}
                          {pairs.map(([a, b]) => (
                            <HStack key={pairKey([a.id, b.id])} minH="48px" gap={4}>
                              <Text flex="1" fontSize="sm" color="fg.muted" minW={0}>{a.name} · {b.name}</Text>
                              <Button minH="44px" variant="ghost" colorPalette="gray" aria-label={`No son lo mismo: ${a.name} y ${b.name}`}
                                onClick={() => distinct(a, b)}>No son lo mismo</Button>
                            </HStack>
                          ))}
                          <Box pt={3}>
                            <Button minH="44px" colorPalette="brand" onClick={() => {
                              const target = items.find((i) => i.id === kept)!;
                              setMerging({ target, sources: items.filter((i) => i.id !== kept).map((i) => i.id) });
                            }}>Revisar unión</Button>
                          </Box>
                        </Stack>
                      )}
                    </Box>
                  );
                })}
              </Stack>
            )}
          </DrawerBody>
          <DrawerFooter>
            <HStack w="100%" gap={3}>
              {pending ? (
                <>
                  <Text flex="1" fontSize="sm" role="status">Par descartado: {pending.label}</Text>
                  <Button minH="44px" variant="outline" onClick={undo}>Deshacer</Button>
                </>
              ) : <Box flex="1" />}
              <Button minH="44px" variant="outline" onClick={onClose}>Listo</Button>
            </HStack>
          </DrawerFooter>
        </DrawerContent>
      </DrawerRoot>
      {merging && (
        <MergeSheet target={merging.target} initialSources={merging.sources} onClose={() => setMerging(null)}
          onDone={() => { setMerging(null); setOpen(null); void qc.invalidateQueries({ queryKey: ['duplicate-groups'] }); onMerged(); }} />
      )}
    </>
  );
}
