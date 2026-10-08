import { ConfirmSheet } from '../../components/ConfirmSheet';
import { money } from '../../utils/format';

// DESCARTAR UNA CUENTA QUE NO SE HA MANDADO (spec 030, US5; D-7; lienzo V2-4).
//
// Una cuenta vacía se descarta sin preguntar: no tiene nada que perder. Con productos pregunta en
// una hoja de la app —no en el `confirm()` del sistema— y la acción principal es la que NO borra.
// Su nombre vuelve a la bolsa; el rastro se conserva en el servidor como descartada.
//
// Una cuenta ya enviada no pasa por aquí: no se descarta, se cancela el pedido con motivo y permiso.

interface Props {
  isOpen: boolean;
  nombre: string;
  productos: number;
  total: number;
  loading?: boolean;
  onSeguir: () => void;
  onDescartar: () => void;
}

export function DescartarCuentaSheet({ isOpen, nombre, productos, total, loading, onSeguir, onDescartar }: Props) {
  const cuantos = productos === 1 ? 'Se pierde 1 producto' : `Se pierden ${productos} productos`;
  return (
    <ConfirmSheet isOpen={isOpen} destructive loading={loading}
      title={`¿Descartar la cuenta de ${nombre || 'esta cuenta'}?`}
      description={`${cuantos} (${money(total)}); no se ha mandado a cocina.`}
      cancelLabel="Seguir capturando" confirmLabel="Descartar"
      onCancel={onSeguir} onConfirm={onDescartar} />
  );
}
