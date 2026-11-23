import { useState } from 'react';
import type { Warehouse } from '../../../api/backoffice';

const KEY = 'egb:compra:almacen';

function readStored(): number | null {
  try {
    const v = Number(localStorage.getItem(KEY));
    return Number.isInteger(v) && v > 0 ? v : null;
  } catch {
    return null;
  }
}

// useLastWarehouse es a dónde se guarda una compra nueva: el último almacén que se eligió en ESTE
// equipo (T045), si sigue activo; si no, el de venta. Es comodidad del equipo, no un dato del
// negocio: sin almacenamiento (ventana privada) arranca en el de venta y todo sigue funcionando.
export function useLastWarehouse(warehouses: Warehouse[]): [number | null, (id: number) => void] {
  const [picked, setPicked] = useState<number | null>(() => readStored());
  const sales = warehouses.find((w) => w.isSales)?.id ?? null;
  const current = picked !== null && warehouses.some((w) => w.id === picked) ? picked : sales;
  const set = (id: number) => {
    setPicked(id);
    try {
      localStorage.setItem(KEY, String(id));
    } catch {
      /* sin almacenamiento: se recuerda solo mientras dure la pantalla */
    }
  };
  return [current, set];
}
