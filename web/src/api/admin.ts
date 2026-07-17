import { api } from './client';

export interface AdminUser {
  id: number;
  name: string;
  username: string | null;
  role: string;
  isActive: boolean;
}
export interface AdminProduct {
  id: number;
  name: string;
  price: number;
  current_cost: number;
  type: string;
  is_active: boolean;
  is_favorite: boolean;
  category: string;
  availableFrom: string | null;  // "YYYY-MM-DD" o null (temporada)
  availableUntil: string | null;
}

export interface UpdateProductBody {
  name: string;
  price: number;
  favorite: boolean;
  active: boolean;
  availableFrom?: string | null;
  availableUntil?: string | null;
}

export const adminApi = {
  users: () => api.get<{ items: AdminUser[] }>('/users'),
  createUser: (b: { name: string; role: string; username?: string; pin?: string; password?: string }) =>
    api.post<AdminUser>('/users', b),

  products: () => api.get<{ items: AdminProduct[] }>('/admin/products'),
  updateProduct: (id: number, b: UpdateProductBody) =>
    api.patch<void>(`/admin/products/${id}`, b),
};
