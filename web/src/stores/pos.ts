import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import type { ServiceType } from '../types/pos';

// Qué cuenta tiene abierta ESTA tableta. Es lo único del POS que se guarda en el dispositivo
// (spec 030, research R-13): la cuenta misma —renglones, cliente, canal, descuento— vive en el
// servidor desde el primer producto y la sirve TanStack Query.
//
// Reemplaza a `egb:ticket:v2`, que guardaba las cuentas enteras en la tableta. De ahí salían tres
// defectos: una tableta que se apagaba se llevaba lo capturado, las otras tabletas no veían esas
// cuentas, y una cuenta guardada con la forma de una versión vieja dejaba la pantalla en blanco.
export type Seleccion = { kind: 'draft'; id: string } | { kind: 'order'; id: number };

// La cabecera de la cuenta que TODAVÍA NO NACE: se eligió Uber antes del primer producto. Vive en
// memoria y viaja como `header` al crear la cuenta; no se guarda porque una cuenta sin productos no
// existe para nadie más.
export interface CabeceraNueva {
  platformId: number | null;
  platformOrderRef: string;
  serviceType: ServiceType;
  customerName: string;
}

const MOSTRADOR: CabeceraNueva = { platformId: null, platformOrderRef: '', serviceType: 'mostrador', customerName: '' };

// claveDeSeleccion es la llave con la que la fila nombra a cada cuenta (`AccountItem.key`).
export function claveDeSeleccion(s: Seleccion): string {
  return s.kind === 'draft' ? `d:${s.id}` : `o:${s.id}`;
}

const MAX_RECIENTES = 20;

interface PosState {
  selected: Seleccion | null;
  // Las cuentas que esta tableta abrió, de la más reciente a la más vieja. En memoria a propósito:
  // solo ordena la fila, y tras un F5 la fila sigue mostrándolas todas.
  recientes: string[];
  nueva: CabeceraNueva;
  seleccionar: (s: Seleccion) => void;
  // Olvida la selección SOLO si sigue siendo ésa: la respuesta tardía de una cuenta vieja no puede
  // borrar la que el operador acaba de abrir.
  olvidar: (s: Seleccion) => void;
  // «+» de la fila: sin selección y con la cabecera en mostrador. La cuenta nace al primer producto.
  cuentaNueva: () => void;
  cambiarNueva: (c: Partial<CabeceraNueva>) => void;
  reiniciarNueva: () => void;
}

// seleccionValida descarta lo que no tiene la forma exacta. Una selección rara se tira en silencio:
// lo peor que pasa es que la tableta abre sin cuenta seleccionada, y la fila sigue mostrándolas todas.
export function seleccionValida(s: unknown): s is Seleccion {
  if (typeof s !== 'object' || s === null) return false;
  const { kind, id } = s as { kind?: unknown; id?: unknown };
  if (kind === 'draft') return typeof id === 'string' && id.length > 0;
  if (kind === 'order') return typeof id === 'number' && Number.isInteger(id) && id > 0;
  return false;
}

export function mismaSeleccion(a: Seleccion | null, b: Seleccion | null): boolean {
  return a !== null && b !== null && a.kind === b.kind && a.id === b.id;
}

export const usePosStore = create<PosState>()(
  persist(
    (set) => ({
      selected: null,
      recientes: [],
      nueva: MOSTRADOR,
      seleccionar: (selected) => set((st) => {
        const k = claveDeSeleccion(selected);
        return { selected, recientes: [k, ...st.recientes.filter((r) => r !== k)].slice(0, MAX_RECIENTES) };
      }),
      olvidar: (s) => set((st) => (mismaSeleccion(st.selected, s) ? { selected: null } : st)),
      cuentaNueva: () => set({ selected: null, nueva: MOSTRADOR }),
      cambiarNueva: (c) => set((st) => {
        const nueva = { ...st.nueva, ...c };
        if (c.platformId !== undefined && c.platformId !== st.nueva.platformId && c.platformOrderRef === undefined) {
          nueva.platformOrderRef = '';
        }
        return { nueva };
      }),
      reiniciarNueva: () => set({ nueva: MOSTRADOR }),
    }),
    {
      name: 'egb:pos:v3',
      partialize: (s) => ({ selected: s.selected }),
      merge: (persisted, current) => {
        const sel = (persisted as { selected?: unknown } | undefined)?.selected;
        return { ...current, selected: seleccionValida(sel) ? sel : null };
      },
    },
  ),
);
