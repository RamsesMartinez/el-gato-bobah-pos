import { api } from './client';

// Lo que se echó a perder (spec 039). Las cantidades y los pesos viajan como texto decimal.

export interface WasteReason {
  id: number;
  name: string;
  description: string;
  isStaffMeal: boolean;
  uses: number;
}

export type WasteStatus = 'applied' | 'pending' | 'rejected' | 'covered' | 'undone';

export interface WasteRow {
  id: number;
  itemType: 'ingrediente' | 'producto';
  itemName: string;
  quantity: string;
  unitCode: string;
  totalCost: string | null;
  status: WasteStatus;
  reason: string;
  recordedBy: string;
  recordedAt: string;
  canUndo: boolean;
  isStaffMeal?: boolean;
  note?: string | null;
}

export interface WasteItems {
  ingredients: { id: number; name: string; unitCode: string; units: { id: number; code: string }[]; packs: { id: number; name: string; qtyInBase: string }[] }[];
  products: { id: number; name: string }[];
  recent: { itemType: 'ingrediente' | 'producto'; id: number }[];
}

export interface RecordWasteInput {
  reasonId: number;
  ingredientId?: number;
  productId?: number;
  qty: string;
  unitId?: number;
  presentationId?: number;
  note?: string;
}

export interface WasteResult {
  id: number;
  status: WasteStatus;
  totalCost: string | null;
}

export const wasteApi = {
  reasons: () => api.get<WasteReason[]>('/waste/reasons'),
  items: () => api.get<WasteItems>('/waste/items'),
  createReason: (name: string, description: string) => api.post<{ id: number }>('/waste/reasons', { name, description }),
  record: (input: RecordWasteInput) => api.post<WasteResult>('/waste', input),
  today: () => api.get<WasteRow[]>('/waste/today'),
  undo: (id: number) => api.post<void>(`/waste/${id}/undo`),
  pending: () => api.get<WasteRow[]>('/waste/pending'),
  approve: (id: number) => api.post<void>(`/waste/${id}/approve`),
  reject: (id: number) => api.post<void>(`/waste/${id}/reject`),
  setThreshold: (threshold: string | null) => api.put<void>('/settings/waste-threshold', { threshold }),
};
