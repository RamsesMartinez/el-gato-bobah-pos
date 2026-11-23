import { ESLint } from 'eslint';
import { expect, it } from 'vitest';

// LA BARRERA ENTRE EL POS Y EL MENÚ PÚBLICO, EN LAS DOS DIRECCIONES (spec 097). Se prueba lintando
// código de verdad: una regla de eslint mal escrita no falla, simplemente deja de aplicar.
const eslint = new ESLint();
const errores = async (code: string, filePath: string) =>
  (await eslint.lintText(code, { filePath }))[0].messages.map((m) => m.message).join('\n');

it('el POS no importa el menú público, ni estático ni dinámico', async () => {
  expect(await errores("import { renderMenu } from '../../menu/plantilla';\nexport const x = renderMenu;\n", 'src/features/x/a.ts')).toContain('menú público');
  expect(await errores("export const x = () => import('../../menu/borde/handler');\n", 'src/features/x/a.ts')).toContain('menú público');
});

it('el menú público no importa el POS ni React', async () => {
  expect(await errores("import { visibleModules } from '../../src/app/nav';\nexport const x = visibleModules;\n", 'menu/plantilla/a.ts')).toContain('menú público');
  expect(await errores("import React from 'react';\nexport const x = React;\n", 'menu/plantilla/a.ts')).toContain('menú público');
  expect(await errores("export const x = () => import('../../src/app/nav');\n", 'menu/borde/a.ts')).toContain('menú público');
});

it('tampoco desde una carpeta que redefine las reglas de sintaxis (pantallas, shared, api…)', async () => {
  expect(await errores("export const x = () => import('../../consola/api');\n", 'src/features/x/a.ts')).toContain('consola');
});
