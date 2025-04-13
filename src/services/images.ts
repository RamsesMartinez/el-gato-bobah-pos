import { CategoryStyle } from '../types/category';

export class ImageService {
  getCategoryStyle(categoryId: string): CategoryStyle {
    // Convertir el ID en un número más continuo
    const hash = categoryId.split('').reduce((acc, char, index) => {
      return acc + (char.charCodeAt(0) * (index + 1));
    }, 0);

    // Generar un tono base (hue) entre 0 y 360
    const hue = hash % 360;
    
    // Mantener la saturación y luminosidad constantes para coherencia
    const saturation = 85;
    const lightness = 95;
    const textLightness = 25;

    return {
      background: `linear-gradient(135deg, 
        hsl(${hue}, ${saturation - 25}%, ${lightness}%) 0%,
        hsl(${hue}, ${saturation - 15}%, ${lightness - 5}%) 100%
      )`,
      color: `hsl(${hue}, ${saturation}%, ${textLightness}%)`
    };
  }
}

export const imageService = new ImageService(); 