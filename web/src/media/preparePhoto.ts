// Prepara la foto de un producto o una categoría en el dispositivo de quien la sube (spec 091): el
// recorte cuadrado y dos WebP, de 1200 y de 400, cada uno bajo su tope. El servidor es chico y no
// convierte imágenes: solo valida lo que llega. Los topes son los de domain/media.go.

export const LARGE_SIDE = 1200;
export const SMALL_SIDE = 400;
export const MAX_LARGE_BYTES = 400 * 1024;
export const MAX_SMALL_BYTES = 100 * 1024;

export type PhotoErrorReason = 'cannot_open' | 'too_small' | 'no_webp' | 'too_heavy';

const MESSAGES: Record<PhotoErrorReason, string> = {
  cannot_open: 'Esta foto no se puede abrir. Elige una en JPG o PNG.',
  too_small: `Elige una foto de al menos ${SMALL_SIDE} px por lado.`,
  no_webp: 'Este navegador no puede preparar la foto. Ábrelo en Chrome o Edge.',
  too_heavy: 'La foto tiene demasiado detalle para guardarse. Prueba con otra.',
};

export class PhotoError extends Error {
  constructor(public readonly reason: PhotoErrorReason) {
    super(MESSAGES[reason]);
  }
}

export interface Crop { x: number; y: number; size: number }

/** Encuadre elegido: acercamiento (1 = el cuadrado más grande) y centro, de 0 a 1 en cada eje. */
export interface Framing { zoom: number; cx: number; cy: number }

const clamp = (v: number, lo: number, hi: number) => Math.min(Math.max(v, lo), hi);

/** El cuadrado de la foto original que se guarda. Nunca se sale de la foto. */
export function cropFor(width: number, height: number, f: Framing = { zoom: 1, cx: 0.5, cy: 0.5 }): Crop {
  const size = Math.round(Math.min(width, height) / Math.max(f.zoom, 1));
  const x = Math.round(clamp(f.cx * width - size / 2, 0, width - size));
  const y = Math.round(clamp(f.cy * height - size / 2, 0, height - size));
  return { x, y, size };
}

const QUALITIES = [0.86, 0.78, 0.7, 0.62, 0.54, 0.46];

/** Codifica bajando la calidad hasta que cabe en max bytes. */
export async function encodeUnder(encode: (quality: number) => Promise<Blob>, max: number): Promise<Blob> {
  for (const q of QUALITIES) {
    const b = await encode(q);
    // Safari no codifica WebP en canvas y en silencio entrega PNG: mandarlo sería un rechazo seguro.
    if (b.type !== 'image/webp') throw new PhotoError('no_webp');
    if (b.size <= max) return b;
  }
  throw new PhotoError('too_heavy');
}

interface Decoded { width: number; height: number; close: () => void }

export interface PhotoDeps {
  decode: (file: Blob) => Promise<Decoded>;
  render: (src: Decoded, crop: Crop, side: number) => (quality: number) => Promise<Blob>;
}

const browserDeps: PhotoDeps = {
  // La orientación guardada en la foto se aplica al leerla: una foto de celular sale derecha.
  decode: (file) => createImageBitmap(file, { imageOrientation: 'from-image' }),
  // Se dibuja UNA vez por medida y solo se repite la codificación: un canvas de 1200² son ~5.7 MB, y
  // uno por intento de calidad se juntaba hasta seis esperando al recolector.
  render: (src, crop, side) => {
    const canvas = document.createElement('canvas');
    canvas.width = side;
    canvas.height = side;
    const ctx = canvas.getContext('2d')!;
    ctx.imageSmoothingQuality = 'high';
    ctx.drawImage(src as ImageBitmap, crop.x, crop.y, crop.size, crop.size, 0, 0, side, side);
    return (quality) => new Promise<Blob>((resolve, reject) =>
      canvas.toBlob((b) => (b ? resolve(b) : reject(new PhotoError('no_webp'))), 'image/webp', quality));
  },
};

/** La foto ya derecha, leída para el recorte. Quien la recibe la cierra (close) al terminar. */
export type OpenedPhoto = Decoded;

/** Abre la foto para la vista previa del recorte: se dibuja en un canvas, sin URL de por medio. */
export async function openPhoto(file: Blob, deps: PhotoDeps = browserDeps): Promise<OpenedPhoto> {
  const img = await deps.decode(file).catch(() => { throw new PhotoError('cannot_open'); });
  if (Math.min(img.width, img.height) < SMALL_SIDE) {
    img.close();
    throw new PhotoError('too_small');
  }
  return img;
}

/** Las dos variantes listas para subir. */
export async function preparePhoto(file: Blob, framing?: Framing, deps: PhotoDeps = browserDeps): Promise<{ large: Blob; small: Blob }> {
  const img = await deps.decode(file).catch(() => { throw new PhotoError('cannot_open'); });
  try {
    if (Math.min(img.width, img.height) < SMALL_SIDE) throw new PhotoError('too_small');
    const crop = cropFor(img.width, img.height, framing);
    const large = await encodeUnder(deps.render(img, crop, LARGE_SIDE), MAX_LARGE_BYTES);
    const small = await encodeUnder(deps.render(img, crop, SMALL_SIDE), MAX_SMALL_BYTES);
    return { large, small };
  } finally {
    img.close();
  }
}

let webpSupport: boolean | undefined;

/** Si este navegador puede preparar fotos. Se pregunta al montar el control, no después del recorte. */
export function canPreparePhotos(): boolean {
  if (webpSupport === undefined) {
    try {
      const c = document.createElement('canvas');
      c.width = c.height = 1;
      webpSupport = typeof createImageBitmap === 'function' && c.toDataURL('image/webp').startsWith('data:image/webp');
    } catch {
      webpSupport = false;
    }
  }
  return webpSupport;
}

/** Base64 estándar de un archivo, para el cuerpo JSON de la subida. */
export async function toBase64(b: Blob): Promise<string> {
  const bytes = new Uint8Array(await b.arrayBuffer());
  let s = '';
  for (let i = 0; i < bytes.length; i += 0x8000) s += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  return btoa(s);
}
