// uuidv5 deriva un uuid ESTABLE de un nombre dentro de un espacio (RFC 4122 §4.3, SHA-1).
//
// Se escribe a mano y no con `crypto.subtle` porque éste solo existe en contextos seguros, y la
// tableta puede entrar por http en la red local. Lo usa la subida de las cuentas de la versión
// anterior: el mismo renglón tiene que mandar la misma llave en cada reintento.

function sha1(bytes: Uint8Array): Uint8Array {
  const ml = bytes.length * 8;
  const conRelleno = new Uint8Array(((bytes.length + 9 + 63) >> 6) << 6);
  conRelleno.set(bytes);
  conRelleno[bytes.length] = 0x80;
  const dv = new DataView(conRelleno.buffer);
  dv.setUint32(conRelleno.length - 4, ml >>> 0);
  dv.setUint32(conRelleno.length - 8, Math.floor(ml / 2 ** 32));
  let [h0, h1, h2, h3, h4] = [0x67452301, 0xefcdab89, 0x98badcfe, 0x10325476, 0xc3d2e1f0];
  const w = new Uint32Array(80);
  const rotl = (x: number, n: number) => (x << n) | (x >>> (32 - n));
  for (let off = 0; off < conRelleno.length; off += 64) {
    for (let i = 0; i < 16; i++) w[i] = dv.getUint32(off + i * 4);
    for (let i = 16; i < 80; i++) w[i] = rotl(w[i - 3] ^ w[i - 8] ^ w[i - 14] ^ w[i - 16], 1);
    let [a, b, c, d, e] = [h0, h1, h2, h3, h4];
    for (let i = 0; i < 80; i++) {
      const [f, k] = i < 20 ? [(b & c) | (~b & d), 0x5a827999]
        : i < 40 ? [b ^ c ^ d, 0x6ed9eba1]
          : i < 60 ? [(b & c) | (b & d) | (c & d), 0x8f1bbcdc]
            : [b ^ c ^ d, 0xca62c1d6];
      const t = (rotl(a, 5) + f + e + k + w[i]) >>> 0;
      e = d; d = c; c = rotl(b, 30) >>> 0; b = a; a = t;
    }
    h0 = (h0 + a) >>> 0; h1 = (h1 + b) >>> 0; h2 = (h2 + c) >>> 0; h3 = (h3 + d) >>> 0; h4 = (h4 + e) >>> 0;
  }
  const out = new Uint8Array(20);
  const ov = new DataView(out.buffer);
  [h0, h1, h2, h3, h4].forEach((h, i) => ov.setUint32(i * 4, h));
  return out;
}

export function uuidv5(nombre: string, espacio: string): string {
  const hex = espacio.replace(/-/g, '');
  const ns = new Uint8Array(16);
  for (let i = 0; i < 16; i++) ns[i] = parseInt(hex.slice(i * 2, i * 2 + 2), 16);
  const n = new TextEncoder().encode(nombre);
  const todo = new Uint8Array(16 + n.length);
  todo.set(ns);
  todo.set(n, 16);
  const h = sha1(todo).slice(0, 16);
  h[6] = (h[6] & 0x0f) | 0x50;
  h[8] = (h[8] & 0x3f) | 0x80;
  const s = Array.from(h, (b) => b.toString(16).padStart(2, '0')).join('');
  return `${s.slice(0, 8)}-${s.slice(8, 12)}-${s.slice(12, 16)}-${s.slice(16, 20)}-${s.slice(20)}`;
}
