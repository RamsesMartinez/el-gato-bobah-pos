import { useState } from 'react';
import { Badge, Box, Button, Grid, HStack, Text, VStack } from '@chakra-ui/react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { ApiError } from '../../api/client';
import { medirAccion } from '../../api/uso';
import { shoppingApi, type ShoppingLine } from '../../api/notifications';
import { formatBaseQty } from '../../shared/units';

// La lista de compra (spec 084, maqueta M6, opción B): tarjetas por lugar de compra (el proveedor de la
// última compra), con qué comprar y por qué en cada renglón. La vuelta de compras sale ya armada.

const ESTADO: Record<ShoppingLine['state'], { texto: string; color: string }> = {
  out: { texto: 'Se acabó', color: 'red' },
  low: { texto: 'Bajo el mínimo', color: 'orange' },
  count: { texto: 'Pedir conteo', color: 'gray' },
};
const SIN_LUGAR = 'Sin lugar de compra';

export function ListaDeCompra() {
  const lista = useQuery({ queryKey: ['lista-de-compra'], queryFn: shoppingApi.list });
  const [aviso, setAviso] = useState<string | null>(null);
  const enviar = useMutation({
    mutationFn: shoppingApi.emailMe,
    onSuccess: (r) => { medirAccion('inventario', 'enviar-lista'); setAviso(`Te la enviamos a ${r.sentTo}.`); },
    onError: (e) => setAviso(e instanceof ApiError ? e.message : 'No se pudo enviar; vuelve a intentar.'),
  });
  const items = lista.data?.items ?? [];
  const grupos = [...new Set(items.map((l) => l.supplier || SIN_LUGAR))];
  return (
    <VStack align="stretch" gap={3} flex="1" minH={0} overflowY="auto">
      <HStack justify="space-between" flexWrap="wrap" gap={2}>
        <Box>
          <Text fontSize="lg" fontWeight="700">Lista de compra</Text>
          <Text color="fg.muted">Lo que se acabó o está bajo su mínimo, con cuánto conviene comprar</Text>
        </Box>
        <Button minH="44px" variant="outline" loading={enviar.isPending} disabled={items.length === 0} onClick={() => enviar.mutate()}>Enviarme por correo</Button>
      </HStack>
      {aviso && <Text role="status">{aviso}</Text>}
      {lista.data && items.length === 0 && <Text color="fg.muted" py={4}>No hay nada que comprar: nada se ha acabado ni está bajo su mínimo.</Text>}
      <Grid templateColumns="repeat(auto-fit, minmax(min(440px, 100%), 1fr))" gap={4}>
        {grupos.map((g) => (
          <Box key={g} borderWidth="1px" borderRadius="lg" p={4} bg="bg.panel">
            <Text fontWeight="800" fontSize="lg" mb={2}>{g}</Text>
            <VStack align="stretch" gap={0}>
              {items.filter((l) => (l.supplier || SIN_LUGAR) === g).map((l) => (
                <Box key={`${l.itemType}:${l.id}`} data-row py={2} borderBottomWidth="1px">
                  <HStack justify="space-between" align="start" gap={2}>
                    <Box minW={0}>
                      <Text fontWeight="700">{l.name}</Text>
                      <Text fontSize="sm" color="fg.muted">
                        Hay {formatBaseQty(l.onHand, l.unit)}{l.min ? ` · mínimo ${formatBaseQty(l.min, l.unit)}` : ''}
                      </Text>
                    </Box>
                    <Badge colorPalette={ESTADO[l.state].color} flexShrink={0}>{ESTADO[l.state].texto}</Badge>
                  </HStack>
                  <Text mt={1}>
                    <b>{l.suggested ? (l.suggested.packs ? `${l.suggested.packs} × ${l.suggested.packName}` : formatBaseQty(l.suggested.qty, l.unit)) : 'Sin sugerencia'}</b>
                    <Text as="span" fontSize="sm" color="fg.muted"> · {l.why}</Text>
                  </Text>
                </Box>
              ))}
            </VStack>
          </Box>
        ))}
      </Grid>
    </VStack>
  );
}
