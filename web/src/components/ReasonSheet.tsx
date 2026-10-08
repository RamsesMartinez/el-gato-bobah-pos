import { useState } from 'react';
import { Box, Button, HStack, Input, Text } from '@chakra-ui/react';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
} from './ui/drawer';

interface Props {
  isOpen: boolean;
  title: string;
  label: string;
  placeholder?: string;
  confirmLabel: string;
  required?: boolean;
  // La acción confirma algo que no se deshace (cancelar un pedido): va roja y APARTE de «Volver»,
  // como en ConfirmSheet, para que el dedo que se resbala no la toque.
  destructive?: boolean;
  loading?: boolean;
  // El motivo recortado ('' si es opcional y quedó vacío) o `null` si se arrepintió. Son dos
  // respuestas distintas: «sin motivo» confirma, «volver» no hace nada.
  onDone: (reason: string | null) => void;
}

// ReasonSheet reemplaza al `prompt()` del navegador para pedir un motivo.
export function ReasonSheet({
  isOpen, title, label, placeholder, confirmLabel, required, destructive, loading, onDone,
}: Props) {
  const [texto, setTexto] = useState('');
  const limpio = texto.trim();
  // Se vacía al SALIR, no con un efecto al abrir (la hoja queda montada entre aperturas).
  const salir = (r: string | null) => { setTexto(''); onDone(r); };

  return (
    <DrawerRoot open={isOpen} placement="bottom" onOpenChange={(e) => { if (!e.open) salir(null); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="l3" maxH="85dvh" display="flex" flexDirection="column">
        <DrawerHeader pb={1}>
          <Text fontSize="lg" fontWeight="700">{title}</Text>
        </DrawerHeader>
        <DrawerBody flex="1" minH={0} overflowY="auto" pb={2}>
          <Input autoFocus size="lg" minH="52px" aria-label={label}
            placeholder={placeholder ?? (required ? label : `${label} (opcional)`)}
            value={texto} onChange={(e) => setTexto(e.target.value)}
            onKeyDown={(e) => { if (e.key === 'Enter' && (!required || limpio)) salir(limpio); }} />
        </DrawerBody>
        <DrawerFooter borderTopWidth="1px" pt={3}>
          <HStack w="100%" gap={destructive ? 6 : 2} justify="space-between">
            <Box flex="1" display="flex">
              <Button flex="1" minH="52px" variant={destructive ? 'solid' : 'outline'}
                colorPalette={destructive ? undefined : 'gray'} disabled={loading} onClick={() => salir(null)}>
                Volver
              </Button>
            </Box>
            <Box flex={destructive ? undefined : '1'} display="flex">
              <Button flex="1" minH="52px" px={6} colorPalette="red" variant={destructive ? 'outline' : 'solid'}
                loading={loading} disabled={required === true && !limpio} onClick={() => salir(limpio)}>
                {confirmLabel}
              </Button>
            </Box>
          </HStack>
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}
