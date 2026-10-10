import { Box, Text } from '@chakra-ui/react';
import { money } from '../../utils/format';

// AvisosDeCaja: lo que hay que resolver de las cajas abiertas (spec 032, punto 7). La propina que no
// llega a un peso no se avisa: no se puede entregar.
export function AvisosDeCaja({ tipsPending, cashOutsWithoutConcept }: { tipsPending: string; cashOutsWithoutConcept: number }) {
  const propina = Number(tipsPending);
  if (propina < 1 && cashOutsWithoutConcept === 0) return null;
  return (
    <Box borderWidth="1px" borderRadius="lg" p={3} colorPalette="orange" bg="colorPalette.subtle" role="status">
      {propina >= 1 && <Text fontWeight="600">Propinas sin entregar: {money(propina)}</Text>}
      {cashOutsWithoutConcept > 0 && (
        <Text fontWeight="600">
          {cashOutsWithoutConcept === 1 ? '1 salida sin concepto' : `${cashOutsWithoutConcept} salidas sin concepto`}: corrígelas en Caja.
        </Text>
      )}
    </Box>
  );
}
