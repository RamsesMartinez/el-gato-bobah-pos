import { useEffect, useMemo, useState } from 'react';
import { Badge, Box, Button, Center, HStack, Input, Spinner, Stack, Table, Text } from '@chakra-ui/react';
import { useQuery } from '@tanstack/react-query';

import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
} from '../../../components/ui/drawer';
import { backofficeApi, type ArticleRow, type MergePreview } from '../../../api/backoffice';
import { ApiError } from '../../../api/client';
import { mensajeDeError } from '../../../api/mensajes';
import { medirAccion } from '../../../api/uso';
import { toaster } from '../../../components/ui/toaster';
import { NumericKeypad } from '../../../components/NumericKeypad';
import { useKeypadField } from '../../../components/useKeypadField';
import { PreviewGroup, PreviewTable, StaleNotice, num, signed, tone } from '../../../shared/preview/PreviewTable';
import { formatBaseQty, formatUnitCost } from '../../../shared/units';
import { matchesWords } from '../../../utils/format';

// Unir insumos (spec 056, maqueta «Unir insumos», opción A): desde la ficha del que se queda. Tres
// pasos en la misma hoja, nunca una hoja encima de otra (revisión de tableta): elegir cuáles se unen
// (los parecidos arriba), la equivalencia de los que se miden distinto, y la vista previa que calcula
// el servidor con la misma transacción que confirma, revertida.

interface Target { id: number; name: string; baseUnitId: number; baseUnitCode: string }

interface Props {
  target: Target;
  // Abierta desde un grupo de posibles duplicados: los demás del grupo llegan marcados.
  initialSources?: number[];
  onClose: () => void;
  onDone: () => void;
}

type Step = 'pick' | 'equiv' | 'preview';

const MAX_SIMILAR = 3;

// La equivalencia se pregunta como la diría alguien: siempre «1 pieza = …» si una de las dos se cuenta
// por pieza; si no, «1 <unidad del otro> = … <unidad de éste>». El servidor quiere cuánto del destino
// vale una unidad base del origen.
function equivalence(src: ArticleRow, target: Target) {
  if (src.baseUnitCode === 'pieza' || target.baseUnitCode !== 'pieza') {
    return {
      label: `1 ${src.baseUnitCode} de ${src.name} en ${target.baseUnitCode} de ${target.name}`,
      left: `1 ${src.baseUnitCode} de ${src.name}`, right: `${target.baseUnitCode} de ${target.name}`,
      factor: (x: number) => x,
    };
  }
  return {
    label: `1 pieza de ${target.name} en ${src.baseUnitCode} de ${src.name}`,
    left: `1 pieza de ${target.name}`, right: `${src.baseUnitCode} de ${src.name}`,
    factor: (x: number) => 1 / x,
  };
}

