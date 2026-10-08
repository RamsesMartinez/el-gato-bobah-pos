import { useEffect, useState } from 'react';
import { Box, Button, HStack, Text, VStack } from '@chakra-ui/react';

import { DrawerRoot, DrawerBackdrop, DrawerContent, DrawerCloseTrigger } from '../../components/ui/drawer';
import { mensajeDeError } from '../../api/mensajes';
import type { BoardOrder } from '../../types/pos';
import { pendientes, renglonesDe } from './entrega';
import { RemoveReasons } from './RemoveReasons';

// 44 px es el mínimo con el que un dedo acierta a la primera.
const TAP = '44px';

interface Props {
  order: BoardOrder;
  onClose: () => void;
  // onConfirm hace la petición. La hoja espera su respuesta para cerrarse: ver abajo.
  onConfirm: (reason: string) => Promise<unknown>;
}

function productos(n: number): string {
  return n === 1 ? '1 producto' : `${n} productos`;
}

// CancelPendingSheet quita de un pedido todo lo que falta por entregar, y deja lo entregado.
//
// Es la salida de un pedido del que ya salió algo: cancelarlo entero lo rechaza el servidor, porque
// reponer lo que el cliente se llevó le inventaría existencias al almacén, y quitar producto por
// producto eran diez toques con el cliente enfrente.
//
// No se cierra al tocar «Quitar»: espera la respuesta. Cerrarse antes le dice al operador que ya se
// quitó, y si el servidor lo rechaza el aviso sale cuando la hoja ya no está y el pedido sigue igual.
export function CancelPendingSheet({ order, onClose, onConfirm }: Props) {
  // Monta cerrada y se abre en el render siguiente: una hoja que nace con `open` puesto no se monta
  // si en la misma actualización se cierra otra cosa —aquí, el menú de la tarjeta— (ver CobrarSheet).
  const [visible, setVisible] = useState(false);
  // eslint-disable-next-line react-hooks/set-state-in-effect
  useEffect(() => { setVisible(true); }, []);

  const [reason, setReason] = useState<string | null>(null);
  const [sending, setSending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const pending = pendientes(order).length;
  const delivered = renglonesDe(order).filter((l) => Number(l.delivered) > 0).length;
  const folio = order.folioName || `#${order.number}`;

  const confirm = async () => {
    if (!reason) return;
    setSending(true);
    setError(null);
    try {
      await onConfirm(reason);
      onClose();
    } catch (e) {
      setError(mensajeDeError(e));
      setSending(false);
    }
  };

  return (
    <DrawerRoot open={visible} placement="bottom" size="md"
      onOpenChange={(e) => { if (!e.open && !sending) onClose(); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="2xl" maxH="100dvh" overflowY="auto">
        <DrawerCloseTrigger />
        <VStack align="stretch" gap={3} p={4} pb={5}>
          <Box>
            <Text fontWeight="800" fontSize="lg">
              {delivered > 0 ? `${folio} ya entregó ${productos(delivered)}` : `Quitar lo que falta de ${folio}`}
            </Text>
            {delivered > 0 && (
              <Text fontSize="sm" color="fg.muted">Lo entregado no se puede cancelar.</Text>
            )}
          </Box>

          <Text fontSize="sm" fontWeight="700">Falta por entregar: {productos(pending)}</Text>

          <RemoveReasons value={reason} onChange={setReason} />

          {error && <Text fontSize="sm" color="fg.error" role="alert">{error}</Text>}

          {/* Separados por un hueco que crece: la acción destructiva no va pegada a la que deja
              todo como estaba. */}
          <HStack gap={2}>
            <Button minH={TAP} px={5} variant="outline" colorPalette="gray" disabled={sending}
              onClick={onClose}>
              Dejarlo
            </Button>
            <Box flex="1" />
            <Button minH={TAP} px={5} colorPalette="red" fontWeight="800"
              loading={sending} disabled={reason === null}
              onClick={confirm}>
              {pending === 1 ? 'Quitar el que falta' : `Quitar los ${pending} que faltan`}
            </Button>
          </HStack>
        </VStack>
      </DrawerContent>
    </DrawerRoot>
  );
}
