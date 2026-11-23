import { describe, expect, it } from 'vitest';
import type { PublicMenu, PublicProduct } from './datos';
import { renderMenu } from './index';
import { precio } from './formato';

const MEDIA = 'https://media.test/2';
let next = 100;
const prod = (over: Partial<PublicProduct> = {}): PublicProduct => ({
  id: next++, name: 'Producto', description: null, price: '45', imageUrl: null, soldOut: false, ...over,
});

function menu(over: Partial<PublicMenu> = {}): PublicMenu {
  return {
    kind: 'menu', name: 'fonda-lupe', qrPath: '/q/0123456789ab', business: { name: 'Fonda Doña Lupe' },
    brand: { palette: 'green' }, template: 'classic',
    categories: [
      {
        id: 1, name: 'Café', family: 'coffee', imageUrl: null,
        products: [
          prod({ name: 'Americano', imageUrl: `${MEDIA}/products/1/a-1200.webp` }),
          prod({ name: 'De olla', description: 'Con piloncillo y canela.' }),
        ],
        subcategories: [{ id: 11, name: 'Fríos', family: 'cold_drink', products: [prod({ name: 'Cold brew', price: '59.5' })] }],
      },
      {
        id: 2, name: 'Postres', family: null, imageUrl: `${MEDIA}/categories/2/b-1200.webp`,
        products: [
          prod({ name: 'Flan', soldOut: true, imageUrl: `${MEDIA}/products/2/c-1200.webp` }),
          prod({ imageUrl: `${MEDIA}/products/3/d-1200.webp` }),
          prod({ imageUrl: `${MEDIA}/products/4/e-1200.webp` }),
          prod({ imageUrl: `${MEDIA}/products/5/f-1200.webp` }),
          prod({ imageUrl: `${MEDIA}/products/6/g-1200.webp` }),
        ],
        subcategories: [],
      },
    ],
    ...over,
  };
}

const OPTS = { canonicalUrl: 'https://menu.test/fonda-lupe', ogDefaultImage: 'https://menu.test/og-default.png' };

describe('plantilla classic (A + mosaico)', () => {
  it('no lleva ni una línea de JavaScript', () => {
    expect(renderMenu(menu(), OPTS)).not.toMatch(/<script/i);
  });

  it('escapa todo lo que escribe el negocio', () => {
    const m = menu({ business: { name: 'Lupe <script>alert(1)</script>' } });
    m.categories[0].products[0].name = '"><img src=x onerror=alert(1)>';
    m.categories[0].products[1].description = '</details><script>robo()</script>';
    const html = renderMenu(m, OPTS);
    expect(html).not.toMatch(/<script/i);
    expect(html).not.toContain('<img src=x');
    expect(html).toContain('&lt;script&gt;');
  });

  it('trae la vista previa para compartir en el HTML inicial', () => {
    const html = renderMenu(menu(), OPTS);
    expect(html).toContain('<meta property="og:title" content="Fonda Doña Lupe">');
    expect(html).toContain(`<meta property="og:image" content="${MEDIA}/products/1/a-400.webp">`);
    expect(html).toContain('<meta property="og:url" content="https://menu.test/fonda-lupe">');
    expect(html).toContain('<link rel="canonical" href="https://menu.test/fonda-lupe">');
    expect(html).toMatch(/<meta property="og:description" content="Menú de Fonda Doña Lupe: 8 productos/);
  });

  it('sin ninguna foto, la vista previa usa la imagen por omisión', () => {
    const m = menu();
    for (const c of m.categories) for (const p of c.products) p.imageUrl = null;
    expect(renderMenu(m, OPTS)).toContain('<meta property="og:image" content="https://menu.test/og-default.png">');
  });

  it('marca el agotado sin decir cuánto queda', () => {
    const html = renderMenu(menu(), OPTS);
    expect(html).toContain('Agotado');
    expect(html).not.toMatch(/quedan|existencia|\d+ piezas/i);
  });

  it('dibuja la ilustración de la familia; sin familia, la genérica', () => {
    const html = renderMenu(menu(), OPTS);
    expect(html).toContain('<use href="#coffee"/>');
    expect(html).toContain('<use href="#cold_drink"/>');
    const sinFamilia = menu();
    sinFamilia.categories[1].products.push(prod({ name: 'Sin foto' }));
    expect(renderMenu(sinFamilia, OPTS)).toContain('<use href="#dish"/>');
  });

  it('declara el tamaño de toda imagen y solo las primeras 4 van con prioridad', () => {
    const html = renderMenu(menu(), OPTS);
    const imgs = html.match(/<img [^>]*>/g) ?? [];
    expect(imgs.length).toBeGreaterThan(5);
    for (const i of imgs) expect(i).toMatch(/width="\d+" height="\d+"/);
    const prio = imgs.filter((i) => i.includes('fetchpriority="high"'));
    expect(prio).toHaveLength(4);
    for (const i of imgs.filter((x) => !x.includes('fetchpriority'))) expect(i).toContain('loading="lazy"');
  });

  it('la barra y el mosaico llevan a cada categoría principal', () => {
    const html = renderMenu(menu(), OPTS);
    expect(html).toContain('href="#categorias"');
    expect(html).toContain('id="categorias"');
    for (const id of [1, 2]) {
      expect(html).toContain(`id="c${id}"`);
      expect(html.match(new RegExp(`href="#c${id}"`, 'g'))!.length).toBeGreaterThanOrEqual(3);
    }
    expect(html).toContain('<h3 id="s11"');
  });

  it('un menú sin productos lo dice, sin barra ni mosaico', () => {
    const html = renderMenu(menu({ categories: [] }), OPTS);
    expect(html).toContain('Este menú todavía no tiene productos');
    expect(html).not.toContain('id="categorias"');
  });

  it('no difiere el pintado de las secciones: el salto a una categoría caería en otra', () => {
    // Con content-visibility:auto, una sección sin pintar mide lo que dice contain-intrinsic-size y
    // el salto desde el mosaico calculaba mal: tocar «Snacks» dejaba al cliente en «Ramen» (e2e).
    expect(renderMenu(menu(), OPTS)).not.toContain('content-visibility');
  });

  it('usa el color del negocio', () => {
    expect(renderMenu(menu(), OPTS)).toContain('--m700:#116932');
  });
});

describe('precio', () => {
  it.each([['45', '$45'], ['59.5', '$59.50'], ['1250', '$1,250'], ['0.5', '$0.50']])('%s → %s', (p, s) => {
    expect(precio(p)).toBe(s);
  });
});
