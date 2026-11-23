import { describe, expect, it, vi } from 'vitest';
import { cropFor, encodeUnder, preparePhoto, PhotoError, type PhotoDeps } from './preparePhoto';

const blob = (bytes: number, type = 'image/webp') => new Blob([new Uint8Array(bytes)], { type });

describe('cropFor', () => {
  it('sin ajuste toma el cuadrado más grande, centrado', () => {
    expect(cropFor(4000, 3000)).toEqual({ x: 500, y: 0, size: 3000 });
    expect(cropFor(3000, 4000)).toEqual({ x: 0, y: 500, size: 3000 });
  });
  it('acercar achica el cuadrado alrededor del punto elegido', () => {
    expect(cropFor(4000, 3000, { zoom: 2, cx: 0.5, cy: 0.5 })).toEqual({ x: 1250, y: 750, size: 1500 });
  });
  it('mover el encuadre nunca saca el cuadrado de la foto', () => {
    // Arrastrar hasta la orilla deja el cuadrado pegado a ella, no medio afuera con una franja negra.
    expect(cropFor(4000, 3000, { zoom: 1, cx: 0, cy: 0 })).toEqual({ x: 0, y: 0, size: 3000 });
    expect(cropFor(4000, 3000, { zoom: 2, cx: 1, cy: 1 })).toEqual({ x: 2500, y: 1500, size: 1500 });
  });
  it('un acercamiento menor que 1 se trata como 1', () => {
    expect(cropFor(4000, 3000, { zoom: 0.2, cx: 0.5, cy: 0.5 })).toEqual({ x: 500, y: 0, size: 3000 });
  });
});

describe('encodeUnder', () => {
  it('baja la calidad hasta que cabe', async () => {
    const tried: number[] = [];
    const out = await encodeUnder((q) => { tried.push(q); return Promise.resolve(blob(q > 0.7 ? 500_000 : 300_000)); }, 400 * 1024);
    expect(out.size).toBe(300_000);
    expect(tried[0]).toBeGreaterThan(0.8);
  });
  it('si ni con la calidad más baja cabe, lo dice en vez de mandar algo que el servidor rechazará', async () => {
    await expect(encodeUnder(() => Promise.resolve(blob(999_999)), 100 * 1024)).rejects.toMatchObject({ reason: 'too_heavy' });
  });
  it('un navegador que entrega PNG en vez de WebP se detecta (Safari)', async () => {
    await expect(encodeUnder(() => Promise.resolve(blob(10, 'image/png')), 100 * 1024)).rejects.toMatchObject({ reason: 'no_webp' });
  });
});

describe('preparePhoto', () => {
  const deps = (w: number, h: number, fail = false, close = vi.fn()): PhotoDeps => ({
    decode: () => (fail ? Promise.reject(new Error('x')) : Promise.resolve({ width: w, height: h, close })),
    render: (_src, _crop, side) => (q: number) => Promise.resolve(blob(side === 1200 ? (q > 0.8 ? 500_000 : 200_000) : 50_000)),
  });

  it('entrega la de 1200 y la de 400, ambas WebP dentro del tope, y libera la foto leída', async () => {
    const close = vi.fn();
    const out = await preparePhoto(blob(10, 'image/jpeg'), undefined, deps(4000, 3000, false, close));
    expect(close).toHaveBeenCalledTimes(1);
    expect(out.large.size).toBeLessThanOrEqual(400 * 1024);
    expect(out.small.size).toBeLessThanOrEqual(100 * 1024);
    expect(out.large.type).toBe('image/webp');
  });
  it('una foto de menos de 400 px se rechaza (en el menú se vería borrosa) y también se libera', async () => {
    const close = vi.fn();
    await expect(preparePhoto(blob(10), undefined, deps(300, 600, false, close))).rejects.toMatchObject({ reason: 'too_small' });
    expect(close).toHaveBeenCalledTimes(1);
  });
  it('un archivo que el navegador no abre (HEIC, un PDF) se rechaza con su propio motivo', async () => {
    const err = await preparePhoto(blob(10), undefined, deps(0, 0, true)).catch((e) => e);
    expect(err).toBeInstanceOf(PhotoError);
    expect(err.reason).toBe('cannot_open');
    expect(err.message).toMatch(/JPG o PNG/);
  });
});
