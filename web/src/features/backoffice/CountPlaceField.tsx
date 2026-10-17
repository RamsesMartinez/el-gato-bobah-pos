import { useState } from 'react';
import { Box, HStack, Text } from '@chakra-ui/react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { backofficeApi } from '../../api/backoffice';
import { ApiError } from '../../api/client';
import { Picker } from '../../components/Picker';

// El lugar de un insumo (spec 040, maqueta A): se elige de la lista de lugares de la empresa, que
// ordena el recorrido del conteo. No es un almacén. Uno nuevo se crea escribiéndolo; si ya existía
// con otra mayúscula o acento, se elige el que existe y se dice. Lo usan la hoja del artículo y el
// conteo (FR-015f del 038: el lugar también se pone al contar): una sola regla para los dos.

function usePlaceChooser(ingredientId: number, onSaved?: () => void) {
  const qc = useQueryClient();
  const [notice, setNotice] = useState<string | null>(null);
  const { data: places = [] } = useQuery({ queryKey: ['count-places'], queryFn: backofficeApi.countPlaces });
  const save = useMutation({
    mutationFn: (id: number | null) => backofficeApi.setCountPlace(ingredientId, id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['ingredients'] });
      void qc.invalidateQueries({ queryKey: ['count-places'] });
      onSaved?.();
    },
  });
  const create = async (name: string) => {
    setNotice(null);
    try {
      const { id } = await backofficeApi.createCountPlace(name);
      await qc.invalidateQueries({ queryKey: ['count-places'] });
      return { value: String(id), label: name };
    } catch (e) {
      const existing = e instanceof ApiError ? (e.details as { existingId?: number } | undefined)?.existingId : undefined;
      if (!existing) throw e;
      const found = places.find((p) => p.id === existing);
      setNotice(`Ya tenías «${found?.name ?? name}»; quedó elegido`);
      return { value: String(existing), label: found?.name ?? name };
    }
  };
  return { places, save, create, notice };
}

// PlaceChooser es el botón que abre la hoja; sin rótulo, para que cada pantalla ponga el suyo.
export function PlaceChooser({ ingredientId, placeId, title = 'Lugar', onSaved }: {
  ingredientId: number; placeId: number | null; title?: string; onSaved?: () => void;
}) {
  const { places, save, create, notice } = usePlaceChooser(ingredientId, onSaved);
  return (
    <>
      <Picker value={placeId === null ? '' : String(placeId)} options={places.map((p) => ({ value: String(p.id), label: p.name }))}
        title={title} placeholder="Sin lugar" clearable clearLabel="Sin lugar" focusSearch={false}
        onChange={(v) => save.mutate(v === '' ? null : Number(v))} onCreate={create} />
      {notice && <Text fontSize="sm" color="fg.muted">{notice}</Text>}
      {save.isError && <Text fontSize="sm" color="red.600">No se pudo guardar el lugar</Text>}
    </>
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
