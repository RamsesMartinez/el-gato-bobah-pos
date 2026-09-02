import { useMutation, useQueryClient } from '@tanstack/react-query';
import { toaster } from '../../components/ui/toaster';
import { ApiError } from '../../api/client';
import { posApi } from '../../api/pos';
import type { BoardOrder, OrderView } from '../../types/pos';
import { useActiveTicket, useTicketStore } from '../../stores/ticket';
import { armarPedido } from '../../domain/pedido';

// Agregarle renglones a un pedido que ya está en cocina.
//
// Existe para que llegar a un pedido en curso cueste UN toque. El camino que ya había vivía dentro
// de la hoja de cobro —armar el carrito, abrir Cobrar, bajar hasta un selector, elegir el pedido—:
// cinco toques, y en producción no se usó ni una sola vez.
export function useAgregarAPedido(onListo: (pedido: OrderView, agregados: number[]) => void) {
  const qc = useQueryClient();
  const cuenta = useActiveTicket();
  const closeTab = useTicketStore((s) => s.closeTab);

  const mutation = useMutation({
    mutationFn: async (pedido: BoardOrder) => {
      const body = armarPedido({ cuenta, lineas: cuenta.lines, clientUuid: cuenta.id, deliveryFee: 0 });
      return posApi.addOrderLines(pedido.id, body.lines);
    },
    onSuccess: (respuesta) => {
      closeTab(cuenta.id);
      qc.invalidateQueries({ queryKey: ['orders', 'open'] });
      qc.invalidateQueries({ queryKey: ['orders', 'active'] });
      onListo(respuesta, respuesta.agregados ?? []);
    },
    onError: (e: unknown) => {
      // El pedido pudo haberse cancelado o reembolsado desde la otra estación mientras esta tableta
      // dormía, y el renglón sigue en pantalla hasta el siguiente refresco. El mensaje del servidor
      // trae el estado, así que se muestra tal cual en vez de un "no se pudo" que no dice qué pasó.
      toaster.create({
        title: 'No se pudo agregar',
        description: e instanceof ApiError ? e.message : String(e),
        type: 'error',
      });
      qc.invalidateQueries({ queryKey: ['orders', 'open'] });
    },
  });

  return { agregar: mutation.mutate, agregando: mutation.isPending };
}
