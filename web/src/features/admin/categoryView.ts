import { create } from 'zustand';
import { LuLayoutGrid, LuPanelLeft, LuTable2 } from 'react-icons/lu';

// La vista de la pestaña Categorías (spec 091, M3): mosaico por omisión y la última elegida por equipo.
export type CategoryView = 'mosaico' | 'tabla' | 'panel';

const VIEW_KEY = 'egb:catalogo:vista-categorias';
export const VIEWS: { id: CategoryView; label: string; icon: typeof LuLayoutGrid }[] = [
  { id: 'mosaico', label: 'mosaico', icon: LuLayoutGrid },
  { id: 'tabla', label: 'tabla', icon: LuTable2 },
  { id: 'panel', label: 'lista y panel', icon: LuPanelLeft },
];

function readView(): CategoryView {
  try {
    const v = localStorage.getItem(VIEW_KEY);
    return v === 'tabla' || v === 'panel' ? v : 'mosaico';
  } catch {
    return 'mosaico';
  }
}

// La vista se recuerda por equipo; si el aparato no deja guardarla, se queda en memoria.
export const useCategoryView = create<{ view: CategoryView; setView: (v: CategoryView) => void }>((set) => ({
  view: readView(),
  setView: (view) => {
    try { localStorage.setItem(VIEW_KEY, view); } catch { /* sin almacenamiento: vale para esta sesión */ }
    set({ view });
  },
}));

