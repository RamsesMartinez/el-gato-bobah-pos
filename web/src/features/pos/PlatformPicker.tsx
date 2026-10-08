import { Box, Button, HStack, Input, Text } from '@chakra-ui/react';
import { LuStore, LuSmartphone } from 'react-icons/lu';

import { useMenu } from '../../hooks/useMenu';
import { nombreDeLista } from './precioPlataforma';

// Selector de lista de precios. Siempre visible y siempre diciendo con cuál se está cobrando: el
// riesgo de esta feature no es equivocarse al elegir, es no darse cuenta de que quedó elegida.
//
// El indicador cambia de color cuando NO es mostrador para que se note de reojo, sin leerlo.
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
  const plataformas = menu?.platforms ?? [];

  // Un negocio sin plataformas configuradas no ve el selector: sería un control que no hace nada.
  if (plataformas.length === 0) return null;

  const enPlataforma = platformId !== null;

  return (
    <Box>
      {/* El campo del folio va EN ESTE MISMO renglón, no debajo: apilado, el bloque crece 48 px y el
          mosaico pierde un renglón a 1024×600. */}
      <HStack gap={1} flexWrap="wrap" align="center">
        <Button
          size="sm" minH="44px" px={3} disabled={bloqueado}
          variant={platformId === null ? 'solid' : 'outline'}
          colorPalette={platformId === null ? undefined : 'gray'}
          onClick={() => onCambiar(null)}
        >
          <LuStore /> Mostrador
        </Button>
        {plataformas.map((p) => (
          <Button
            key={p.id} size="sm" minH="44px" px={3} disabled={bloqueado}
            variant={platformId === p.id ? 'solid' : 'outline'}
            colorPalette={platformId === p.id ? 'orange' : 'gray'}
            onClick={() => onCambiar(p.id)}
          >
            <LuSmartphone /> {p.name}
          </Button>
        ))}
        {enPlataforma && !bloqueado && (
          <Input
            size="sm" minH="44px" w="220px" px={3}
            // El rótulo nombra la PLATAFORMA: en esta pantalla "folio" ya es el nombre del turno.
            aria-label={`Folio de ${nombreDeLista(menu, platformId)}`}
            placeholder={`Folio de ${nombreDeLista(menu, platformId)}`}
            value={platformOrderRef}
            onChange={(e) => onFolio(e.target.value)}
            onBlur={onFolioListo}
            autoComplete="off" autoCapitalize="off" autoCorrect="off" spellCheck={false}
          />
        )}
      </HStack>
      {enPlataforma && (
        <>
          <Text fontSize="sm" fontWeight="700" color="orange.fg" mt={1}>
            Cobrando con precios de {nombreDeLista(menu, platformId)}
          </Text>
          {!bloqueado && (
            <Text fontSize="xs" color="fg.muted">
              Mantén presionado un producto para corregir su precio.
            </Text>
          )}
        </>
      )}
    </Box>
  );
}
