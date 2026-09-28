import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { toaster } from '../../components/ui/toaster';
import { aceptarPedidoDePlataforma, type PedidoDePlataforma } from '../../api/pedidosDePlataforma';
import { posApi } from '../../api/pos';
import type { OrderView } from '../../types/pos';
import { KitchenTicket } from '../../shared/tickets/AutoPrintTicket';
import { AvisoDePedidoEntrante } from './AvisoDePedidoEntrante';
import { PedidosEntrantesSheet } from './PedidosEntrantesSheet';
import { CLAVE_PENDIENTES, usePedidosDePlataforma } from './usePedidosDePlataforma';

/**
 * AvisoDePlataforma conecta el aviso con los datos y con la acción de aceptar.
 *
 * Existe aparte del aviso para que éste se pueda probar sin red ni caché: lo que decide si la
 * feature sirve es DÓNDE se pinta y qué dice, y eso no debería necesitar montar media aplicación
 * para verificarse.
 */
export function AvisoDePlataforma() {
  const { data } = usePedidosDePlataforma();
  const pedidos = data ?? [];
  const qc = useQueryClient();
  const [aceptandoId, setAceptandoId] = useState<number | null>(null);
  const [listaAbierta, setListaAbierta] = useState(false);
  // La comanda del último aceptado. KitchenTicket respeta el ajuste del negocio y recuerda lo ya
  // impreso, así que un re-render no saca un segundo papel. Solo la imprime la tableta que aceptó:
  // la otra se entera por el evento y no imprime nada.
  const [comanda, setComanda] = useState<OrderView | null>(null);

  const aceptar = async (pedido: PedidoDePlataforma) => {
    setAceptandoId(pedido.id);
    let creado;
    try {
      creado = await aceptarPedidoDePlataforma(pedido.id);
      toaster.create({
        title: `Pedido #${creado.number} aceptado`,
        description: 'Ya está en cocina.',
        type: 'success',
      });
    } catch {
      // EL MENSAJE DICE QUÉ HACER, no qué falló. La causa más probable es que la plataforma no
      // confirmó —y entonces el pedido NO quedó aceptado de nuestro lado, que es lo correcto—, o
      // que otra tableta ganó. En los dos casos lo accionable es lo mismo: volver a mirar la lista.
      toaster.create({
        title: 'No se pudo aceptar',
        description: 'Puede que otra tableta ya lo haya atendido. Revisa la lista.',
        type: 'error',
      });
    } finally {
      setAceptandoId(null);
      void qc.invalidateQueries({ queryKey: CLAVE_PENDIENTES });
    }
    if (!creado) return;
    // La comanda sale del pedido que quedó en el POS y no del aviso: es la misma que cocina ve en
    // Pedidos y la que se reimprime desde ahí. Si no se puede traer, el pedido ya está aceptado y
    // se dice dónde buscarlo, en vez de perder el papel en silencio.
    try {
      setComanda(await posApi.order(creado.id));
    } catch {
      toaster.create({
        title: 'No salió la comanda',
        description: 'El pedido ya está aceptado. Imprímela desde Pedidos.',
        type: 'warning',
      });
    }
  };

  // Sin pendientes no hay lista que mostrar: aceptado el último, la hoja se cierra sola.
  const lista = listaAbierta && pedidos.length > 0;

  return (
    <>
      {/* La franja se quita mientras la lista está abierta: va por encima de todo, y taparía la
          hoja que ya muestra lo mismo. */}
      {!lista && (
        <AvisoDePedidoEntrante
          pedidos={pedidos}
          onAceptar={(p) => void aceptar(p)}
          onVerTodos={() => setListaAbierta(true)}
          aceptando={aceptandoId !== null}
        />
      )}
      <PedidosEntrantesSheet
        abierta={lista}
        pedidos={pedidos}
        aceptandoId={aceptandoId}
        onCerrar={() => setListaAbierta(false)}
        onAceptar={(p) => void aceptar(p)}
      />
      <KitchenTicket order={comanda} />
    </>
  );
}
