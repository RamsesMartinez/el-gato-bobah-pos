import { FudoProduct, FudoCategory } from '../types/fudo';
import { Product } from '../types/sales';

export function adaptFudoProduct(fudoProduct: any): Product {
  return {
    id: fudoProduct.id,
    name: fudoProduct.attributes.name,
    description: fudoProduct.attributes.description || '',
    price: fudoProduct.attributes.price,
    image_url: '', // Por ahora no tenemos imágenes en Fudo
    category: 'Todos los productos', // La categoría se maneja de forma diferente en Fudo
  };
}

export function adaptFudoCategory(fudoCategory: any): string {
  return fudoCategory.attributes.name;
} 