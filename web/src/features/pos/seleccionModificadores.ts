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
