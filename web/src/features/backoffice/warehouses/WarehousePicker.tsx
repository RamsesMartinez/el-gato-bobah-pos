import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { HStack, Text } from '@chakra-ui/react';
import { backofficeApi, type Warehouse } from '../../../api/backoffice';
import { Segmented } from '../../../components/Segmented';
import { can } from '../../../app/permissions';
import { useSessionStore } from '../../../stores/session';

// useWarehousePick es el almacén que eligió quien registra (spec 094): por omisión el de venta. La
// regla de lo que se manda es una sola para merma, «se acabó», mínimos y conteo: el de venta va SIN
// warehouseId (ausente es el de venta, y así lo entiende también la API anterior durante la subida);
// cualquier otro va con su id. Si el elegido se archivó en otra tableta, se vuelve al de venta.
//
// Quien ve el inventario lee la lista completa; quien solo cuenta, registra merma o «se acabó» o
// configura mínimos lee solo los nombres. Sin ninguno de esos permisos no se pide nada (un 403 por
// pantalla llenaría la bitácora de seguridad) y todo va al de venta.
const NAMES_PERMS = ['inventory.write_off', 'inventory.report_out', 'inventory.count', 'inventory.configure'] as const;

export function useWarehousePick() {
  const user = useSessionStore((s) => s.user);
  const canView = can('inventory.view', user);
  const canNames = !canView && NAMES_PERMS.some((p) => can(p, user));
  const full = useQuery({ queryKey: ['warehouses'], queryFn: () => backofficeApi.warehouses(), enabled: canView });
  const names = useQuery({ queryKey: ['warehouse-names'], queryFn: () => backofficeApi.warehouseNames(), enabled: canNames });
  const list: Pick<Warehouse, 'id' | 'name' | 'isSales'>[] = (canView ? full.data?.items : names.data?.items) ?? [];
  const [picked, setPicked] = useState<number | undefined>(undefined);
  const chosen = list.find((w) => w.id === picked && !w.isSales);
  const sales = list.find((w) => w.isSales);
  return {
    list,
    multi: list.length > 1,
    // Ya se sabe cuántos hay (o no hay permiso para preguntarlo): lo que pide distinto con uno que
    // con varios espera a esto en vez de pedir dos veces.
    known: canView ? !full.isPending : canNames ? !names.isPending : true,
    warehouseId: chosen?.id,
    // El nombre del elegido, para decir dónde quedó lo registrado.
    name: (chosen ?? sales)?.name,
    value: String((chosen ?? sales)?.id ?? ''),
    setPicked: (id: number | undefined) => setPicked(id),
  };
}

export type WarehousePick = ReturnType<typeof useWarehousePick>;

// WarehousePicker pinta la pregunta solo con dos o más almacenes; con uno no aparece nada.
// La pregunta se ve escrita junto a los botones: sin ella, «Local / Bodega» no decía qué se elegía.
export function WarehousePicker({ pick, label }: { pick: WarehousePick; label: string }) {
  if (!pick.multi) return null;
  return (
    <HStack gap={3} flexWrap="wrap" minW={0}>
      <Text fontWeight="600">{label}</Text>
      <Segmented label={label} value={pick.value}
        options={pick.list.map((w) => ({ value: String(w.id), label: w.isSales ? `${w.name} · de venta` : w.name }))}
        onChange={(v) => pick.setPicked(Number(v))} />
    </HStack>
  );
}

// withWarehouse agrega warehouseId al cuerpo solo cuando no es el de venta.
export function withWarehouse<T extends object>(body: T, warehouseId: number | undefined): T & { warehouseId?: number } {
  return warehouseId === undefined ? body : { ...body, warehouseId };
}
