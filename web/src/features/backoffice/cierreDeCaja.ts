// Qué métodos le faltan al operador por contar antes de poder cerrar el turno.
//
// Nace de un corte real: se cerró con el efectivo declarado en $0 y quedó registrado un faltante
// de $1,662 que no era dinero perdido — nadie capturó el conteo y el campo vacío se guardó como
// cero. Un arqueo con un faltante inventado es peor que no tener arqueo: manda a buscar dinero que
// está en el cajón, y desconfía del sistema para siempre.
//
// Un cero ESCRITO sí es válido: puede no haber efectivo. Lo que no vale es el campo en blanco.

export interface MetodoPorContar {
  methodId: number;
  name: string;
  expected: string;
  autoDeclare: boolean;
}

// faltanPorContar devuelve los métodos que exigen conteo físico, esperan dinero y siguen sin
// capturar. Si devuelve algo, el cierre no debe proceder.
export function faltanPorContar(
  totales: MetodoPorContar[],
  capturado: Record<string, string>,
): MetodoPorContar[] {
  return totales.filter((t) => {
    // Los que se autodeclaran los resuelve el servidor: el cajero no captura nada.
    if (t.autoDeclare) return false;
    // Un método que no esperaba nada no obliga a capturar: no hubo movimiento que contar.
    if (Number(t.expected) === 0) return false;
    const v = capturado[String(t.methodId)];
    return v === undefined || v.trim() === '';
  });
}
