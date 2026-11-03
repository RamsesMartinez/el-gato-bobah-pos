// Cómo se pinta un cambio en una vista previa «antes / después» (specs 053 y 056).

export const num = (s: string | null | undefined) => Number(s ?? 0);

// signed pinta un cambio con su signo; cero es «sin cambio».
export const signed = (v: number, fmt: (abs: number) => string) => (v > 0 ? `+${fmt(v)}` : v < 0 ? `−${fmt(-v)}` : 'sin cambio');

// tone: lo que sube en verde, lo que baja en rojo, lo que no cambia apagado.
export const tone = (v: number) => (v > 0 ? 'fg.success' : v < 0 ? 'fg.error' : 'fg.muted');
