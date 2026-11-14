import { useState } from 'react';
import { Button, HStack, Input, SimpleGrid, Text } from '@chakra-ui/react';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
} from '../../components/ui/drawer';
import { MOTIVOS_DE_APERTURA } from './motivosDeApertura';

// MotivoDeApertura: lo contado al abrir no coincide con el cierre anterior (spec 032, punto 6). No
// dice cuánto había: el conteo es a ciegas.
export function MotivoDeApertura({ isOpen, guardando, onConfirmar, onVolver }: {
  isOpen: boolean;
  guardando: boolean;
  onConfirmar: (motivo: string, nota: string) => void;
  onVolver: () => void;
}) {
  const [motivo, setMotivo] = useState('');
  const [nota, setNota] = useState('');
  const salir = () => { setMotivo(''); setNota(''); onVolver(); };
  const listo = motivo !== '' && (motivo !== 'other' || nota.trim() !== '');
  return (
    <DrawerRoot open={isOpen} placement="bottom" onOpenChange={(e) => { if (!e.open && !guardando) salir(); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="l3" maxH="85dvh" display="flex" flexDirection="column">
        <DrawerHeader pb={1}>
          {/* El paso en que va la apertura (#290, FR-035): contar, luego esto, luego abrir. */}
          <Text fontSize="sm" color="fg.muted" fontWeight="600">Paso 2 · Por qué no coincide</Text>
          <Text fontSize="lg" fontWeight="700">Lo contado no coincide con el cierre anterior</Text>
          <Text fontSize="sm" color="fg.muted">Elige el motivo. Si contaste mal, vuelve y cuenta otra vez.</Text>
        </DrawerHeader>
        <DrawerBody flex="1" minH={0} overflowY="auto">
          <SimpleGrid columns={2} gap={2}>
            {MOTIVOS_DE_APERTURA.map((m) => (
              <Button key={m.value} minH="52px" variant={motivo === m.value ? 'solid' : 'outline'}
                colorPalette={motivo === m.value ? 'orange' : 'gray'} aria-pressed={motivo === m.value}
                onClick={() => setMotivo(m.value)}>{m.label}</Button>
            ))}
          </SimpleGrid>
          <Input mt={3} minH="52px" aria-label="Motivo" maxLength={200}
            placeholder={motivo === 'other' ? 'Escribe el motivo' : 'Detalle (opcional)'}
            value={nota} onChange={(e) => setNota(e.target.value)} />
        </DrawerBody>
        <DrawerFooter borderTopWidth="1px" pt={3}>
          <HStack w="100%" gap={6}>
            <Button flex="1" minH="52px" variant="outline" disabled={guardando} onClick={salir}>Volver a contar</Button>
            <Button flex="1" minH="52px" colorPalette="orange" disabled={!listo} loading={guardando}
              onClick={() => onConfirmar(motivo, nota.trim())}>Abrir caja</Button>
          </HStack>
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}
