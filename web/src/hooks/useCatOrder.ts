import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';

// Orden personalizado de categorías/subcategorías del POS, por usuario, guardado en el backend
// (preferencia 'pos.cat-order') → sincroniza entre tablets. Los ids no listados quedan después.
export interface CatOrder {
  roots: number[];
  subs: Record<number, number[]>; // parentId → orden de subcategorías
}

const KEY = 'pos.cat-order';
const QK = ['prefs', KEY] as const;
type Pref = { value: CatOrder | null };

export function useCatOrder() {
  const qc = useQueryClient();
  const { data } = useQuery({
    queryKey: QK,
    queryFn: () => api.get<Pref>(`/me/preferences/${KEY}`),
    staleTime: 5 * 60_000,
  });
  const order = data?.value ?? undefined;

  const save = useMutation({
    mutationFn: (next: CatOrder) => api.put<void>(`/me/preferences/${KEY}`, next),
    onMutate: async (next) => {
      await qc.cancelQueries({ queryKey: QK });
      const prev = qc.getQueryData<Pref>(QK);
      qc.setQueryData<Pref>(QK, { value: next }); // optimista: el rail se reordena al instante
      return { prev };
    },
    onError: (_e, _v, ctx) => { if (ctx?.prev) qc.setQueryData(QK, ctx.prev); },
  });

  const setRootOrder = (ids: number[]) => save.mutate({ roots: ids, subs: order?.subs ?? {} });
  const setSubOrder = (parentId: number, ids: number[]) =>
    save.mutate({ roots: order?.roots ?? [], subs: { ...(order?.subs ?? {}), [parentId]: ids } });

  return { order, setRootOrder, setSubOrder };
}
