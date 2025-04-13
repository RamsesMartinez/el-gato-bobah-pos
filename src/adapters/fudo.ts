import { FudoProduct, FudoCategory } from '../types/fudo';
import { Product } from '../types/sales';

export function adaptFudoProduct(fudoProduct: FudoProduct): Product {
  return {
    id: fudoProduct.id,
    name: fudoProduct.attributes.name,
    description: fudoProduct.attributes.description || '',
    price: fudoProduct.attributes.price,
    image_url: fudoProduct.attributes.imageUrl || '',
    category: fudoProduct.relationships.productCategory.data?.id || 'default',
    active: fudoProduct.attributes.active,
    sellAlone: fudoProduct.attributes.sellAlone,
  };
}

export function adaptFudoCategory(fudoCategory: FudoCategory): FudoCategory {
  return {
    type: fudoCategory.type,
    id: fudoCategory.id,
    attributes: {
      enableOnlineMenu: fudoCategory.attributes.enableOnlineMenu,
      name: fudoCategory.attributes.name,
      preparationTime: fudoCategory.attributes.preparationTime,
      position: fudoCategory.attributes.position,
    },
    relationships: {
      kitchen: fudoCategory.relationships.kitchen,
      parentCategory: fudoCategory.relationships.parentCategory,
      products: fudoCategory.relationships.products,
    },
  };
} 