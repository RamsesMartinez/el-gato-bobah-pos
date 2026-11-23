import { useQueries, useQuery, useQueryClient } from '@tanstack/react-query';
import { backofficeApi } from '../../api/backoffice';
import { ApiError } from '../../api/client';
import { type PickerOption } from '../../components/Picker';
import { toaster } from '../../components/ui/toaster';
import { can } from '../../app/permissions';
import { useSessionStore } from '../../stores/session';
import { useWarehousePick } from './warehouses/WarehousePicker';

// Las ubicaciones de la empresa (spec 040, «Ubicación» desde la 043) como opciones del Picker, con el
// texto del campo vacío y el alta según el permiso. Lo usan la ficha, el alta, la lista y el conteo.

export const NO_LOCATION = 'Sin definir';
const EXAMPLE = 'Ej. Refri, Bodega, Barra';

// useLocations da las ubicaciones como opciones, el texto del campo vacío y el alta según el permiso.
// Los avisos van en un toast y no en el renglón: en el conteo el renglón mide 160 px, y al guardar el
// conteo se reordena por ubicación y el renglón se va a otra parte (revisión de tableta).
// Con warehouseId (spec 094) son las de ese almacén, y la que se crea nace en él. Con all (la ficha
// del artículo y el filtro de Artículos), las de todos los almacenes, cada una con el suyo: un
// artículo tiene un solo lugar y puede estar en la Bodega. La que se crea ahí nace en el de venta.
export function useLocations(warehouseId?: number, { all = false }: { all?: boolean } = {}) {
  const qc = useQueryClient();
  const canCreate = can('inventory.configure', useSessionStore((s) => s.user));
  const { data: places = [] } = useQuery({
    queryKey: warehouseId === undefined ? ['count-places'] : ['count-places', warehouseId],
    queryFn: () => backofficeApi.countPlaces(warehouseId),
  });
  const wh = useWarehousePick();
  const others = all && wh.multi ? wh.list.filter((w) => !w.isSales) : [];
  const more = useQueries({ queries: others.map((w) => ({
    queryKey: ['count-places', w.id], queryFn: () => backofficeApi.countPlaces(w.id),
  })) });
  const salesName = wh.list.find((w) => w.isSales)?.name;
  const options: PickerOption[] = [
    ...places.map((p) => ({ value: String(p.id), label: p.name, ...(others.length ? { hint: salesName } : {}) })),
    ...others.flatMap((w, i) => (more[i]?.data ?? []).map((p) => ({ value: String(p.id), label: p.name, hint: w.name }))),
  ];
  const create = async (name: string): Promise<PickerOption | null> => {
    try {
      const { id } = await backofficeApi.createCountPlace(name, warehouseId);
      await qc.invalidateQueries({ queryKey: ['count-places'] });
      return { value: String(id), label: name };
    } catch (e) {
      const existing = e instanceof ApiError ? (e.details as { existingId?: number } | undefined)?.existingId : undefined;
      if (!existing) {
        toaster.create({
          title: e instanceof ApiError && e.status === 400 ? 'La ubicación lleva de 1 a 30 letras' : 'No se pudo crear la ubicación',
          type: 'error',
        });
        return null;
      }
      const found = places.find((p) => p.id === existing);
      toaster.create({ title: `Ya tenías «${found?.name ?? name}»; quedó elegida`, type: 'info' });
      return { value: String(existing), label: found?.name ?? name };
    }
  };
  return {
    options,
    pickerProps: {
      // El campo dice «Sin definir» siempre: el ejemplo en el campo parecía un valor ya puesto (prueba sin
      // contexto). Va dentro del buscador, y solo cuando no hay ninguna (decisión del dueño, 2026-10-20).
      title: 'Ubicación', placeholder: NO_LOCATION, clearable: true, clearLabel: NO_LOCATION,
      searchPlaceholder: options.length ? undefined : EXAMPLE,
      focusSearch: false, searchMaxLength: 30, createFirst: true,
      onCreate: canCreate ? create : undefined,
      cannotCreateHint: canCreate ? undefined : 'Pídele a quien configura el almacén que la cree.',
    },
  };
}
