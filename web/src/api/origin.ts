// Desde qué aparato trabaja el POS (spec 033). La clave la genera el propio navegador la primera
// vez y la manda en cada request; la API la guarda en la bitácora junto con la IP y una descripción
// corta. Es una pista para auditar, no una identificación: se renueva si se borran los datos del
// navegador.

const KEY = 'egb:device:v1';
const SHAPE = /^[0-9a-f]{32}$/;

let key: string | null = null;
let info: Record<string, string> | null = null;

function newKey(): string {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
}

// deviceKey devuelve la clave de este navegador. Leer o escribir el almacenamiento puede tronar
// (ventana privada, datos bloqueados): entonces la clave vive en memoria y el request sale igual.
export function deviceKey(): string {
  if (key) return key;
  let stored: string | null = null;
  try { stored = localStorage.getItem(KEY); } catch { /* sin almacenamiento */ }
  key = stored && SHAPE.test(stored) ? stored : newKey();
  if (key !== stored) {
    try { localStorage.setItem(KEY, key); } catch { /* sin almacenamiento */ }
  }
  return key;
}

interface UAData {
  platform?: string;
  brands?: { brand: string; version: string }[];
  getHighEntropyValues?: (hints: string[]) => Promise<{ platformVersion?: string; model?: string }>;
}

function describe(): Record<string, string> {
  if (info) return info;
  const ua = (navigator as Navigator & { userAgentData?: UAData }).userAgentData;
  const brand = ua?.brands?.find((b) => !/not.?a.?brand|chromium/i.test(b.brand));
  info = {
    platform: ua?.platform ?? navigator.platform ?? '',
    browser: brand ? `${brand.brand} ${brand.version}` : navigator.userAgent.slice(0, 120),
    screen: `${screen.width}x${screen.height}`,
  };
  // El modelo y la versión del sistema llegan después (es asíncrono): los primeros requests salen
  // sin ellos, y está bien.
  ua?.getHighEntropyValues?.(['platformVersion', 'model'])
    .then((v) => {
      if (info) info = { ...info, ...(v.platformVersion ? { platformVersion: v.platformVersion } : {}), ...(v.model ? { model: v.model } : {}) };
    })
    .catch(() => { /* el navegador no lo da */ });
  return info;
}

// deviceHeaders son los dos headers que acompañan cada request a la API.
export function deviceHeaders(): { 'X-Device-Key': string; 'X-Device-Info': string } {
  return { 'X-Device-Key': deviceKey(), 'X-Device-Info': JSON.stringify(describe()).slice(0, 300) };
}

export function resetDeviceOriginForTests(): void {
  key = null;
  info = null;
}
