import { AxiosResponse } from 'axios';
import api from './axios';
import { FudoProduct, FudoResponse } from '../../types/fudo';
import productsData from '../../mocks/fudo.products.json';

// Tipo para las respuestas de la API
type ApiResponse<T> = Promise<AxiosResponse<FudoResponse<T>>>;

interface ProductsData {
  data: FudoProduct[];
}

class ProductService {
  private mockDelay = 300; // Simular delay de red

  // Obtener todos los productos
  async getProducts(): Promise<FudoResponse<FudoProduct>> {
    if (process.env.NODE_ENV === 'development') {
      const data = productsData as ProductsData;
      const products = data.data.filter((item): item is FudoProduct => {
        return item.type === 'Product' && item.attributes.active;
      });
      return { data: products };
    }
    throw new Error('Método no implementado para producción');
  }

  // Obtener un producto por ID
  async getProductById(id: string): Promise<FudoResponse<FudoProduct>> {
    if (process.env.NODE_ENV === 'development') {
      const data = productsData as ProductsData;
      const product = data.data.find((item): item is FudoProduct => {
        return item.type === 'Product' && item.id === id;
      });
      return { data: product ? [product] : [] };
    }
    throw new Error('Método no implementado para producción');
  }

  // Obtener productos por categoría
  async getProductsByCategory(categoryId: string): Promise<FudoResponse<FudoProduct>> {
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

export const productService = new ProductService(); 