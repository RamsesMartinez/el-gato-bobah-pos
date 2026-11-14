import { useState } from 'react';
import { Box, Button, HStack, Input, SimpleGrid, Text, Textarea, VStack } from '@chakra-ui/react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
} from '../../components/ui/drawer';
import { backofficeApi, type AdjustmentInput } from '../../api/backoffice';
import { toaster } from '../../components/ui/toaster';
import { mensajeDeError } from '../../api/mensajes';
import { montoTecleado } from '../../domain/numeros';
import { MOTIVOS_DE_CORRECCION } from './motivosDeCorreccion';
import { montoOSinDato } from './montosDeCaja';

const QUE: { kind: AdjustmentInput['kind']; label: string }[] = [
  { kind: 'count_correction', label: 'Lo contado en el cajón' },
  { kind: 'float_correction', label: 'El fondo que se dejó' },
  { kind: 'note', label: 'Solo una nota' },
];

// CorregirCorte: la hoja para corregir un corte cerrado (#290, fase 3; Maquetas M4). Lo firmado al
// cerrar no se borra: se guarda la corrección con el nombre de quien la hace, y el corte muestra las
// dos cosas.
export function CorregirCorte({ sessionId, contado, fondo, currency, isOpen, onClose }: {
  sessionId: number; contado: string | null; fondo: string | null; currency: string;
  isOpen: boolean; onClose: () => void;
}) {
  const qc = useQueryClient();
  const [kind, setKind] = useState<AdjustmentInput['kind'] | null>(null);
  const [monto, setMonto] = useState('');
  const [motivo, setMotivo] = useState('');
  const [nota, setNota] = useState('');
  const guardar = useMutation({
    mutationFn: (input: AdjustmentInput) => backofficeApi.adjustCashSession(sessionId, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['cash'] });
      cerrar();
    },
    onError: (e) => toaster.create({ title: 'No se pudo guardar la corrección', description: mensajeDeError(e), type: 'error' }),
  });
  const cerrar = () => { setKind(null); setMonto(''); setMotivo(''); setNota(''); onClose(); };
  const conMonto = kind !== null && kind !== 'note';
  // Vacío no es $0: sin monto no se guarda una corrección de monto.
  const montoValido = !conMonto || montoTecleado(monto) !== undefined;
  const listo = kind !== null && motivo !== '' && nota.trim() !== '' && montoValido && !guardar.isPending;
  const firmado = kind === 'count_correction' ? contado : kind === 'float_correction' ? fondo : null;
  return (
    <DrawerRoot open={isOpen} placement="bottom" onOpenChange={(e) => { if (!e.open && !guardar.isPending) cerrar(); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="l3" maxH="90dvh" display="flex" flexDirection="column">
        <DrawerHeader><Text fontWeight="700" fontSize="lg">Corregir el corte #{sessionId}</Text></DrawerHeader>
        <DrawerBody flex="1" overflowY="auto">
          <VStack align="stretch" gap={4}>
            <Text fontSize="sm" color="fg.muted">Se guarda con tu nombre y no borra lo firmado.</Text>
            <Box>
              <Text fontWeight="700" mb={2}>Qué corriges</Text>
              <HStack gap={2} flexWrap="wrap">
                {QUE.map((q) => (
                  <Button key={q.kind} minH="48px" variant={kind === q.kind ? 'solid' : 'outline'}
                    aria-pressed={kind === q.kind} onClick={() => { setKind(q.kind); setMonto(''); }}>{q.label}</Button>
                ))}
              </HStack>
            </Box>
            {conMonto && (
              <SimpleGrid columns={2} gap={3}>
                <Box borderWidth="1px" borderRadius="lg" p={3}>
                  <Text fontSize="sm" color="fg.muted">{kind === 'count_correction' ? 'Contado al cerrar' : 'Fondo que se dejó'}</Text>
                  <Text fontSize="xl" fontWeight="700">{montoOSinDato(firmado, currency)}</Text>
                </Box>
                <Box>
                  <Text fontSize="sm" color="fg.muted">Debió decir</Text>
                  <Input id="debio-decir" aria-label="Debió decir" inputMode="decimal" minH="52px" fontSize="xl"
                    value={monto} onChange={(e) => setMonto(e.target.value)} placeholder="0.00" />
                </Box>
              </SimpleGrid>
            )}
            <Box>
              <Text fontWeight="700" mb={2}>Motivo</Text>
              <HStack gap={2} flexWrap="wrap">
                {MOTIVOS_DE_CORRECCION.map((m) => (
                  <Button key={m.value} minH="48px" variant={motivo === m.value ? 'solid' : 'outline'}
                    aria-pressed={motivo === m.value} onClick={() => setMotivo(m.value)}>{m.label}</Button>
                ))}
              </HStack>
            </Box>
            <Textarea aria-label="Qué pasó" placeholder="Qué pasó" value={nota} maxLength={200} minH="72px"
              onChange={(e) => setNota(e.target.value)} />
          </VStack>
        </DrawerBody>
        <DrawerFooter>
          <Button variant="outline" minH="52px" onClick={cerrar} disabled={guardar.isPending}>Cancelar</Button>
          <Button colorPalette="brand" minH="52px" disabled={!listo} loading={guardar.isPending}
            onClick={() => kind && guardar.mutate({
              kind, reason: motivo, note: nota.trim(),
              ...(conMonto ? { after: String(montoTecleado(monto)) } : {}),
            })}>
            Guardar corrección
          </Button>
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}
