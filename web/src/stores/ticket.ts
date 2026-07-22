import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import type { ServiceType, TicketLine, TicketModifier } from '../types/pos';
import { uuid } from '../utils/uuid';

export function lineUnitPrice(line: TicketLine): number {
  const mods = line.modifiers.reduce((s, m) => s + m.priceDelta * m.qty, 0);
  return line.unitPrice + mods;
}
export function lineTotal(line: TicketLine): number {
  return Math.round(lineUnitPrice(line) * line.qty * 100) / 100;
}

// Una "cuenta" abierta: un pedido en curso que el cajero puede dejar y retomar.
export interface TicketTab {
  id: string;
  num: number; // etiqueta estable "Cuenta N" mientras no haya nombre de cliente
  lines: TicketLine[];
  serviceType: ServiceType;
  customerName: string;
}

function emptyTab(num: number): TicketTab {
  return { id: uuid(), num, lines: [], serviceType: 'mostrador', customerName: '' };
}

interface TicketState {
  tabs: TicketTab[];
  activeId: string;
  seq: number; // siguiente número de cuenta; no se reutiliza al cerrar
  // acciones sobre la cuenta activa (mismos nombres que antes)
  addLine: (line: Omit<TicketLine, 'lineId'>) => void;
  incrementLine: (lineId: string) => void;
  decrementLine: (lineId: string) => void;
  removeLine: (lineId: string) => void;
  updateLineModifiers: (lineId: string, modifiers: TicketModifier[], notes?: string) => void;
  setServiceType: (t: ServiceType) => void;
  setCustomerName: (name: string) => void;
  clearActive: () => void; // vacía la cuenta activa sin cerrarla
  // manejo de cuentas
  newTab: () => void;
  switchTab: (id: string) => void;
  closeTab: (id: string) => void; // al cobrar/cancelar; siempre queda ≥1 cuenta
}

const first = emptyTab(1);

export const useTicketStore = create<TicketState>()(
  persist(
    (set) => {
      // aplica fn a la cuenta activa, dejando el resto igual
      const onActive = (s: TicketState, fn: (t: TicketTab) => TicketTab) => ({
        tabs: s.tabs.map((t) => (t.id === s.activeId ? fn(t) : t)),
      });
      return {
        tabs: [first],
        activeId: first.id,
        seq: 2,

        addLine: (line) =>
          set((s) =>
            onActive(s, (t) => {
              // Producto "directo" (sin modificadores ni nota): tocarlo de nuevo SUMA a la línea
              // existente en vez de duplicarla. Con modificadores/nota cada línea puede diferir
              // (distinta configuración), así que esas sí van en líneas separadas.
              const mergeable = line.modifiers.length === 0 && !line.notes;
              if (mergeable) {
                const idx = t.lines.findIndex(
                  (l) => l.productId === line.productId && l.modifiers.length === 0 && !l.notes,
                );
                if (idx !== -1) {
                  return {
                    ...t,
                    lines: t.lines.map((l, i) => (i === idx ? { ...l, qty: l.qty + line.qty } : l)),
                  };
                }
              }
              return { ...t, lines: [...t.lines, { ...line, lineId: uuid() }] };
            }),
          ),
        incrementLine: (lineId) =>
          set((s) =>
            onActive(s, (t) => ({
              ...t,
              lines: t.lines.map((l) => (l.lineId === lineId ? { ...l, qty: l.qty + 1 } : l)),
            })),
          ),
        decrementLine: (lineId) =>
          set((s) =>
            onActive(s, (t) => ({
              ...t,
              lines: t.lines
                .map((l) => (l.lineId === lineId ? { ...l, qty: l.qty - 1 } : l))
                .filter((l) => l.qty > 0),
            })),
          ),
        removeLine: (lineId) =>
          set((s) => onActive(s, (t) => ({ ...t, lines: t.lines.filter((l) => l.lineId !== lineId) }))),
        updateLineModifiers: (lineId, modifiers, notes) =>
          set((s) =>
            onActive(s, (t) => ({
              ...t,
              lines: t.lines.map((l) => (l.lineId === lineId ? { ...l, modifiers, notes } : l)),
            })),
          ),
        setServiceType: (serviceType) => set((s) => onActive(s, (t) => ({ ...t, serviceType }))),
        setCustomerName: (customerName) => set((s) => onActive(s, (t) => ({ ...t, customerName }))),
        clearActive: () =>
          set((s) => onActive(s, (t) => ({ ...t, lines: [], customerName: '', serviceType: 'mostrador' }))),

        newTab: () =>
          set((s) => {
            const t = emptyTab(s.seq);
            return { tabs: [...s.tabs, t], activeId: t.id, seq: s.seq + 1 };
          }),
        switchTab: (id) => set({ activeId: id }),
        closeTab: (id) =>
          set((s) => {
            const tabs = s.tabs.filter((t) => t.id !== id);
            if (tabs.length === 0) {
              const t = emptyTab(s.seq);
              return { tabs: [t], activeId: t.id, seq: s.seq + 1 };
            }
            return { tabs, activeId: s.activeId === id ? tabs[0].id : s.activeId };
          }),
      };
    },
    { name: 'egb:ticket:v2' }, // ponytail: v2 nueva forma; el ticket v1 (una sola cuenta) se descarta al cargar
  ),
);

// Cuenta activa. Fallback a la primera por si activeId quedara desincronizado.
export const useActiveTicket = (): TicketTab =>
  useTicketStore((s) => s.tabs.find((t) => t.id === s.activeId) ?? s.tabs[0]);

export function ticketTotal(lines: TicketLine[]): number {
  return Math.round(lines.reduce((s, l) => s + lineTotal(l), 0) * 100) / 100;
}
export function ticketCount(lines: TicketLine[]): number {
  return lines.reduce((s, l) => s + l.qty, 0);
}
