import { readFileSync } from 'node:fs';
import { slugDelMenu } from './slug';

// La misma tabla que usan Go y la base (server/internal/domain/testdata/menu_slug_cases.json): la
// vista previa «Quedará: …» tiene que ser exactamente lo que el servidor guarda.
test('la pantalla normaliza el nombre igual que el servidor', () => {
  const casos: [string, string][] = JSON.parse(readFileSync('../server/internal/domain/testdata/menu_slug_cases.json', 'utf8'));
  for (const [entra, sale] of casos) expect(slugDelMenu(entra), entra).toBe(sale);
});
