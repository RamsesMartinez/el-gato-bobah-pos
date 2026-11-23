// archivedDescription dice en cuántos productos activos deja de ofrecerse un grupo al archivarlo (spec
// 090): antes el aviso solo decía «Grupo archivado» y nadie sabía qué productos se quedaban sin él.
export function archivedDescription(activeProducts: number): string | undefined {
  if (activeProducts <= 0) return undefined;
  return `Ya no se ofrece en ${activeProducts} ${activeProducts === 1 ? 'producto' : 'productos'}`;
}
