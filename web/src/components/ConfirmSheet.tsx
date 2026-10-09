import { Box, Button, HStack, Text } from '@chakra-ui/react';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
} from './ui/drawer';

interface Props {
  isOpen: boolean;
  title: string;
  description?: string;
  confirmLabel: string;
  cancelLabel?: string;
  // Destructiva: la acción segura es la principal y la que borra va roja y aparte. No destructiva
  // (cerrar caja): la principal es confirmar.
  destructive?: boolean;
  loading?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

// ConfirmSheet reemplaza al `confirm()` del navegador.
//
// El del sistema lo pinta el sistema operativo: botones de ~20 px fuera de la marca de la app, y en
// una tableta de 7" se acierta al revés. Aquí los dos botones miden 44 px o más y, cuando la acción
// borra algo, la segura es la que se ofrece primero y la que borra vive separada: el dedo que se
// resbala cae en «Seguir», no en «Descartar».
//
// Cerrar la hoja de cualquier forma —fondo, Escape, arrastrar— es cancelar.
export function ConfirmSheet({
  isOpen, title, description, confirmLabel, cancelLabel = 'Volver', destructive, loading,
  onConfirm, onCancel,
}: Props) {
  const seguro = (
    <Button flex="1" minH="52px" variant={destructive ? 'solid' : 'outline'}
      colorPalette={destructive ? undefined : 'gray'} disabled={loading} onClick={onCancel}>
      {cancelLabel}
    </Button>
  );
  const accion = (
    <Button flex={destructive ? undefined : '1'} minH="52px" px={6}
      variant={destructive ? 'outline' : 'solid'} colorPalette={destructive ? 'red' : undefined}
      data-destructive={destructive ? 'true' : 'false'} loading={loading} onClick={onConfirm}>
      {confirmLabel}
    </Button>
  );
  return (
    <DrawerRoot open={isOpen} placement="bottom" onOpenChange={(e) => { if (!e.open) onCancel(); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="l3" maxH="85dvh" display="flex" flexDirection="column">
        <DrawerHeader pb={1}>
          <Text fontSize="lg" fontWeight="700">{title}</Text>
        </DrawerHeader>
        {description && (
          <DrawerBody flex="1" minH={0} overflowY="auto" pb={2}>
            <Text color="fg.muted">{description}</Text>
          </DrawerBody>
        )}
        <DrawerFooter borderTopWidth="1px" pt={3}>
          {destructive ? (
            // Separadas por todo el ancho: cada una en su propia caja.
            <HStack w="100%" justify="space-between" gap={6}>
              <Box flex="1" display="flex">{seguro}</Box>
              <Box flexShrink={0}>{accion}</Box>
            </HStack>
          ) : (
            <HStack w="100%" gap={2}>
              {seguro}
              {accion}
            </HStack>
          )}
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}
