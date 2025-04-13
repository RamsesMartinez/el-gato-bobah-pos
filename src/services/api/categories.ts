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
  // Obtener todas las categorías
  async getCategories(): Promise<FudoResponse<FudoCategory>> {
    const response = await api.get<any>('/categories');
    const adaptedCategories: FudoCategory[] = response.data.data.map(adaptFudoCategory);
    return { data: adaptedCategories };
  }

  // Obtener una categoría por ID
  async getCategoryById(id: string): Promise<FudoResponse<FudoCategory>> {
    const response = await api.get<any>(`/categories/${id}`);
    const adaptedCategory = adaptFudoCategory(response.data.data);
    return { data: [adaptedCategory] };
  }

  // Obtener productos por categoría
  async getProductsByCategory(categoryId: string): Promise<FudoResponse<any>> {
    const response = await api.get<any>(`/categories/${categoryId}/products`);
    return response.data;
  }
}

export const categoryService = new CategoryService(); 