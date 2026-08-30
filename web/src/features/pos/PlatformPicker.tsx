import { Box, Button, HStack, Text } from '@chakra-ui/react';
import { LuStore, LuSmartphone } from 'react-icons/lu';

import { useMenu } from '../../hooks/useMenu';
import { useActiveTicket, useTicketStore } from '../../stores/ticket';
import { nombreDeLista } from './precioPlataforma';

// Selector de lista de precios. Siempre visible y siempre diciendo con cuál se está cobrando: el
// riesgo de esta feature no es equivocarse al elegir, es no darse cuenta de que quedó elegida.
//
// El indicador cambia de color cuando NO es mostrador para que se note de reojo, sin leerlo.
export function PlatformPicker() {
  const { data: menu } = useMenu();
  const activa = useActiveTicket().platformId;
  const setPlatform = useTicketStore((s) => s.setPlatform);
  const plataformas = menu?.platforms ?? [];

  // Un negocio sin plataformas configuradas no ve el selector: sería un control que no hace nada.
  if (plataformas.length === 0) return null;

  const enPlataforma = activa !== null;

  return (
    <Box>
      <HStack gap={1} flexWrap="wrap">
        <Button
          size="sm" minH="40px" px={3}
          variant={activa === null ? 'solid' : 'outline'}
          colorPalette={activa === null ? undefined : 'gray'}
          onClick={() => setPlatform(null)}
        >
          <LuStore /> Mostrador
        </Button>
        {plataformas.map((p) => (
          <Button
            key={p.id} size="sm" minH="40px" px={3}
            variant={activa === p.id ? 'solid' : 'outline'}
            colorPalette={activa === p.id ? 'orange' : 'gray'}
            onClick={() => setPlatform(p.id)}
          >
            <LuSmartphone /> {p.name}
          </Button>
        ))}
      </HStack>
      {enPlataforma && (
        <Text fontSize="sm" fontWeight="700" color="orange.fg" mt={1}>
          Cobrando con precios de {nombreDeLista(menu, activa)}
        </Text>
      )}
    </Box>
  );
}
