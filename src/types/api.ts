export interface CategoryImage {
  id: string;
  categoryId: string;
  imageUrl: string;
  isActive: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface CategoryImageResponse {
  data: CategoryImage;
  success: boolean;
} 