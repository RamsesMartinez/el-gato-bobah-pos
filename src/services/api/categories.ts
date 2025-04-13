import api from './axios';
import { FudoCategory, FudoProduct, FudoResponse } from '../../types/fudo';

// Función adaptadora para transformar los datos de Fudo al formato FudoCategory
function adaptFudoCategory(category: Record<string, any>): FudoCategory {
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

// Función adaptadora para transformar los datos de Fudo al formato FudoProduct
function adaptFudoProduct(product: any): FudoProduct {
  return {
    type: "Product",
    id: product.id,
    attributes: {
      name: product.attributes.name,
      description: product.attributes.description || null,
      price: product.attributes.price || 0,
      imageUrl: product.attributes.imageUrl || null,
      active: product.attributes.active || true,
      code: product.attributes.code || null,
      cost: product.attributes.cost || null,
      enableOnlineMenu: product.attributes.enableOnlineMenu || null,
      enableQrMenu: product.attributes.enableQrMenu || null,
      favourite: product.attributes.favourite || null,
      position: product.attributes.position || 0,
      preparationTime: product.attributes.preparationTime || null,
      sellAlone: product.attributes.sellAlone || true,
      stock: product.attributes.stock || null,
      stockControl: product.attributes.stockControl || false
    },
    relationships: {
      kitchen: { data: product.relationships?.kitchen?.data || null },
      productCategory: { data: product.relationships?.productCategory?.data || null },
      productModifiersGroups: { data: product.relationships?.productModifiersGroups?.data || [] },
      productProportions: { data: product.relationships?.productProportions?.data || [] }
    }
  };
}

export class CategoryService {
  // Obtener solo las categorías padre que tienen productos
  async getCategories(): Promise<FudoResponse<FudoCategory>> {
    const response = await api.get<any>('/product-categories', {
      params: {
        'include': 'products',
        'sort': 'name'
      }
    });
    
    console.log('🔍 DEBUG - Categorías ordenadas por nombre:', {
      url: response.config.url,
      params: response.config.params,
      totalCategories: response.data.data.length
    });

    // Filtrar solo las categorías padre (sin parentCategory) y que tienen productos
    const adaptedCategories: FudoCategory[] = response.data.data
      .map(adaptFudoCategory)
      .filter((category: FudoCategory) => {
        const isParentCategory = !category.relationships.parentCategory.data;
        const hasProducts = category.relationships.products.data.length > 0;
        return isParentCategory && hasProducts;
      });

    console.log('🔍 DEBUG - Categorías filtradas:', {
      totalCategorias: adaptedCategories.length,
      categorias: adaptedCategories.map(cat => ({
        id: cat.id,
        nombre: cat.attributes.name,
        esCategoriaPadre: !cat.relationships.parentCategory.data,
        cantidadProductos: cat.relationships.products.data.length
      }))
    });

    return { data: adaptedCategories };
  }

  // Obtener una categoría por ID con sus productos
  async getCategoryById(id: string): Promise<FudoResponse<FudoCategory>> {
    const response = await api.get<any>(`/product-categories/${id}`, {
      params: {
        include: 'products'
      }
    });
    const adaptedCategory = adaptFudoCategory(response.data.data);
    return { data: [adaptedCategory] };
  }

  // Obtener productos por categoría
  async getProductsByCategory(categoryId: string): Promise<FudoResponse<FudoProduct | FudoCategory>> {
    console.log('🔍 DEBUG - Servicio: Solicitando productos para categoría:', categoryId);
    
    try {
      // Primero obtenemos la información de la categoría
      const categoryResponse = await this.getCategoryById(categoryId);
      const category = categoryResponse.data[0];

      // Luego obtenemos los productos filtrados por categoría
      const response = await api.get<any>('/products', {
        params: {
          'filter[categoryId]': `eq.${categoryId}`,
          'filter[active]': true,
          'sort': 'position'
        }
      });
      
      console.log('🔍 DEBUG - Servicio: Respuesta cruda del API:', {
        data: response.data.data
      });

      if (!response.data.data) {
        throw new Error('No se recibieron datos de productos');
      }
      
      // Adaptar los productos
      const rawProducts: any[] = response.data.data;
      const products = rawProducts
        .map(adaptFudoProduct)
        .filter(product => product.attributes.active);
      
      console.log('🔍 DEBUG - Servicio: Productos adaptados:', products);

      // Devolver tanto la categoría como los productos
      return {
        data: [category, ...products]
      };
    } catch (error) {
      console.error('❌ ERROR - Error en getProductsByCategory:', error);
      throw error;
    }
  }
}

export const categoryService = new CategoryService(); 