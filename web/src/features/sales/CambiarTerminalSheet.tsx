import { useEffect, useState } from 'react';
import { Box, Button, HStack, Input, Text, VStack, Wrap } from '@chakra-ui/react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { posApi } from '../../api/pos';
import { ApiError } from '../../api/client';
import { DrawerRoot, DrawerBackdrop, DrawerContent, DrawerCloseTrigger } from '../../components/ui/drawer';
import { money } from '../../utils/format';
import { Picker } from '../../components/Picker';
import type { PaymentView } from '../../types/pos';

// Los dos motivos de siempre se eligen con un toque y el primero llega marcado: así el caso común son
// tres toques sin abrir el teclado, que en la tableta tapa medio alto de la hoja. Uno solo prellenado
// dejaba en la bitácora «se cobró en otra terminal» también cuando fue un toque equivocado.
const MOTIVOS = ['Se cobró en otra terminal', 'Se eligió mal al cobrar'];
// Hasta cuántas terminales caben como botones sin bajar el motivo y Confirmar; la misma regla que en
// el cobro (TerminalDelCobro).
const BOTONES_MAX = 4;

// La hoja con la que se corrige en qué terminal se cobró un pago con tarjeta (spec 036).
export function CambiarTerminalSheet({ orderId, pago, onCerrar }: {
  orderId: number;
  pago: PaymentView;
  onCerrar: () => void;
}) {
  // Monta cerrada y se abre en el render siguiente, como DevolucionSheet: una hoja que nace con
  // `open` puesto no se monta si otra se está cerrando en la misma actualización.
  const [visible, setVisible] = useState(false);
  // eslint-disable-next-line react-hooks/set-state-in-effect
  useEffect(() => { setVisible(true); }, []);

  const qc = useQueryClient();
  const [elegida, setElegida] = useState<number | null>(null);
  const [motivo, setMotivo] = useState(MOTIVOS[0]);
  const [error, setError] = useState('');
  const terminales = useQuery({ queryKey: ['card-terminals'], queryFn: posApi.cardTerminals });
  const activas = (terminales.data?.items ?? []).filter((t) => !t.archived);
  const actual = pago.terminalId ?? null;

  const cambiar = useMutation({
    mutationFn: () => posApi.changePaymentTerminal(orderId, pago.id,
      { fromTerminalId: actual, terminalId: elegida as number, reason: motivo.trim() }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['orders', orderId] });
      void qc.invalidateQueries({ queryKey: ['sales'] });
      void qc.invalidateQueries({ queryKey: ['cash'] });
      onCerrar();
    },
    onError: (e: Error) => {
      if (e instanceof ApiError && e.status === 409) {
        // Otro cambio ganó: se recarga la venta para que se vea dónde quedó el pago.
        setError('El pago ya cambió. Revisa la terminal que tiene ahora.');
        void qc.invalidateQueries({ queryKey: ['orders', orderId] });
        return;
      }
      setError(e.message);
    },
  });
  const motivoValido = motivo.trim().length >= 3 && motivo.trim().length <= 200;

  return (
    <DrawerRoot open={visible} placement="bottom" size="md"
      onOpenChange={(e) => { if (!e.open) onCerrar(); }}>
      <DrawerBackdrop />
      <DrawerContent borderTopRadius="2xl" maxH="85dvh" overflowY="auto">
        <DrawerCloseTrigger />
        <VStack align="stretch" gap={3} p={4} pb={5}>
          <Box>
            <Text fontWeight="800" fontSize="lg">Cambiar terminal</Text>
            <Text fontSize="sm" color="fg.muted">{pago.methodName} · {money(pago.amount)}</Text>
          </Box>
          <Text fontSize="sm">
            Ahora: {pago.terminalName ?? 'Sin terminal'}. ¿En cuál se cobró?
          </Text>
          {activas.length > BOTONES_MAX ? (
            <Picker value={elegida === null ? '' : String(elegida)} title="Terminal" placeholder="Elige la terminal"
              options={activas.filter((t) => t.id !== actual).map((t) => ({ value: String(t.id), label: t.name }))}
              onChange={(v) => { setElegida(Number(v)); setError(''); }} />
          ) : (
          <Wrap gap={2}>
            {activas.map((t) => (
              <Button key={t.id} minH="52px" px={5} fontWeight="700"
                variant={elegida === t.id ? 'solid' : 'outline'}
                aria-pressed={elegida === t.id}
                disabled={t.id === actual}
                onClick={() => { setElegida(t.id); setError(''); }}>
                {t.id === actual ? `${t.name} (actual)` : t.name}
              </Button>
            ))}
          </Wrap>
          )}
          <Box>
            <Text fontSize="sm" color="fg.muted" mb={1}>Motivo</Text>
            <Wrap gap={2} mb={2}>
              {MOTIVOS.map((m) => (
                <Button key={m} size="sm" minH="44px" px={4} variant={motivo === m ? 'solid' : 'outline'}
                  aria-pressed={motivo === m} onClick={() => setMotivo(m)}>
                  {m}
                </Button>
              ))}
            </Wrap>
            <Input aria-label="Motivo" minH="44px" maxLength={200}
              value={motivo} onChange={(e) => setMotivo(e.target.value)} />
          </Box>
          {error && <Text fontSize="sm" color="fg.error" role="status">{error}</Text>}
          <HStack justify="flex-end" gap={2}>
            <Button minH="44px" px={5} variant="outline" onClick={onCerrar}>Cancelar</Button>
            <Button minH="44px" px={5} fontWeight="800" loading={cambiar.isPending}
              disabled={elegida === null || !motivoValido}
              onClick={() => cambiar.mutate()}>
              Confirmar
            </Button>
          </HStack>
        </VStack>
      </DrawerContent>
    </DrawerRoot>
  );
}
