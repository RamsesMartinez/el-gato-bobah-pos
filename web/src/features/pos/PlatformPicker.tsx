import { Box, Button, HStack, Text } from '@chakra-ui/react';
import { LuStore, LuSmartphone } from 'react-icons/lu';

import { useMenu } from '../../hooks/useMenu';
import { useActiveTicket, useTicketStore } from '../../stores/ticket';
import { nombreDeLista, repreciador } from './precioPlataforma';

// Selector de lista de precios. Siempre visible y siempre diciendo con cuál se está cobrando: el
// riesgo de esta feature no es equivocarse al elegir, es no darse cuenta de que quedó elegida.
//
// El indicador cambia de color cuando NO es mostrador para que se note de reojo, sin leerlo.
export function PlatformPicker() {
  const { data: menu } = useMenu();
  const activa = useActiveTicket().platformId;
  const setPlatform = useTicketStore((s) => s.setPlatform);
  const plataformas = menu?.platforms ?? [];

  // El selector es quien tiene el menú, así que es quien puede volver a precisar lo ya agregado.
  // Sin esto, cambiar de lista a media cuenta deja el ticket cobrando los precios de la anterior.
  const cambiarLista = (id: number | null) => setPlatform(id, repreciador(menu, id));

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
          onClick={() => cambiarLista(null)}
        >
          <LuStore /> Mostrador
        </Button>
        {plataformas.map((p) => (
          <Button
            key={p.id} size="sm" minH="40px" px={3}
            variant={activa === p.id ? 'solid' : 'outline'}
            colorPalette={activa === p.id ? 'orange' : 'gray'}
            onClick={() => cambiarLista(p.id)}
          >
            <LuSmartphone /> {p.name}
          </Button>
        ))}
      </HStack>
      {enPlataforma && (
        <>
          <Text fontSize="sm" fontWeight="700" color="orange.fg" mt={1}>
            Cobrando con precios de {nombreDeLista(menu, activa)}
          </Text>
          {/* La corrección de precio vive en una pulsación larga para no gastar un tap del flujo
              normal ni espacio del mosaico. Un gesto que nadie ve no existe, así que el renglón lo
              enseña; va aquí y no en un icono de ayuda porque es una instrucción de una línea. */}
          <Text fontSize="xs" color="fg.muted">
            Mantén presionado un producto para corregir su precio.
          </Text>
        </>
      )}
    </Box>
  );
}
