import { useState } from 'react';
import { Box, Button, HStack, Input, Stack, Text } from '@chakra-ui/react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
} from '../../components/ui/drawer';
import { backofficeApi, type Presentation, type PresentationBody, type Unit } from '../../api/backoffice';
import { ApiError } from '../../api/client';
import { formatBaseQty, purchaseUnitsFor, unitLabel } from '../../shared/units';
import { round2 } from '../../domain/numeros';
import { CountPlaceField } from './CountPlaceField';

// «Cómo llega» un insumo (spec 038, maqueta «Cómo llega un insumo», opción B): sus empaques de
// compra. Cada empaque se captura con número Y unidad, y se ve su equivalencia antes de guardar:
// «25» a secas en un insumo en gramos sería un error de 1,000 veces que nadie ve.

export interface PresentationsIngredient {
  id: number;
  name: string;
  baseUnitCode: string;
  baseUnitKind: string;
  // Con él, la hoja deja cambiar el lugar del insumo (lo hace quien configura el almacén); sin él,
  // como desde la captura de una compra, no se ofrece.
  // El lugar del recorrido (spec 040). Solo desde Almacén: undefined no ofrece el renglón.
  countPlaceId?: number | null;
}

interface Props {
  isOpen: boolean;
  ingredient: PresentationsIngredient;
  presentations: Presentation[];
  units: Unit[];
  // startNew abre con un empaque nuevo listo para capturar: desde la ficha, «+ Agregar empaque» se
  // tocaba dos veces (prueba sin contexto, spec 042).
  startNew?: boolean;
  onClose: () => void;
}

// Cómo se llama un empaque aproximado: por la unidad en que se compra («Kilo ≈ 4 piezas»).
const APPROX_NAME: Record<string, string> = { kg: 'Kilo', g: 'Gramo', l: 'Litro', ml: 'Mililitro' };

const norm = (s: string) => s.trim().toLowerCase().normalize('NFD').replace(/[̀-ͯ]/g, '').replace(/\s+/g, ' ');

// bodyOf expresa un empaque guardado como se capturó: en la unidad de compra más grande en la que
// su contenido queda redondo (12000 ml → 12 L).
function bodyOf(p: Presentation, units: Unit[], kind: string): PresentationBody {
  if (p.approximate && p.purchaseUnitId) {
    return { name: p.name, qty: String(Number(p.qtyInBase)), unitId: p.purchaseUnitId, approximate: true };
  }
  const base = Number(p.qtyInBase);
  const options = purchaseUnitsFor(kind, units);
  // La más grande en la que el contenido queda con dos decimales o menos (12000 ml → 12 L).
  const fit = options.find((u) => { const v = base / Number(u.toBase); return Math.abs(round2(v) - v) < 1e-9; })
    ?? options[options.length - 1];
  const qty = Number((base / Number(fit.toBase)).toFixed(4));
  return { name: p.name, qty: String(qty), unitId: fit.id, approximate: false };
}

interface Draft {
  editing: Presentation | null;
  name: string;
  other: boolean;
  qty: string;
  unitId: number | null;
  approximate: boolean;
}

