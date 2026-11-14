import { Button, HStack, Input, Stack, Text } from '@chakra-ui/react';
import { BIG, SMALL, type Kind } from './articleUnits';

// Cantidades del artículo con su unidad elegida aparte (revisión de tableta, M4): número más un
// selector, nunca «5 kg» escrito en un campo de texto.

interface QtyProps {
  label: string;
  optional?: boolean;
  kind: Kind;
  value: { qty: string; big: boolean };
  onChange: (v: { qty: string; big: boolean }) => void;
  // onCommit: salir del campo o Enter (autoguardado de la ficha, spec 043).
  onCommit?: () => void;
  // keepAmount: cambiar g/kg convierte lo escrito en vez de cambiar la cantidad (R7).
  keepAmount?: boolean;
}

export function QtyWithUnit({ label, optional, kind, value, onChange, onCommit, keepAmount }: QtyProps) {
  const big = BIG[kind];
  const id = `qty-${label.toLowerCase().replace(/\s+/g, '-')}`;
  return (
    <Stack gap={1}>
      <Text asChild><label htmlFor={id}>{label}{optional && <Text as="span" color="fg.muted"> (opcional)</Text>}</label></Text>
      <HStack gap={2}>
        <Input id={id} minH="48px" inputMode="decimal" placeholder="0" value={value.qty}
          onChange={(e) => onChange({ ...value, qty: e.target.value.replace(',', '.') })}
          onBlur={onCommit} onKeyDown={(e) => { if (e.key === 'Enter') onCommit?.(); }} />
        {big ? (
          <UnitSegment options={[false, true].map((isBig) => ({
            key: String(isBig), label: isBig ? big.label : SMALL[kind], pressed: value.big === isBig,
            onClick: () => onChange(keepAmount ? switchUnit(value, isBig, big.factor) : { ...value, big: isBig }),
          }))} />
        ) : <Text flexShrink={0}>{SMALL[kind]}</Text>}
      </HStack>
    </Stack>
  );
}

// switchUnit cambia la unidad sin cambiar la cantidad: 10 kg pasa a 10000 g. Lo que todavía no es un
// número se queda como está.
function switchUnit(v: { qty: string; big: boolean }, toBig: boolean, factor: number) {
  if (v.big === toBig || !/^\d+(\.\d+)?$/.test(v.qty)) return { ...v, big: toBig };
  const n = Number(v.qty) * (toBig ? 1 / factor : factor);
  return { qty: String(Math.round(n * 10000) / 10000), big: toBig };
}

// UnitSegment es el selector de unidad junto a un número: el mismo en el mínimo de la ficha y en lo que
// trae un empaque (spec 043, FR-017), para que se vea y mida igual.
export function UnitSegment({ options }: { options: { key: string; label: string; pressed: boolean; onClick: () => void }[] }) {
  return (
    <HStack role="group" aria-label="Unidad" gap={0} borderWidth="1px" borderRadius="lg" overflow="hidden" flexShrink={0}>
      {options.map((o) => (
        <Button key={o.key} minH="44px" borderRadius="0" variant={o.pressed ? 'solid' : 'ghost'} colorPalette="gray"
          aria-pressed={o.pressed} onClick={o.onClick}>
          {o.label}
        </Button>
      ))}
    </HStack>
  );
}
