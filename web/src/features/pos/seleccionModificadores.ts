import type { MenuOption } from '../../types/pos';

// Cuántas veces se eligió cada opción de UN grupo. La cantidad ya viajaba hasta la base y el ticket
// impreso; lo único que faltaba era una forma de subirla desde la pantalla.
export type Picks = Record<number, number>;

export function cantidadDe(picks: Picks, optionId: number): number {
  return picks[optionId] ?? 0;
}

function elegidasEnElGrupo(picks: Picks): number {
  return Object.values(picks).reduce((a, b) => a + b, 0);
}

// cabeOtra decide si mostrar el "+" que repite una opción ya elegida.
//
// Pide TRES cosas y la conjunción es deliberada:
//
//   - que la opción ya esté elegida — sobre una que no lo está, el toque normal ya la agrega;
//   - que la opción admita otra (`maxPerLine`), que es la columna que el negocio ya configura y
//     que hoy vale 2 en las 64 salsas y 1 en 818 opciones donde repetir no tiene sentido;
//   - que al GRUPO le quede cupo.
//
// Lo tercero es lo que hace que el "+" solo sume. Sin ello habría que quitarle una a otra opción
// para hacer espacio, y un control que quita algo que el operador eligió a propósito no puede
// esconderse detrás de un "+".
export function cabeOtra(picks: Picks, option: MenuOption, maxDelGrupo: number): boolean {
  const n = cantidadDe(picks, option.id);
  if (n === 0) return false;
  if (n >= option.maxPerLine) return false;
  return elegidasEnElGrupo(picks) < maxDelGrupo;
}

// sumarUna devuelve los picks con una unidad más de esa opción. No valida: quien llama ya preguntó
// con cabeOtra, y duplicar la regla aquí la dejaría desincronizada con lo que la pantalla muestra.
export function sumarUna(picks: Picks, optionId: number): Picks {
  return { ...picks, [optionId]: cantidadDe(picks, optionId) + 1 };
}

// alTocarUnaSola resuelve el toque sobre una opción de un grupo de UNA SOLA (max = 1).
//
// Tocar la que ya está elegida la QUITA, pero solo donde el grupo admite quedarse vacío. El caso
// que lo pidió es el aderezo de cortesía: marcado por error, no había forma de desmarcarlo y la
// línea se iba a cocina con algo que el cliente no pidió — el toque repetido volvía a elegir lo
// mismo.
//
// En un grupo OBLIGATORIO el toque repetido no hace nada, a propósito: vaciarlo no lleva a ningún
// lado —hay que elegir algo de todos modos y cambiar de opción ya cuesta un solo toque—, así que
// lo único que produciría es una línea inválida que el operador tiene que deshacer. Además la hoja
// nunca abre un grupo obligatorio vacío: siempre lo pre-marca.
export function alTocarUnaSola(picks: Picks, optionId: number, minDelGrupo: number): Picks {
  if (minDelGrupo === 0 && cantidadDe(picks, optionId) > 0) return {};
  return { [optionId]: 1 };
}
