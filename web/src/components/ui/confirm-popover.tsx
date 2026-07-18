import { useState } from 'react';
import type { ReactElement } from 'react';
import { Popover, Portal, Button, HStack, Text, VStack } from '@chakra-ui/react';

interface Props {
  children: ReactElement;       // botón disparador (se ancla el popover a él)
  title: string;
  description?: string;
  confirmLabel?: string;
  confirmPalette?: string;
  onConfirm: () => void;
}

// Confirmación ligera anclada al propio botón. Evita acciones destructivas por toque
// accidental (target 7") sin el peso de un modal: un segundo toque deliberado en «Archivar».
// El deshacer real va aparte, en el toast de éxito (action «Deshacer»).
export function ConfirmPopover({
  children, title, description, confirmLabel = 'Archivar', confirmPalette = 'red', onConfirm,
}: Props) {
  const [open, setOpen] = useState(false);
  return (
    <Popover.Root open={open} onOpenChange={(e) => setOpen(e.open)} positioning={{ placement: 'top-end' }}>
      <Popover.Trigger asChild>{children}</Popover.Trigger>
      <Portal>
        <Popover.Positioner>
          <Popover.Content maxW="256px" colorPalette="gray">
            <Popover.Arrow />
            <Popover.Body>
              <VStack align="stretch" gap={3}>
                <VStack align="stretch" gap={1}>
                  <Text fontWeight="600" fontSize="sm">{title}</Text>
                  {description && <Text fontSize="xs" color="fg.muted">{description}</Text>}
                </VStack>
                <HStack justify="end" gap={2}>
                  <Button size="xs" variant="ghost" onClick={() => setOpen(false)}>Cancelar</Button>
                  <Button size="xs" colorPalette={confirmPalette} onClick={() => { setOpen(false); onConfirm(); }}>
                    {confirmLabel}
                  </Button>
                </HStack>
              </VStack>
            </Popover.Body>
          </Popover.Content>
        </Popover.Positioner>
      </Portal>
    </Popover.Root>
  );
}
