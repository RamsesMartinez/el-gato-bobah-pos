import api from './axios';
import { FudoCategory, FudoResponse } from '../../types/fudo';

// Función adaptadora para transformar los datos de Fudo al formato FudoCategory
function adaptFudoCategory(category: any): FudoCategory {
  return {
    type: "ProductCategory",
    id: category.id,
    attributes: {
      name: category.attributes.name,
      enableOnlineMenu: category.attributes.enableOnlineMenu || null,
      preparationTime: category.attributes.preparationTime || null,
      position: category.attributes.position || 0
    },
    relationships: {
      kitchen: { data: category.relationships.kitchen?.data || null },
      parentCategory: { data: category.relationships.parentCategory?.data || null },
      products: { data: category.relationships.products?.data || [] }
    }
  };
}

export class CategoryService {
  // Obtener todas las categorías con sus productos
  async getCategories(): Promise<FudoResponse<FudoCategory>> {
    const response = await api.get<any>('/product-categories', {
      params: {
        sort: 'id',
        include: 'products'
      }
    });
    console.log('Respuesta completa de categorías:', response.data); // Para debug
    const adaptedCategories: FudoCategory[] = response.data.data.map(adaptFudoCategory);
    return { data: adaptedCategories };
  }

  // Obtener una categoría por ID con sus productos
  async getCategoryById(id: string): Promise<FudoResponse<FudoCategory>> {
    const response = await api.get<any>(`/product-categories/${id}`, {
      params: {
        include: 'products'
      }
    });
    console.log('Respuesta de categoría específica:', response.data); // Para debug
    const adaptedCategory = adaptFudoCategory(response.data.data);
    return { data: [adaptedCategory] };
  }

  // Obtener productos por categoría
  async getProductsByCategory(categoryId: string): Promise<FudoResponse<any>> {
    const response = await api.get<any>(`/product-categories/${categoryId}`, {
      params: {
        include: 'products'
      }
    });
    console.log('Respuesta de productos por categoría:', response.data); // Para debug
    return response.data;
  }
}

export const categoryService = new CategoryService(); 