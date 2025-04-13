// Configuración de colores que puede ser fácilmente ajustada
export enum ColorConfig {
  // Puntos de inicio disponibles en el círculo cromático
  GREEN_START = 120,
  BLUE_START = 180,
  PURPLE_START = 240,
  RED_START = 0,
  YELLOW_START = 60
}

export enum ColorRange {
  FULL = 360,        // Círculo cromático completo
  HALF = 180,        // Media vuelta
  QUARTER = 90,      // Cuarto de vuelta
  EXTENDED = 260     // Rango extendido (actual)
}

interface ColorSettings {
  startHue: number;
  hueRange: number;
  saturation: number;
  lightness: number;
}

interface CategoryStyle {
  background: string;
  color: string;
}

// Función para obtener un color inicial aleatorio
const getRandomStartHue = (): number => {
  const colorValues = Object.values(ColorConfig).filter(value => typeof value === 'number') as number[];
  const randomIndex = Math.floor(Math.random() * colorValues.length);
  return colorValues[randomIndex];
};

// Configuración por defecto con color inicial aleatorio
const DEFAULT_COLOR_SETTINGS: ColorSettings = {
  startHue: getRandomStartHue(),
  hueRange: ColorRange.EXTENDED,
  saturation: 70,
  lightness: 91,
};

export class ImageService {
  private colorSettings: ColorSettings;

  constructor(settings: Partial<ColorSettings> = {}) {
    // Si no se proporciona un startHue, usar uno aleatorio
    if (!settings.startHue) {
      settings.startHue = getRandomStartHue();
    }
    this.colorSettings = { ...DEFAULT_COLOR_SETTINGS, ...settings };
  }

  getCategoryStyle(index: number, totalCategories: number): CategoryStyle {
    // Calculamos el paso basado en el rango total y el número de categorías
    const step = this.colorSettings.hueRange / totalCategories;
    const baseHue = this.colorSettings.startHue + (index * step);
    
    return {
      background: `linear-gradient(135deg, 
        hsl(${baseHue}, ${this.colorSettings.saturation}%, ${this.colorSettings.lightness}%) 0%, 
        hsl(${baseHue}, ${this.colorSettings.saturation + 8}%, ${this.colorSettings.lightness - 5}%) 100%
      )`,
      color: `hsl(${baseHue}, ${this.colorSettings.saturation + 20}%, 30%)`
    };
  }

  // Método para actualizar la configuración en tiempo de ejecución
  updateSettings(newSettings: Partial<ColorSettings>) {
    this.colorSettings = { ...this.colorSettings, ...newSettings };
  }

  // Método para cambiar aleatoriamente el color inicial
  randomizeStartColor() {
    this.colorSettings.startHue = getRandomStartHue();
  }
}

export const imageService = new ImageService(); 