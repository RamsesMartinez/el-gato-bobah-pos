import type { SimilarAlt } from '../components/altNames';
import { api } from './client';

export interface AdminUser {
  id: number;
  name: string;
  username: string | null;
  // Código del rol sugerido o "" si es propio; lo que se muestra es el nombre del rol (roleId).
  role: string;
  roleId?: number | null;
  isActive: boolean;
  recoveryEmail?: string | null;
  mustChangePassword?: boolean;
}
// Un permiso del catálogo del producto (spec 068), en el orden en que se muestra.
export interface PermissionInfo {
  code: string;
  area: string;
  label: string;
}
// Un rol de la empresa. systemCode es "" en los propios. added/removed: lo que un sugerido tiene de
// más o de menos contra lo que el producto le da. Las listas son opcionales para que el compilador
// obligue a la guarda si llegaran como null.
export interface Role {
  id: number;
  systemCode: string;
  name: string;
  active: boolean;
  version: number;
  activeUsers: number;
  allUsers: number;
  permissions?: string[];
  added?: string[];
  removed?: string[];
}
export interface AdminProduct {
  id: number;
  name: string;
  price: string; // dinero = string decimal exacto (ver types/pos.ts)
  current_cost: string;
  type: string;
  is_active: boolean;
  is_favorite: boolean;
  // Si el producto necesita prepararse. Es lo que decide si su pedido va al tablero de Pedidos.
  needsPrep: boolean;
  category: string;
  categoryId: number;
  availableFrom: string | null;  // "YYYY-MM-DD" o null (temporada)
  availableUntil: string | null;
  groupCount: number;    // grupos de modificadores activos ligados al producto
  overrideCount: number; // grupos con min/max personalizado en este producto
  compositionStatus?: CompositionStatus;
  // De dónde sale current_cost. Opcionales para que el compilador obligue a la guarda en quien
  // los lea: una respuesta vieja sin ellos no debe tumbar el diálogo.
  costSource?: CostSource;
  manualCost?: string | null;
  hasRecipe?: boolean;
  // Otros nombres con que se busca (spec 063). Opcional: una respuesta de la API anterior no los trae.
  altNames?: string[];
  // El otro nombre por el que salió en la búsqueda; ausente si salió por su nombre.
  matchedAlt?: string;
}

export type CostSource = 'manual' | 'compra' | 'receta';
// Cambio de costo en el PATCH: ausente = no tocarlo.
export type CostChange = { source: 'manual'; amount: number } | { source: 'receta' };

// Qué lleva un producto o un extra: lo que descuenta del almacén al venderse.
export type CompositionStatus = '' | 'estimated' | 'confirmed';
export type CompositionKind = 'product' | 'option' | 'ingredient';
export interface CompositionItem {
  ingredientId: number;
  ingredientName: string;
  quantity: string;
  unitId: number;
  unitCode: string;
}
export interface Composition {
  status: CompositionStatus;
  confirmedBy?: string;
  confirmedAt?: string;
  linkedProductId?: number;
  linkedProductName?: string;
  // Opcional a propósito: obliga a la guarda si el servidor llegara a mandar null.
  items?: CompositionItem[];
  components?: { productId: number; productName: string; quantity: number }[];
  // Solo un insumo: cuánto rinde si se prepara aquí, en su unidad base.
  yield?: string;
  yieldUnitCode?: string;
  editable: boolean;
  reason?: 'package_choices';
  // El producto todavía cuenta su propia existencia (spec 037): al elegir otra tarjeta se traslada o
  // se cierra. productOnHand es la cifra que se muestra al preguntar «¿Cuántas tienes de verdad?».
  ownStock?: boolean;
  productOnHand?: string;
  ownStockBranches?: number;
  // Cuándo se guardó por última vez: se devuelve al guardar para saber si otra persona la cambió.
  stamp?: string;
  // Extras que se llaman igual en otro grupo.
  // today: lo que ese extra lleva hoy, para que copiarle la receta no esconda que es otra cosa.
  sameName?: { id: number; group: string; stamp?: string; today?: string }[];
}
export interface CompositionBody {
  // Un insumo nuevo va con `newArticle` y sin id: el servidor lo crea al guardar la receta (spec 042).
  items: { ingredientId?: number; newArticle?: { name: string; baseUnitId: number }; quantity: string; unitId: number }[];
  linkedProductId: number | null;
  components: { productId: number; quantity: number }[];
  yield?: string | null;
  alsoOptionIds?: number[];
  // La marca de cada extra de alsoOptionIds: copiarle la receta no pisa lo que alguien le guardó.
  alsoBasedOn?: Record<number, string>;
  basedOn?: string;
  // La tarjeta elegida: el servidor revisa que el contenido le corresponda (spec 037).
  mode?: 'items' | 'package' | 'product' | 'nothing';
  // «¿Cuántas tienes de verdad?» y las cifras que se mostraron al preguntar.
  actualStock?: string;
  seenProductOnHand?: string;
  seenIngredientOnHand?: string;
}

