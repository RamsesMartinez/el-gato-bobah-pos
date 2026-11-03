// alReconectar llama `fn` cada vez que el EventSource vuelve a abrir después de caerse, nunca en la
// primera conexión.
//
// Una subida cambia de copia de la API (spec 041), y cada copia solo avisa a sus propias
// conexiones: lo que pasó en la vieja mientras la tableta ya estaba en la nueva no llega como
// aviso. Volver a pedir al reconectar es lo que hace que no se pierda; sin esto se veía hasta el
// siguiente sondeo, o nunca, en lo que no se sondea.
export function alReconectar(es: EventSource, fn: () => void): void {
  let cayo = false;
  es.addEventListener('error', () => {
    cayo = true;
  });
  es.addEventListener('open', () => {
    if (!cayo) return;
    cayo = false;
    fn();
  });
}
