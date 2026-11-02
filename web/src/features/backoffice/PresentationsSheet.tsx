import { useState } from 'react';
import { Box, Button, HStack, Input, Stack, Text } from '@chakra-ui/react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
} from '../../components/ui/drawer';
import { backofficeApi, type Presentation, type PresentationBody, type Unit } from '../../api/backoffice';
import { ApiError } from '../../api/client';
import { formatBaseQty, packExample, purchaseUnitsFor, unitLabel } from '../../shared/units';
import { round2 } from '../../domain/numeros';
import { UnitSegment } from './articles/qty';
import { BASE_PACKS } from './articles/articleUnits';
import { presentationLabel } from '../../shared/presentationLabel';

// «Cómo llega» un insumo (spec 038, maqueta «Cómo llega un insumo», opción B): sus empaques de
// compra. Cada empaque se captura con número Y unidad, y se ve su equivalencia antes de guardar:
// «25» a secas en un insumo en gramos sería un error de 1,000 veces que nadie ve.

export interface PresentationsIngredient {
  id: number;
  name: string;
  baseUnitCode: string;
  baseUnitKind: string;
}

interface Props {
  isOpen: boolean;
  ingredient: PresentationsIngredient;
  presentations: Presentation[];
  units: Unit[];
  // startNew abre con un empaque nuevo listo para capturar: desde la ficha, «+ Agregar empaque» se
  // tocaba dos veces (prueba sin contexto, spec 042).
  startNew?: boolean;
  // editId abre con ese empaque en edición: desde la ficha, tocar su renglón (revisión de la 042).
  editId?: number;
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
  // La marca o el proveedor (spec 056), opcional.
  brand: string;
}

