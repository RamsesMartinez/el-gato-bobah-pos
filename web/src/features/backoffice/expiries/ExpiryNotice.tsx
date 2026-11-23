import { Box, Button, HStack, Text } from '@chakra-ui/react';
import { LuTriangleAlert } from 'react-icons/lu';
import { useQuery } from '@tanstack/react-query';
import { backofficeApi } from '../../../api/backoffice';

// ExpiryNotice es el aviso de una línea de Existencias (spec 098, maqueta «Aviso en Existencias»,
// opción A): lo caducado y lo que caduca en 7 días del almacén que se ve. Sin nada, no ocupa espacio.
export function ExpiryNotice({ warehouse, onOpen }: { warehouse: number | 'all' | undefined; onOpen: () => void }) {
  const s = useQuery({ queryKey: ['stock', 'expiries', 'summary', warehouse ?? 'venta'], queryFn: () => backofficeApi.expirySummary(warehouse) });
  const expired = s.data?.expired ?? 0;
  const soon = s.data?.within7 ?? 0;
  if (expired + soon === 0) return null;
  const parts = [
    expired > 0 ? `${expired} ${expired === 1 ? 'caducado' : 'caducados'}` : '',
    soon > 0 ? `${soon} ${soon === 1 ? 'caduca' : 'caducan'} en 7 días` : '',
  ].filter(Boolean);
  return (
    <HStack data-testid="aviso-caducidad" px={3} minH="44px" borderRadius="lg" bg="orange.subtle" gap={2}>
      <Box color="fg.warning"><LuTriangleAlert /></Box>
      <Text flex="1" fontSize="sm" fontWeight="600">{parts.join(' · ')}</Text>
      <Button minH="44px" size="sm" variant="ghost" onClick={onOpen}>Ver caducidades ›</Button>
    </HStack>
  );
}
