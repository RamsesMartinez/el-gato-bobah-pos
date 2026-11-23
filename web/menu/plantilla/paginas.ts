// Las páginas que no son un menú: la portada del dominio, «no existe» y «no se pudo cargar». Texto
// para quien abrió un enlace, sin nada técnico, y sin decir qué negocios existen.
import { esc } from './html';

const CSS = `:root{--tx:#18181b;--mu:#52525b;--bg:#fff;color-scheme:light}
@media (prefers-color-scheme:dark){:root{--tx:#fafafa;--mu:#a1a1aa;--bg:#111113;color-scheme:dark}}
body{margin:0;font:18px/1.5 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;color:var(--tx);background:var(--bg)}
main{max-width:560px;margin:0 auto;padding:64px 16px}h1{font-size:26px;margin:0 0 12px}p{color:var(--mu);margin:0}`;

export function paginaSimple(titulo: string, texto: string): string {
  return `<!doctype html><html lang="es"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="robots" content="noindex"><title>${esc(titulo)}</title><style>${CSS}</style></head>
<body><main><h1>${esc(titulo)}</h1><p>${esc(texto)}</p></main></body></html>`;
}
