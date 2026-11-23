import { useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { posApi } from '../../api/pos';
import { mensajeDeError } from '../../api/mensajes';
import { toaster } from '../../components/ui/toaster';
import { nombreDeCuenta } from '../../domain/cuentas';
import type { AccountItem } from '../../types/pos';

// useDeliverAccount entrega un pedido desde Vender (spec 089): el mismo POST que «Entregar todo» en
// Pedidos. Sin estado optimista: la cuenta sale de la fila cuando el servidor lo confirma, así una
// recarga muestra lo mismo. Con éxito o con error se relee la fila: si otra tableta ya la cerró, el
// error se explica y la fila queda con el estado real.
// `onDelivered` corre cuando el servidor confirma la entrega (ver POSPage: si era la cuenta abierta y
// ya estaba pagada, la tableta pasa a una cuenta nueva).
export function useDeliverAccount({ onDelivered }: { onDelivered?: (c: AccountItem) => void } = {}) {
  const qc = useQueryClient();
  // Las cuentas que se están entregando. Por cuenta y no una sola: con un candado único, «Entregar» en
  // otra cuenta mientras iba la primera se perdía sin aviso.
  const [delivering, setDelivering] = useState<ReadonlySet<string>>(new Set());
  // El ref y no el estado: dos toques en el mismo cuadro todavía no ven el estado nuevo.
  const enCurso = useRef(new Set<string>());

  const deliver = (c: AccountItem) => {
    if (c.kind !== 'order' || !c.orderId || enCurso.current.has(c.key)) return;
    enCurso.current.add(c.key);
    setDelivering(new Set(enCurso.current));
    posApi.deliverOrder(c.orderId)
      .then(() => {
        toaster.create({ title: `${nombreDeCuenta(c)}: entregada`, type: 'success' });
        onDelivered?.(c);
      })
      .catch((e: unknown) => toaster.create({ title: 'No se pudo entregar', description: mensajeDeError(e), type: 'error' }))
      .finally(() => {
        enCurso.current.delete(c.key);
        setDelivering(new Set(enCurso.current));
        qc.invalidateQueries({ queryKey: ['pos', 'accounts'] });
        qc.invalidateQueries({ queryKey: ['orders'] });
      });
  };

  return { deliver, delivering };
}
