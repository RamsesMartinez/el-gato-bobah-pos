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
    try {
      // Primero obtenemos la información de la categoría y todas las categorías
      const [categoryResponse, allCategoriesResponse] = await Promise.all([
        this.getCategoryById(categoryId),
        this.getCategories()
      ]);
      
      const category = categoryResponse.data[0];
      const allCategories = allCategoriesResponse.data;

      // Encontrar subcategorías (categorías que tienen como padre la categoría seleccionada)
      const subCategories = allCategories.filter((cat: FudoCategory) => 
        cat.relationships.parentCategory.data?.id === categoryId
      );
      
      console.log('🌳 JERARQUÍA DE CATEGORÍAS 🌳\n', JSON.stringify({
        categoríaPadre: {
          id: category.id,
          nombre: category.attributes.name,
          cantidadProductos: category.relationships.products.data.length
        },
        subcategorías: subCategories.map((sub: FudoCategory) => ({
          id: sub.id,
          nombre: sub.attributes.name,
          cantidadProductos: sub.relationships.products.data.length
        }))
      }, null, 2));

      // Luego obtenemos los productos filtrados por categoría
      const response = await api.get<any>('/products', {
        params: {
          'filter[categoryId]': `eq.${categoryId}`,
          'filter[active]': true,
          'sort': 'position'
        }
      });
      
      if (!response.data.data) {
        throw new Error('No se recibieron datos de productos');
      }
      
      // Adaptar los productos
      const rawProducts: any[] = response.data.data;
      const products = rawProducts
        .map(adaptFudoProduct)
        .filter(product => product.attributes.active);
      
      // Devolver tanto la categoría como los productos
      return {
        data: [category, ...products]
      };
    } catch (error) {
      console.error('❌ ERROR - Error en getProductsByCategory:', error);
      throw error;
    }
  }

  // Nuevo método para obtener todas las categorías (incluyendo subcategorías)
  async getAllCategories(): Promise<FudoResponse<FudoCategory>> {
    const response = await api.get<any>('/product-categories', {
      params: {
        'include': 'products,parentCategory',
        'sort': 'name'
      }
    });
    
    const adaptedCategories: FudoCategory[] = response.data.data
      .map(adaptFudoCategory);

    return { data: adaptedCategories };
  }

  // Nuevo método para obtener subcategorías de una categoría específica
  async getSubCategories(parentId: string): Promise<FudoResponse<FudoCategory>> {
    const response = await this.getAllCategories();
    const subCategories = response.data.filter(category => 
      category.relationships.parentCategory.data?.id === parentId
    );
    
    return { data: subCategories };
  }
}

export const categoryService = new CategoryService(); 