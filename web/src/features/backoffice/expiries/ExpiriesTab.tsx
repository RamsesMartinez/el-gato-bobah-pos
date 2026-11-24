import { useState } from 'react';
import { Box, Button, Center, HStack, Spinner, Text, VStack } from '@chakra-ui/react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { backofficeApi, type ExpiryLot } from '../../../api/backoffice';
import { Segmented } from '../../../components/Segmented';
import { can } from '../../../app/permissions';
import { useSessionStore } from '../../../stores/session';
import { formatDay } from '../../../utils/calendar';
import { formatBaseQty } from '../../../shared/units';
import { GROUPS, groupOf, projection, whenText } from './expiryText';
import { WasteLotSheet } from './WasteLotSheet';

// Caducidades (spec 098, maqueta «Caducidades», opción A): lo que ya caducó y lo que caduca, por plazo,
// del que caduca antes al que caduca después. Cada lote dice en una frase qué pasa con él al ritmo de
// venta de los últimos 30 días, y lo caducado se manda a merma desde aquí.
export function ExpiriesTab({ onGoToCount }: { onGoToCount?: () => void }) {
  const qc = useQueryClient();
  const user = useSessionStore((s) => s.user);
  const canWaste = can('inventory.write_off', user);
  const warehouses = useQuery({ queryKey: ['warehouses'], queryFn: () => backofficeApi.warehouses() });
  const whList = warehouses.data?.items ?? [];
  const [picked, setPicked] = useState<number | 'all'>('all');
  const data = useQuery({ queryKey: ['stock', 'expiries', picked], queryFn: () => backofficeApi.expiries(picked), placeholderData: (p) => p });
  const [later, setLater] = useState(false);
  const [wasting, setWasting] = useState<ExpiryLot | null>(null);
  const [done, setDone] = useState('');

  if (data.isLoading) return <Center py={16}><Spinner size="xl" /></Center>;
  if (data.isError) {
    return (
      <VStack py={10} gap={3}>
        <Text color="fg.error">No se pudieron leer las caducidades.</Text>
        <Button minH="44px" variant="outline" onClick={() => void data.refetch()}>Reintentar</Button>
      </VStack>
    );
  }
  const lots = data.data?.lots ?? [];

  return (
    <VStack align="stretch" gap={3} opacity={data.isPlaceholderData ? 0.4 : 1} aria-busy={data.isPlaceholderData || undefined}>
      <HStack justify="space-between" gap={2} flexWrap="wrap">
        {whList.length > 1 ? (
          <Segmented label="Almacén" value={String(picked)} onChange={(v) => { setDone(''); setPicked(v === 'all' ? 'all' : Number(v)); }}
            options={[...whList.map((w) => ({ value: String(w.id), label: w.name })), { value: 'all', label: 'Todos' }]} />
        ) : <Box />}
      </HStack>
      {done && <Text p={3} borderRadius="lg" bg="green.subtle" fontSize="sm" data-testid="listo">{done}</Text>}
      {lots.length === 0 && (
        <VStack py={10} gap={3} data-testid="sin-caducidades">
          <Text fontWeight="600">Aún no hay fechas de caducidad.</Text>
          <Text color="fg.muted">Regístralas al recibir una compra o al contar.</Text>
          {onGoToCount && <Button minH="44px" variant="outline" onClick={onGoToCount}>Ir a Conteo</Button>}
        </VStack>
      )}
      {GROUPS.map((g) => {
        const ls = lots.filter((l) => groupOf(l) === g);
        // «Esta semana» se ve aunque esté vacío: así se sabe que nada caduca pronto, no que falta el grupo.
        if (ls.length === 0) {
          return g === 'Esta semana' && lots.length > 0
            ? <Text key={g} color="fg.muted" data-testid="grupo-vacio">Esta semana · nada caduca en los próximos 7 días</Text>
            : null;
        }
        const collapsed = g === 'Más adelante' && !later;
        return (
          <Box key={g} data-testid={`grupo-${g}`}>
            <HStack justify="space-between" mb={1}>
              <Text fontWeight="700">{g} · {ls.length}</Text>
              {g === 'Más adelante' && (
                <Button minH="44px" variant="ghost" size="sm" onClick={() => setLater((v) => !v)}>{later ? 'Ocultar' : 'Ver'}</Button>
              )}
            </HStack>
            {!collapsed && (
              <Box borderWidth="1px" borderRadius="lg" bg="bg.panel">
                {ls.map((l) => {
                  const p = projection(l);
                  const pending = Number(l.pendingWaste) > 0;
                  return (
                    <HStack key={`${l.warehouseId}-${l.ingredientId}-${l.expiresOn}`} data-testid="lote" minH="56px" px={3} py={1} gap={3} borderBottomWidth="1px">
                      <VStack align="start" gap={0} flex="1" minW={0}>
                        <HStack gap={1} minW={0} w="100%">
                          <Text fontWeight="600" truncate>{l.name}</Text>
                          <Text color="fg.muted" flexShrink={0}>· {l.warehouseName}</Text>
                        </HStack>
                        <Text fontSize="sm" color={pending ? 'fg.muted' : p.warn ? 'fg.warning' : 'fg.muted'}>
                          {pending ? 'Merma pendiente de aprobación' : p.text}
                        </Text>
                      </VStack>
                      <VStack align="end" gap={0} minW="120px">
                        <Text fontWeight="700">{formatBaseQty(l.qty, l.unitCode)}</Text>
                        <Text fontSize="sm" color="fg.muted">{formatDay(l.expiresOn)} · {whenText(l.daysLeft)}</Text>
                      </VStack>
                      {canWaste && (
                        <Button minH="44px" variant={l.expired ? 'solid' : 'outline'} colorPalette={l.expired ? 'brand' : 'gray'}
                          disabled={pending} onClick={() => { setDone(''); setWasting(l); }}>A merma</Button>
                      )}
                    </HStack>
                  );
                })}
              </Box>
            )}
          </Box>
        );
      })}
      {wasting && (
        <WasteLotSheet lot={wasting} onClose={() => setWasting(null)} onDone={(r) => {
          setWasting(null);
          setDone(r.status === 'pending'
            ? `Listo. La merma de ${wasting.name} espera aprobación; el lote baja al aprobarse.`
            : `Listo. ${wasting.name}: se mandó a merma.`);
          void qc.invalidateQueries({ queryKey: ['stock'] });
          void qc.invalidateQueries({ queryKey: ['waste'] });
        }} />
      )}
    </VStack>
  );
}
