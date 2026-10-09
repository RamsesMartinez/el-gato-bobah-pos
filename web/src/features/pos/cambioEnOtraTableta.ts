// QUÉ SE AVISA CUANDO LA CUENTA ABIERTA CAMBIA EN OTRA TABLETA (spec 030, FR-017; research R-12).
//
// La cuenta se actualiza sola con cada evento del servidor; el aviso dice QUÉ pasó para que quien
// la tiene abierta no cobre dos veces, ni siga capturando en una cuenta que ya no existe.
//
// Lo que NO se avisa es tan importante como lo que sí: lo que hizo esta misma tableta, y un agregado
// de la otra (D-5: lo que agrega cada una se suma). Avisar cada toque ajeno sería ruido, y quien
// opera aprendería a ignorar justo el aviso que sí importa.

export interface FotoDeCuenta {
  nombre: string;
  // De la cuenta en captura (capturando · enviada · descartada) o del pedido.
  estado: string;
  falta: number;
  // Cantidad por renglón.
  renglones: Record<string, number>;
}

export function cambioEnOtraTableta(
  antes: FotoDeCuenta | undefined,
  despues: FotoDeCuenta | undefined,
  propia: boolean,
): string | null {
  if (!antes || !despues || propia) return null;
  const quien = despues.nombre || 'La cuenta';
  if (antes.estado !== despues.estado) {
    if (despues.estado === 'enviada') return `${quien} se mandó a cocina en otra tableta`;
    if (despues.estado === 'descartada') return `${quien} se descartó en otra tableta`;
    if (despues.estado === 'cancelada' || despues.estado === 'reembolsada') return `${quien} se canceló en otra tableta`;
  }
  if (despues.falta < antes.falta - 0.005 && sinMenos(antes, despues)) return `${quien} se cobró en otra tableta`;
  if (!sinMenos(antes, despues)) return `${quien} cambió en otra tableta`;
  return null;
}

// sinMenos: todo lo que había sigue, con la misma cantidad o más. Es la forma de un agregado.
function sinMenos(antes: FotoDeCuenta, despues: FotoDeCuenta): boolean {
  return Object.entries(antes.renglones).every(([id, q]) => (despues.renglones[id] ?? 0) >= q);
}
