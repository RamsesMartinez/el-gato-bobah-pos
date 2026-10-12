import { Button, Text, VStack } from '@chakra-ui/react';
import { isOffline } from '../api/mensajes';

// LoadError ocupa el lugar de una lista que no se pudo traer. Va EN LUGAR de la lista y no encima:
// una lista vieja bajo un filtro nuevo se lee como la respuesta al filtro.
export function LoadError({ error, offline, onRetry, retrying }: {
  error: unknown; offline?: boolean; onRetry: () => void; retrying?: boolean;
}) {
  return (
    <VStack flex="1" justify="center" gap={3} p={6}>
      {offline || isOffline(error) ? (
        <>
          <Text fontSize="lg" fontWeight="700">Sin conexión</Text>
          <Text color="fg.muted">Revisa el internet de la tableta y vuelve a intentar.</Text>
        </>
      ) : (
        <Text fontSize="lg" fontWeight="700">No se pudo cargar</Text>
      )}
      <Button minH="44px" loading={retrying} onClick={onRetry}>Reintentar</Button>
    </VStack>
  );
}
