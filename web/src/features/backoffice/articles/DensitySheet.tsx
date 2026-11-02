import { useEffect, useState } from 'react';
import { Badge, Box, Button, HStack, Input, Stack, Text } from '@chakra-ui/react';
import { useMutation, useQueryClient } from '@tanstack/react-query';

import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
} from '../../../components/ui/drawer';
import { NumericKeypad } from '../../../components/NumericKeypad';
import { useKeypadField } from '../../../components/useKeypadField';
import { backofficeApi, type DensityBody, type DensityView } from '../../../api/backoffice';
import { ApiError } from '../../../api/client';
import { InfoButton } from '../../../components/InfoButton';
import { medirAccion } from '../../../api/uso';
import { parseNumero } from '../../../domain/numeros';
import { densityWords } from '../expenseDraft';

// La densidad de un artículo en g o ml (spec 058, maqueta Densidad A). Opcional: sin ella todo
// funciona como antes; con ella una compra en kg de algo que se guarda en ml entra sola. La pesada es
// el camino por omisión; quien no tiene báscula escribe el valor o lo saca de la etiqueta. El
// resultado se ve antes de guardar, y el servidor lo vuelve a calcular con la misma regla.

type Mode = 'weighing' | 'manual';
type Manual = 'value' | 'label';
type Field = 'fullG' | 'emptyG' | 'contentMl' | 'density' | 'netG' | 'labelMl';

const MIN = 0.3;
const MAX = 3;
// Un número, no una fecha: no lleva zona.
const g = (n: number) => n.toLocaleString('es-MX', { maximumFractionDigits: 4 });

// read lee una casilla: vacía es «sin capturar» (null), nunca cero.
const read = (s: string): number | null => {
  if (s.trim() === '') return null;
  const n = parseNumero(s);
  return n.estado === 'valido' ? n.valor : NaN;
};

interface Outcome { value: number | null; words: string; problem: string; body: DensityBody | null }

// outcome calcula la densidad de lo capturado, la cuenta en palabras y lo que impide guardarla. Las
// reglas son las de domain.DensityFromWeighing/FromLabel/ValidDensity: aquí es para decirlo antes del 400.
function outcome(mode: Mode, manual: Manual, v: Record<Field, string>): Outcome {
  const none: Outcome = { value: null, words: '', problem: '', body: null };
  let value: number;
  let words: (d: string) => string;
  let body: DensityBody;
  if (mode === 'weighing') {
    const [full, empty, content] = [read(v.fullG), read(v.emptyG), read(v.contentMl)];
    if (full === null || empty === null || content === null) return none;
    if ([full, empty, content].some(Number.isNaN)) return { ...none, problem: 'Escribe los pesos con números.' };
    if (empty >= full) return { ...none, problem: 'Lo vacío tiene que pesar menos que lo lleno.' };
    if (content <= 0) return { ...none, problem: 'Lo que contiene tiene que ser mayor que cero.' };
    value = (full - empty) / content;
    words = (d) => `(${g(full)} g − ${g(empty)} g) ÷ ${g(content)} ml = ${d}`;
    body = { method: 'weighing', fullG: String(full), emptyG: String(empty), contentMl: String(content) };
  } else if (manual === 'label') {
    const [net, content] = [read(v.netG), read(v.labelMl)];
    if (net === null || content === null) return none;
    if (Number.isNaN(net) || Number.isNaN(content)) return { ...none, problem: 'Escribe los datos con números.' };
    if (net <= 0) return { ...none, problem: 'El peso neto tiene que ser mayor que cero.' };
    if (content <= 0) return { ...none, problem: 'El contenido tiene que ser mayor que cero.' };
    value = net / content;
    words = (d) => `${g(net)} g ÷ ${g(content)} ml = ${d}`;
    body = { method: 'label', netG: String(net), contentMl: String(content) };
  } else {
    const d = read(v.density);
    if (d === null) return none;
    if (Number.isNaN(d)) return { ...none, problem: 'Escribe la densidad con números.' };
    value = d;
    words = (x) => x;
    body = { method: 'manual', density: String(d) };
  }
  value = Number(value.toFixed(4));
  const said = `${densityWords(value)} gramos por mililitro`;
  if (!(value >= MIN && value <= MAX)) {
    return { value, words: '', body: null, problem: `Da ${said}: va de 0.30 a 3.00. Revisa que sean gramos y no kilos.` };
  }
  return { value, words: words(said), problem: '', body };
}

