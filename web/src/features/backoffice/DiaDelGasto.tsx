import { Field, Input, Text } from '@chakra-ui/react';

const MESES = ['enero', 'febrero', 'marzo', 'abril', 'mayo', 'junio', 'julio', 'agosto', 'septiembre',
  'octubre', 'noviembre', 'diciembre'];

function diaLegible(iso: string): string {
  const [, m, d] = iso.split('-').map(Number);
  return `${d} de ${MESES[m - 1]}`;
}

// DiaDelGasto: con caja abierta el día del gasto es el del turno y solo se informa; sin caja se
// elige (spec 032, punto 5). El servidor aplica la misma regla.
export function DiaDelGasto({ turno, value, onChange }: {
  turno: string | null; // día de negocio del turno abierto, o null sin caja abierta
  value: string;
  onChange: (v: string) => void;
}) {
  if (turno) {
    return <Text fontSize="sm" color="fg.muted">Día del gasto: {diaLegible(turno)} (turno abierto)</Text>;
  }
  return (
    <Field.Root w="170px">
      <Field.Label>Día del gasto</Field.Label>
      <Input type="date" minH="44px" aria-label="Día del gasto" value={value} onChange={(e) => onChange(e.target.value)} />
    </Field.Root>
  );
}
