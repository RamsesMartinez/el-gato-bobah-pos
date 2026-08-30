import { useMutation, useQueryClient } from '@tanstack/react-query';

import { posApi } from '../api/pos';

// Capturar y quitar el precio de un producto en una plataforma.
//
// Ambas invalidan el menú: el precio vive dentro de ese payload, así que sin esto la pantalla
// seguiría cobrando el anterior hasta que caduque el caché. El servidor además publica
// `menu.updated`, que refresca las OTRAS tablets; esta invalidación es para la que hizo el cambio,
// que no debe esperar a que le llegue su propio evento.
export function usePlatformPrice() {
  const qc = useQueryClient();
  const invalidar = () => qc.invalidateQueries({ queryKey: ['menu'] });

  const guardar = useMutation({
    mutationFn: ({ productId, platformId, price }: { productId: number; platformId: number; price: number }) =>
      posApi.setPlatformPrice(productId, platformId, price),
    onSuccess: invalidar,
  });

  const quitar = useMutation({
    mutationFn: ({ productId, platformId }: { productId: number; platformId: number }) =>
      posApi.removePlatformPrice(productId, platformId),
    onSuccess: invalidar,
  });

  return { guardar, quitar };
}
