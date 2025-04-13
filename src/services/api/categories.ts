import { FudoCategory } from '../../types/fudo';
import { Product } from '../../types/sales';
import categoriesData from '../../mocks/fudo.categories.json';
import productsData from '../../mocks/fudo.products.json';

interface CategoryProductsResponse {
  data: Array<{
    id: string;
    attributes: {
      name: string;
      description: string | null;
      price: number;
      imageUrl: string | null;
      active: boolean;
      sellAlone: boolean;
    };
    relationships: {
      productCategory: {
        data: {
          id: string;
        } | null;
      };
    };
  }>;
}

export const categoryService = {
  getCategories: async (): Promise<{ data: FudoCategory[] }> => {
    // Simulando una llamada a la API
    return new Promise((resolve) => {
      setTimeout(() => {
        resolve({ data: categoriesData.data as FudoCategory[] });
      }, 500);
    });
  },

  getCategoryProducts: async (categoryId: string): Promise<CategoryProductsResponse> => {
    // Simulando una llamada a la API
    return new Promise((resolve) => {
      setTimeout(() => {
        const categoryProducts = productsData.data.filter(
          (product: any) => product.relationships.productCategory.data?.id === categoryId
        );
        resolve({ data: categoryProducts });
      }, 500);
    });
  },
}; 