import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { backofficeApi, type Warehouse } from '../../../api/backoffice';
import { Segmented } from '../../../components/Segmented';
import { can } from '../../../app/permissions';
import { useSessionStore } from '../../../stores/session';

// useWarehousePick es el almacén que eligió quien registra (spec 094): por omisión el de venta. La
// regla de lo que se manda es una sola para merma, «se acabó», mínimos y conteo: el de venta va SIN
// warehouseId (ausente es el de venta, y así lo entiende también la API anterior durante la subida);
// cualquier otro va con su id. Si el elegido se archivó en otra tableta, se vuelve al de venta.
//
// La lista de almacenes pide «ver inventario»: sin ese permiso no se pide (un 403 por pantalla llenaría
// la bitácora de seguridad) y se registra en el de venta, como antes.
export function useWarehousePick() {
  const canView = can('inventory.view', useSessionStore((s) => s.user));
  const { data } = useQuery({ queryKey: ['warehouses'], queryFn: () => backofficeApi.warehouses(), enabled: canView });
  const list: Warehouse[] = data?.items ?? [];
  const [picked, setPicked] = useState<number | undefined>(undefined);
  const chosen = list.find((w) => w.id === picked && !w.isSales);
  const sales = list.find((w) => w.isSales);
  return {
    list,
    multi: list.length > 1,
    warehouseId: chosen?.id,
    // El nombre del elegido, para decir dónde quedó lo registrado.
    name: (chosen ?? sales)?.name,
    value: String((chosen ?? sales)?.id ?? ''),
    setPicked: (id: number | undefined) => setPicked(id),
  };
}

export type WarehousePick = ReturnType<typeof useWarehousePick>;

// WarehousePicker pinta la pregunta solo con dos o más almacenes; con uno no aparece nada.
export function WarehousePicker({ pick, label }: { pick: WarehousePick; label: string }) {
  if (!pick.multi) return null;
  return (
    <Segmented label={label} value={pick.value}
      options={pick.list.map((w) => ({ value: String(w.id), label: w.isSales ? `${w.name} · de venta` : w.name }))}
      onChange={(v) => pick.setPicked(Number(v))} />
  );
}

// withWarehouse agrega warehouseId al cuerpo solo cuando no es el de venta.
export function withWarehouse<T extends object>(body: T, warehouseId: number | undefined): T & { warehouseId?: number } {
  return warehouseId === undefined ? body : { ...body, warehouseId };
}
