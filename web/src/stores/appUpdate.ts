import { create } from 'zustand';

// Estado de "hay una versión nueva del frontend esperando" (service worker en espera). Lo prende
// registerPwa (onNeedRefresh) y lo lee el riel: el botón «Actualizar» en el lugar del logo
// (UpdateRailButton) y «Actualizar ahora» en «Detalles del sistema».
interface AppUpdateState {
  needRefresh: boolean;
  apply: () => void; // activa el SW nuevo (skipWaiting); la recarga la dispara el guard de registerPwa
  markNeedRefresh: (apply: () => void) => void;
}

export const useAppUpdate = create<AppUpdateState>((set) => ({
  needRefresh: false,
  apply: () => {},
  markNeedRefresh: (apply) => set({ needRefresh: true, apply }),
}));
