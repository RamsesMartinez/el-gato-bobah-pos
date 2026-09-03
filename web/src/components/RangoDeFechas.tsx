import { Box, Button, HStack, Input, Text, Wrap } from '@chakra-ui/react';

import { mensajeDeRango, validarRango } from '../domain/rangoDeFechas';

export interface PresetOpcion {
  id: string;
  label: string;
}

interface Props {
  presets: PresetOpcion[];
  preset: string;
  onPreset: (id: string) => void;
  desde: string;
  hasta: string;
  onRango: (desde: string, hasta: string) => void;
  // hoy: el día del NEGOCIO, no el del navegador. Topa los dos campos Y decide el rechazo: el
  // calendario no impide teclear la fecha, así que el `max` solo por sí mismo no es una barrera.
  hoy: string;
}

// El selector de periodo de Ventas y de Reportes.
//
// Es UN componente para las dos pantallas a propósito. Cada una tenía su propio encabezado de
// periodo —una con chips, la otra con la frase fija "últimos 30 días" que seguía diciéndolo pasara
// lo que pasara— y las reglas de qué rango se puede pedir viven en `domain/rangoDeFechas`, no aquí:
// esto solo las pinta.
//
// Los campos de fecha son `input type="date"`, que en tableta abre el calendario del sistema a
// pantalla completa. No es el `<select>` que la constitución prohíbe: lo que ahí se prohíbe es el
// desplegable de renglones de 20 px, y el calendario nativo es justo lo contrario.
export function RangoDeFechas({ presets, preset, onPreset, desde, hasta, onRango, hoy }: Props) {
  const esRango = preset === 'rango';
  const motivo = esRango ? validarRango(desde, hasta, hoy) : null;

  return (
    <Box>
      <Wrap gap={2}>
        {[...presets, { id: 'rango', label: 'Rango' }].map((p) => (
          <Button
            key={p.id}
            size="sm"
            // 44 px es el mínimo con el que un dedo acierta a la primera; por debajo el operador
            // toca dos veces y la segunda cae en otra cosa.
            minH="44px"
            px={4}
            variant={preset === p.id ? 'solid' : 'outline'}
            colorPalette={preset === p.id ? undefined : 'gray'}
            aria-pressed={preset === p.id}
            onClick={() => onPreset(p.id)}
          >
            {p.label}
          </Button>
        ))}
      </Wrap>

      {esRango && (
        <HStack gap={2} mt={2} flexWrap="wrap" align="center">
          <Input
            type="date"
            size="sm"
            minH="44px"
            w="160px"
            max={hoy}
            aria-label="Desde"
            value={desde}
            onChange={(e) => onRango(e.target.value, hasta)}
          />
          <Text fontSize="sm" color="fg.muted">al</Text>
          <Input
            type="date"
            size="sm"
            minH="44px"
            w="160px"
            max={hoy}
            aria-label="Hasta"
            value={hasta}
            onChange={(e) => onRango(desde, e.target.value)}
          />
          {/* El aviso va JUNTO a los campos y no donde iría la tabla: mientras el rango está a
              medias la pantalla sigue mostrando el periodo anterior, y hay que poder ver de un
              vistazo que lo que se está capturando todavía no se aplicó. */}
          {motivo && (
            <Text fontSize="sm" color="fg.error" role="status">{mensajeDeRango(motivo)}</Text>
          )}
        </HStack>
      )}
    </Box>
  );
}
