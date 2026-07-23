import { create } from 'zustand';

// Estado de "hay una versión nueva del frontend esperando" (service worker en espera). Lo prende
// registerPwa (onNeedRefresh) y lo lee el pie de sistema para mostrar un indicador persistente +
// el botón "Actualizar" — complementa el toast, que es transitorio.
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
