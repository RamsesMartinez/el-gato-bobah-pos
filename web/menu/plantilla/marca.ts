// El ESTILO DE MARCA del menú, aparte de los datos y de la plantilla (#330 agregará logo, portada y
// tipografía). Hoy es la paleta que el negocio elige en Ajustes, con los mismos ids que la pantalla
// Interfaz del POS y los mismos tonos de su tema.

export interface Tonos {
  c50: string;
  c100: string;
  c600: string;
  c700: string;
}

export const PALETAS: Record<string, Tonos> = {
  brand: { c50: '#fff1f0', c100: '#ffdad6', c600: '#c62b21', c700: '#9e2018' },
  blue: { c50: '#eff6ff', c100: '#dbeafe', c600: '#2563eb', c700: '#173da6' },
  teal: { c50: '#f0fdfa', c100: '#ccfbf1', c600: '#0d9488', c700: '#0c5d56' },
  green: { c50: '#f0fdf4', c100: '#dcfce7', c600: '#16a34a', c700: '#116932' },
  cyan: { c50: '#ecfeff', c100: '#cffafe', c600: '#0891b2', c700: '#0c5c72' },
  purple: { c50: '#faf5ff', c100: '#f3e8ff', c600: '#9333ea', c700: '#641ba3' },
  pink: { c50: '#fdf2f8', c100: '#fce7f3', c600: '#db2777', c700: '#a41752' },
  orange: { c50: '#fff7ed', c100: '#ffedd5', c600: '#ea580c', c700: '#92310a' },
};

/** La paleta del negocio, o la de la marca del producto si llega una que esta versión no conoce. */
export function tonos(palette: string): Tonos {
  return PALETAS[palette] ?? PALETAS.brand;
}

function luminancia(hex: string): number {
  const [r, g, b] = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255)
    .map((c) => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4));
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

/** Contraste WCAG entre dos colores #rrggbb. */
export function contraste(a: string, b: string): number {
  const [x, y] = [luminancia(a), luminancia(b)].sort((p, q) => q - p);
  return (x + 0.05) / (y + 0.05);
}
