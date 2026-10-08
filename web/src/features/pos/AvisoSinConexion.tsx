import { Box, HStack, Text } from '@chakra-ui/react';
import { LuWifiOff } from 'react-icons/lu';
import { useSinConexion } from './useSinConexion';

// EL AVISO DE SIN CONEXIÓN (spec 030, US7; lienzo V2-5).
//
// Capturar exige red (D-4), así que sin ella no se puede agregar, mandar ni cobrar, y la pantalla
// tiene que decirlo de forma que se vea con el panel abierto o cerrado. Va ENCIMA de la franja del
// encabezado, sin empujar nada: en 600 px de alto un aviso que empuja se lleva un renglón de
// productos y mueve los botones bajo el dedo justo cuando la red parpadea.
export function AvisoSinConexion() {
  const sin = useSinConexion();
  if (!sin) return null;
  return (
    <Box role="alert" position="absolute" top={0} left={0} right={0} zIndex={30}
      bg="red.solid" color="red.contrast" px={3} py={1.5} boxShadow="md">
      <HStack gap={2} justify="center">
        <LuWifiOff />
        <Text fontSize="sm" fontWeight="700">Sin conexión.</Text>
        <Text fontSize="sm">No se puede agregar, mandar a cocina ni cobrar. Lo guardado se conserva; se reintenta solo.</Text>
      </HStack>
    </Box>
  );
}