// Menú › Recetas.
export type RecipeKind = 'product' | 'extra' | 'prep';
export type RecipeStatus = 'pending' | 'review' | 'done';
export interface RecipeRow {
  id: number;
  name: string;
  group: string;
  status: RecipeStatus;
  mode: '' | 'items' | 'combo' | 'product' | 'own';
  summary: string;
  lines: number;
  soldPerMonth: string;
  // Con un solo insumo por pieza (se compra hecho): cuánto resta cada venta y cuánto hay.
  perSale?: string;
  onHand?: string;
}
export interface RecipeCounts { pending: number; review: number; done: number }
export interface RecipePage {
  // Opcional a propósito: obliga a la guarda si el servidor llegara a mandar null.
  items?: RecipeRow[];
  total: number;
  counts: RecipeCounts;
  totals: { product: RecipeCounts; extra: RecipeCounts; prep: RecipeCounts };
}
export interface RecipeQuery {
  kind: RecipeKind;
  status: RecipeStatus;
  sort: 'sales' | 'az';
  q?: string;
  category?: number;
  limit?: number;
  offset?: number;
}

// Categoría (para filtro y alta de productos).
export interface Category {
  id: number;
  name: string;
  parentId: number | null;
}

// Columnas ordenables de la tabla de productos (espejo de AdminListProducts en el backend).
export type ProductSort = 'name' | 'price' | 'cost' | 'margin' | 'category' | 'groups';

export interface ProductsQuery {
  status?: 'act' | 'inact' | 'all';
  search?: string;
  // 'options'/'products' solo aplican a la página de grupos; el resto son columnas de productos.
  sort?: 'options' | 'products' | ProductSort;
  dir?: 'asc' | 'desc';
  groups?: 'none' | 'some'; // solo productos: filtra por con/sin grupos activos
  composition?: 'none' | 'estimated'; // productos y extras: sin capturar / por revisar
  categoryId?: number;      // solo productos: filtra por categoría (incluye subcategorías)
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
  needsPrep: boolean;
  // Ausente o 0 = no mover el producto de categoría. El servidor valida que sea de esta empresa
  // antes de escribirla; mandar la de otra devuelve el mismo error que una que no existe.
  categoryId?: number;
  availableFrom?: string | null;
  availableUntil?: string | null;
  cost?: CostChange;
}

// query-string común de paginación admin (products / modifier-options).
function pageQs(p: ProductsQuery): string {
  const qs = new URLSearchParams();
  if (p.status && p.status !== 'all') qs.set('status', p.status);
  if (p.search) qs.set('search', p.search);
  if (p.sort) qs.set('sort', p.sort);
  if (p.dir) qs.set('dir', p.dir);
  if (p.groups) qs.set('groups', p.groups);
  if (p.composition) qs.set('composition', p.composition);
  if (p.categoryId) qs.set('categoryId', String(p.categoryId));
  qs.set('limit', String(p.limit ?? 25));
  qs.set('offset', String(p.offset ?? 0));
  return qs.toString();
}

function compositionPath(kind: CompositionKind, id: number): string {
  if (kind === 'ingredient') return `/stock/ingredients/${id}/composition`;
  return `/admin/${kind === 'product' ? 'products' : 'modifier-options'}/${id}/composition`;
}

