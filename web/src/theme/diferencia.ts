// Los colores de una diferencia de caja (#290, FR-004): rojo anaranjado para el faltante y verde
// azulado para el sobrante, que se distinguen con las formas comunes de daltonismo, y a 7:1 en los
// dos temas (lo mide diferencia.test.ts). Solo marcan diferencias: lo seleccionado y la acción
// principal siguen con la marca, como en el resto del POS.
export const FALTANTE = { light: '#7c2d12', dark: '#fdba74' } as const;
export const SOBRANTE = { light: '#134e4a', dark: '#5eead4' } as const;
