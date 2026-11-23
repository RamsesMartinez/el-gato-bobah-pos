import { Box, Grid, HStack, Input, Spinner, Text, VStack } from '@chakra-ui/react';
import { useQuery } from '@tanstack/react-query';
import { adminApi } from '../../api/admin';
import { moneyExact } from '../../utils/format';
import { Field } from '../../components/ui/field';
import { pctTecleado } from '../merma/pct';
import { conMermaDelProducto, r4 } from '../merma/desglose';
import { CostBreakdownTable } from './CostBreakdownTable';

function Cifra({ label, value, strong }: { label: string; value: string; strong?: boolean }) {
  return (
    <Box borderWidth="1px" borderRadius="lg" px={3} py={2} role="group" aria-label={label}>
      <Text fontSize="sm" color="fg.muted">{label}</Text>
      <Text fontSize="xl" fontWeight={strong ? '800' : '600'}>{value}</Text>
    </Box>
  );
}

// CostTab es la pestaña «Costo» de la ficha del producto (spec 096, maqueta B): el % del producto
// arriba junto a las tres cifras, y de dónde sale cada peso abajo. El % se guarda con el «Guardar»
// de la ficha; aquí solo se teclea y se ve su efecto.
export function CostTab({ productId, text, onText }: { productId: number; text: string; onText: (t: string) => void }) {
  const { data, isLoading } = useQuery({ queryKey: ['admin', 'costing', productId], queryFn: () => adminApi.productCosting(productId) });
  if (isLoading || !data) return <Spinner m={6} />;
  if (data.source === 'combo') {
    return (
      <VStack align="stretch" gap={2}>
        <Text>El costo de este paquete es la suma de lo que incluye, cada uno con su merma.</Text>
        {(data.components ?? []).map((c) => (
          <HStack key={c.productId} minH="44px" borderBottomWidth="1px" justify="space-between">
            <Text>{c.name} × {c.qty}</Text>
            <Text>{moneyExact(c.cost)} <Text as="span" color="fg.muted" fontSize="sm">(merma {moneyExact(c.waste)})</Text></Text>
          </HStack>
        ))}
        <Text fontWeight="700">Costo {moneyExact(data.cost)}</Text>
      </VStack>
    );
  }
  if (data.source !== 'receta') {
    return (
      <Text>
        {data.source === 'compra' ? 'El costo de este producto sale de su compra: la merma no aplica.' : 'Este producto tiene su costo a mano: la merma no aplica.'}
      </Text>
    );
  }
  const typed = pctTecleado(text);
  const preview = conMermaDelProducto(data, typed === 'invalido' ? data.productPct : typed);
  return (
    <VStack align="stretch" gap={3}>
      <HStack align="end" gap={4} flexWrap="wrap">
        <Field label="Merma al preparar" w="auto" invalid={typed === 'invalido'} errorText="Escribe de 0 a 99.99"
          helperText={typed === null ? `Usa la general: ${data.generalPct ?? 0} %` : 'Reemplaza a la general en este producto'}>
          <HStack gap={2}>
            <Input aria-label="Merma del producto, en porcentaje" maxW="110px" minH="44px" inputMode="decimal"
              value={text} onChange={(e) => onText(e.target.value)} />
            <Text>%</Text>
          </HStack>
        </Field>
        <Grid templateColumns="repeat(3, minmax(0, 1fr))" gap={2} flex="1" minW="320px">
          <Cifra label="Sin merma" value={moneyExact(preview.base)} />
          <Cifra label="La merma suma" value={`+${moneyExact(preview.waste)}`} />
          <Cifra label="Costo" value={moneyExact(r4(preview.base + preview.waste))} strong />
        </Grid>
      </HStack>
      <CostBreakdownTable lines={preview.lines} />
    </VStack>
  );
}
