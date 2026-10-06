import { useState } from 'react';
import { Box, Button, HStack, Text, VStack } from '@chakra-ui/react';

import {
  DialogRoot, DialogBackdrop, DialogContent, DialogHeader, DialogBody, DialogFooter, DialogTitle,
} from '../../components/ui/dialog';
import { avisoDeInventario } from '../../domain/devolucion';
import { RemoveReasons } from './RemoveReasons';

const TAP = '44px';

interface Props {
  nombre: string;
  // yaSalioACocina decide el aviso, y el aviso es la mitad del valor de esta pantalla.
  yaSalioACocina: boolean;
  enviando: boolean;
  onCerrar: () => void;
  onConfirmar: (motivo: string) => void;
}

// Quitar un renglón del pedido.
//
// Lleva confirmación y no se ejecuta al tocar: es destructiva y vive dentro de una fila apretada, al
// lado de "Entregar". La constitución pide separar las acciones destructivas; cuando no hay espacio
// para separarlas, la barrera es el paso extra.
//
// Y ANUNCIA QUÉ PASA CON EL INSUMO antes de confirmar. Cancelar algo que ya salió a cocina baja el
// total del pedido pero NO devuelve el ingrediente, porque se gastó. Callarlo hace que el almacén
// cuadre mal y que nadie sepa por qué — el operador cree que deshizo la venta entera.
export function CancelarRenglonDialog({ nombre, yaSalioACocina, enviando, onCerrar, onConfirmar }: Props) {
  const [motivo, setMotivo] = useState<string | null>(null);

  return (
    <DialogRoot open placement="center" onOpenChange={(e) => { if (!e.open) onCerrar(); }}>
      <DialogBackdrop />
      <DialogContent>
        <DialogHeader><DialogTitle>Quitar {nombre}</DialogTitle></DialogHeader>
        <DialogBody>
          <VStack align="stretch" gap={3}>
            <Box borderWidth="1px" borderColor={yaSalioACocina ? 'orange.300' : 'border'}
              borderRadius="md" px={3} py={2}>
              <Text fontSize="sm" role="status">{avisoDeInventario(yaSalioACocina)}</Text>
            </Box>
            <RemoveReasons value={motivo} onChange={setMotivo} />
          </VStack>
        </DialogBody>
        <DialogFooter>
          <HStack gap={2} w="100%">
            <Button flex="1" minH={TAP} variant="outline" colorPalette="gray" onClick={onCerrar}>
              Dejarlo
            </Button>
            <Button flex="1" minH={TAP} colorPalette="red" loading={enviando} disabled={motivo === null}
              onClick={() => { if (motivo) onConfirmar(motivo); }}>
              Quitar del pedido
            </Button>
          </HStack>
        </DialogFooter>
      </DialogContent>
    </DialogRoot>
  );
}