export function PresentationsSheet({ isOpen, ingredient, presentations, units, startNew, onClose }: Props) {
  const qc = useQueryClient();
  const kind = ingredient.baseUnitKind;
  const byPiece = kind === 'pieza';
  const [draft, setDraft] = useState<Draft | null>(() => {
    if (!startNew) return null;
    const first = purchaseUnitsFor(kind, units)[0];
    return { editing: null, name: '', other: false, qty: '', unitId: first?.id ?? null, approximate: false };
  });
  const [error, setError] = useState('');
  // Lo que se sugiere son empaques que la empresa ya usa en artículos que se miden igual, con su
  // cantidad (spec 042, FR-011): antes «Bolsa de 15 kg» aparecía en un artículo en litros.
  const sugg = useQuery({ queryKey: ['presentation-suggestions', kind], queryFn: () => backofficeApi.presentationSuggestions(kind), enabled: isOpen });

  const save = useMutation({
    mutationFn: ({ id, body }: { id: number | null; body: PresentationBody }) =>
      id ? backofficeApi.updatePresentation(id, body) : backofficeApi.createPresentation(ingredient.id, body),
    onSuccess: () => {
      setDraft(null);
      setError('');
      void qc.invalidateQueries({ queryKey: ['presentations'] });
    },
    onError: (e) => setError(e instanceof ApiError && e.status === 409
      ? 'Ya hay un empaque que se llama así'
      : 'No se pudo guardar el empaque'),
  });

  const active = presentations.filter((p) => p.active);
  const inactive = presentations.filter((p) => !p.active);
  const units4draft = draft?.approximate
    ? [...purchaseUnitsFor('masa', units), ...purchaseUnitsFor('volumen', units)]
    : purchaseUnitsFor(kind, units);
  const unit = units.find((u) => u.id === draft?.unitId) ?? null;
  const taken = new Set(active.filter((p) => p.id !== draft?.editing?.id).map((p) => norm(p.name)));
  const suggestions = (sugg.data ?? []).filter((s) => !taken.has(norm(s.name)));
  const suggestedNames = (sugg.data ?? []).map((s) => s.name);
  const name = draft ? (draft.approximate && unit ? APPROX_NAME[unit.code] ?? unit.name : draft.name.trim()) : '';
  const qty = Number(draft?.qty);
  const qtyOk = draft !== null && draft.qty.trim() !== '' && Number.isFinite(qty) && qty > 0;
  const wholeOk = !byPiece || Number.isInteger(qty);
  const canSave = draft !== null && name !== '' && unit !== null && qtyOk && wholeOk && !save.isPending;

  const open = (p: Presentation | null) => {
    setError('');
    if (p) {
      const b = bodyOf(p, units, kind);
      setDraft({ editing: p, name: b.name, other: !suggestedNames.includes(b.name), qty: b.qty, unitId: b.unitId, approximate: b.approximate });
    } else {
      const first = purchaseUnitsFor(kind, units)[0];
      setDraft({ editing: null, name: '', other: false, qty: '', unitId: first?.id ?? null, approximate: false });
    }
  };
  const toggle = (p: Presentation) => save.mutate({ id: p.id, body: { ...bodyOf(p, units, kind), active: !p.active } });
  const submit = () => {
    if (!draft || !unit || !canSave) return;
    const body: PresentationBody = { name, qty: draft.qty.trim(), unitId: unit.id, approximate: draft.approximate };
    save.mutate({ id: draft.editing?.id ?? null, body: draft.editing ? { ...body, active: true } : body });
  };

  return (
    <DrawerRoot open={isOpen} placement="bottom" onOpenChange={(e) => { if (!e.open) { setDraft(null); onClose(); } }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="l3" maxH="90dvh" display="flex" flexDirection="column">
        <DrawerHeader pb={1}>
          <Text fontSize="lg" fontWeight="700">Cómo llega {ingredient.name}</Text>
          <Text fontSize="sm" color="fg.muted">
            Cómo lo compras, por ejemplo: bolsa de 5 kg. Cada empaque se convierte a {formatBaseQty(1, ingredient.baseUnitCode).replace(/^1 /, '')} al recibirlo.
          </Text>
          {ingredient.countPlaceId !== undefined && <CountPlaceField ingredientId={ingredient.id} placeId={ingredient.countPlaceId} />}
        </DrawerHeader>
        <DrawerBody flex="1" minH={0} overflowY="auto" pb={2}>
          <Stack gap={2}>
            {active.length === 0 && !draft && (
              <Text color="fg.muted">Todavía no dices cómo llega. Agrega su empaque para capturar compras sin hacer cuentas.</Text>
            )}
            {active.map((p) => (
              <HStack key={p.id} data-row borderWidth="1px" borderRadius="lg" px={3} minH="56px" gap={3}>
                <Text flex="1" fontWeight="600">
                  {p.approximate ? `${p.name} ≈ ${formatBaseQty(p.qtyInBase, ingredient.baseUnitCode)}` : `${p.name} · ${formatBaseQty(p.qtyInBase, ingredient.baseUnitCode)}`}
                </Text>
                <Button minH="44px" variant="outline" mr={4} onClick={() => open(p)}>Editar</Button>
                <Button minH="44px" variant="outline" onClick={() => toggle(p)} disabled={save.isPending}>Ya no llega así</Button>
              </HStack>
            ))}
            {inactive.length > 0 && (
              <HStack gap={2} flexWrap="wrap">
                <Text color="fg.muted">Ya no llega en: {inactive.map((p) => p.name).join(', ')}</Text>
                {inactive.map((p) => (
                  <Button key={p.id} size="sm" minH="44px" variant="ghost" onClick={() => toggle(p)}>Vuelve a llegar en {p.name}</Button>
                ))}
              </HStack>
            )}

            {draft ? (
              <Stack gap={3} borderWidth="1px" borderRadius="lg" p={3}>
                <Text fontWeight="700">{draft.editing ? `Cambiar ${draft.editing.name}` : 'Nuevo empaque'}</Text>
                {byPiece && (
                  <HStack gap={2}>
                    <Button minH="44px" variant={draft.approximate ? 'outline' : 'solid'} colorPalette="gray"
                      onClick={() => setDraft({ ...draft, approximate: false, unitId: purchaseUnitsFor(kind, units)[0]?.id ?? null })}>
                      Se compra por pieza o paquete
                    </Button>
                    <Button minH="44px" variant={draft.approximate ? 'solid' : 'outline'} colorPalette="gray"
                      onClick={() => setDraft({ ...draft, approximate: true, unitId: null })}>
                      Se compra por peso o líquido
                    </Button>
                  </HStack>
                )}
                {!draft.approximate && (
                  <Stack gap={2}>
                    <Text fontSize="sm" color="fg.muted">¿Cómo llega?{suggestions.length > 0 ? ' Los que ya usas en artículos parecidos, u otro' : ''}</Text>
                    <HStack gap={2} flexWrap="wrap">
                      {suggestions.map((sg) => {
                        const b = bodyOf({ name: sg.name, qtyInBase: sg.qtyInBase, approximate: false, purchaseUnitId: null } as Presentation, units, kind);
                        const on = !draft.other && draft.name === sg.name && draft.qty === b.qty && draft.unitId === b.unitId;
                        return (
                          <Button key={`${sg.name}-${sg.qtyInBase}`} minH="44px" variant={on ? 'solid' : 'outline'} colorPalette="gray"
                            onClick={() => setDraft({ ...draft, name: sg.name, other: false, qty: b.qty, unitId: b.unitId })}>
                            {sg.name} · {formatBaseQty(sg.qtyInBase, ingredient.baseUnitCode)}
                          </Button>
                        );
                      })}
                      <Button minH="44px" variant={draft.other || suggestions.length === 0 ? 'solid' : 'outline'} colorPalette="gray"
                        onClick={() => setDraft({ ...draft, name: '', other: true })}>Otro…</Button>
                    </HStack>
                    {(draft.other || suggestions.length === 0) && (
                      <Input minH="48px" maxLength={40} aria-label="Nombre del empaque" placeholder="Escribe el nombre"
                        value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} />
                    )}
                  </Stack>
                )}
                <HStack gap={3} flexWrap="wrap">
                  <Text>{draft.approximate ? 'Por cada' : 'Trae'}</Text>
                  {draft.approximate && (
                    <HStack gap={0} borderWidth="1px" borderRadius="lg" overflow="hidden">
                      {units4draft.map((u) => (
                        <Button key={u.id} minH="44px" borderRadius="0" variant={draft.unitId === u.id ? 'solid' : 'ghost'} colorPalette="gray"
                          onClick={() => setDraft({ ...draft, unitId: u.id })}>{unitLabel(u)}</Button>
                      ))}
                    </HStack>
                  )}
                  {draft.approximate && <Text>trae aproximadamente</Text>}
                  <Input w="110px" minH="48px" inputMode="decimal" aria-label="Cuánto trae" placeholder="0"
                    value={draft.qty} onChange={(e) => setDraft({ ...draft, qty: e.target.value.replace(',', '.') })} />
                  {draft.approximate ? <Text>piezas</Text> : (
                    <HStack gap={0} borderWidth="1px" borderRadius="lg" overflow="hidden">
                      {units4draft.map((u) => (
                        <Button key={u.id} minH="44px" borderRadius="0" variant={draft.unitId === u.id ? 'solid' : 'ghost'} colorPalette="gray"
                          onClick={() => setDraft({ ...draft, unitId: u.id })}>{unitLabel(u)}</Button>
                      ))}
                    </HStack>
                  )}
                  {qtyOk && unit && wholeOk && (
                    <Text fontWeight="700" color="green.fg">
                      {draft.approximate
                        ? `1 ${unitLabel(unit)} ≈ ${qty} ${qty === 1 ? 'pieza' : 'piezas'}`
                        : `= ${formatBaseQty(qty * Number(unit.toBase), ingredient.baseUnitCode)}`}
                    </Text>
                  )}
                </HStack>
                {!wholeOk && <Text color="orange.fg">Un empaque trae piezas enteras: escribe cuántas dice la etiqueta.</Text>}
                {byPiece && !draft.approximate && (
                  <Text fontSize="sm" color="fg.muted">Si es otro tamaño de pieza (un sobre más grande, otra lata), créalo como otro insumo.</Text>
                )}
                {error && <Text color="red.fg">{error}</Text>}
                <HStack justify="flex-end" gap={2}>
                  <Button minH="48px" variant="outline" onClick={() => { setDraft(null); setError(''); }}>Cancelar</Button>
                  <Button minH="48px" colorPalette="red" disabled={!canSave} loading={save.isPending} onClick={submit}>Guardar empaque</Button>
                </HStack>
              </Stack>
            ) : (
              <Box>
                <Button minH="48px" w="100%" variant="outline" borderStyle="dashed" onClick={() => open(null)}>+ Agregar empaque</Button>
              </Box>
            )}
            <Text fontSize="sm" color="fg.muted">Cambiar lo que trae un empaque no cambia las compras que ya registraste.</Text>
          </Stack>
        </DrawerBody>
        <DrawerFooter borderTopWidth="1px" pt={3}>
          <Button minH="48px" variant="outline" onClick={() => { setDraft(null); onClose(); }}>Listo</Button>
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}
