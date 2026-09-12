// La fecha en la consola se pinta en la zona de QUIEN LA MIRA, y eso es deliberado.
//
// El POS formatea todo en la zona del NEGOCIO (`utils/horaDelNegocio.ts`) porque un ticket con la
// hora del navegador miente sobre cuándo se cobró. Aquí no aplica, y no por descuido: esta pantalla
// la abre quien VENDE el sistema, en su computadora, mirando varias empresas a la vez — no hay una
// zona del negocio que honrar. Y aunque la hubiera, la consola no podría leerla: su rol de base no
// tiene permiso sobre `business_settings`.
//
// Por eso el guardia de `formateoUnico.test.ts` lista este archivo aparte en vez de obligar a
// importar el del POS: son dos productos con dos preguntas distintas.

// soloFecha: "9 sep 2026". Sin hora — para saber desde cuándo es cliente, el día basta.
export function soloFecha(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  return d.toLocaleDateString('es-MX', { dateStyle: 'medium' });
}
