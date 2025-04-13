import { env } from '../config/env';

interface CategoryStyle {
  background: string;
  color: string;
}

export class ImageService {
  private readonly defaultCategoryImage: string;
  private styleCache: Map<string, CategoryStyle>;

  constructor() {
    this.defaultCategoryImage = process.env.REACT_APP_DEFAULT_CATEGORY_IMAGE || '';
    this.styleCache = new Map();
  }

  getCategoryStyle(categoryId: string): CategoryStyle {
    // Usar caché para evitar recálculos
    const cacheKey = categoryId;
    if (this.styleCache.has(cacheKey)) {
      return this.styleCache.get(cacheKey)!;
    }

    const hue = this.generateHue(categoryId);
    const saturation = 65 + (Math.abs(this.generateHash(categoryId)) % 20);
    
    // Generar un estilo más distintivo y profesional
    const style = {
      background: `linear-gradient(135deg, 
        hsl(${hue}, ${saturation}%, 97%) 0%, 
        hsl(${hue}, ${saturation + 10}%, 92%) 50%,
        hsl(${hue}, ${saturation + 5}%, 88%) 100%
      )`,
      color: `hsl(${hue}, ${saturation + 15}%, 35%)`
    };

    this.styleCache.set(cacheKey, style);
    return style;
  }

  private generateHue(id: string): number {
    return Math.abs(this.generateHash(id) % 360);
  }

  private generateHash(str: string): number {
    let hash = 0;
    for (let i = 0; i < str.length; i++) {
      hash = str.charCodeAt(i) + ((hash << 5) - hash);
    }
    return hash;
  }
}

export const imageService = new ImageService(); 