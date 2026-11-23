import { useState } from 'react';
import { Box, Button, HStack, IconButton, Input, Text } from '@chakra-ui/react';
import { LuCircleHelp } from 'react-icons/lu';

import { useMenu } from '../../hooks/useMenu';
import { nombreDeLista } from './precioPlataforma';

// Selector de lista de precios. Siempre visible y siempre diciendo con cuál se está cobrando: el
// riesgo de esta feature no es equivocarse al elegir, es no darse cuenta de que quedó elegida.
//
// El canal elegido se distingue por COLOR (naranja si no es mostrador), de reojo y sin leerlo. Desde
// la spec 089 ya no hay renglones de texto debajo («Cobrando con precios de…», «Mantén presionado…»):
// costaban ~40 px justo con plataforma, cuando hay más pedidos, y el ticket ya dice el canal bajo el
// nombre. Cómo corregir un precio queda detrás de la ayuda.
//
// Desde la 030 pinta la lista de la CUENTA ABIERTA, que vive en el servidor: cambiarla la reprecia
// allá (el precio nunca se calcula aquí) y tira el folio de la plataforma anterior.
interface Props {
  platformId: number | null;
  platformOrderRef: string;
  // Un pedido ya enviado no cambia de lista: sus precios ya se cobraron con ella.
  bloqueado?: boolean;
  onCambiar: (platformId: number | null) => void;
  // Cada tecla, para que «Enviar» vea el folio aunque el campo no haya perdido el foco.
  onFolio: (ref: string) => void;
  // Al salir del campo: es cuando se guarda en el servidor.
  onFolioListo: () => void;
}

export function PlatformPicker({ platformId, platformOrderRef, bloqueado, onCambiar, onFolio, onFolioListo }: Props) {
  const { data: menu } = useMenu();
  const [ayuda, setAyuda] = useState(false);
  const plataformas = menu?.platforms ?? [];

  // Un negocio sin plataformas configuradas no ve el selector: sería un control que no hace nada.
  if (plataformas.length === 0) return null;

  const enPlataforma = platformId !== null;
  const canal = (id: number | null, nombre: string) => {
    const on = platformId === id;
    return (
      <Button key={id ?? 'mostrador'} size="sm" h="44px" minH="44px" px={3} disabled={bloqueado} aria-pressed={on}
        variant={on ? 'solid' : 'outline'} colorPalette={on ? (id === null ? undefined : 'orange') : 'gray'}
        onClick={() => onCambiar(id)}>
        {nombre}
      </Button>
    );
  };

  return (
    <Box minW={0}>
      {/* El folio y la ayuda van EN ESTE MISMO renglón: apilados, el bloque crece y el mosaico
          pierde un renglón. Con tres plataformas cabe justo a 1024 con el ticket abierto (medido); el
          wrap es la red para un negocio con más: baja de renglón en vez de recortar las herramientas. */}
      <HStack gap={1} align="center" minW={0} flexWrap="wrap">
        {canal(null, 'Mostrador')}
        {plataformas.map((p) => canal(p.id, p.name))}
        {enPlataforma && !bloqueado && (
          <>
            <Input
              size="sm" h="44px" minH="44px" flex="1" minW="100px" maxW="200px" px={3}
              // El rótulo nombra la PLATAFORMA: en esta pantalla "folio" ya es el nombre del turno.
              aria-label={`Folio de ${nombreDeLista(menu, platformId)}`}
              placeholder={`Folio de ${nombreDeLista(menu, platformId)}`}
              value={platformOrderRef}
              onChange={(e) => onFolio(e.target.value)}
              onBlur={onFolioListo}
              autoComplete="off" autoCapitalize="off" autoCorrect="off" spellCheck={false}
            />
            <IconButton aria-label="Cómo corregir un precio" variant="ghost" colorPalette="gray" minW="44px" minH="44px"
              aria-expanded={ayuda} onClick={() => setAyuda((v) => !v)}>
              <LuCircleHelp />
            </IconButton>
          </>
        )}
      </HStack>
      {ayuda && enPlataforma && !bloqueado && (
        <Text fontSize="sm" color="fg.muted" mt={1}>
          Mantén presionado un producto para corregir su precio en {nombreDeLista(menu, platformId)}.
        </Text>
      )}
    </Box>
  );
}
