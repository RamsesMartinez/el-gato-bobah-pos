// La PLANTILLA «classic» (spec 097, maqueta elegida «A + mosaico»): lista con la foto a la derecha,
// barra de categorías fija (columna en computadora) y un mosaico de categorías que se abre sin
// JavaScript (:target). Recibe los datos y el estilo de marca por separado; una plantilla nueva
// (#330) es otro archivo con la misma firma.
import type { PublicCategory, PublicMenu, PublicProduct } from './datos';
import { productsInOrder } from './datos';
import { chica, precio } from './formato';
import { esc } from './html';
import type { Tonos } from './marca';
import { SIMBOLOS, familia } from './sprite';

function origenDe(u: string): string | null {
  try {
    return new URL(u).origin;
  } catch {
    return null;
  }
}

export interface OpcionesDePagina {
  /** La dirección vigente del menú: canonical y og:url. */
  canonicalUrl: string;
  /** La imagen de vista previa cuando ningún producto tiene foto. */
  ogDefaultImage: string;
}

/** Las primeras fotos de la página se piden sin esperar: son las candidatas a LCP. */
const CON_PRIORIDAD = 4;

export function renderClassic(m: PublicMenu, t: Tonos, o: OpcionesDePagina): string {
  const todos = productsInOrder(m);
  const total = todos.length;
  const conFoto = todos.find((p) => p.imageUrl);
  const ogImage = conFoto?.imageUrl ? chica(conFoto.imageUrl) : o.ogDefaultImage;
  const negocio = esc(m.business.name);
  const cuantos = total === 1 ? '1 producto' : `${total} productos`;
  const descripcion = total > 0 ? `Menú de ${negocio}: ${cuantos}. Precios y fotos.` : `Menú de ${negocio}.`;

  let n = 0;
  const imagen = (p: PublicProduct, fam: string | null, tam: number) => {
    if (!p.imageUrl) return `<span class="ilu" aria-hidden="true"><svg><use href="#${familia(fam)}"/></svg></span>`;
    const prioridad = n++ < CON_PRIORIDAD ? 'fetchpriority="high"' : 'loading="lazy" decoding="async"';
    return `<img src="${esc(chica(p.imageUrl))}" width="${tam}" height="${tam}" alt="" ${prioridad}>`;
  };
  const renglon = (p: PublicProduct, fam: string | null) => {
    const grande = p.imageUrl ? `<img src="${esc(p.imageUrl)}" width="600" height="600" alt="" loading="lazy" decoding="async">` : '';
    const ds = p.description ? `<span class="ds">${esc(p.description)}</span>` : '';
    const ago = p.soldOut ? ' <span class="et">Agotado</span>' : '';
    const det = grande || p.description ? `<div class="det">${grande}${p.description ? `<p>${esc(p.description)}</p>` : ''}</div>` : '';
    return `<li><details class="ren${p.soldOut ? ' ago' : ''}"><summary><span class="tx"><b>${esc(p.name)}</b>${ds}<span class="pr">${precio(p.price)}${ago}</span></span>${imagen(p, fam, 96)}</summary>${det}</details></li>`;
  };
  const seccion = (c: PublicCategory) => {
    const propios = c.products.length ? `<ul class="lis">${c.products.map((p) => renglon(p, c.family)).join('')}</ul>` : '';
    const hijas = c.subcategories.map((s) =>
      `<h3 id="s${s.id}">${esc(s.name)}</h3><ul class="lis">${s.products.map((p) => renglon(p, s.family)).join('')}</ul>`).join('');
    return `<section id="c${c.id}" aria-labelledby="h${c.id}"><h2 id="h${c.id}">${esc(c.name)}</h2>${propios}${hijas}</section>`;
  };
  const mosaico = (c: PublicCategory) => {
    const img = c.imageUrl
      ? `<img src="${esc(chica(c.imageUrl))}" width="160" height="160" alt="" loading="lazy" decoding="async">`
      : `<span class="ilu" aria-hidden="true"><svg><use href="#${familia(c.family)}"/></svg></span>`;
    return `<li><a href="#c${c.id}">${img}<span>${esc(c.name)}</span></a></li>`;
  };
  const enlaces = m.categories.map((c) => `<li><a href="#c${c.id}">${esc(c.name)}</a></li>`).join('');
  const botonMosaico = `<li><a href="#categorias" class="mosb"><svg aria-hidden="true" viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/></svg>Categorías</a></li>`;

  const cuerpo = total === 0
    ? `<main class="vacio"><p>Este menú todavía no tiene productos.</p></main>`
    : `<nav class="chips" aria-label="Categorías"><ul>${botonMosaico}${enlaces}</ul></nav>
<div id="categorias" class="hoja" role="dialog" aria-label="Categorías"><a href="#cerrar" class="fondo" aria-hidden="true" tabindex="-1"></a><div class="hoja-in"><div class="hoja-cab"><b>Categorías</b><a href="#cerrar" class="cerrar">Cerrar</a></div><ul class="portada">${m.categories.map(mosaico).join('')}</ul></div></div>
<main class="con-lado"><nav class="lado" aria-label="Categorías"><ul>${botonMosaico}${enlaces}</ul></nav><div class="cuerpo">${m.categories.map(seccion).join('')}</div></main>`;

  return `<!doctype html><html lang="es"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>${negocio} · Menú</title>
<meta name="description" content="${descripcion}">
<link rel="canonical" href="${esc(o.canonicalUrl)}">
<meta property="og:type" content="website">
<meta property="og:title" content="${negocio}">
<meta property="og:description" content="${descripcion}">
<meta property="og:image" content="${esc(ogImage)}">
<meta property="og:url" content="${esc(o.canonicalUrl)}">
<meta name="twitter:card" content="summary">
<meta name="theme-color" content="${t.c700}">
${conFoto?.imageUrl && origenDe(conFoto.imageUrl) ? `<link rel="preconnect" href="${esc(origenDe(conFoto.imageUrl)!)}">` : ''}
<style>${css(t)}</style></head><body>
<svg xmlns="http://www.w3.org/2000/svg" style="display:none">${SIMBOLOS}</svg>
<header class="cab"><h1>${negocio}</h1><p>Menú${total ? ` · ${cuantos}` : ''}</p></header>
${cuerpo}
<footer class="pie">Precios en pesos mexicanos. Lo marcado como «Agotado» no está disponible por ahora.</footer>
</body></html>`;
}

