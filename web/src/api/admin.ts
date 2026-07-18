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
  groupCount: number;    // grupos de modificadores activos ligados al producto
  overrideCount: number; // grupos con min/max personalizado en este producto
}

export interface ProductsQuery {
  status?: 'act' | 'inact' | 'all';
  search?: string;
  sort?: 'name' | 'options' | 'products' | 'groups'; // 'options'/'products' solo grupos; 'groups' solo productos
  dir?: 'asc' | 'desc';
  groups?: 'none' | 'some'; // solo productos: filtra por con/sin grupos activos
  limit?: number;
  offset?: number;
}
export interface ProductsPage {
  items: AdminProduct[];
  total: number; // total del filtro (status+search) — para el paginador
  counts: { act: number; inact: number }; // catálogo completo — para las pestañas
}

export interface UpdateProductBody {
  name: string;
  price: number;
  favorite: boolean;
  active: boolean;
  availableFrom?: string | null;
  availableUntil?: string | null;
}

// query-string común de paginación admin (products / modifier-options).
function pageQs(p: ProductsQuery): string {
  const qs = new URLSearchParams();
  if (p.status && p.status !== 'all') qs.set('status', p.status);
  if (p.search) qs.set('search', p.search);
  if (p.sort) qs.set('sort', p.sort);
  if (p.dir) qs.set('dir', p.dir);
  if (p.groups) qs.set('groups', p.groups);
  qs.set('limit', String(p.limit ?? 25));
  qs.set('offset', String(p.offset ?? 0));
  return qs.toString();
}

export const adminApi = {
  users: () => api.get<{ items: AdminUser[] }>('/users'),
  createUser: (b: { name: string; role: string; username?: string; pin?: string; password?: string }) =>
    api.post<AdminUser>('/users', b),

  products: (p: ProductsQuery = {}) => api.get<ProductsPage>(`/admin/products?${pageQs(p)}`),
  updateProduct: (id: number, b: UpdateProductBody) =>
    api.patch<void>(`/admin/products/${id}`, b),

  modifierOptions: (p: ProductsQuery = {}) => api.get<OptionsPage>(`/admin/modifier-options?${pageQs(p)}`),
  setOptionFavorite: (id: number, favorite: boolean) =>
    api.patch<void>(`/admin/modifier-options/${id}`, { favorite }),
  setOptionActive: (id: number, active: boolean) =>
    api.patch<void>(`/admin/modifier-options/${id}`, { active }),
  updateOption: (id: number, b: { name: string; priceDelta: number; maxPerLine: number }) =>
    api.patch<void>(`/admin/modifier-options/${id}`, b),

  // Catálogo de grupos
  groups: (p: ProductsQuery = {}) => api.get<GroupsPage>(`/admin/groups?${pageQs(p)}`),
  createGroup: (name: string, defaultMin: number, defaultMax: number) =>
    api.post<{ id: number }>('/admin/groups', { name, defaultMin, defaultMax }),
  updateGroup: (id: number, b: { name: string; active: boolean; defaultMin: number; defaultMax: number }) =>
    api.patch<void>(`/admin/groups/${id}`, b),
  groupOptions: (id: number) => api.get<{ items: GroupOption[] }>(`/admin/groups/${id}/options`),
  createOption: (groupId: number, b: { name: string; priceDelta: number; maxPerLine: number }) =>
    api.post<{ id: number }>(`/admin/groups/${groupId}/options`, b),
  reorderOptions: (groupId: number, ids: number[]) =>
    api.post<void>(`/admin/groups/${groupId}/options/reorder`, { ids }),
  groupProducts: (id: number) => api.get<{ items: GroupProduct[] }>(`/admin/groups/${id}/products`),

  // Grupos asignados a un producto (min/max/obligatorio por producto)
  productGroups: (productId: number) => api.get<{ items: ProductGroup[] }>(`/admin/products/${productId}/groups`),
  attachProductGroup: (productId: number, b: { groupId: number; title: string; override: boolean; minSelect: number; maxSelect: number; position: number }) =>
    api.post<void>(`/admin/products/${productId}/groups`, b),
  detachProductGroup: (productId: number, groupId: number) =>
    api.del<void>(`/admin/products/${productId}/groups/${groupId}`),
};

export interface Group {
  id: number;
  name: string;
  isActive: boolean;
  defaultMin: number;
  defaultMax: number;
  optionCount: number;
  productCount: number;
  overrideCount: number; // productos que sobrescriben el default
}
export interface GroupsPage {
  items: Group[];
  total: number;
  counts: { act: number; inact: number };
}
export interface GroupOption {
  id: number;
  groupId: number;
  name: string;
  priceDelta: number;
  maxPerLine: number;
  currentCost: number;
  favorite: boolean;
  active: boolean;
}
export interface ProductGroup {
  groupId: number;
  groupName: string;
  groupActive: boolean;
  title: string;
  minSelect: number;   // efectivo (override o default del grupo)
  maxSelect: number;
  overridden: boolean; // true = personaliza el default del grupo
  defaultMin: number;  // default del grupo
  defaultMax: number;
  position: number;
  optionCount: number;
}
export interface GroupProduct {
  id: number;
  name: string;
  required: boolean;
  overridden: boolean;
  minSelect: number;
  maxSelect: number;
}

export interface AdminModifierOption {
  id: number;
  groupId: number;
  groupName: string;
  name: string;
  priceDelta: number;
  favorite: boolean;
  active: boolean;
}
export interface OptionsPage {
  items: AdminModifierOption[];
  total: number; // total del filtro (status+search) — para el paginador
  counts: { act: number; inact: number }; // opciones de grupos activos — para las pestañas
}
