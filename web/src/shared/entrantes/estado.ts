import { create } from 'zustand';

// Si la columna «Por aceptar» está a la vista. La franja se quita SOLO entonces: en Pedidos con la
// pestaña «Cerrados» abierta la columna no existe, y sin franja la alarma sonaría sin decir por qué.
export const useColumnaPorAceptar = create<{ visible: boolean }>(() => ({ visible: false }));

// La pestaña de Pedidos vive aquí y no en la pantalla: «Ver pedido» de la franja tiene que poder
// regresar a «En curso» aunque ya se esté en Pedidos.
export type PestanaDePedidos = 'curso' | 'cerrados';
export const usePestanaDePedidos = create<{ pestana: PestanaDePedidos }>(() => ({ pestana: 'curso' }));
