import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { toaster } from '../../components/ui/toaster';
import { aceptarPedidoDePlataforma, type PedidoDePlataforma } from '../../api/pedidosDePlataforma';
import { AvisoDePedidoEntrante } from './AvisoDePedidoEntrante';
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
  const qc = useQueryClient();
  const [aceptando, setAceptando] = useState(false);

  const aceptar = async (pedido: PedidoDePlataforma) => {
    setAceptando(true);
    try {
      const creado = await aceptarPedidoDePlataforma(pedido.id);
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
      setAceptando(false);
      void qc.invalidateQueries({ queryKey: CLAVE_PENDIENTES });
    }
  };

  return (
    <AvisoDePedidoEntrante
      pedidos={data ?? []}
      onAceptar={(p) => void aceptar(p)}
      // ponytail: la lista completa y rechazar llegan en la siguiente entrega. Mientras tanto el
      // contador no hace nada al tocarse, así que no se muestra como si hiciera — el aviso solo
      // aparece con el más urgente. El techo es que con dos pendientes hay que aceptar el primero
      // para ver el segundo.
      onVerTodos={() => {}}
      aceptando={aceptando}
    />
  );
}