function css(t: Tonos): string {
  return `:root{--m50:${t.c50};--m100:${t.c100};--m600:${t.c600};--m700:${t.c700};--tx:#18181b;--mu:#52525b;--bo:#e4e4e7;--bg:#fff;--bg2:#fafafa;--ilu:${t.c600};color-scheme:light}
@media (prefers-color-scheme:dark){:root{--tx:#fafafa;--mu:#a1a1aa;--bo:#3f3f46;--bg:#111113;--bg2:#18181b;--m50:#1f1f23;--m100:#2a2a30;--ilu:${t.c100};color-scheme:dark}}
*{box-sizing:border-box}html{scroll-behavior:smooth;scroll-padding-top:64px}body{margin:0;font:16px/1.4 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;color:var(--tx);background:var(--bg)}
@media (prefers-reduced-motion:reduce){html{scroll-behavior:auto}}
ul{list-style:none;margin:0;padding:0}a{color:inherit}a:focus-visible,summary:focus-visible{outline:3px solid var(--m600);outline-offset:2px}
.cab{background:var(--m700);color:#fff;padding:20px 16px 16px}.cab h1{margin:0;font-size:26px;line-height:1.2}.cab p{margin:4px 0 0;font-size:14px}
.chips{position:sticky;top:0;z-index:2;background:var(--bg);border-bottom:1px solid var(--bo)}
.chips ul{display:flex;gap:8px;overflow-x:auto;padding:8px 16px;scrollbar-width:none}
.chips a{display:flex;align-items:center;gap:8px;min-height:44px;padding:0 16px;border-radius:999px;background:var(--bg2);border:1px solid var(--bo);text-decoration:none;white-space:nowrap;font-weight:600;font-size:15px}
.chips a:active{background:var(--m100)}
.mosb{background:var(--m700)!important;color:#fff!important;border-color:var(--m700)!important}
main{max-width:1200px;margin:0 auto;padding:0 16px}

h2{font-size:20px;margin:24px 0 8px;color:var(--m700)}h3{font-size:16px;margin:20px 0 4px;color:var(--mu)}
@media (prefers-color-scheme:dark){h2{color:var(--tx)}a:focus-visible,summary:focus-visible{outline-color:var(--tx)}}
summary{list-style:none;cursor:pointer}summary::-webkit-details-marker{display:none}
.ilu{display:flex;align-items:center;justify-content:center;background:var(--m50);color:var(--ilu);border-radius:12px;flex-shrink:0;aspect-ratio:1}
.ilu svg{width:45%;height:auto;aspect-ratio:1}
img{border-radius:12px;background:var(--m50);display:block;max-width:100%;height:auto;object-fit:cover}
.pr{font-weight:700;color:var(--m700);margin-top:4px}
@media (prefers-color-scheme:dark){.pr{color:var(--tx)}}
.et{display:inline-block;font-size:12px;font-weight:700;padding:2px 8px;border-radius:999px;background:var(--bo);color:var(--tx);margin-left:6px;vertical-align:1px}
.ago img,.ago .ilu{opacity:.45}
.det{padding:8px 0 16px}.det img{width:100%;max-width:480px}.det p{margin:8px 0 0;color:var(--mu)}
.lis{display:grid;grid-template-columns:1fr}
.ren summary{display:flex;gap:12px;align-items:flex-start;padding:12px 0;border-bottom:1px solid var(--bo);min-height:120px}
.ren .tx{flex:1;display:flex;flex-direction:column;min-width:0}
.ren .ds{color:var(--mu);font-size:14px;display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden;margin-top:2px}
.ren img,.ren .ilu{width:96px;height:96px}
.ren[open] .ds{display:none}
.hoja{display:none;position:fixed;inset:0;z-index:5;background:rgba(0,0,0,.5);overscroll-behavior:contain}.hoja:target{display:block}
html:has(.hoja:target){overflow:hidden}
.hoja-in{position:relative;background:var(--bg);max-height:100dvh;overflow:auto;overscroll-behavior:contain}
.fondo{position:absolute;inset:0}
.hoja-cab{position:sticky;top:0;background:var(--bg);display:flex;justify-content:space-between;align-items:center;padding:8px 16px;border-bottom:1px solid var(--bo);font-size:18px}
.cerrar{display:flex;align-items:center;min-height:44px;padding:0 16px;border-radius:999px;border:1px solid var(--bo);text-decoration:none;font-weight:600;font-size:15px}
.portada{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:10px;padding:16px}
.portada a{display:flex;flex-direction:column;align-items:center;gap:6px;text-decoration:none;font-weight:600;font-size:14px;text-align:center;min-height:44px}
.portada img,.portada .ilu{width:100%;aspect-ratio:1;height:auto}
.lado{display:none}
.vacio{padding:48px 16px;color:var(--mu);font-size:18px}
.pie{max-width:1200px;margin:32px auto;padding:16px;color:var(--mu);font-size:14px;border-top:1px solid var(--bo)}
@media (min-width:700px){.lis{grid-template-columns:repeat(2,minmax(0,1fr));column-gap:32px}.portada{grid-template-columns:repeat(6,minmax(0,1fr))}.cab{padding:28px max(16px,calc(50% - 584px)) 20px}}
@media (min-width:1024px){.chips{display:none}.hoja-in{max-width:900px;margin:40px auto;border-radius:16px;max-height:calc(100dvh - 80px)}.con-lado{display:grid;grid-template-columns:220px 1fr;gap:32px}.lado{display:block;position:sticky;top:0;align-self:start;max-height:100dvh;overflow:auto;padding:16px 0}.lado a{display:flex;align-items:center;gap:8px;min-height:44px;padding:0 12px;border-radius:10px;text-decoration:none;font-weight:600}.lado a:hover{background:var(--m50)}.lado .mosb{margin-bottom:8px}}`;
}