export function MergeSheet({ target, initialSources = [], onClose, onDone }: Props) {
  // Monta cerrada y abre en el render siguiente: se abre desde otra hoja que se cierra en la misma
  // actualización, y sin esa transición Chakra no la monta (AGENTS.md §3, CobrarSheet).
  const [visible, setVisible] = useState(false);
  // eslint-disable-next-line react-hooks/set-state-in-effect
  useEffect(() => { setVisible(true); }, []);

  const [step, setStep] = useState<Step>('pick');
  const [selected, setSelected] = useState<number[]>(initialSources.filter((id) => id !== target.id));
  const [needle, setNeedle] = useState('');
  const [allSimilar, setAllSimilar] = useState(false);
  const [equiv, setEquiv] = useState<Record<number, string>>({});
  const [preview, setPreview] = useState<MergePreview | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [stale, setStale] = useState(false);
  const keypad = useKeypadField<string>();

  const articles = useQuery({ queryKey: ['articles', 'merge'], queryFn: () => backofficeApi.articles({}) });
  const similar = useQuery({ queryKey: ['articles-similar', target.name], queryFn: () => backofficeApi.similarArticles(target.name) });

  const candidates = useMemo(
    () => (articles.data ?? []).filter((a) => a.isActive && !a.isPrep && a.id !== target.id),
    [articles.data, target.id],
  );
  const byId = useMemo(() => new Map(candidates.map((a) => [a.id, a])), [candidates]);
  // Los parecidos que dice el servidor, y antes los que ya llegaron marcados de un grupo.
  const similarRows = useMemo(() => {
    const ids = [...initialSources, ...(similar.data?.items ?? []).map((s) => s.id)];
    return [...new Set(ids)].map((id) => byId.get(id)).filter((a): a is ArticleRow => !!a);
  }, [initialSources, similar.data, byId]);
  const shownSimilar = allSimilar ? similarRows : similarRows.slice(0, MAX_SIMILAR);
  const others = candidates.filter((a) => !similarRows.includes(a) && (needle.trim() === '' || matchesWords(a.name, needle)));

  const chosen = selected.map((id) => byId.get(id)).filter((a): a is ArticleRow => !!a);
  const differing = chosen.filter((a) => a.baseUnitId !== target.baseUnitId);
  const equivReady = differing.every((a) => Number(equiv[a.id]) > 0);

  const toggle = (id: number) => setSelected((s) => (s.includes(id) ? s.filter((x) => x !== id) : [...s, id]));

  const body = () => ({
    sources: chosen.map((a) => {
      if (a.baseUnitId === target.baseUnitId) return { id: a.id };
      const f = equivalence(a, target).factor(Number(equiv[a.id]));
      return { id: a.id, factor: String(Number(f.toPrecision(12))) };
    }),
  });

  const ask = async () => {
    setLoading(true);
    setError('');
    try {
      setPreview(await backofficeApi.mergeArticles(target.id, body(), true));
      setStep('preview');
    } catch (e) {
      setError(mensajeDeError(e));
    } finally {
      setLoading(false);
    }
  };

  const next = () => {
    if (differing.length > 0) setStep('equiv');
    else void ask();
  };

  const confirm = async () => {
    if (!preview) return;
    setLoading(true);
    setError('');
    try {
      await backofficeApi.mergeArticles(target.id, { ...body(), fingerprint: preview.fingerprint }, false);
      medirAccion('inventario', 'unir-insumos');
      toaster.create({
        title: chosen.length === 1 ? `Unido. ${chosen[0].name} quedó archivado.` : `Unidos. ${chosen.length} insumos quedaron archivados.`,
        type: 'success',
      });
      onDone();
    } catch (e) {
      if (e instanceof ApiError && e.code === 'MERGE_STALE') {
        setStale(true);
        setLoading(false);
        await ask();
        return;
      }
      setError(mensajeDeError(e));
    } finally {
      setLoading(false);
    }
  };

  const conflicts = (preview?.presentations ?? []).filter((p) => p.action === 'conflict');

  const row = (a: ArticleRow) => {
    const on = selected.includes(a.id);
    return (
      <HStack key={a.id} as="button" role="checkbox" aria-checked={on} aria-label={`${a.name} · ${formatBaseQty(a.onHand, a.baseUnitCode)}`}
        minH="52px" w="100%" px={3} gap={3} borderBottomWidth="1px" textAlign="start" cursor="pointer"
        bg={on ? 'brand.subtle' : undefined} onClick={() => toggle(a.id)}>
        <Box w="26px" h="26px" borderRadius="md" borderWidth="2px" flexShrink={0} display="grid" placeItems="center"
          borderColor={on ? 'brand.solid' : 'border'} bg={on ? 'brand.solid' : undefined} color="white" fontWeight="700">{on ? '✓' : ''}</Box>
        <Text flex="1" minW={0}>{a.name}</Text>
        {a.baseUnitId !== target.baseUnitId && <Badge colorPalette="orange">se mide en {a.baseUnitCode}</Badge>}
        <Text color="fg.muted" flexShrink={0}>{formatBaseQty(a.onHand, a.baseUnitCode)}</Text>
      </HStack>
    );
  };

  return (
    <DrawerRoot open={visible} placement="bottom" onOpenChange={(e) => { if (!e.open) onClose(); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="2xl" maxH="92dvh" display="flex" flexDirection="column">
        <DrawerHeader py={3}>
          <HStack>
            <Text fontWeight="700" fontSize="lg" flex="1">
              {step === 'preview' ? 'Antes de unir' : step === 'equiv' ? '¿Cuánto equivale?' : `Unir en ${target.name}`}
            </Text>
            {step === 'pick' && <Text color="fg.muted">{selected.length} elegido{selected.length === 1 ? '' : 's'}</Text>}
          </HStack>
        </DrawerHeader>
        <DrawerBody flex="1" minH={0} overflowY="auto">
          {stale && <StaleNotice />}
          {error && <Text p={3} mb={3} borderRadius="lg" bg="red.subtle" fontSize="sm" role="alert">{error}</Text>}

          {step === 'pick' && (
            <Stack gap={2}>
              <Input minH="48px" aria-label="Buscar otro insumo" placeholder="Buscar otro insumo…" value={needle} onChange={(e) => setNeedle(e.target.value)} />
              {articles.isLoading ? <Center py={6}><Spinner /></Center> : (
                <>
                  {similarRows.length > 0 && (
                    <Box role="group" aria-label="Parecidos">
                      <Text fontSize="xs" fontWeight="700" color="fg.muted" textTransform="uppercase" letterSpacing="wide" py={1}>Parecidos</Text>
                      {shownSimilar.map(row)}
                      {!allSimilar && similarRows.length > MAX_SIMILAR && (
                        <Button variant="ghost" minH="44px" onClick={() => setAllSimilar(true)} aria-label="Ver todos los parecidos">
                          Ver todos ({similarRows.length})
                        </Button>
                      )}
                    </Box>
                  )}
                  <Box role="group" aria-label="Todos">
                    <Text fontSize="xs" fontWeight="700" color="fg.muted" textTransform="uppercase" letterSpacing="wide" py={1}>Todos</Text>
                    {others.map(row)}
                    {others.length === 0 && <Text color="fg.muted" py={2}>No hay otro insumo con ese nombre.</Text>}
                  </Box>
                </>
              )}
            </Stack>
          )}

          {step === 'equiv' && (
            <Stack gap={3}>
              <Text color="fg.muted">Estos se miden distinto que {target.name}. Escribe cuánto equivale cada uno.</Text>
              {differing.map((a) => {
                const eq = equivalence(a, target);
                const key = String(a.id);
                return (
                  <HStack key={a.id} gap={3} flexWrap="wrap" minH="52px">
                    <Text>{eq.left} =</Text>
                    <Input ref={keypad.ref(key)} aria-label={eq.label} inputMode="none" readOnly minH="48px" w="120px" textAlign="end"
                      value={equiv[a.id] ?? ''} borderColor={keypad.field === key ? 'brand.solid' : undefined}
                      onFocus={() => keypad.setField(key)} onClick={() => keypad.setField(key)} />
                    <Text>{eq.right}</Text>
                  </HStack>
                );
              })}
              {keypad.field && (
                <Box maxW="320px">
                  <NumericKeypad allowDecimal onKey={(k) => {
                    const id = Number(keypad.field);
                    setEquiv((e) => ({ ...e, [id]: keypad.apply(e[id] ?? '', k, true) }));
                  }} />
                </Box>
              )}
            </Stack>
          )}

          {step === 'preview' && (!preview ? <Center py={10}><Spinner size="lg" /></Center> : (
            <Box opacity={loading ? 0.6 : 1} overflowX="auto">
              {conflicts.length > 0 && (
                <Text p={3} mb={3} borderRadius="lg" bg="red.subtle" fontSize="sm">
                  {conflicts.map((c) => `«${c.name} · ${c.brand}»`).join(', ')} se llama igual que una de {target.name} y trae distinto.
                  Cambia el nombre o la marca de una en sus empaques antes de unir.
                </Text>
              )}
              <PreviewTable>
                <PreviewGroup label="Existencia" />
                {(preview.stock ?? []).map((s) => {
                  const diff = num(s.after) - num(s.before);
                  const source = s.ingredientId !== preview.target.id;
                  const branches = new Set((preview.stock ?? []).map((x) => x.branchId)).size;
                  return (
                    <Table.Row key={`s-${s.ingredientId}-${s.branchId}`}>
                      <Table.Cell>
                        {s.name}{branches > 1 ? ` · ${s.branch}` : ''}
                        {source && <Badge ml={2} colorPalette="gray">se archiva</Badge>}
                        {s.negativeAfter && <Text as="span" color="fg.warning"> · ⚠ quedará en negativo</Text>}
                      </Table.Cell>
                      <Table.Cell textAlign="end">{formatBaseQty(s.before, s.baseUnit)}</Table.Cell>
                      <Table.Cell textAlign="end" fontWeight="600">{formatBaseQty(s.after, s.baseUnit)}</Table.Cell>
                      <Table.Cell textAlign="end" color={tone(diff)}>{signed(diff, (v) => formatBaseQty(v, s.baseUnit))}</Table.Cell>
                    </Table.Row>
                  );
                })}
                {(preview.recipes ?? []).length > 0 && <PreviewGroup label={`Recetas que pasan a ${preview.target.name}`} />}
                {(preview.recipes ?? []).map((r, i) => {
                  const from = preview.sources?.find((s) => s.id === r.sourceId)?.name ?? '';
                  return (
                    <Table.Row key={`r-${i}`}>
                      <Table.Cell>{r.owner}</Table.Cell>
                      <Table.Cell textAlign="end">{Number(r.beforeQty)} {r.beforeUnit} de {from}</Table.Cell>
                      <Table.Cell textAlign="end" fontWeight="600">{Number(r.afterQty)} {r.afterUnit}</Table.Cell>
                      <Table.Cell textAlign="end" color="fg.muted">{r.merged ? 'se juntan' : 'cambia de insumo'}</Table.Cell>
                    </Table.Row>
                  );
                })}
                {(preview.presentations ?? []).length > 0 && <PreviewGroup label="Presentaciones" />}
                {(preview.presentations ?? []).map((p) => (
                  <Table.Row key={`p-${p.id}`}>
                    <Table.Cell colSpan={3} color={p.action === 'conflict' ? 'fg.error' : undefined}>
                      {p.name} · {p.brand} · {formatBaseQty(p.qtyInBase, p.baseUnit)}
                    </Table.Cell>
                    <Table.Cell textAlign="end" color={p.action === 'conflict' ? 'fg.error' : 'fg.muted'}>
                      {p.action === 'move' ? 'llega' : p.action === 'archive' ? 'ya existe, se archiva' : '⚠ choca'}
                    </Table.Cell>
                  </Table.Row>
                ))}
                <PreviewGroup label={`Costo por ${preview.target.baseUnit}`} />
                <Table.Row>
                  <Table.Cell>{preview.target.name}</Table.Cell>
                  <Table.Cell textAlign="end">{formatUnitCost(preview.costBefore, preview.target.baseUnit) ?? 'Sin costo'}</Table.Cell>
                  <Table.Cell textAlign="end" fontWeight="600">{formatUnitCost(preview.costAfter, preview.target.baseUnit) ?? 'Sin costo'}</Table.Cell>
                  <Table.Cell textAlign="end" color="fg.muted">{num(preview.costBefore) === num(preview.costAfter) ? 'se queda igual' : 'la compra más reciente'}</Table.Cell>
                </Table.Row>
                {preview.supplierItems > 0 && (
                  <Table.Row>
                    <Table.Cell colSpan={4} color="fg.muted">
                      Lo que el sistema aprendió de {preview.supplierItems} renglón{preview.supplierItems === 1 ? '' : 'es'} de ticket pasa a {preview.target.name}.
                    </Table.Cell>
                  </Table.Row>
                )}
              </PreviewTable>
            </Box>
          ))}
        </DrawerBody>
        <DrawerFooter>
          <HStack w="100%" gap={3}>
            {step === 'pick' ? (
              <>
                <Button minH="44px" variant="outline" onClick={onClose}>Cancelar</Button>
                <Box flex="1" />
                <Button minH="44px" colorPalette="brand" disabled={chosen.length === 0} loading={loading} onClick={next}>Siguiente ›</Button>
              </>
            ) : step === 'equiv' ? (
              <>
                <Button minH="44px" variant="outline" onClick={() => setStep('pick')}>Volver</Button>
                <Box flex="1" />
                <Button minH="44px" colorPalette="brand" disabled={!equivReady} loading={loading} onClick={() => void ask()}>Ver qué cambia ›</Button>
              </>
            ) : (
              <>
                <Button minH="44px" variant="outline" onClick={() => { setStale(false); setStep(differing.length > 0 ? 'equiv' : 'pick'); }}>Volver</Button>
                <Box flex="1" />
                <Button minH="44px" colorPalette="brand" disabled={!preview || loading || conflicts.length > 0} loading={loading}
                  onClick={() => void confirm()}>Unir</Button>
              </>
            )}
          </HStack>
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}

