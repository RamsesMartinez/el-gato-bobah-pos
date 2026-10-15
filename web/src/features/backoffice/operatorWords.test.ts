import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import ts from 'typescript';
import { expect, test } from 'vitest';

// FR-019 (spec 038): las pantallas de compras, empaques y conteo hablan como quien opera. Con
// usuarios sin contexto (14-oct) «existencia», «movimiento» y «unidad base» no se entendieron, y
// «presentación» se leyó como exposición. Se revisa todo texto del código —no solo lo que un test
// alcanza a pintar— porque el aviso de un caso raro es justo el que nadie vuelve a leer. Los
// comentarios y los nombres internos sí pueden usarlas.
const PANTALLAS = [
  'features/backoffice/ExpenseDialog.tsx',
  'features/backoffice/ExpenseDetailDialog.tsx',
  'features/backoffice/PendingReceiptsTab.tsx',
  'features/backoffice/ArrivedItemSheet.tsx',
  'features/backoffice/receiptError.ts',
  'features/backoffice/PresentationsSheet.tsx',
  'features/backoffice/StockCountTab.tsx',
  'features/backoffice/CountPlaceField.tsx',
  'features/admin/InventorySettingsSection.tsx',
  'components/NumericKeypad.tsx',
];
const PROHIBIDAS = [/existencia/i, /movimiento/i, /unidad base/i, /presentaci[oó]n/i];

function textosDePantalla(file: string): string[] {
  const src = ts.createSourceFile(file, readFileSync(resolve(__dirname, '../..', file), 'utf8'), ts.ScriptTarget.Latest, true);
  const out: string[] = [];
  const visit = (n: ts.Node) => {
    if (ts.isImportDeclaration(n) || ts.isExportDeclaration(n)) return;
    if (ts.isStringLiteral(n) || ts.isNoSubstitutionTemplateLiteral(n) || ts.isTemplateHead(n) ||
      ts.isTemplateMiddle(n) || ts.isTemplateTail(n) || ts.isJsxText(n)) {
      out.push(n.text);
    }
    ts.forEachChild(n, visit);
  };
  visit(src);
  return out;
}

test.each(PANTALLAS)('%s no usa palabras que quien opera no entendió', (file) => {
  const malas = textosDePantalla(file).filter((t) => PROHIBIDAS.some((p) => p.test(t)));
  expect(malas, `textos con palabras que no se entendieron en la prueba sin contexto`).toEqual([]);
});
