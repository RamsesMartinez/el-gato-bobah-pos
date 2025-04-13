export const ROUTES = {
  SALES: {
    ROOT: '/sales' as const,
    NEW: '/sales/new' as const,
    CATEGORY: '/sales/category/:categoryId' as const,
    PRODUCTS: '/sales/category/:categoryId/products' as const,
    HISTORY: '/sales/history' as const
  }
} as const;

export const generateCategoryRoute = (categoryId: string): string => `/sales/category/${categoryId}`;
export const generateProductsRoute = (categoryId: string): string => `/sales/category/${categoryId}/products`; 