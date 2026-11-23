// El QR del menú (spec 097): se genera en la tableta, sin pedirle nada al servidor. `uqr` se carga
// solo al abrir Venta en web, para no sumarle peso al resto del POS.

/** SVG del QR. Corrección «M»: aguanta un volante doblado o una mesa con una mancha. */
export async function qrSvg(texto: string): Promise<string> {
  const { renderSVG } = await import('uqr');
  return renderSVG(texto, { ecc: 'M', border: 2 });
}

function bajar(blob: Blob, archivo: string) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = archivo;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

export function descargarSvg(svg: string, archivo: string) {
  bajar(new Blob([svg], { type: 'image/svg+xml' }), archivo);
}

/** PNG de 1024 px dibujando el mismo SVG: nítido impreso a media carta. */
export async function descargarPng(svg: string, archivo: string) {
  const img = new Image();
  const url = URL.createObjectURL(new Blob([svg], { type: 'image/svg+xml' }));
  await new Promise<void>((ok, mal) => { img.onload = () => ok(); img.onerror = () => mal(new Error('qr')); img.src = url; });
  const lado = 1024;
  const c = document.createElement('canvas');
  c.width = c.height = lado;
  const g = c.getContext('2d')!;
  g.imageSmoothingEnabled = false;
  g.fillStyle = '#fff';
  g.fillRect(0, 0, lado, lado);
  g.drawImage(img, 0, 0, lado, lado);
  URL.revokeObjectURL(url);
  const png = await new Promise<Blob>((ok) => c.toBlob((b) => ok(b!), 'image/png'));
  bajar(png, archivo);
}

/** La tarjeta para mesas y mostrador: se imprime desde un marco aparte para no heredar la pantalla. */
export function imprimirTarjeta(svg: string, negocio: string, direccion: string) {
  const f = document.createElement('iframe');
  f.style.position = 'fixed';
  f.style.width = '0';
  f.style.height = '0';
  f.style.border = '0';
  document.body.appendChild(f);
  const esc = (s: string) => s.replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c]!);
  const doc = f.contentDocument!;
  doc.open();
  doc.write(`<!doctype html><html lang="es"><head><meta charset="utf-8"><title>${esc(negocio)}</title><style>
@page{size:letter;margin:20mm}body{font-family:system-ui,sans-serif;text-align:center;color:#111}
.t{border:2px solid #111;border-radius:16px;padding:32px;max-width:120mm;margin:0 auto}
h1{font-size:28pt;margin:0 0 8px}p{font-size:14pt;margin:8px 0}.qr svg{width:90mm;height:90mm}.u{font-size:13pt;word-break:break-all}
</style></head><body><div class="t"><h1>${esc(negocio)}</h1><p>Escanea para ver el menú</p><div class="qr">${svg}</div><p class="u">${esc(direccion)}</p></div></body></html>`);
  doc.close();
  setTimeout(() => {
    f.contentWindow?.focus();
    f.contentWindow?.print();
    setTimeout(() => f.remove(), 1000);
  }, 50);
}
