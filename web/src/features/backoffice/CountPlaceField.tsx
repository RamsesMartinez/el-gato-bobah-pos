import { useState } from 'react';
import { Box, HStack, Text } from '@chakra-ui/react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { backofficeApi } from '../../api/backoffice';
import { Picker } from '../../components/Picker';
import { toaster } from '../../components/ui/toaster';
import { useLocations } from './useLocations';

// La ubicación de un artículo (spec 040, maqueta A; «Ubicación» desde la 043): se elige de la lista de
// la empresa, que ordena el recorrido del conteo. No es un almacén. Una nueva se crea escribiéndola,
// solo con permiso para configurar el almacén; si ya existía con otra mayúscula o acento, se elige la
// que existe y se dice. Lo usan la ficha del artículo y el conteo: una sola regla para los dos.

// PlaceChooser es el botón que abre la hoja y guarda al elegir; sin rótulo, para que cada pantalla
// ponga el suyo.
export function PlaceChooser({ ingredientId, placeId, title = 'Ubicación', onSaved }: {
  ingredientId: number; placeId: number | null; title?: string; onSaved?: () => void;
}) {
  const qc = useQueryClient();
  const { options, pickerProps } = useLocations();
  const save = useMutation({
    mutationFn: (id: number | null) => backofficeApi.setCountPlace(ingredientId, id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['ingredients'] });
      void qc.invalidateQueries({ queryKey: ['count-places'] });
      onSaved?.();
    },
    onError: () => toaster.create({ title: 'No se pudo guardar la ubicación', type: 'error' }),
  });
  // Lo elegido se muestra de inmediato: la hoja del artículo guarda una copia de la ubicación al
  // abrirse y no se vuelve a leer, así que el botón seguía diciendo la anterior (segunda prueba, T022).
  const [chosen, setChosen] = useState<number | null | undefined>(undefined);
  const current = chosen === undefined ? placeId : chosen;
  return (
    <Picker {...pickerProps} title={title} value={current === null ? '' : String(current)} options={options}
      onChange={(v) => { const id = v === '' ? null : Number(v); setChosen(id); save.mutate(id); }} />
  );
}

export function CountPlaceField({ ingredientId, placeId }: { ingredientId: number; placeId: number | null }) {
  return (
    <HStack gap={3} mt={2} flexWrap="wrap" minH="52px">
      <Text fontSize="sm">Ubicación</Text>
      <Box><PlaceChooser ingredientId={ingredientId} placeId={placeId} /></Box>
    </HStack>
  );
}
