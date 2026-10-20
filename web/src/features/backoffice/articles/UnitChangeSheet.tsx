import { useState } from 'react';
import { medirAccion } from '../../../api/uso';
import { Badge, Box, Button, HStack, Input, Stack, Text } from '@chakra-ui/react';
import { useMutation, useQuery } from '@tanstack/react-query';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
} from '../../../components/ui/drawer';
import { backofficeApi, type ArticleSheet, type UnitChangePreview, type UnitConversion } from '../../../api/backoffice';
import { ApiError } from '../../../api/client';
import { toaster } from '../../../components/ui/toaster';
import { BIG, KINDS, SMALL, type Kind } from './articleUnits';

// Cambiar en qué se mide (spec 042, US5, maqueta de la ficha y revisión de tableta A4). Dos pasos: la
// clase nueva con su equivalencia, y la revisión de cada cantidad convertida, con el botón de
// confirmar en un pie fijo. Con compras o ventas el servidor dice que se crea uno nuevo y este se
// archiva; sin ellas, cambia el mismo artículo.

interface Props {
  article: ArticleSheet;
  onClose: () => void;
  onDone: () => void;
}

// Las clases a las que se puede pasar: entre peso y líquido no, porque haría falta la densidad.
const targetsOf = (k: Kind): Kind[] => (k === 'pieza' ? ['masa', 'volumen', 'longitud'] : ['pieza']);
const word = (k: Kind) => KINDS.find((x) => x.kind === k)!.label.toLowerCase();

