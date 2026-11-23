// Familias de comida (spec 091): deciden la ilustración de lo que no tiene foto. La lista es la de
// domain.FoodFamilies y la del enum food_family de la 0117; el dibujo de cada una es un <symbol> del
// sprite, que se descarga una sola vez para toda la pantalla.

export type FoodFamily =
  | 'cold_drink' | 'coffee' | 'tea' | 'frappe' | 'soda' | 'soup' | 'snack' | 'wings' | 'dessert' | 'breakfast' | 'dish';

export const FOOD_FAMILIES: { id: FoodFamily; label: string }[] = [
  { id: 'cold_drink', label: 'Bebida fría' },
  { id: 'coffee', label: 'Café' },
  { id: 'tea', label: 'Té o bubble tea' },
  { id: 'frappe', label: 'Frappé o malteada' },
  { id: 'soda', label: 'Soda' },
  { id: 'soup', label: 'Ramen o sopa' },
  { id: 'snack', label: 'Snack' },
  { id: 'wings', label: 'Alitas o boneless' },
  { id: 'dessert', label: 'Crepa o postre' },
  { id: 'breakfast', label: 'Desayuno' },
  { id: 'dish', label: 'Platillo' },
];

// Versionado en el nombre: public/ no lleva hash, y un dibujo nuevo tiene que llegar aunque el
// navegador guarde el viejo.
export const FAMILY_SPRITE = '/ilustraciones/food-families.v1.svg';

const known = new Set<string>(FOOD_FAMILIES.map((f) => f.id));

/** El símbolo a dibujar: una familia vacía o que esta versión no conoce cae en la genérica. */
export function familySymbol(family: string | null | undefined): FoodFamily {
  return family && known.has(family) ? (family as FoodFamily) : 'dish';
}

export function familyLabel(family: string | null | undefined): string {
  if (!family || !known.has(family)) return 'Platillo (genérica)';
  return FOOD_FAMILIES.find((f) => f.id === family)!.label;
}

/** La URL guardada es la de 1200; la de 400 se llama igual con -400 (spec 091). */
export const smallOf = (url: string) => url.replace(/-1200\.webp$/, '-400.webp');
