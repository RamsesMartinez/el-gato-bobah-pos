import { useState } from 'react';
import { Button, HStack, Input, SimpleGrid, Text } from '@chakra-ui/react';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
} from '../../components/ui/drawer';

const MOTIVOS = ['Se fue sin checar', 'Salió por una emergencia', 'Otro'];

// motivoDelCierre es lo que viaja como `authorizedClose.reason`: el motivo elegido y el detalle.
function motivoDelCierre(motivo: string, detalle: string): string {
  const d = detalle.trim();
  return d ? `${motivo}: ${d}` : motivo;
}

// CierreAutorizadoSheet: quien abrió la caja se fue sin marcar su salida y quien tiene el permiso
// cierra sin ella (spec 046, plan › «Pantallas y maquetas», punto 4). Mismo patrón que
// MotivoDeApertura: botón fijo en el pie y `maxH` en dvh, para que el teclado del sistema no lo tape.
// No cierra la caja: solo decide; el cierre sigue con el conteo de siempre.
export function CierreAutorizadoSheet({ isOpen, nombre, onConfirmar, onVolver }: {
  isOpen: boolean;
  nombre: string;
  onConfirmar: (reason: string) => void;
  onVolver: () => void;
}) {
  const [motivo, setMotivo] = useState('');
  const [detalle, setDetalle] = useState('');
  const salir = () => { setMotivo(''); setDetalle(''); onVolver(); };
  const otro = motivo === 'Otro';
  const listo = motivo !== '' && (!otro || detalle.trim() !== '');
  return (
    <DrawerRoot open={isOpen} placement="bottom" onOpenChange={(e) => { if (!e.open) salir(); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="l3" maxH="85dvh" display="flex" flexDirection="column">
        <DrawerHeader pb={1}>
          <Text fontSize="lg" fontWeight="700">Cerrar sin la salida de {nombre}</Text>
        </DrawerHeader>
        <DrawerBody flex="1" minH={0} overflowY="auto">
          <Text fontSize="sm" color="fg.muted" mb={3}>
            La entrada de {nombre} quedará «sin salida» y se corrige después en Asistencia. Queda
            registrado que lo autorizaste tú.
          </Text>
          <SimpleGrid columns={3} gap={2}>
            {MOTIVOS.map((m) => (
              <Button key={m} minH="52px" whiteSpace="normal" variant={motivo === m ? 'solid' : 'outline'}
                colorPalette={motivo === m ? 'orange' : 'gray'} aria-pressed={motivo === m}
                onClick={() => setMotivo(m)}>{m}</Button>
            ))}
          </SimpleGrid>
          <Text id="detalle-cierre" mt={3} mb={1} fontSize="sm" color="fg.muted">
            {otro ? '¿Qué pasó?' : 'Detalle (opcional)'}
          </Text>
          <Input aria-labelledby="detalle-cierre" minH="52px" maxLength={150} placeholder="Ej. avisó por teléfono"
            value={detalle} onChange={(e) => setDetalle(e.target.value)} />
        </DrawerBody>
        <DrawerFooter borderTopWidth="1px" pt={3}>
          <HStack w="100%" gap={6}>
            <Button flex="1" minH="52px" variant="outline" onClick={salir}>Volver</Button>
            <Button flex="1" minH="52px" colorPalette="orange" disabled={!listo}
              onClick={() => { onConfirmar(motivoDelCierre(motivo, detalle)); setMotivo(''); setDetalle(''); }}>
              Seguir con el cierre
            </Button>
          </HStack>
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}
