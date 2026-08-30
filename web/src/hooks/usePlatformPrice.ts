import { useMutation } from '@tanstack/react-query';

import { posApi } from '../api/pos';
import { useReprecio } from './useReprecio';

// Capturar y quitar el precio de un producto en una plataforma.
//
// Ambas traen el menú de nuevo Y vuelven a precisar las cuentas abiertas: el precio vive dentro de
// ese payload y además se congela en cada línea al agregarla, así que sin las dos cosas el producto
// que se acaba de corregir seguiría cobrándose al precio viejo en el ticket en curso — justo el que
// el operador estaba corrigiendo.
export function usePlatformPrice() {
  const invalidar = useReprecio();

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

// Lo mismo para el cargo de un extra en una plataforma.
export function usePlatformOptionPrice() {
  const invalidar = useReprecio();

  const guardar = useMutation({
    mutationFn: ({ optionId, platformId, priceDelta }: { optionId: number; platformId: number; priceDelta: number }) =>
      posApi.setPlatformOptionPrice(optionId, platformId, priceDelta),
    onSuccess: invalidar,
  });

  const quitar = useMutation({
    mutationFn: ({ optionId, platformId }: { optionId: number; platformId: number }) =>
      posApi.removePlatformOptionPrice(optionId, platformId),
    onSuccess: invalidar,
  });

  return { guardar, quitar };
}
