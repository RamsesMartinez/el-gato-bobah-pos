// Cómo se le dice a cada tipo de movimiento del almacén en pantalla. Uno nuevo que la API agregue antes
// que el front se muestra legible en vez de con su nombre interno.
const TIPOS: Record<string, string> = {
  venta: 'Venta',
  compra: 'Compra',
  ajuste: 'Ajuste',
  merma: 'Merma',
  produccion: 'Producción',
  cancelacion: 'Cancelación',
  traspaso: 'Traspaso',
  merma_esperada: 'Merma esperada',
};

export function tipoDeMovimiento(tipo: string): string {
  if (TIPOS[tipo]) return TIPOS[tipo];
  const t = tipo.replace(/_/g, ' ');
  return t.charAt(0).toUpperCase() + t.slice(1);
}
