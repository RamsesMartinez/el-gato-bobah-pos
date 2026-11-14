import { FALTANTE, SOBRANTE } from './diferencia';

// Contraste WCAG entre dos colores #rrggbb.
function luminancia(hex: string): number {
  const c = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255)
    .map((v) => (v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4));
  return 0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2];
}
function contraste(a: string, b: string): number {
  const [x, y] = [luminancia(a), luminancia(b)].sort((p, q) => q - p);
  return (x + 0.05) / (y + 0.05);
}

// Los fondos donde se pinta una diferencia: la página y los paneles, en cada tema (Chakra v3).
const CLAROS = ['#ffffff', '#fafafa', '#f4f4f5'];
const OSCUROS = ['#000000', '#111111', '#18181b', '#27272a'];

// Un faltante o un sobrante se lee a 7:1 (#290, FR-004): es la cifra por la que alguien recuenta el
// cajón o deja pasar un corte, y en la tableta del mostrador hay reflejo.
test.each([['faltante', FALTANTE], ['sobrante', SOBRANTE]])('%s contrasta 7:1 en los dos temas', (_, color) => {
  for (const fondo of CLAROS) expect(contraste(color.light, fondo)).toBeGreaterThanOrEqual(7);
  for (const fondo of OSCUROS) expect(contraste(color.dark, fondo)).toBeGreaterThanOrEqual(7);
});
