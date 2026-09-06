import { useMutation, useQueryClient } from '@tanstack/react-query';
import { toaster } from '../../components/ui/toaster';
import { mensajeDeError } from '../../api/mensajes';
import { posApi } from '../../api/pos';
import type { BoardOrder, OrderView } from '../../types/pos';
import { useActiveTicket, useTicketStore } from '../../stores/ticket';
import { useMenu } from '../../hooks/useMenu';
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
  const { data: menu } = useMenu();

  // El MISMO recorte que hace confirmar: un producto que otro gerente desactivó mientras estaba en
  // el carrito lo rechaza el servidor, y mandar el carrito completo tira los cinco renglones por
  // uno muerto. Confirmar ya excluía esos renglones "para no tumbar el pedido entero por uno";
  // agregar nació sin el recorte, así que el mismo carrito pasaba por un camino y no por el otro.
  const disponibles = new Set((menu?.products ?? []).map((p) => p.id));
  const cobrables = menu ? cuenta.lines.filter((l) => disponibles.has(l.productId)) : cuenta.lines;

  const mutation = useMutation({
    mutationFn: async (pedido: BoardOrder) => {
      const body = armarPedido({ cuenta, lineas: cobrables, clientUuid: cuenta.id, deliveryFee: 0 });
      return posApi.addOrderLines(pedido.id, cuenta.id, body.lines);
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
        description: mensajeDeError(e),
        type: 'error',
      });
      qc.invalidateQueries({ queryKey: ['orders', 'open'] });
    },
  });

  return { agregar: mutation.mutate, agregando: mutation.isPending };
}
