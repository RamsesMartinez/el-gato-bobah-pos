import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Box, Button, Grid, HStack, Input, Text } from '@chakra-ui/react';
import { ApiError } from '../../api/client';
import {
  BIG_UNIT, countLimitsApi, formatThousands, UNIT_CLASSES, type CountLimits, type UnitClass,
} from '../../api/countLimits';
import { parseNumero } from '../../domain/numeros';
import { can } from '../../app/permissions';

// Límites del conteo (spec 044, maqueta «Límites del conteo»): por clase de unidad, lo más que se
// registra de un artículo en un conteo y desde qué cifra se pide confirmar. Se capturan en kg, L, m
// y piezas; viajan en unidad base.

type Field = 'max' | 'confirm';
type Draft = Record<UnitClass, Record<Field, string>>;

const FIELDS: { f: Field; label: string; hint: string }[] = [
  { f: 'max', label: 'Máximo por conteo', hint: 'Lo más que se puede registrar de un artículo en un conteo' },
  { f: 'confirm', label: 'Pedir confirmar desde', hint: 'Si lo contado pasa de esta cifra, el conteo pide confirmar' },
];

const toDraft = (l: CountLimits): Draft => Object.fromEntries(UNIT_CLASSES.map((c) => [c, {
  max: String(l[c].max / BIG_UNIT[c].factor), confirm: String(l[c].confirm / BIG_UNIT[c].factor),
}])) as Draft;

// Las comas que pone la pantalla al salir del campo son de miles; cualquier otra coma no se adivina.
function parse(text: string): number | null {
  const s = text.trim();
  const plain = /^\d{1,3}(,\d{3})+(\.\d+)?$/.test(s) ? s.replace(/,/g, '') : s;
  const n = parseNumero(plain);
  return n.estado === 'valido' ? n.valor : null;
}

function rowError(d: Record<Field, string>): string | null {
  const max = parse(d.max);
  const confirm = parse(d.confirm);
  if (!max || !confirm) return 'Escribe cifras mayores que cero.';
  if (max <= confirm) return 'El máximo tiene que ser mayor que la cifra de confirmar.';
  return null;
}

export function CountLimitsSettings() {
  const allowed = can('inventory.configure');
  const qc = useQueryClient();
  const { data } = useQuery({ queryKey: ['count-limits'], queryFn: countLimitsApi.get, enabled: allowed });
  const [draft, setDraft] = useState<Draft | null>(null);
  const [focused, setFocused] = useState<string | null>(null);
  const [error, setError] = useState('');
  const [saved, setSaved] = useState(false);
  const save = useMutation({
    mutationFn: (l: CountLimits) => countLimitsApi.save(l),
    onSuccess: (l) => {
      qc.setQueryData(['count-limits'], l);
      setDraft(null);
      setError('');
      setSaved(true);
    },
    onError: (e) => setError(e instanceof ApiError && e.status === 400 ? e.message : 'No se pudo guardar'),
  });
  if (!allowed || !data) return null;

  const current = toDraft(data);
  const values = draft ?? current;
  const errors = Object.fromEntries(UNIT_CLASSES.map((c) => [c, rowError(values[c])])) as Record<UnitClass, string | null>;
  const dirty = JSON.stringify(UNIT_CLASSES.map((c) => [parse(values[c].max), parse(values[c].confirm)]))
    !== JSON.stringify(UNIT_CLASSES.map((c) => [parse(current[c].max), parse(current[c].confirm)]));
  const valid = UNIT_CLASSES.every((c) => !errors[c]);

  const set = (c: UnitClass, f: Field, v: string) => {
    setSaved(false);
    setDraft({ ...values, [c]: { ...values[c], [f]: v } });
  };
  const shown = (c: UnitClass, f: Field) => {
    const raw = values[c][f];
    if (focused === `${c}.${f}`) return raw;
    const n = parse(raw);
    return n === null ? raw : formatThousands(n);
  };
  const body = (): CountLimits => Object.fromEntries(UNIT_CLASSES.map((c) => [c, {
    max: Number((parse(values[c].max)! * BIG_UNIT[c].factor).toFixed(4)),
    confirm: Number((parse(values[c].confirm)! * BIG_UNIT[c].factor).toFixed(4)),
  }])) as CountLimits;

  return (
    <Box mt={6} pt={4} borderTopWidth="1px" data-testid="limites-conteo">
      <Text fontWeight="700" fontSize="lg">Límites del conteo</Text>
      <Text fontSize="sm" color="fg.muted" mb={3}>
        Lo más que se puede registrar de un artículo en un conteo, y desde qué cifra se pide confirmar si el
        artículo no tiene otra referencia.
      </Text>
      <Grid templateColumns="minmax(90px, auto) 1fr 1fr" gap={2} alignItems="center" maxW="640px">
        <span />
        {FIELDS.map((x) => (
          <Box key={x.f}>
            <Text fontSize="sm" fontWeight="600">{x.label}</Text>
            <Text fontSize="xs" color="fg.muted">{x.hint}</Text>
          </Box>
        ))}
        {UNIT_CLASSES.map((c) => (
          <Box key={c} display="contents">
            <Text fontWeight="600">{BIG_UNIT[c].name}</Text>
            {FIELDS.map((x) => (
              <HStack key={x.f} gap={2}>
                <Input aria-label={`${x.label} de ${BIG_UNIT[c].name} (${BIG_UNIT[c].label})`} minH="44px" inputMode="decimal"
                  value={shown(c, x.f)} borderColor={errors[c] ? 'border.error' : undefined}
                  onFocus={(e) => { setFocused(`${c}.${x.f}`); e.currentTarget.scrollIntoView?.({ block: 'center' }); }}
                  onBlur={() => setFocused(null)}
                  onChange={(e) => set(c, x.f, e.target.value)} />
                <Text minW="48px">{BIG_UNIT[c].label}</Text>
              </HStack>
            ))}
            {errors[c] && <><span /><Text gridColumn="2 / 4" fontSize="sm" color="red.fg">{errors[c]}</Text></>}
          </Box>
        ))}
      </Grid>
      {/* Fijo al fondo de la sección: el teclado del sistema no tapa «Guardar límites». */}
      <HStack position="sticky" bottom={0} bg="bg.panel" py={2} mt={2} gap={3} justify="end" maxW="640px" flexWrap="wrap">
        {error && <Text fontSize="sm" color="red.fg" mr="auto">{error}</Text>}
        {saved && !dirty && <Text fontSize="sm" color="green.fg" mr="auto">Guardado ✓</Text>}
        <Button minH="44px" variant="outline" disabled={!dirty} onClick={() => { setDraft(null); setError(''); }}>Descartar</Button>
        <Button minH="44px" colorPalette="red" disabled={!dirty || !valid} loading={save.isPending}
          onClick={() => save.mutate(body())}>Guardar límites</Button>
      </HStack>
    </Box>
  );
}
