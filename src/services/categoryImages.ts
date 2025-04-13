import { CategoryImage, CategoryImageResponse } from '../types/api';

const mockImages: Record<string, CategoryImage> = {
  '1': {
    id: 'img_1',
    categoryId: '1',
    imageUrl: 'https://images.unsplash.com/photo-1628191139360-4083564d03fd?auto=format&fit=crop&w=600&q=80',
    isActive: true,
    createdAt: new Date().toISOString(),
    updatedAt: new Date().toISOString(),
  },
  '2': {
    id: 'img_2',
    categoryId: '2',
    imageUrl: 'https://images.unsplash.com/photo-1570275239925-4af0aa89f4bc?auto=format&fit=crop&w=600&q=80',
    isActive: true,
    createdAt: new Date().toISOString(),
    updatedAt: new Date().toISOString(),
  },
  '3': {
    id: 'img_3',
    categoryId: '3',
    imageUrl: 'https://images.unsplash.com/photo-1488477181946-6428a0291777?auto=format&fit=crop&w=600&q=80',
    isActive: true,
    createdAt: new Date().toISOString(),
    updatedAt: new Date().toISOString(),
  },
  '4': {
    id: 'img_4',
    categoryId: '4',
    imageUrl: 'https://images.unsplash.com/photo-1568901346375-23c9450c58cd?auto=format&fit=crop&w=600&q=80',
    isActive: true,
    createdAt: new Date().toISOString(),
    updatedAt: new Date().toISOString(),
  },
};

export const categoryImagesService = {
  // Simula obtener la imagen de una categoría
  getCategoryImage: async (categoryId: string): Promise<CategoryImageResponse> => {
    // Simula un delay de red
    await new Promise(resolve => setTimeout(resolve, 300));

    const image = mockImages[categoryId];
    if (!image) {
      return {
        data: {
          id: `img_default_${categoryId}`,
          categoryId,
          imageUrl: 'https://via.placeholder.com/300x200/EDEDED/999999?text=Categoria',
          isActive: true,
          createdAt: new Date().toISOString(),
          updatedAt: new Date().toISOString(),
        },
        success: true
      };
    }

    return {
      data: image,
      success: true
    };
  },

  // Simula actualizar la imagen de una categoría
  updateCategoryImage: async (categoryId: string, imageUrl: string): Promise<CategoryImageResponse> => {
    // Simula un delay de red
    await new Promise(resolve => setTimeout(resolve, 500));

    const newImage: CategoryImage = {
      id: mockImages[categoryId]?.id || `img_${categoryId}`,
      categoryId,
      imageUrl,
      isActive: true,
      createdAt: mockImages[categoryId]?.createdAt || new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    };

    mockImages[categoryId] = newImage;

    return {
      data: newImage,
      success: true
    };
  }
}; 