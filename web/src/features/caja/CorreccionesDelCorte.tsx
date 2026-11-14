import { useState } from 'react';
import { Badge, Box, Button, HStack, Text, VStack } from '@chakra-ui/react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { backofficeApi, type CashSessionDetail, type SessionAdjustment } from '../../api/backoffice';
import { ReasonSheet } from '../../components/ReasonSheet';
import { toaster } from '../../components/ui/toaster';
import { mensajeDeError } from '../../api/mensajes';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';
import { can } from '../../app/permissions';
import { useSessionStore } from '../../stores/session';
import { CorregirCorte } from './CorregirCorte';
import { rotuloDeCorreccion } from './motivosDeCorreccion';
import { montoOSinDato } from './montosDeCaja';

function titulo(a: SessionAdjustment, currency: string): string {
  if (a.kind === 'count_correction') return `Contado ${montoOSinDato(a.before, currency)}, corregido a ${montoOSinDato(a.after, currency)}`;
  if (a.kind === 'float_correction') return `Fondo ${montoOSinDato(a.before, currency)}, corregido a ${montoOSinDato(a.after, currency)}`;
  if (a.kind === 'reversal') return `Revirtió la corrección #${a.reversesId}`;
  return 'Nota';
}

// CorreccionesDelCorte: las correcciones de un corte cerrado y la acción de corregir (#290, fase 3;
// Maquetas M1 y M4). Lo firmado se sigue viendo arriba; aquí va qué se corrigió, cuándo, quién y por
// qué, y se revierte con otra corrección, nunca borrando.
export function CorreccionesDelCorte({ corte }: { corte: CashSessionDetail }) {
  const user = useSessionStore((s) => s.user);
  const puede = can('cash.adjust_count', user);
  const hora = useHoraDelNegocio();
  const qc = useQueryClient();
  const [corrigiendo, setCorrigiendo] = useState(false);
  const [revirtiendo, setRevirtiendo] = useState<SessionAdjustment | null>(null);
  const revertir = useMutation({
    mutationFn: (p: { id: number; note: string }) => backofficeApi.reverseCashSessionAdjustment(corte.id, p.id, p.note),
    onSuccess: () => { setRevirtiendo(null); qc.invalidateQueries({ queryKey: ['cash'] }); },
    onError: (e) => toaster.create({ title: 'No se pudo revertir', description: mensajeDeError(e), type: 'error' }),
  });
  const correcciones = corte.adjustments ?? [];
  // Solo un corte cerrado y con su arqueo a la vista se corrige: con el arqueo oculto, quien lo ve
  // no tiene con qué decidir.
  if (corte.status !== 'cerrada' || corte.arqueoHidden) return null;
  if (correcciones.length === 0 && !puede) return null;
  return (
    <Box borderWidth="1px" borderRadius="lg" p={3} bg="bg.panel">
      <HStack justify="space-between" mb={correcciones.length ? 2 : 0} gap={2} flexWrap="wrap">
        <Text fontWeight="700">Correcciones</Text>
        {puede && <Button size="sm" minH="48px" variant="outline" onClick={() => setCorrigiendo(true)}>Corregir</Button>}
      </HStack>
      <VStack align="stretch" gap={2}>
        {correcciones.map((a) => (
          <HStack key={a.id} justify="space-between" align="start" gap={3} borderTopWidth="1px" pt={2} maxW="70ch">
            <Box minW={0}>
              <Text fontWeight="600">{titulo(a, corte.currency)}</Text>
              <Text fontSize="sm" color="fg.muted">
                {hora.fechaYHora(a.createdAt)} · {a.createdByName} · {rotuloDeCorreccion(a.reason)}
              </Text>
              <Text fontSize="sm">{a.reasonNote}</Text>
            </Box>
            {a.reversedBy != null ? (
              <Badge colorPalette="gray" flexShrink={0}>Revertida</Badge>
            ) : puede && (a.reversible ?? a.kind !== 'reversal') ? (
              <Button size="sm" minH="48px" variant="outline" flexShrink={0} onClick={() => setRevirtiendo(a)}>Revertir</Button>
            ) : null}
          </HStack>
        ))}
      </VStack>
      <CorregirCorte sessionId={corte.id} contado={corte.effective?.drawerCounted ?? corte.drawer?.counted ?? null}
        fondo={corte.effective?.floatLeft ?? corte.floatLeft ?? null} currency={corte.currency}
        isOpen={corrigiendo} onClose={() => setCorrigiendo(false)} />
      <ReasonSheet isOpen={revirtiendo !== null} required title="Revertir la corrección"
        label="Por qué se revierte" confirmLabel="Revertir la corrección" loading={revertir.isPending}
        onDone={(nota) => { if (nota === null) setRevirtiendo(null); else if (revirtiendo) revertir.mutate({ id: revirtiendo.id, note: nota }); }} />
    </Box>
  );
}
