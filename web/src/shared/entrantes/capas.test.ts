import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { CAPA_BLOQUEO, CAPA_FRANJA, CAPA_HOJAS, CAPA_TOASTER } from './capas';

describe('capas', () => {
  // Si alguien baja la franja, un pedido llega con la tableta bloqueada o con una hoja abierta y la
  // alarma suena sin nada a la vista que diga por qué.
  it('la franja va encima de hojas, avisos y bloqueo', () => {
    expect(CAPA_HOJAS).toBeLessThan(CAPA_TOASTER);
    expect(CAPA_TOASTER).toBeLessThan(CAPA_BLOQUEO);
    expect(CAPA_BLOQUEO).toBeLessThan(CAPA_FRANJA);
  });

  // Las constantes solo sirven si dicen lo que de verdad usan esos componentes.
  it('el bloqueo por PIN usa la capa que aquí se declara', () => {
    const lock = readFileSync('src/features/auth/LockScreen.tsx', 'utf8');
    const capas = [...lock.matchAll(/zIndex=\{(\d+)\}/g)].map((m) => Number(m[1]));
    expect(capas.length).toBeGreaterThan(0);
    for (const c of capas) expect(c).toBeLessThanOrEqual(CAPA_BLOQUEO);
  });
});
