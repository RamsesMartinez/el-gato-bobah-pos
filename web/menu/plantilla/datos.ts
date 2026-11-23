// Los DATOS del menú: el contrato de GET /api/v1/public/menus/{name} (specs/097-menu-publico/
// contracts/api.md). La plantilla los recibe tal cual; el estilo de marca viaja aparte (marca.ts).

export interface PublicProduct {
  id: number;
  name: string;
  description: string | null;
  price: string;
  imageUrl: string | null;
  soldOut: boolean;
}

export interface PublicSubcategory {
  id: number;
  name: string;
  family: string | null;
  products: PublicProduct[];
}

export interface PublicCategory {
  id: number;
  name: string;
  family: string | null;
  imageUrl: string | null;
  products: PublicProduct[];
  subcategories: PublicSubcategory[];
}

export interface PublicMenu {
  kind: 'menu';
  name: string;
  qrPath: string;
  business: { name: string };
  brand: { palette: string };
  template: string;
  categories: PublicCategory[];
}

export interface PublicMoved {
  kind: 'moved';
  name: string;
}

/** Todos los productos, en el orden en que se pintan (principal y luego sus hijas). */
export function productsInOrder(m: PublicMenu): PublicProduct[] {
  return m.categories.flatMap((c) => [...c.products, ...c.subcategories.flatMap((s) => s.products)]);
}
