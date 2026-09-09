import { Box, Button, HStack, Input, Text } from '@chakra-ui/react';
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
  const cuenta = useActiveTicket();
  const activa = cuenta.platformId;
  const setPlatform = useTicketStore((s) => s.setPlatform);
  const setFolio = useTicketStore((s) => s.setPlatformOrderRef);
  const plataformas = menu?.platforms ?? [];

  // El selector es quien tiene el menú, así que es quien puede volver a precisar lo ya agregado.
  // Sin esto, cambiar de lista a media cuenta deja el ticket cobrando los precios de la anterior.
  const cambiarLista = (id: number | null) => setPlatform(id, repreciador(menu, id));

  // Un negocio sin plataformas configuradas no ve el selector: sería un control que no hace nada.
  if (plataformas.length === 0) return null;

  const enPlataforma = activa !== null;

  return (
    <Box>
      {/* El campo del folio va EN ESTE MISMO renglón, no debajo.
          Medido a 1024×600: con plataforma activa y sin el aviso de caja quedan 3 renglones de
          mosaico y 25 px de sobra, y el piso táctil son 44 px. Apilado, el bloque crece 48 px y el
          mosaico baja a 2 renglones; en línea crece 4 px (el renglón pasa de 40 a 44) y los 3 se
          conservan. Si con más plataformas configuradas el flexWrap lo baja, cuesta el renglón que
          SC-007 permite — y eso se mide, no se supone. */}
      <HStack gap={1} flexWrap="wrap" align="center">
        <Button
          size="sm" minH="44px" px={3}
          variant={activa === null ? 'solid' : 'outline'}
          colorPalette={activa === null ? undefined : 'gray'}
          onClick={() => cambiarLista(null)}
        >
          <LuStore /> Mostrador
        </Button>
        {plataformas.map((p) => (
          <Button
            key={p.id} size="sm" minH="44px" px={3}
            variant={activa === p.id ? 'solid' : 'outline'}
            colorPalette={activa === p.id ? 'orange' : 'gray'}
            onClick={() => cambiarLista(p.id)}
          >
            <LuSmartphone /> {p.name}
          </Button>
        ))}
        {enPlataforma && (
          <Input
            size="sm" minH="44px" w="220px" px={3}
            // El rótulo nombra la PLATAFORMA. En esta pantalla "folio" ya es el número del turno
            // —el que se canta como "Tigre"—, así que decirlo a secas manda a teclear el dato
            // equivocado con el documento de pago en la mano.
            aria-label={`Folio de ${nombreDeLista(menu, activa)}`}
            placeholder={`Folio de ${nombreDeLista(menu, activa)}`}
            value={cuenta.platformOrderRef}
            onChange={(e) => setFolio(e.target.value)}
            // El teclado del sistema sube desde abajo y este campo vive arriba del mosaico, así que
            // no lo tapa. Es la razón de que no viva en una hoja inferior.
            autoComplete="off" autoCapitalize="off" autoCorrect="off" spellCheck={false}
          />
        )}
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
