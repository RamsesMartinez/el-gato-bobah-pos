import { useState } from 'react';
import { Box, HStack, Text } from '@chakra-ui/react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { backofficeApi } from '../../api/backoffice';
import { ApiError } from '../../api/client';
import { Picker } from '../../components/Picker';
import { toaster } from '../../components/ui/toaster';

// El lugar de un insumo (spec 040, maqueta A): se elige de la lista de lugares de la empresa, que
// ordena el recorrido del conteo. No es un almacén. Uno nuevo se crea escribiéndolo; si ya existía
// con otra mayúscula o acento, se elige el que existe y se dice. Lo usan la hoja del artículo y el
// conteo (FR-015f del 038: el lugar también se pone al contar): una sola regla para los dos.

// Los avisos van en un toast y no en el renglón: en el conteo el renglón mide 160 px, y al guardar
// el conteo se reordena por lugar y el renglón se va a otra parte (revisión de tableta).
function usePlaceChooser(ingredientId: number, onSaved?: () => void) {
  const qc = useQueryClient();
  const { data: places = [] } = useQuery({ queryKey: ['count-places'], queryFn: backofficeApi.countPlaces });
  const save = useMutation({
    mutationFn: (id: number | null) => backofficeApi.setCountPlace(ingredientId, id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['ingredients'] });
      void qc.invalidateQueries({ queryKey: ['count-places'] });
      onSaved?.();
    },
    onError: () => toaster.create({ title: 'No se pudo guardar el lugar', type: 'error' }),
  });
  const create = async (name: string) => {
    try {
      const { id } = await backofficeApi.createCountPlace(name);
      await qc.invalidateQueries({ queryKey: ['count-places'] });
      return { value: String(id), label: name };
    } catch (e) {
      const existing = e instanceof ApiError ? (e.details as { existingId?: number } | undefined)?.existingId : undefined;
      if (!existing) {
        toaster.create({
          title: e instanceof ApiError && e.status === 400 ? 'El lugar lleva de 1 a 30 letras' : 'No se pudo crear el lugar',
          type: 'error',
        });
        return null;
      }
      const found = places.find((p) => p.id === existing);
      toaster.create({ title: `Ya tenías «${found?.name ?? name}»; quedó elegido`, type: 'info' });
      return { value: String(existing), label: found?.name ?? name };
    }
  };
  return { places, save, create };
}

// PlaceChooser es el botón que abre la hoja; sin rótulo, para que cada pantalla ponga el suyo.
export function PlaceChooser({ ingredientId, placeId, title = 'Lugar', onSaved }: {
  ingredientId: number; placeId: number | null; title?: string; onSaved?: () => void;
}) {
  const { places, save, create } = usePlaceChooser(ingredientId, onSaved);
  // Lo elegido se muestra de inmediato: la hoja del artículo guarda una copia del lugar al abrirse y
  // no se vuelve a leer, así que el botón seguía diciendo el lugar anterior (segunda prueba, T022).
  const [chosen, setChosen] = useState<number | null | undefined>(undefined);
  const current = chosen === undefined ? placeId : chosen;
  return (
    <Picker value={current === null ? '' : String(current)} options={places.map((p) => ({ value: String(p.id), label: p.name }))}
      title={title} placeholder="Sin lugar" clearable clearLabel="Sin lugar" focusSearch={false} searchMaxLength={30}
      onChange={(v) => { const id = v === '' ? null : Number(v); setChosen(id); save.mutate(id); }} onCreate={create} />
  );
}

export function CountPlaceField({ ingredientId, placeId }: { ingredientId: number; placeId: number | null }) {
  return (
    <HStack gap={3} mt={2} flexWrap="wrap" minH="52px">
      <Text fontSize="sm">Lugar</Text>
      <Box><PlaceChooser ingredientId={ingredientId} placeId={placeId} /></Box>
    </HStack>
  );
}
