import { AxiosResponse } from 'axios';
import api from './axios';
import { FudoProduct, FudoResponse } from '../../types/fudo';

// Tipo para las respuestas de la API
type ApiResponse<T> = Promise<AxiosResponse<FudoResponse<T>>>;

class ProductService {
  // Obtener todos los productos
  async getProducts(): Promise<FudoResponse<FudoProduct>> {
    const response = await api.get<FudoResponse<FudoProduct>>('/products');
    return response.data;
  }

  // Obtener un producto por ID
  async getProductById(id: string): Promise<FudoResponse<FudoProduct>> {
    const response = await api.get<FudoResponse<FudoProduct>>(`/products/${id}`);
    return response.data;
  }

}

export const productService = new ProductService(); 