export function PresentationsSheet({ isOpen, ingredient, presentations, units, startNew, editId, onClose }: Props) {
  const qc = useQueryClient();
  const kind = ingredient.baseUnitKind;
  const byPiece = kind === 'pieza';
  const [draft, setDraft] = useState<Draft | null>(() => {
    const editing = editId === undefined ? undefined : presentations.find((p) => p.id === editId);
    if (editing) {
      const b = bodyOf(editing, units, kind);
      return { editing, name: b.name, other: true, qty: b.qty, unitId: b.unitId, approximate: b.approximate, brand: editing.brand ?? '' };
    }
    if (!startNew) return null;
    const first = purchaseUnitsFor(kind, units)[0];
    return { editing: null, name: '', other: false, qty: '', unitId: first?.id ?? null, approximate: false, brand: '' };
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
      ? 'Ya hay un empaque que se llama así con esa marca'
      // Un 400 dice qué está mal (prueba con usuario nuevo: «1212 L» salía solo como «No se pudo»).
      : e instanceof ApiError && e.status === 400 ? e.message : 'No se pudo guardar el empaque'),
  });

  const active = presentations.filter((p) => p.active);
  const inactive = presentations.filter((p) => !p.active);
  const units4draft = draft?.approximate
    ? [...purchaseUnitsFor('masa', units), ...purchaseUnitsFor('volumen', units)]
    : purchaseUnitsFor(kind, units);
  const unit = units.find((u) => u.id === draft?.unitId) ?? null;
  const taken = new Set(active.filter((p) => p.id !== draft?.editing?.id).map((p) => norm(p.name)));
  // Botones base de la clase (spec 043, FR-014), y después los que la empresa ya usa que no son base:
  // un aprendido con el nombre de un base no se repite, pero le presta su cantidad al base.
  const base = BASE_PACKS[kind] ?? [];
  const baseSet = new Set(base.map(norm));
  const learnedFor = (n: string) => (sugg.data ?? []).find((s) => norm(s.name) === norm(n));
  const suggestions = (sugg.data ?? []).filter((s) => !taken.has(norm(s.name)) && !baseSet.has(norm(s.name)));
  const suggestedNames = [...base, ...(sugg.data ?? []).map((s) => s.name)];
  const name = draft ? (draft.approximate && unit ? APPROX_NAME[unit.code] ?? unit.name : draft.name.trim()) : '';
  const qty = Number(draft?.qty);
  const qtyOk = draft !== null && draft.qty.trim() !== '' && Number.isFinite(qty) && qty > 0;
  const wholeOk = !byPiece || Number.isInteger(qty);
  const canSave = draft !== null && name !== '' && unit !== null && qtyOk && wholeOk && !save.isPending;

  const unitOptions = units4draft.map((u) => ({
    key: String(u.id), label: unitLabel(u), pressed: draft?.unitId === u.id,
    onClick: () => { if (draft) setDraft({ ...draft, unitId: u.id }); },
  }));
  // phrase dice lo que se va a guardar, junto al pie (R5): «1 bolsa = 5 kg». Con otra unidad que la
  // del artículo, también cómo se cuenta: es lo que antes decía «Se cuenta en g», suelto (#103).
  const phrase = !draft || !qtyOk || !unit || !wholeOk || name === '' ? null
    : draft.approximate ? `1 ${unitLabel(unit)} ≈ ${qty} ${qty === 1 ? 'pieza' : 'piezas'}`
      : `1 ${name.toLowerCase()} = ${qty} ${unitLabel(unit)}`;
  const counted = draft && !draft.approximate && unit && qtyOk && Number(unit.toBase) !== 1
    ? `se cuenta como ${formatBaseQty(qty * Number(unit.toBase), ingredient.baseUnitCode)}` : null;

  const open = (p: Presentation | null) => {
    setError('');
    if (p) {
      const b = bodyOf(p, units, kind);
      setDraft({ editing: p, name: b.name, other: !suggestedNames.includes(b.name), qty: b.qty, unitId: b.unitId, approximate: b.approximate, brand: p.brand ?? '' });
    } else {
      const first = purchaseUnitsFor(kind, units)[0];
      setDraft({ editing: null, name: '', other: false, qty: '', unitId: first?.id ?? null, approximate: false, brand: '' });
    }
  };
  const toggle = (p: Presentation) => save.mutate({ id: p.id, body: { ...bodyOf(p, units, kind), active: !p.active } });
  const submit = () => {
    if (!draft || !unit || !canSave) return;
    // La marca va siempre: vacía la quita al editar (sin mandarla, el servidor la conserva).
    const body: PresentationBody = { name, qty: draft.qty.trim(), unitId: unit.id, approximate: draft.approximate, brand: draft.brand.trim() };
    save.mutate({ id: draft.editing?.id ?? null, body: draft.editing ? { ...body, active: true } : body });
  };

  return (
    <DrawerRoot open={isOpen} placement="bottom" onOpenChange={(e) => { if (!e.open) { setDraft(null); onClose(); } }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="l3" maxH="90dvh" display="flex" flexDirection="column">
        <DrawerHeader pb={1}>
          <Text fontSize="lg" fontWeight="700">Cómo llega {ingredient.name}</Text>
          <Text fontSize="sm" color="fg.muted">Así lo recibes del proveedor, por ejemplo: {packExample(kind)}.</Text>
        </DrawerHeader>
        <DrawerBody flex="1" minH={0} overflowY="auto" pb={2}>
          <Stack gap={2}>
            {active.length === 0 && !draft && (
              <Text color="fg.muted">Todavía no dices cómo llega. Agrega su empaque para capturar compras sin hacer cuentas.</Text>
            )}
            {active.map((p) => (
              <HStack key={p.id} data-row borderWidth="1px" borderRadius="lg" px={3} minH="56px" gap={3}>
                <Text flex="1" fontWeight="600">
                  {p.approximate ? `${presentationLabel(p)} ≈ ${formatBaseQty(p.qtyInBase, ingredient.baseUnitCode)}` : `${presentationLabel(p)} · ${formatBaseQty(p.qtyInBase, ingredient.baseUnitCode)}`}
                </Text>
                <Button minH="44px" variant="outline" onClick={() => open(p)}>Editar</Button>
                {/* Aparte de «Editar» y en segundo plano: juntos, quien quitaba dudaba si estaba editando
                    (prueba sin contexto y decisión del dueño, 2026-10-20). No se borra: queda en «Ya no
                    llega en» y se puede volver a usar. */}
                <Box w={6} flexShrink={0} />
                <Button minH="44px" variant="ghost" colorPalette="gray" data-secondary onClick={() => toggle(p)} disabled={save.isPending}>Quitar este empaque</Button>
              </HStack>
            ))}
            {inactive.length > 0 && (
              <HStack gap={2} flexWrap="wrap">
                <Text color="fg.muted">Ya no llega en: {inactive.map((p) => presentationLabel(p)).join(', ')}</Text>
                {inactive.map((p) => (
                  <Button key={p.id} size="sm" minH="44px" variant="ghost" onClick={() => toggle(p)}>Vuelve a llegar en {presentationLabel(p)}</Button>
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
                    <Text fontWeight="600">¿En qué empaque lo compras?</Text>
                    <HStack gap={2} flexWrap="wrap" data-packs>
                      {base.map((n) => {
                        const on = !draft.other && draft.name === n;
                        const learned = learnedFor(n);
                        return (
                          <Button key={n} minH="44px" variant={on ? 'solid' : 'outline'} colorPalette="gray" aria-pressed={on}
                            onClick={() => {
                              const b = learned && bodyOf({ name: n, qtyInBase: learned.qtyInBase, approximate: false, purchaseUnitId: null } as Presentation, units, kind);
                              setDraft({ ...draft, name: n, other: false, ...(b ? { qty: b.qty, unitId: b.unitId } : {}) });
                            }}>
                            {n}
                          </Button>
                        );
                      })}
                      {suggestions.map((sg) => {
                        const b = bodyOf({ name: sg.name, qtyInBase: sg.qtyInBase, approximate: false, purchaseUnitId: null } as Presentation, units, kind);
                        const on = !draft.other && draft.name === sg.name && draft.qty === b.qty && draft.unitId === b.unitId;
                        return (
                          <Button key={`${sg.name}-${sg.qtyInBase}`} minH="44px" variant={on ? 'solid' : 'outline'} colorPalette="gray" aria-pressed={on}
                            onClick={() => setDraft({ ...draft, name: sg.name, other: false, qty: b.qty, unitId: b.unitId })}>
                            {sg.name} · {formatBaseQty(sg.qtyInBase, ingredient.baseUnitCode)}
                          </Button>
                        );
                      })}
                      <Button minH="44px" variant={draft.other ? 'solid' : 'outline'} colorPalette="gray" aria-pressed={draft.other}
                        onClick={() => setDraft({ ...draft, name: '', other: true })}>Otro</Button>
                    </HStack>
                    {draft.other && (
                      <Input minH="48px" maxLength={40} aria-label="Nombre del empaque" placeholder="Ej. Cubeta, Garrafa"
                        value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} />
                    )}
                  </Stack>
                )}
                <HStack gap={3} flexWrap="wrap">
                  <Text>{draft.approximate ? 'Por cada' : 'Trae'}</Text>
                  {draft.approximate && <UnitSegment options={unitOptions} />}
                  {draft.approximate && <Text>trae aproximadamente</Text>}
                  {/* Al tocarla se selecciona: la cantidad llega precargada de un empaque conocido y escribir
                      encima daba «1212» (prueba con usuario nuevo). */}
                  <Input w="110px" minH="48px" inputMode="decimal" aria-label="Cuánto trae" placeholder="0"
                    onFocus={(e) => e.currentTarget.select()}
                    value={draft.qty} onChange={(e) => setDraft({ ...draft, qty: e.target.value.replace(',', '.') })} />
                  {draft.approximate ? <Text>piezas</Text> : <UnitSegment options={unitOptions} />}
                </HStack>
                <Stack gap={1}>
                  <Text fontWeight="600">Marca o proveedor (opcional)</Text>
                  <Input aria-label="Marca o proveedor (opcional)" minH="48px" maxLength={40} placeholder="Ej. Lala, Costco"
                    value={draft.brand} onChange={(e) => setDraft({ ...draft, brand: e.target.value })} />
                </Stack>
                {!wholeOk && <Text color="orange.fg">Un empaque trae piezas enteras: escribe cuántas dice la etiqueta.</Text>}
                {byPiece && !draft.approximate && (
                  <Text fontSize="sm" color="fg.muted">Si es otro tamaño de pieza (un sobre más grande, otra lata), créalo como otro insumo.</Text>
                )}
                {error && <Text color="fg.error">{error}</Text>}
              </Stack>
            ) : (
              <Box>
                <Button minH="48px" w="100%" variant="outline" borderStyle="dashed" onClick={() => open(null)}>+ Agregar empaque</Button>
              </Box>
            )}
            <Text fontSize="sm" color="fg.muted">Cambiar lo que trae un empaque no cambia las compras que ya registraste.</Text>
          </Stack>
        </DrawerBody>
        {/* Con un empaque a medio capturar, el pie fijo lleva sus botones: abajo del contenido había que
            desplazar para llegar a ellos, y «Listo» tiraba lo escrito sin avisar (revisión de la 042). */}
        <DrawerFooter borderTopWidth="1px" pt={3}>
          {draft ? (
            <HStack gap={3} w="100%" flexWrap="wrap">
              <Stack gap={0} flex="1" minW={0}>
                {phrase && <Text fontSize="lg" fontWeight="700">{phrase}</Text>}
                {counted && <Text fontSize="sm" color="fg.muted">{counted}</Text>}
              </Stack>
              <Button minH="48px" variant="outline" onClick={() => { setDraft(null); setError(''); }}>Cancelar</Button>
              <Button minH="48px" colorPalette="brand" disabled={!canSave} loading={save.isPending} onClick={submit}>Guardar empaque</Button>
            </HStack>
          ) : (
            <Button minH="48px" variant="outline" onClick={onClose}>Listo</Button>
          )}
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}
