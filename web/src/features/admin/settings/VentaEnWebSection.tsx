import { useQuery } from '@tanstack/react-query';
import { useNavigate } from 'react-router';
import { Box, Button, HStack, Spinner, Text, VStack } from '@chakra-ui/react';
import { adminApi } from '../../../api/admin';
import { CompartirMenu } from './menuPublico/CompartirMenu';
import { NombreDelMenu } from './menuPublico/NombreDelMenu';
import { ColorDelMenu } from './menuPublico/ColorDelMenu';

// VentaEnWebSection (spec 097, maqueta B): arriba lo de todos los días —compartir el menú y su QR—,
// debajo lo de una vez —nombre y color— y al final cuántos productos se venden en la web.
export function VentaEnWebSection() {
  const navigate = useNavigate();
  const menu = useQuery({ queryKey: ['admin', 'public-menu'], queryFn: adminApi.publicMenu });
  const resumen = useQuery({ queryKey: ['admin', 'web-sales'], queryFn: adminApi.webSalesSummary });

  return (
    <VStack align="stretch" gap={4}>
      {menu.isError ? (
        <HStack role="alert" justify="space-between" borderWidth="1px" borderColor="border" borderRadius="lg" px={5} py={3}>
          <Text fontSize="sm">No se pudo leer el menú en línea.</Text>
          <Button variant="outline" minH="44px" onClick={() => menu.refetch()}>Reintentar</Button>
        </HStack>
      ) : !menu.data ? <Spinner aria-label="Cargando" /> : (
        <>
          <CompartirMenu menu={menu.data} />
          <Box borderWidth="1px" borderColor="border" borderRadius="lg" p={5}>
            <VStack align="stretch" gap={4}>
              <NombreDelMenu key={menu.data.name} menu={menu.data} />
              <ColorDelMenu menu={menu.data} />
            </VStack>
          </Box>
        </>
      )}
      <HStack justify="space-between" borderWidth="1px" borderColor="border" borderRadius="lg" px={5} py={3} gap={4}>
        <Text fontSize="sm">
          {resumen.isError ? 'No se pudo leer cuántos productos se venden en la web.'
            : !resumen.data ? 'Contando productos…'
            : <><b>{resumen.data.onWeb}</b> de {resumen.data.total} productos salen en el menú en línea{resumen.data.withWebPrice > 0 ? <> · <b>{resumen.data.withWebPrice}</b> con un precio distinto al de mostrador</> : null}</>}
        </Text>
        <Button variant="outline" minH="44px" flexShrink={0} onClick={() => navigate('/catalogo')}>Ir a los productos</Button>
      </HStack>
    </VStack>
  );
}
