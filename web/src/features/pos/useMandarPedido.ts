import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toaster } from '../../components/ui/toaster';
import { ApiError } from '../../api/client';
import { posApi } from '../../api/pos';
import type { OrderView } from '../../types/pos';
import { useMenu } from '../../hooks/useMenu';
import { useActiveTicket, useTicketStore } from '../../stores/ticket';
import { armarPedido } from '../../domain/pedido';

export interface MandarCmd {
  // A qué pedido en curso se le agregan estos renglones. Ausente = se crea un pedido nuevo.
  agregarA?: number;
  deliveryFee?: number;
  // Qué hacer con el pedido recién creado, además de lo de siempre. Es cómo el POS abre el cobro
  // justo después de confirmar sin que este hook sepa nada de cobrar.
  luego?: (order: OrderView) => void;
}

// Manda la cuenta activa al servidor. NUNCA con pagos.
//
// El parámetro `payments` existió mientras crear-y-cobrar era una sola llamada; el servidor dejó de
// aceptarlo —ese atajo se saltaba la cocina— y aquí quedó muerto, con un test verificando la
// ausencia de un campo que ya nadie podía poner.
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
    mutationFn: ({ agregarA, deliveryFee }: MandarCmd) => {
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
        // `armarPedido` vuelve a aplicar `cobraEnvio`; aquí solo se decide CUÁNTO cuesta el envío
        // cuando aplica, no SI aplica. Deducirlo también aquí era la segunda copia de la regla.
        deliveryFee: deliveryFee ?? defaultFee,
      });
      // La MISMA llave para los dos caminos, y por la misma razón: es la de la cuenta, no una por
      // intento. Generarla por intento hacía que la protección del servidor nunca se disparara.
      return agregarA
        ? posApi.addOrderLines(agregarA, cuenta.id, body.lines)
        : posApi.createOrder(body);
    },
    onSuccess: (order, vars) => {
      closeTab(cuenta.id); // la cuenta se envió o se cobró: se cierra y queda la siguiente activa
      // El PREFIJO entero, no solo `active`. Con `['orders','active']` a secas, el pedido recién
      // confirmado no aparecía en la barra del POS —que lee `['orders','open']`— hasta su siguiente
      // refresco de 30 segundos, y con la lista vacía el botón ni siquiera se pintaba: quien mandaba
      // a cocina para cobrar después se quedaba mirando una barra sin nada, con el cliente enfrente.
      qc.invalidateQueries({ queryKey: ['orders'] });
      // El pedido nuevo alimenta las recomendaciones → refetch para verlas al instante.
      qc.invalidateQueries({ queryKey: ['modifier-defaults'] });
      // El pedido acaba de sacar un nombre de la bolsa. Sin esto, la siguiente cuenta se bautizaría
      // con la lista de antes de la venta y podría proponer el nombre que se acaba de usar — el
      // servidor lo cambiaría al confirmar, y el operador ya se lo dijo al cliente.
      qc.invalidateQueries({ queryKey: ['pos', 'folio-names'] });
      onDone(order);
      // La continuación de ESTA llamada. Se pasa como función y no como una bandera de modo: quien
      // manda el pedido es quien sabe qué sigue —cobrarlo o soltarlo— y el hook no tiene por qué
      // enterarse.
      vars.luego?.(order);
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

  return {
    mandar: mutation.mutate,
    // La versión que se puede ESPERAR. La usa la hoja de cobro para crear el pedido justo antes de
    // cobrarlo y no antes: hasta que no se toca el botón final, tocar COBRAR no manda nada a cocina.
    mandarAsync: mutation.mutateAsync,
    enviando: mutation.isPending,
    cobrables,
    noDisponibles,
    defaultFee,
  };
}