export function UnitChangeSheet({ article, onClose, onDone }: Props) {
  const from = article.baseUnitKind as Kind;
  const units = useQuery({ queryKey: ['units'], queryFn: backofficeApi.units });
  const [to, setTo] = useState<Kind | null>(targetsOf(from).length === 1 ? targetsOf(from)[0] : null);
  // La equivalencia se captura como la diría alguien: «1 pieza = 250 g» (la clase que no es pieza).
  const [eq, setEq] = useState({ qty: '', big: false });
  const [step, setStep] = useState<1 | 2>(1);
  const [preview, setPreview] = useState<UnitChangePreview | null>(null);
  const [edits, setEdits] = useState<Record<string, string>>({});
  const [openRecipes, setOpenRecipes] = useState(false);
  // Lo que falta cerrar (compras por recibir, mermas por aprobar, un conteo abierto) se dice aquí
  // mismo, no en un aviso que se va solo: es lo que hay que ir a resolver.
  const [blocked, setBlocked] = useState('');

  const other: Kind | null = to === null ? null : from === 'pieza' ? to : from;
  const eqNum = Number(eq.qty);
  const perPiece = other && Number.isFinite(eqNum) && eqNum > 0 ? eqNum * (eq.big ? BIG[other]?.factor ?? 1 : 1) : 0;
  // El servidor quiere cuánto de la base nueva vale una de la vieja.
  const factor = perPiece > 0 ? (from === 'pieza' ? perPiece : 1 / perPiece) : 0;
  const baseUnitId = (units.data?.items ?? []).find((u) => u.kind === to && Number(u.toBase) === 1)?.id;
  const body = () => ({ baseUnitId: baseUnitId!, factor: String(Number(factor.toPrecision(12))) });

  const review = useMutation({
    mutationFn: () => backofficeApi.previewUnitChange(article.id, body()),
    onSuccess: (p) => { setPreview(p); setEdits({}); setBlocked(''); setStep(2); },
    onError: (e) => {
      if (e instanceof ApiError && (e.code === 'UNIT_CHANGE_PENDING' || e.code === 'IN_USE')) { setBlocked(e.message); return; }
      toaster.create({ title: e instanceof ApiError && e.status !== 500 ? e.message : 'No se pudo revisar', type: 'error' });
    },
  });
  const apply = useMutation({
    mutationFn: () => backofficeApi.changeUnit(article.id, { ...body(), overrides: edits }),
    onSuccess: () => {
      medirAccion('inventario', 'cambiar-unidad');
      toaster.create({ title: preview?.path === 'replace' ? 'Se creó el nuevo y este quedó archivado' : 'Ya se mide en ' + (to ? word(to) : ''), type: 'success' });
      onDone();
    },
    onError: (e) => toaster.create({ title: e instanceof ApiError && e.status !== 500 ? e.message : 'No se pudo cambiar', type: 'error' }),
  });

  const conversions = preview?.conversions ?? [];
  // Primero lo que no cuadra, para que se vea antes de confirmar.
  const sorted = [...conversions].sort((a, b) => Number(a.fits) - Number(b.fits));
  const group = (g: UnitConversion['group']) => sorted.filter((c) => c.group === g);
  const canReview = to !== null && factor > 0 && baseUnitId !== undefined && !review.isPending;

  return (
    <DrawerRoot open placement="bottom" size="full" onOpenChange={(e) => { if (!e.open) onClose(); }}>
      <DrawerBackdrop />
      <DrawerContent display="flex" flexDirection="column">
        <DrawerHeader py={2}>
          <Text fontSize="lg" fontWeight="700">
            Cambiar en qué se mide «{article.name}» · paso {step} de 2
          </Text>
        </DrawerHeader>
        <DrawerBody flex="1" minH={0} overflowY="auto">
          {step === 1 ? (
            <Stack gap={4}>
              <Stack gap={1}>
                <Text>Hoy se mide en <b>{word(from)}</b>. ¿En qué se medirá?</Text>
                <HStack gap={2}>
                  {targetsOf(from).map((k) => (
                    <Button key={k} minH="48px" variant={to === k ? 'solid' : 'outline'} colorPalette="gray" aria-pressed={to === k}
                      onClick={() => setTo(k)}>{KINDS.find((x) => x.kind === k)!.label}</Button>
                  ))}
                </HStack>
              </Stack>
              {blocked && <Box p={3} borderRadius="lg" bg="orange.subtle"><Text>{blocked.charAt(0).toUpperCase() + blocked.slice(1)}.</Text></Box>}
              {other && (
                <Stack gap={1}>
                  <Text asChild><label htmlFor="unit-eq">¿Cuánto es 1 pieza?</label></Text>
                  <HStack gap={2}>
                    <Text fontSize="lg">1 pieza =</Text>
                    <Input id="unit-eq" w="140px" minH="48px" inputMode="decimal" placeholder="0" value={eq.qty}
                      onChange={(e) => setEq({ ...eq, qty: e.target.value.replace(',', '.') })} />
                    <HStack gap={0} borderWidth="1px" borderRadius="lg" overflow="hidden">
                      {[false, true].filter((b) => !b || BIG[other]).map((isBig) => (
                        <Button key={String(isBig)} minH="48px" borderRadius="0" variant={eq.big === isBig ? 'solid' : 'ghost'} colorPalette="gray"
                          aria-pressed={eq.big === isBig} onClick={() => setEq({ ...eq, big: isBig })}>
                          {isBig ? BIG[other]!.label : SMALL[other]}
                        </Button>
                      ))}
                    </HStack>
                  </HStack>
                </Stack>
              )}
            </Stack>
          ) : preview && (
            <Stack gap={3}>
              <Box p={3} borderRadius="lg" bg={preview.path === 'replace' ? 'orange.subtle' : 'bg.subtle'}>
                {preview.path === 'replace'
                  ? <Text>Como ya tiene compras o ventas, se crea «{article.name}» en {to ? word(to) : ''} con lo de abajo, y este se archiva con su historia.</Text>
                  : <Text>Se cambia el mismo artículo. Revisa cada cantidad; puedes corregir la que no cuadre.</Text>}
              </Box>
              <Group title="Existencia" items={group('stock')} preview={preview} edits={edits} setEdits={setEdits} />
              {group('recipe').length > 0 && (
                <Stack gap={1}>
                  <Button minH="44px" variant="ghost" justifyContent="space-between" onClick={() => setOpenRecipes((o) => !o)}>
                    {group('recipe').length} receta(s) convertida(s) <Box as="span">{openRecipes ? '▲' : '▼'}</Box>
                  </Button>
                  {(openRecipes || group('recipe').some((c) => !c.fits)) && (
                    <Group items={group('recipe')} preview={preview} edits={edits} setEdits={setEdits} />
                  )}
                </Stack>
              )}
              <Group title="Empaques" items={group('presentation')} preview={preview} edits={edits} setEdits={setEdits} />
            </Stack>
          )}
        </DrawerBody>
        <DrawerFooter borderTopWidth="1px">
          {step === 1 ? (
            <>
              <Button minH="52px" variant="outline" onClick={onClose}>Cancelar</Button>
              <Button minH="52px" colorPalette="brand" disabled={!canReview} loading={review.isPending} onClick={() => review.mutate()}>
                Revisar cantidades ›
              </Button>
            </>
          ) : (
            <>
              <Button minH="52px" variant="outline" onClick={() => setStep(1)}>‹ Atrás</Button>
              <Button minH="52px" colorPalette="brand" loading={apply.isPending} onClick={() => apply.mutate()}>
                {preview?.path === 'replace' ? 'Crear el nuevo y archivar este' : 'Cambiar'}
              </Button>
            </>
          )}
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}

function Group({ title, items, preview, edits, setEdits }: {
  title?: string; items: UnitConversion[]; preview: UnitChangePreview;
  edits: Record<string, string>; setEdits: (e: Record<string, string>) => void;
}) {
  if (items.length === 0) return null;
  const unit = (c: string) => (c === 'pieza' ? 'pz' : c);
  return (
    <Stack gap={0}>
      {title && <Text fontWeight="700">{title}</Text>}
      {items.map((c) => (
        <HStack key={c.key} minH="52px" borderBottomWidth="1px" gap={3}>
          <Text flex="1" minW={0} truncate>{c.label}</Text>
          {!c.fits && <Badge colorPalette="orange">No cuadra: se archiva</Badge>}
          <Text w="110px" textAlign="end" color="fg.muted">{Number(c.from)} {unit(preview.fromUnit)}</Text>
          <Text>→</Text>
          <Input aria-label={`${c.label} en la unidad nueva`} w="120px" minH="44px" inputMode="decimal"
            value={edits[c.key] ?? String(Number(c.to))}
            onChange={(e) => setEdits({ ...edits, [c.key]: e.target.value.replace(',', '.') })} />
          <Text w="28px">{unit(preview.toUnit)}</Text>
        </HStack>
      ))}
    </Stack>
  );
}
