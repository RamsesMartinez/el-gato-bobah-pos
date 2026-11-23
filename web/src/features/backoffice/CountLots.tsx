import { Box, Button, HStack, Text, VStack } from '@chakra-ui/react';
import { LuX } from 'react-icons/lu';
import { ExpiryPickerButton } from '../../components/ExpiryPickerButton';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';
import { countedWords } from './countEntry';

export interface LotRow { expiresOn: string; qty: string }

// CountLotsFields son las fechas de caducidad de lo contado (spec 098, maqueta «Fechas en el conteo»,
// opción A): dentro de la captura del artículo, con el mismo teclado. Las fechas que el sistema ya
// conocía llegan sin cantidad (no enseñan lo esperado); una fecha sin cantidad no cuenta. Lo que no
// lleva fecha se calcula solo.
export function CountLotsFields({ rows, active, total, unitCode, onPick, onChangeDate, onAdd, onRemove }: {
  rows: LotRow[];
  active: number | null;
  total: number | null;
  unitCode: string;
  onPick: (i: number) => void;
  onChangeDate: (i: number, d: string) => void;
  onAdd: (d: string) => void;
  onRemove: (i: number) => void;
}) {
  const hora = useHoraDelNegocio();
  const today = hora.diaDelNegocio(new Date());
  const sum = rows.reduce((s, r) => s + (Number(r.qty) || 0), 0);
  const over = total !== null && sum > total + 1e-9;
  return (
    <VStack align="stretch" gap={1} flexShrink={0} borderTopWidth="1px" pt={2} data-testid="fechas-conteo">
      <Text fontWeight="700">Fechas de caducidad</Text>
      {rows.map((r, i) => (
        <HStack key={`${r.expiresOn}-${i}`} gap={2}>
          <ExpiryPickerButton value={r.expiresOn} today={today} allowPast label={`Fecha ${i + 1}`} onChange={(d) => (d ? onChangeDate(i, d) : onRemove(i))} />
          <Box as="button" aria-label={`Cuánto caduca el ${r.expiresOn}`} minH="44px" minW="110px" px={3} borderWidth="2px" borderRadius="md"
            borderColor={active === i ? 'brand.solid' : 'border'} bg={active === i ? 'brand.subtle' : undefined}
            fontWeight="700" textAlign="start" data-active={active === i ? '' : undefined} onClick={() => onPick(i)}>
            {r.qty ? `${r.qty} ${unitCode}` : '¿cuánto?'}
          </Box>
          <Box flex="1" />
          <Button aria-label={`Quitar la fecha ${r.expiresOn}`} minH="44px" minW="44px" variant="ghost" onClick={() => onRemove(i)}><LuX /></Button>
        </HStack>
      ))}
      <HStack gap={3} flexWrap="wrap">
        <ExpiryPickerButton value="" today={today} allowPast label="Agregar fecha" placeholder="+ Agregar fecha" onChange={(d) => d && onAdd(d)} />
        {total !== null && sum > 0 && (
          <Text fontSize="sm" color={over ? 'fg.error' : 'fg.muted'} fontWeight={over ? '700' : undefined} role={over ? 'alert' : undefined}>
            {over ? 'Las fechas suman más de lo contado' : `Sin fecha: ${countedWords(Number((total - sum).toFixed(4)), unitCode)}`}
          </Text>
        )}
      </HStack>
    </VStack>
  );
}
