import { CategoryStyle } from '../types/category';

export class ImageService {
  private readonly colorPalette = [
    // Verde menta muy suave
    { bg: 'linear-gradient(135deg, hsl(150, 25%, 97%) 0%, hsl(150, 25%, 94%) 100%)', color: 'hsl(150, 25%, 30%)' },
    { bg: 'linear-gradient(135deg, hsl(155, 25%, 97%) 0%, hsl(155, 25%, 94%) 100%)', color: 'hsl(155, 25%, 30%)' },
    // Azul pastel claro
    { bg: 'linear-gradient(135deg, hsl(195, 25%, 97%) 0%, hsl(195, 25%, 94%) 100%)', color: 'hsl(195, 25%, 30%)' },
    { bg: 'linear-gradient(135deg, hsl(200, 25%, 97%) 0%, hsl(200, 25%, 94%) 100%)', color: 'hsl(200, 25%, 30%)' },
    // Morado muy suave
    { bg: 'linear-gradient(135deg, hsl(245, 25%, 97%) 0%, hsl(245, 25%, 94%) 100%)', color: 'hsl(245, 25%, 30%)' },
    { bg: 'linear-gradient(135deg, hsl(250, 25%, 97%) 0%, hsl(250, 25%, 94%) 100%)', color: 'hsl(250, 25%, 30%)' },
    // Amarillo cremoso
    { bg: 'linear-gradient(135deg, hsl(45, 25%, 97%) 0%, hsl(45, 25%, 94%) 100%)', color: 'hsl(45, 25%, 30%)' },
    { bg: 'linear-gradient(135deg, hsl(50, 25%, 97%) 0%, hsl(50, 25%, 94%) 100%)', color: 'hsl(50, 25%, 30%)' }
  ];

  private categoryOrder: { [key: string]: number } = {};
  private nextIndex = 0;

  getCategoryStyle(categoryId: string): CategoryStyle {
    // Si la categoría no tiene un índice asignado, asignarle el siguiente disponible
    if (!(categoryId in this.categoryOrder)) {
      this.categoryOrder[categoryId] = this.nextIndex;
      this.nextIndex = (this.nextIndex + 1) % this.colorPalette.length;
    }

    const index = this.categoryOrder[categoryId];
    return {
      background: this.colorPalette[index].bg,
      color: this.colorPalette[index].color
    };
  }
}

export const imageService = new ImageService(); 