export const adminApi = {
  users: () => api.get<{ items: AdminUser[] }>('/users'),
  createUser: (b: { name: string; roleId: number; username?: string; pin?: string; password: string; recoveryEmail?: string }) =>
    api.post<AdminUser>('/users', b),
  updateUser: (id: number, b: { name: string; roleId: number; isActive: boolean; recoveryEmail?: string | null }) =>
    api.patch<AdminUser>(`/users/${id}`, b),
  permissionCatalog: () => api.get<{ items: PermissionInfo[] }>('/permissions'),
  roles: () => api.get<{ items: Role[] }>('/roles'),
  createRole: (b: { name: string; permissions: string[] }) => api.post<{ id: number }>('/roles', b),
  // Solo lo que la persona tocó, nunca la lista completa: un permiso que el producto agregó mientras
  // la hoja estaba abierta no se vuelve «quitado».
  updateRole: (id: number, b: { version: number; name?: string; add?: string[]; remove?: string[] }) =>
    api.patch<{ id: number }>(`/roles/${id}`, b),
  resetRole: (id: number, version: number) => api.post<{ id: number }>(`/roles/${id}/reset`, { version }),
  deactivateRole: (id: number, version: number) => api.post<{ id: number }>(`/roles/${id}/deactivate`, { version }),
  reactivateRole: (id: number, version: number) => api.post<{ id: number }>(`/roles/${id}/reactivate`, { version }),
  deleteRole: (id: number) => api.del<void>(`/roles/${id}`),
  resetUserPassword: (id: number, password: string) =>
    api.post<void>(`/users/${id}/password`, { password }),
  setUserPin: (id: number, pin: string) =>
    api.post<void>(`/users/${id}/pin`, { pin }),

  categories: () => api.get<{ items: Category[] }>('/admin/categories'),
  products: (p: ProductsQuery = {}) => api.get<ProductsPage>(`/admin/products?${pageQs(p)}`),
  // cost ausente = sin costo capturado.
  createProduct: (b: { name: string; categoryId: number; price: number; favorite?: boolean; trackStock?: boolean; cost?: number }) =>
    api.post<{ id: number }>('/admin/products', b),
  // Duplica un producto con todas sus relaciones (grupos, canales, receta, slots de combo).
  // cost ausente = la copia hereda el costo del original.
  duplicateProduct: (id: number, name: string, cost?: number) =>
    api.post<{ id: number }>(`/admin/products/${id}/duplicate`, { name, cost }),
  updateProduct: (id: number, b: UpdateProductBody) =>
    api.patch<void>(`/admin/products/${id}`, b),
  // Lista completa de otros nombres (spec 063); va aparte del PATCH para que una pantalla que no la
  // conoce no la borre.
  setProductAltNames: (id: number, altNames: string[]) =>
    api.put<{ altNames: string[] }>(`/admin/products/${id}/alt-names`, { altNames }),
  similarProductAltNames: (id: number, name: string) =>
    api.get<SimilarAlt[]>(`/admin/products/${id}/alt-names/similar?name=${encodeURIComponent(name)}`),

  composition: (kind: CompositionKind, id: number) =>
    api.get<Composition>(`${compositionPath(kind, id)}`),
  saveComposition: (kind: CompositionKind, id: number, b: CompositionBody) =>
    api.put<Composition>(`${compositionPath(kind, id)}`, b),
  confirmComposition: (kind: CompositionKind, id: number) =>
    api.post<Composition>(`${compositionPath(kind, id)}/confirm`),
  recipes: (r: RecipeQuery) => {
    const qs = new URLSearchParams({ kind: r.kind, status: r.status, sort: r.sort, limit: String(r.limit ?? 25), offset: String(r.offset ?? 0) });
    if (r.q) qs.set('q', r.q);
    if (r.category) qs.set('category', String(r.category));
    return api.get<RecipePage>(`/admin/recipes?${qs}`);
  },
  confirmRecipes: (kind: RecipeKind, ids: number[]) =>
    api.post<{ confirmed: number }>('/admin/recipes/confirm', { kind, ids }),

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
  // Las primeras opciones por su orden real: la tarjeta dice QUÉ tiene el grupo, no solo cuántas.
  optionPreview: string;
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
  priceDelta: string;
  maxPerLine: number;
  currentCost: string;
  favorite: boolean;
  active: boolean;
  compositionStatus?: CompositionStatus;
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
  priceDelta: string;
  favorite: boolean;
  active: boolean;
  compositionStatus?: CompositionStatus;
}
export interface OptionsPage {
  items: AdminModifierOption[];
  total: number; // total del filtro (status+search) — para el paginador
  counts: { act: number; inact: number }; // opciones de grupos activos — para las pestañas
}
