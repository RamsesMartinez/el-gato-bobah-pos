import { useQuery, useQueryClient } from '@tanstack/react-query';
import { backofficeApi } from '../../api/backoffice';
import { ApiError } from '../../api/client';
import { type PickerOption } from '../../components/Picker';
import { toaster } from '../../components/ui/toaster';
import { can } from '../../app/permissions';
import { useSessionStore } from '../../stores/session';

// Las ubicaciones de la empresa (spec 040, «Ubicación» desde la 043) como opciones del Picker, con el
// texto del campo vacío y el alta según el permiso. Lo usan la ficha, el alta, la lista y el conteo.

export const NO_LOCATION = 'Sin definir';
const EXAMPLE = 'Ej. Refri, Bodega, Barra';

// useLocations da las ubicaciones como opciones, el texto del campo vacío y el alta según el permiso.
// Los avisos van en un toast y no en el renglón: en el conteo el renglón mide 160 px, y al guardar el
// conteo se reordena por ubicación y el renglón se va a otra parte (revisión de tableta).
export function useLocations() {
  const qc = useQueryClient();
  const canCreate = can('inventory.configure', useSessionStore((s) => s.user));
  const { data: places = [] } = useQuery({ queryKey: ['count-places'], queryFn: backofficeApi.countPlaces });
  const options: PickerOption[] = places.map((p) => ({ value: String(p.id), label: p.name }));
  const create = async (name: string): Promise<PickerOption | null> => {
    try {
      const { id } = await backofficeApi.createCountPlace(name);
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
      cannotCreateHint: canCreate ? undefined : 'Pídele a un administrador que la cree.',
    },
  };
}
