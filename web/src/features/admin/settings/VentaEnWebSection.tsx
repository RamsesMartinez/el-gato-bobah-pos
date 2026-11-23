import { useQuery } from '@tanstack/react-query';
import { useNavigate } from 'react-router';
import { Box, Button, HStack, Spinner, Text } from '@chakra-ui/react';
import { adminApi } from '../../../api/admin';

// VentaEnWebSection dice cuántos productos se venden en la web y cuántos con precio propio (spec
// 092, FR-025). Aquí vivirán después el menú en línea, el QR y la marca (#316).
export function VentaEnWebSection() {
  const navigate = useNavigate();
  const { data, isLoading, isError } = useQuery({ queryKey: ['admin', 'web-sales'], queryFn: adminApi.webSalesSummary });
  return (
    <Box borderWidth="1px" borderColor="border" borderRadius="lg" p={5}>
      <Text fontWeight="700" mb={1}>Productos en la web</Text>
      <Text fontSize="sm" color="fg.muted" mb={4}>
        Lo que se ofrece en el menú en línea. Se decide en cada producto, en su pestaña «Venta en web».
      </Text>
      {isError ? <Text fontSize="sm" color="fg.muted" mb={4}>No se pudo leer cuántos productos se venden en la web.</Text>
        : isLoading || !data ? <Spinner /> : (
        <HStack gap={10} mb={4} align="start">
          <Box>
            <Text fontSize="2xl" fontWeight="700">
              {data.onWeb} <Text as="span" fontSize="md" color="fg.muted">de {data.total}</Text>
            </Text>
            <Text fontSize="sm" color="fg.muted">se venden en la web</Text>
          </Box>
          <Box>
            <Text fontSize="2xl" fontWeight="700">{data.withWebPrice}</Text>
            <Text fontSize="sm" color="fg.muted">con precio web propio</Text>
          </Box>
        </HStack>
      )}
      <Button variant="outline" minH="44px" onClick={() => navigate('/catalogo')}>Ver productos en Menú</Button>
    </Box>
  );
}