export function DensitySheet({ article, onClose, onDone }: {
  article: { id: number; name: string; density?: DensityView | null };
  onClose: () => void;
  onDone: () => void;
}) {
  const qc = useQueryClient();
  // La hoja monta CERRADA y se abre en el render siguiente: desde el gasto o el cambio de unidad se
  // abre mientras otra capa cambia, y sin una transición de cerrado a abierto Chakra no la monta
  // (AGENTS.md §3, CobrarSheet).
  const [visible, setVisible] = useState(false);
  // eslint-disable-next-line react-hooks/set-state-in-effect
  useEffect(() => { setVisible(true); }, []);

  const [mode, setMode] = useState<Mode>('weighing');
  const [manual, setManual] = useState<Manual>('value');
  const [values, setValues] = useState<Record<Field, string>>({ fullG: '', emptyG: '', contentMl: '', density: '', netG: '', labelMl: '' });
  const [help, setHelp] = useState(false);
  const [asking, setAsking] = useState(false);
  const [error, setError] = useState('');
  const { field, setField, apply, ref } = useKeypadField<Field>();

  const out = outcome(mode, manual, values);
  const finish = async () => {
    await Promise.all([
      qc.invalidateQueries({ queryKey: ['article', article.id] }),
      qc.invalidateQueries({ queryKey: ['articles'] }),
      qc.invalidateQueries({ queryKey: ['article-audit', article.id] }),
    ]);
    onDone();
  };
  const fail = (e: unknown, fallback: string) =>
    setError(e instanceof ApiError && e.code === 'DENSITY_IN_USE' ? 'Hay una compra por recibir que la necesita'
      : e instanceof ApiError && e.status < 500 ? e.message.charAt(0).toUpperCase() + e.message.slice(1) : fallback);
  const save = useMutation({
    mutationFn: (b: DensityBody) => backofficeApi.setDensity(article.id, b),
    onSuccess: async () => { medirAccion('inventario', 'capturar-densidad'); await finish(); },
    onError: (e) => fail(e, 'No se pudo guardar la densidad'),
  });
  const clear = useMutation({
    mutationFn: () => backofficeApi.clearDensity(article.id),
    onSuccess: finish,
    onError: (e) => { setAsking(false); fail(e, 'No se pudo quitar la densidad'); },
  });

  const key = (k: string) => {
    if (!field) return;
    setError('');
    setValues((v) => ({ ...v, [field]: apply(v[field], k, true) }));
  };
  const box = (f: Field, label: string) => (
    <HStack gap={3} justify="space-between">
      <Text asChild><label htmlFor={`density-${f}`}>{label}</label></Text>
      <Input id={`density-${f}`} ref={ref(f)} w="140px" minH="48px" inputMode="none" readOnly placeholder="—"
        value={values[f]} textAlign="center" fontWeight="700" borderWidth="2px" borderColor={field === f ? 'brand.solid' : undefined}
        onClick={() => setField(f)} onFocus={() => setField(f)} />
    </HStack>
  );
  const segment = <T extends string>(value: T, options: [T, string][], onChange: (v: T) => void, label: string) => (
    <HStack gap={0} borderWidth="1px" borderRadius="md" alignSelf="start" role="group" aria-label={label}>
      {options.map(([v, text]) => (
        <Button key={v} minH="44px" aria-pressed={value === v} variant={value === v ? 'solid' : 'ghost'}
          colorPalette={value === v ? 'brand' : 'gray'} onClick={() => { onChange(v); setField(null); setError(''); }}>
          {text}
        </Button>
      ))}
    </HStack>
  );

  return (
    <DrawerRoot open={visible} placement="bottom" onOpenChange={(e) => { if (!e.open) onClose(); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="2xl" maxH="90dvh" display="flex" flexDirection="column">
        <DrawerHeader py={2}>
          <HStack gap={2}>
            <Text fontWeight="700" fontSize="lg" truncate>Densidad · {article.name}</Text>
            <Badge colorPalette="gray" flexShrink={0}>Opcional</Badge>
            <InfoButton label="¿Para qué sirve la densidad?" open={help} onToggle={() => setHelp((h) => !h)} />
          </HStack>
        </DrawerHeader>
        <DrawerBody flex="1" minH={0} overflowY="auto">
          <Stack gap={3}>
            {help && (
              <Box p={3} borderRadius="lg" bg="bg.subtle">
                <Text fontSize="sm">
                  Sirve para comprar en peso algo que guardas en líquido, o al revés. Por ejemplo, el galón dice 4.3 kg y lo
                  guardas en ml; con la densidad 1.10 entra solo como 4,300 g ÷ 1.10 = 3,909 ml. Sin ella todo funciona igual que hoy.
                </Text>
              </Box>
            )}
            {segment(mode, [['weighing', 'Con una pesada'], ['manual', 'Escribir el valor']], setMode, 'Cómo la capturas')}
            {mode === 'weighing' ? (
              <Stack gap={2}>
                {box('fullG', 'Pesa lleno (g)')}
                {box('emptyG', 'Pesa vacío (g)')}
                {box('contentMl', 'Contiene (ml, según la etiqueta)')}
              </Stack>
            ) : (
              <Stack gap={2}>
                {segment(manual, [['value', 'Gramos por ml'], ['label', 'Con la etiqueta']], setManual, 'Qué escribes')}
                {manual === 'value' ? box('density', 'Gramos por mililitro') : (
                  <>
                    {box('netG', 'Peso neto (g)')}
                    {box('labelMl', 'Contenido (ml)')}
                  </>
                )}
              </Stack>
            )}
            {out.words && <Text fontSize="xl" fontWeight="800" textAlign="center" data-testid="resultado">{out.words}</Text>}
            {out.problem && <Text color="fg.error">{out.problem}</Text>}
            {error && <Text color="fg.error">{error}</Text>}
          </Stack>
        </DrawerBody>
        <DrawerFooter borderTopWidth="1px">
          {field ? (
            <HStack w="100%" gap={3} align="end">
              <Box flex="1" maxW="320px"><NumericKeypad allowDecimal onKey={key} /></Box>
              <Box flex="1" />
              <Button minH="56px" colorPalette="brand" onClick={() => setField(null)}>Listo</Button>
            </HStack>
          ) : (
            <HStack w="100%" gap={3}>
              {article.density && (asking ? (
                <HStack gap={2}>
                  <Text>¿Quitar?</Text>
                  <Button minH="44px" variant="outline" colorPalette="red" loading={clear.isPending} onClick={() => clear.mutate()}>Sí</Button>
                  <Button minH="44px" variant="ghost" onClick={() => setAsking(false)}>No</Button>
                </HStack>
              ) : (
                <Button minH="44px" variant="ghost" colorPalette="red" onClick={() => { setError(''); setAsking(true); }}>Quitar densidad</Button>
              ))}
              {/* Separado de «Guardar» al menos 16 px: quitar no se deshace con un toque. */}
              <Box flex="1" minW="16px" />
              <Button minH="44px" variant="outline" onClick={onClose}>Cancelar</Button>
              <Button minH="44px" colorPalette="brand" disabled={!out.body} loading={save.isPending}
                onClick={() => { if (out.body) { setError(''); save.mutate(out.body); } }}>Guardar</Button>
            </HStack>
          )}
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}
