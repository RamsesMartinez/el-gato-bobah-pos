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

interface UiState {
  palette: PaletteId;
  setPalette: (p: PaletteId) => void;
  topCount: number; // cuántos productos mostrar en la pestaña "Top" del POS
  setTopCount: (n: number) => void;
}

export const useUiStore = create<UiState>()(
  persist(
    (set) => ({
      palette: 'brand',
      setPalette: (palette) => set({ palette }),
      topCount: 12,
      setTopCount: (topCount) => set({ topCount: Math.max(1, Math.min(60, Math.round(topCount) || 1)) }),
    }),
    { name: 'egb:ui:v1' },
  ),
);
