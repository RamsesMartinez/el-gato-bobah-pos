import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toaster } from '../../components/ui/toaster';
import { ApiError } from '../../api/client';
import { posApi } from '../../api/pos';
import type { CreateOrderBody } from '../../api/pos';
import type { OrderView } from '../../types/pos';
import { useMenu } from '../../hooks/useMenu';
import { useActiveTicket, useTicketStore } from '../../stores/ticket';
import { armarPedido } from './armarPedido';

// Manda la cuenta activa al servidor, con pagos o sin ellos.
//
// Es un hook y no código dentro de la hoja de cobro porque ahora lo usan dos pantallas: el panel
// del pedido —que manda a cocina sin cobrar— y la hoja, que cobra. Antes vivía solo en la hoja, y
// por eso mandar a cocina obligaba a pasar por controles de dinero que después se descartaban.
export function useMandarPedido(onDone: (order: OrderView) => void) {
  const qc = useQueryClient();
  const cuenta = useActiveTicket();
  const closeTab = useTicketStore((s) => s.closeTab);
  const { data: menu } = useMenu();
  const { data: settings } = useQuery({ queryKey: ['business-settings'], queryFn: posApi.businessSettings });

  // Un producto puede haberse inactivado mientras estaba en el carrito. El servidor lo rechaza; se
  // excluye del envío para no tumbar el pedido entero por un renglón.
  const disponibles = new Set((menu?.products ?? []).map((p) => p.id));
  const noDisponibles = menu ? cuenta.lines.filter((l) => !disponibles.has(l.productId)) : [];
  const cobrables = menu ? cuenta.lines.filter((l) => disponibles.has(l.productId)) : cuenta.lines;

  const defaultFee = settings ? Number(settings.deliveryFee) : 20;

  const mutation = useMutation({
    mutationFn: ({ payments, agregarA, deliveryFee }: {
      payments?: CreateOrderBody['payments'];
      agregarA?: number;
      deliveryFee?: number;
    }) => {
      const body = armarPedido({
        cuenta,
        lineas: cobrables,
        // El identificador es el de la CUENTA, no uno por intento.
        //
        // El servidor tiene idempotencia por este campo —la columna es única y devuelve el pedido
        // que ya existe—, pero generándolo aquí adentro cada reintento mandaba uno distinto y esa
        // protección nunca se disparaba: un corte de red al confirmar dejaba dos pedidos idénticos,
        // el operador cobraba uno, y el otro se quedaba abierto pidiendo comida que nadie preparó.
        // El id de la cuenta ya es un uuid, ya se persiste, y muere cuando la cuenta se cierra.
        clientUuid: cuenta.id,
        deliveryFee: cuenta.serviceType === 'domicilio' ? (deliveryFee ?? defaultFee) : 0,
        payments,
      });
      return agregarA ? posApi.addOrderLines(agregarA, body.lines) : posApi.createOrder(body);
    },
    onSuccess: (order) => {
      closeTab(cuenta.id); // la cuenta se envió o se cobró: se cierra y queda la siguiente activa
      qc.invalidateQueries({ queryKey: ['orders', 'active'] });
      // El pedido nuevo alimenta las recomendaciones → refetch para verlas al instante.
      qc.invalidateQueries({ queryKey: ['modifier-defaults'] });
      onDone(order);
    },
    onError: (e: unknown) => {
      // El backend dice QUÉ producto tumbó el pedido; aquí solo se pinta. El nombre va en el título
      // para que sea lo primero que se lee: con el carrito lleno, un mensaje que solo trae un id no
      // le dice al operador qué renglón quitar.
      const detalle = e instanceof ApiError ? e.details : undefined;
      toaster.create({
        title: detalle?.productName ? `No se pudo: ${detalle.productName}` : 'No se pudo registrar el pedido',
        description: e instanceof ApiError ? e.message : String(e),
        type: 'error',
      });
    },
  });

  return { mandar: mutation.mutate, enviando: mutation.isPending, cobrables, noDisponibles, defaultFee };
}
