interface CategoryStyle {
  background: string;
  color: string;
}

export class ImageService {
  private readonly baseHues = [
    160, // Verde menta
    190, // Azul claro
    220, // Azul suave
    250, // Morado suave
    40,  // Amarillo cremoso
  ];

  getCategoryStyle(index: number): CategoryStyle {
    // Usar el índice para seleccionar un tono base y añadir una pequeña variación
    const baseHue = this.baseHues[index % this.baseHues.length];
    const hueVariation = Math.floor(index / this.baseHues.length) * 5;
    const hue = (baseHue + hueVariation) % 360;
    
    return {
      background: `linear-gradient(135deg, 
        hsl(${hue}, 20%, 97%) 0%, 
        hsl(${hue}, 25%, 94%) 100%
      )`,
      color: `hsl(${hue}, 35%, 35%)`
    };
  }
}

export const imageService = new ImageService(); 