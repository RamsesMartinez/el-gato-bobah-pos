import { env } from '../config/env';

const DEFAULT_IMAGE_SIZE = '400x300';
const DEFAULT_BACKGROUND_COLOR = '1B1D21';
const DEFAULT_TEXT_COLOR = 'FFFFFF';
const DEFAULT_FONT_SIZE = '72';

export class ImageService {
  private readonly defaultCategoryImage: string;

  constructor() {
    this.defaultCategoryImage = process.env.REACT_APP_DEFAULT_CATEGORY_IMAGE || '';
  }

  /**
   * Obtiene la URL de la imagen para una categoría
   * @param categoryName Nombre de la categoría
   * @param categoryId ID único de la categoría
   * @returns URL de la imagen
   */
  getCategoryImage(categoryName: string, categoryId: string): string {
    // Si hay una imagen por defecto configurada, usarla
    if (this.defaultCategoryImage) {
      return this.defaultCategoryImage;
    }

    // Generar un color basado en el ID de la categoría para tener consistencia
    const hue = this.generateHue(categoryId);
    const backgroundColor = this.HSLToHex(hue, 70, 60);
    
    // Obtener las iniciales del nombre de la categoría
    const initials = this.getInitials(categoryName);
    
    // Crear URL de placeholder con las iniciales
    return `https://placehold.co/${DEFAULT_IMAGE_SIZE}/${backgroundColor}/${DEFAULT_TEXT_COLOR}?text=${initials}&font=montserrat&size=${DEFAULT_FONT_SIZE}`;
  }

  /**
   * Obtiene las iniciales de un texto
   * @param text Texto del cual obtener las iniciales
   * @returns Iniciales en mayúsculas
   */
  private getInitials(text: string): string {
    // Dividir por espacios y caracteres especiales
    const words = text.split(/[\s&]+/);
    
    // Tomar la primera letra de cada palabra, máximo 3 letras
    const initials = words
      .map(word => word.charAt(0).toUpperCase())
      .slice(0, 3)
      .join('');
    
    return initials;
  }

  /**
   * Genera un valor de matiz (hue) consistente basado en el ID
   */
  private generateHue(id: string): number {
    let hash = 0;
    for (let i = 0; i < id.length; i++) {
      hash = id.charCodeAt(i) + ((hash << 5) - hash);
    }
    return Math.abs(hash % 360);
  }

  /**
   * Convierte color HSL a Hexadecimal
   */
  private HSLToHex(h: number, s: number, l: number): string {
    s /= 100;
    l /= 100;

    const c = (1 - Math.abs(2 * l - 1)) * s;
    const x = c * (1 - Math.abs((h / 60) % 2 - 1));
    const m = l - c/2;
    let r = 0, g = 0, b = 0;

    if (0 <= h && h < 60) {
      r = c; g = x; b = 0;
    } else if (60 <= h && h < 120) {
      r = x; g = c; b = 0;
    } else if (120 <= h && h < 180) {
      r = 0; g = c; b = x;
    } else if (180 <= h && h < 240) {
      r = 0; g = x; b = c;
    } else if (240 <= h && h < 300) {
      r = x; g = 0; b = c;
    } else if (300 <= h && h < 360) {
      r = c; g = 0; b = x;
    }

    const rHex = Math.round((r + m) * 255).toString(16).padStart(2, '0');
    const gHex = Math.round((g + m) * 255).toString(16).padStart(2, '0');
    const bHex = Math.round((b + m) * 255).toString(16).padStart(2, '0');

    return rHex + gHex + bHex;
  }
}

export const imageService = new ImageService(); 