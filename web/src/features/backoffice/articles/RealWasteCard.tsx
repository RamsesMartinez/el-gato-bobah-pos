import { useState } from 'react';
import { Box, HStack, Spinner, Text, VStack } from '@chakra-ui/react';
import { InfoButton } from '../../../components/InfoButton';
import { useQuery } from '@tanstack/react-query';
import { LuTriangleAlert } from 'react-icons/lu';
import { backofficeApi } from '../../../api/backoffice';
import type { RealWaste } from '../../../api/admin';

// La merma real de los últimos 90 días por almacén (spec 096, maqueta A2): lo mermado entre lo que
// las ventas consumieron. Solo informa: no cambia el costo. El aviso va en el mismo renglón del
// almacén para no comerse el historial de compras a 1024×600.
export function RealWasteCard({ id, expected, unit }: { id: number; expected: number | null; unit: string }) {
  const { data, isLoading } = useQuery({ queryKey: ['article', id, 'real-waste'], queryFn: () => backofficeApi.articleRealWaste(id) });
  const [info, setInfo] = useState(false);
  return (
    <Box bg="bg.muted" borderRadius="lg" p={3} flex="1" minW={0}>
      <HStack justify="space-between">
        <Text fontSize="sm" color="fg.muted">Merma real, últimos 90 días, por almacén</Text>
        <InfoButton label="Qué es la merma real" open={info} onToggle={() => setInfo(!info)} />
      </HStack>
      {info && (
        <Text fontSize="sm" color="fg.muted">
          Lo registrado en «Se echó a perder» entre lo que se vendió de este artículo. Solo informa: si es
          mucho mayor que la merma al preparar, sube la merma o revisa la presentación que compras.
        </Text>
      )}
      {isLoading ? <Spinner size="sm" mt={2} /> : <RealWasteRows items={data?.items ?? []} expected={expected} unit={unit} />}
    </Box>
  );
}

export function RealWasteRows({ items, expected, unit }: { items: RealWaste[]; expected: number | null; unit: string }) {
  if (items.length === 0) return <Text fontSize="sm" mt={1}>Sin datos en 90 días</Text>;
  return (
    <VStack align="stretch" gap={0} mt={1}>
      {items.map((r) => {
        const over = r.state === 'ratio' && r.ratio !== null && expected !== null && r.ratio > expected;
        return (
          // El nombre no encoge: en una tarjeta angosta, lo demás baja de renglón. Con `flex="1"` y
          // truncado quedaba con ancho 0 a 1024×600 y no se sabía de qué almacén era la cifra.
          <HStack key={r.warehouseId} gap={2} minH="32px" flexWrap="wrap" rowGap={0}>
            <Box w="16px" color="fg.warning" flexShrink={0}>{over && <LuTriangleAlert aria-hidden="true" />}</Box>
            <Text flexGrow={1} flexShrink={0} maxW="100%">{r.warehouseName}</Text>
            {over && <Text fontSize="sm" color="fg.warning">Pasa lo esperado</Text>}
            {/* Lo mermado sin ventas va en texto: un title no se ve al tocar. */}
            {r.state === 'no_sales' && <Text fontSize="sm" color="fg.muted">{Number(r.wasted)} {unit} mermados</Text>}
            {/* nowrap: en una tarjeta angosta la cifra y su explicación bajan juntas, no partidas. */}
            <Text fontWeight="700" whiteSpace="nowrap">
              {r.state === 'ratio' ? `${r.ratio} %` : r.state === 'no_sales' ? 'Sin ventas' : 'Sin datos'}
              {/* Un 0 % con ventas se leía como «sin datos»: dice que no se registró merma. */}
              {r.state === 'ratio' && r.ratio === 0 && <Text as="span" fontSize="sm" fontWeight="normal" color="fg.muted"> · sin merma registrada</Text>}
            </Text>
          </HStack>
        );
      })}
    </VStack>
  );
}
