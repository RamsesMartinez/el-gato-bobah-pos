import { readFileSync } from 'node:fs';
import { expect, it } from 'vitest';
import { normalizaNombre } from './normaliza';

it('el borde normaliza igual que el servidor', () => {
  const casos: [string, string][] = JSON.parse(readFileSync('../server/internal/domain/testdata/menu_slug_cases.json', 'utf8'));
  for (const [entra, sale] of casos) expect(normalizaNombre(entra), entra).toBe(sale);
});
