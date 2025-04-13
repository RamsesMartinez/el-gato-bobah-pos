import { AxiosResponse } from 'axios';
import api from './axios';
import { FudoCategory, FudoResponse, FudoProduct } from '../../types/fudo';
import categoriesData from '../../mocks/fudo.categories.json';
import productsData from '../../mocks/fudo.products.json';

// Tipo para las respuestas de la API
type ApiResponse<T> = Promise<AxiosResponse<FudoResponse<T>>>;

interface ProductsData {
  data: (FudoProduct | FudoCategory)[];
}

export class CategoryService {
  private mockDelay = 300; // Simular delay de red

  // Obtener todas las categorías principales (sin parent)
  async getCategories(): Promise<FudoResponse<FudoCategory>> {
    if (process.env.NODE_ENV === 'development') {
      const data = productsData as ProductsData;
      const mainCategories = data.data.filter((item): item is FudoCategory => {
        return item.type === 'ProductCategory' && 
               (!item.relationships.parentCategory.data);
      });
      return { data: mainCategories };
    }
    throw new Error('Método no implementado para producción');
  }

  // Obtener una categoría por ID
  async getCategoryById(id: string): Promise<FudoResponse<FudoCategory>> {
    if (process.env.NODE_ENV === 'development') {
      const data = productsData as ProductsData;
      const category = data.data.find((item): item is FudoCategory => {
        return item.type === 'ProductCategory' && item.id === id;
      });
      return { data: category ? [category] : [] };
    }
    throw new Error('Método no implementado para producción');
  }

  // Obtener productos de una categoría
  async getCategoryProducts(categoryId: string): Promise<FudoResponse<FudoProduct>> {
    if (process.env.NODE_ENV === 'development') {
      const data = productsData as ProductsData;
      const products = data.data.filter((item): item is FudoProduct => {
        return item.type === 'Product' && 
               item.relationships.productCategory.data?.id === categoryId &&
               item.attributes.active;
      });
      return { data: products };
    }
    throw new Error('Método no implementado para producción');
  }
}

export const categoryService = new CategoryService(); 