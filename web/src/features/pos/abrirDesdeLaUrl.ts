import { useEffect, useRef } from 'react';
import { useSearchParams } from 'react-router';
import { posApi } from '../../api/pos';
import { toaster } from '../../components/ui/toaster';
import { usePosStore, type Seleccion } from '../../stores/pos';

// ABRIR UNA CUENTA DESDE LA URL (spec 030, FR-019; research R-15).
//
// «Abrir cuenta» del tablero y del cierre de caja llevan a `/pos?pedido=<id>` o `/pos?cuenta=<uuid>`.
// La pantalla la selecciona y LIMPIA el parámetro, para que un F5 no la vuelva a forzar encima de
// lo que el operador abrió después.
//
// Un parámetro presente y malformado se RECHAZA con el mismo aviso que uno que no existe: nunca cae
// en silencio a otra cuenta (constitución V).

export type CuentaPedida = Seleccion | { kind: 'malformada' };

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export function leerCuentaDeLaUrl(p: URLSearchParams): CuentaPedida | null {
  const pedido = p.get('pedido');
  const cuenta = p.get('cuenta');
  if (pedido !== null) {
    if (!/^\d+$/.test(pedido) || Number(pedido) <= 0) return { kind: 'malformada' };
    return { kind: 'order', id: Number(pedido) };
  }
  if (cuenta !== null) {
    if (!UUID.test(cuenta)) return { kind: 'malformada' };
    return { kind: 'draft', id: cuenta.toLowerCase() };
  }
  return null;
}

// `alAbrir` deja el ticket a la vista: quien toca «Abrir cuenta» quiere verla, no buscar después el
// botón que muestra el panel.
export function useAbrirDesdeLaUrl(alAbrir?: () => void): void {
  const [params, setParams] = useSearchParams();
  const abrir = useRef(alAbrir);
  useEffect(() => { abrir.current = alAbrir; });
  useEffect(() => {
    const pedida = leerCuentaDeLaUrl(params);
    if (pedida === null) return;
    setParams({}, { replace: true });
    const seleccionar = (sel: Seleccion) => {
      usePosStore.getState().seleccionar(sel);
      abrir.current?.();
    };
    void (async () => {
      try {
        if (pedida.kind === 'order') {
          await posApi.order(pedida.id);
          seleccionar(pedida);
          return;
        }
        if (pedida.kind === 'draft') {
          const d = await posApi.getDraft(pedida.id);
          if (d.status === 'capturando') { seleccionar({ kind: 'draft', id: d.id }); return; }
          if (d.status === 'enviada' && d.orderId !== null) { seleccionar({ kind: 'order', id: d.orderId }); return; }
        }
      } catch {
        // Cae al aviso: para quien opera, «no existe» y «no se encontró» son lo mismo.
      }
      toaster.create({ title: 'Esa cuenta ya no existe', type: 'info' });
    })();
  }, [params, setParams]);
}
