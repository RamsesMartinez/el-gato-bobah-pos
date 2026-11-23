// Plantilla + datos + estilo de marca (spec 097): renderMenu elige la plantilla por su clave y le pasa
// los datos y la marca por separado. El #330 agrega plantillas aquí sin tocar el resto.
import type { PublicMenu } from './datos';
import { tonos } from './marca';
import { renderClassic, type OpcionesDePagina } from './classic';

const PLANTILLAS: Record<string, typeof renderClassic> = { classic: renderClassic };

export function renderMenu(m: PublicMenu, o: OpcionesDePagina): string {
  const plantilla = PLANTILLAS[m.template] ?? renderClassic;
  return plantilla(m, tonos(m.brand.palette), o);
}

export type { OpcionesDePagina };
export { paginaSimple } from './paginas';
