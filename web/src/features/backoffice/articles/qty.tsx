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
}

export function QtyWithUnit({ label, optional, kind, value, onChange }: QtyProps) {
  const big = BIG[kind];
  const id = `qty-${label.toLowerCase().replace(/\s+/g, '-')}`;
  return (
    <Stack gap={1}>
      <Text asChild><label htmlFor={id}>{label}{optional && <Text as="span" color="fg.muted"> (opcional)</Text>}</label></Text>
      <HStack gap={2}>
        <Input id={id} minH="48px" inputMode="decimal" placeholder="0" value={value.qty}
          onChange={(e) => onChange({ ...value, qty: e.target.value.replace(',', '.') })} />
        {big ? (
          <HStack gap={0} borderWidth="1px" borderRadius="lg" overflow="hidden" flexShrink={0}>
            {[false, true].map((isBig) => (
              <Button key={String(isBig)} minH="44px" borderRadius="0" variant={value.big === isBig ? 'solid' : 'ghost'} colorPalette="gray"
                aria-pressed={value.big === isBig} onClick={() => onChange({ ...value, big: isBig })}>
                {isBig ? big.label : SMALL[kind]}
              </Button>
            ))}
          </HStack>
        ) : <Text flexShrink={0}>{SMALL[kind]}</Text>}
      </HStack>
    </Stack>
  );
}
