import api from './axios';
import { FudoCategory, FudoProduct, FudoResponse } from '../../types/fudo';
import { rateLimiter } from './rateLimit';

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
  private sessionProducts: FudoProduct[] | null = null;
  private categoriesCache: FudoResponse<FudoCategory> | null = null;
  private categoriesLastCacheTime = 0;
  private readonly CATEGORIES_CACHE_DURATION = 300000; // 5 minutos de caché para categorías

  // Método para cargar todos los productos una sola vez por sesión de venta
  private async loadAllProducts(): Promise<FudoProduct[]> {
    try {
      const response = await api.get<any>('/products', {
        params: {
          'sort': 'name'
        }
      });

      if (!response.data.data) {
        return [];
      }

      const products = response.data.data
        .map(adaptFudoProduct)
        // Ordenar localmente por position después de obtener los datos
        .sort((a: FudoProduct, b: FudoProduct) => (a.attributes.position || 0) - (b.attributes.position || 0));
      
      console.log('🔍 DEBUG - Carga inicial de productos:', {
        total: products.length,
        timestamp: new Date().toISOString(),
        ordenamiento: 'Ordenado por position localmente después de sort=name en API'
      });

      return products;
    } catch (error) {
      console.error('❌ ERROR - Error cargando productos:', error);
      throw error; // Propagar el error para mejor manejo
    }
  }

  // Método público para inicializar los productos de la sesión
  async initializeSessionProducts(): Promise<void> {
    try {
      if (!this.sessionProducts) {
        this.sessionProducts = await this.loadAllProducts();
        console.log('🔄 Productos inicializados para la sesión de venta');
      }
    } catch (error) {
      console.error('❌ ERROR - Error inicializando productos:', error);
      this.sessionProducts = null; // Asegurar que sessionProducts esté limpio en caso de error
      throw error; // Propagar el error para manejo en el componente
    }
  }

  // Método público para obtener productos por categoría
  async getProductsByCategory(categoryId: string): Promise<FudoResponse<FudoProduct>> {
    // Si no hay productos en sesión, cargarlos
    if (!this.sessionProducts) {
      await this.initializeSessionProducts();
    }
    
    const filteredProducts = (this.sessionProducts || []).filter(product => 
      product.relationships.productCategory.data?.id === categoryId &&
      product.attributes.active &&
      product.attributes.sellAlone
    );

    console.log('🔍 DEBUG - Productos filtrados por categoría:', {
      categoryId,
      totalProductos: filteredProducts.length,
      productos: filteredProducts.map(p => ({
        id: p.id,
        nombre: p.attributes.name,
        precio: p.attributes.price,
        sellAlone: p.attributes.sellAlone
      }))
    });

    return { data: filteredProducts };
  }

  // Método para limpiar los productos de la sesión
  public clearSessionProducts(): void {
    this.sessionProducts = null;
    console.log('🧹 Productos de sesión limpiados');
  }

  // Método para obtener categorías con cache mejorado
  async getCategories(): Promise<FudoResponse<FudoCategory>> {
    const now = Date.now();
    if (this.categoriesCache && (now - this.categoriesLastCacheTime) < this.CATEGORIES_CACHE_DURATION) {
      return this.categoriesCache;
    }

    return rateLimiter.enqueue(async () => {
      // Asegurarnos de tener los productos cargados
      if (!this.sessionProducts) {
        await this.initializeSessionProducts();
      }

      const response = await api.get<any>('/product-categories', {
        params: {
          'sort': 'name'
        }
      });
      
      const allCategories = response.data.data.map(adaptFudoCategory);
      
      // Filtrar solo categorías padre (sin parentCategory)
      const adaptedCategories: FudoCategory[] = allCategories
        .filter((category: FudoCategory) => {
          // Verificar que sea una categoría padre (sin parentCategory)
          const isParentCategory = !category.relationships.parentCategory.data;
          
          if (!isParentCategory) {
            return false; // Si tiene categoría padre, no la mostramos en la vista principal
          }

          // Verificar si la categoría tiene productos activos y vendibles
          const hasActiveProducts = (this.sessionProducts || []).some(product => 
            product.relationships.productCategory.data?.id === category.id &&
            product.attributes.active &&
            product.attributes.sellAlone
          );

          // Verificar si tiene subcategorías con productos
          const hasSubcategoriesWithProducts = allCategories.some((subCat: FudoCategory) => {
            const isSubcategory = subCat.relationships.parentCategory.data?.id === category.id;
            if (!isSubcategory) return false;

            // Verificar si la subcategoría tiene productos activos
            return (this.sessionProducts || []).some(product => 
              product.relationships.productCategory.data?.id === subCat.id &&
              product.attributes.active &&
              product.attributes.sellAlone
            );
          });
          
          // Mostrar la categoría si tiene productos propios o subcategorías con productos
          return hasActiveProducts || hasSubcategoriesWithProducts;
        });

      console.log('🔍 DEBUG - Categorías filtradas:', {
        totalOriginal: allCategories.length,
        totalFiltradas: adaptedCategories.length,
        categorias: adaptedCategories.map(cat => ({
          id: cat.id,
          nombre: cat.attributes.name,
          productosActivos: (this.sessionProducts || []).filter(p => 
            p.relationships.productCategory.data?.id === cat.id &&
            p.attributes.active &&
            p.attributes.sellAlone
          ).length,
          subcategoriasConProductos: allCategories
            .filter((subCat: FudoCategory) => subCat.relationships.parentCategory.data?.id === cat.id)
            .filter((subCat: FudoCategory) => (this.sessionProducts || []).some(p => 
              p.relationships.productCategory.data?.id === subCat.id &&
              p.attributes.active &&
              p.attributes.sellAlone
            )).length
        }))
      });

      this.categoriesCache = { data: adaptedCategories };
      this.categoriesLastCacheTime = now;

      return this.categoriesCache;
    });
  }

  // Los demás métodos permanecen igual, pero usando el cache mejorado
  async getCategoryById(id: string): Promise<FudoResponse<FudoCategory>> {
    const allCategories = await this.getAllCategories();
    const category = allCategories.data.find(cat => cat.id === id);
    return { data: category ? [category] : [] };
  }

  async getAllCategories(): Promise<FudoResponse<FudoCategory>> {
    const now = Date.now();
    if (this.categoriesCache && (now - this.categoriesLastCacheTime) < this.CATEGORIES_CACHE_DURATION) {
      return this.categoriesCache;
    }

    return rateLimiter.enqueue(async () => {
      const response = await api.get<any>('/product-categories', {
        params: {
          'include': 'parentCategory',
          'sort': 'name'
        }
      });
      
      const adaptedCategories: FudoCategory[] = response.data.data
        .map(adaptFudoCategory);

      this.categoriesCache = { data: adaptedCategories };
      this.categoriesLastCacheTime = now;

      return this.categoriesCache;
    });
  }

  async getSubCategories(parentId: string): Promise<FudoResponse<FudoCategory>> {
    const allCategories = await this.getAllCategories();
    const subCategories = allCategories.data.filter(category => 
      category.relationships.parentCategory.data?.id === parentId
    );
    
    return { data: subCategories };
  }
}

export const categoryService = new CategoryService(); 