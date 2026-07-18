import { create } from 'zustand';
import { persist } from 'zustand/middleware';

// Paletas "inteligentes": reusamos las escalas 50–950 que ya trae Chakra
// (accesibles y afinadas por diseño). El id es el nombre del colorPalette.
export const PALETTES = [
  { id: 'brand', label: 'Gato Bobah', hint: 'Rojo de la marca' },
  { id: 'blue', label: 'Clásico', hint: 'El de siempre' },
  { id: 'teal', label: 'Menta', hint: 'Fresco y sobrio' },
  { id: 'green', label: 'Bosque', hint: 'Natural' },
  { id: 'cyan', label: 'Cielo', hint: 'Claro y vivo' },
  { id: 'purple', label: 'Uva', hint: 'Elegante' },
  { id: 'pink', label: 'Fresa', hint: 'Cálido' },
  { id: 'orange', label: 'Mango', hint: 'Energético' },
] as const;

export type PaletteId = (typeof PALETTES)[number]['id'];

// Estrategia para ordenar/recomendar opciones de modificador (patrón Strategy en el POS).
export const REC_STRATEGIES = [
  { id: 'smart', label: 'Inteligente', hint: 'Ordena por probabilidad según lo que más se pide (con %)' },
  { id: 'favorites', label: 'Favoritos', hint: 'Primero las opciones marcadas como favoritas' },
  { id: 'alphabetical', label: 'Alfabético', hint: 'Simplemente de la A a la Z' },
] as const;

export type RecStrategy = (typeof REC_STRATEGIES)[number]['id'];

// Orden personalizado de categorías/subcategorías en el POS, POR USUARIO (userId → orden).
// Local al dispositivo; sigue al usuario activo en su estación. Los ids no listados quedan
// después, en el orden del menú (sort_key). "Top"/"Todos"/"Populares" nunca se reordenan.
export interface CatOrder {
  roots: number[];
  subs: Record<number, number[]>; // parentId → orden de subcategorías
}

interface UiState {
  palette: PaletteId;
  setPalette: (p: PaletteId) => void;
  topCount: number; // cuántos productos mostrar en la pestaña "Top" del POS
  setTopCount: (n: number) => void;
  recStrategy: RecStrategy;
  setRecStrategy: (s: RecStrategy) => void;
  catOrder: Record<number, CatOrder>; // userId → orden
  setRootOrder: (userId: number, ids: number[]) => void;
  setSubOrder: (userId: number, parentId: number, ids: number[]) => void;
}

export const useUiStore = create<UiState>()(
  persist(
    (set) => ({
      palette: 'brand',
      setPalette: (palette) => set({ palette }),
      topCount: 12,
      setTopCount: (topCount) => set({ topCount: Math.max(1, Math.min(60, Math.round(topCount) || 1)) }),
      recStrategy: 'smart',
      setRecStrategy: (recStrategy) => set({ recStrategy }),
      catOrder: {},
      setRootOrder: (userId, ids) => set((s) => {
        const cur = s.catOrder[userId] ?? { roots: [], subs: {} };
        return { catOrder: { ...s.catOrder, [userId]: { ...cur, roots: ids } } };
      }),
      setSubOrder: (userId, parentId, ids) => set((s) => {
        const cur = s.catOrder[userId] ?? { roots: [], subs: {} };
        return { catOrder: { ...s.catOrder, [userId]: { ...cur, subs: { ...cur.subs, [parentId]: ids } } } };
      }),
    }),
    { name: 'egb:ui:v1' },
  ),
);
