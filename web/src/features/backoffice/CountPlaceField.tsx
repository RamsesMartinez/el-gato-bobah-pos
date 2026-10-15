import { useState } from 'react';
import { Button, HStack, Input, Text } from '@chakra-ui/react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { backofficeApi } from '../../api/backoffice';

// CountPlaceField es el «lugar» de un insumo (spec 038): dónde está, para que el conteo vaya en el
// orden en que se camina. No es un almacén; vacío lo quita.
export function CountPlaceField({ ingredientId, place }: { ingredientId: number; place: string | null }) {
  const qc = useQueryClient();
  const [value, setValue] = useState(place ?? '');
  const save = useMutation({
    mutationFn: () => backofficeApi.setCountPlace(ingredientId, value.trim() || null),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['ingredients'] }),
  });
  const changed = value.trim() !== (place ?? '');
  return (
    <HStack gap={2} mt={2} flexWrap="wrap">
      <Text fontSize="sm">Lugar para contar</Text>
      <Input aria-label="Lugar para contar" placeholder="Refri, Barra, Bodega…" maxLength={30} w="200px" minH="44px"
        value={value} onChange={(e) => setValue(e.target.value)} />
      <Button minH="44px" variant="outline" disabled={!changed} loading={save.isPending} onClick={() => save.mutate()}>
        Guardar lugar
      </Button>
      {save.isError && <Text fontSize="sm" color="red.600">No se pudo guardar el lugar</Text>}
    </HStack>
  );
}
