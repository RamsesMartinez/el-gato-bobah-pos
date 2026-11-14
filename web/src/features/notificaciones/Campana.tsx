import { useState } from 'react';
import { Badge, Box, Button, Flex, HStack, Tabs, Text, VStack } from '@chakra-ui/react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { LuBell } from 'react-icons/lu';
import { useNavigate } from 'react-router';
import { DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader } from '../../components/ui/drawer';
import { Picker } from '../../components/Picker';
import { ApiError } from '../../api/client';
import { backofficeApi } from '../../api/backoffice';
import { medirAccion } from '../../api/uso';
import { notificationsApi, REASON_WORDS, type NotificationView, type ResolveAction } from '../../api/notifications';
import { can } from '../../app/permissions';
import { useSessionStore } from '../../stores/session';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';
import { formatBaseQty } from '../../shared/units';
import { useAvisos } from './useAvisos';

// La campana al pie del riel (spec 084, maqueta M3, opción A v2): solo icono de 44 px, con la misma
// insignia que «Se echó a perder», y un panel de 440 px que sale junto al riel sin tapar la pantalla
// de trabajo. Abrir el panel marca como leídos, para esta persona, los avisos abiertos que ve.

const TIPO: Record<NotificationView['kind'], { texto: string; color: string }> = {
  stock_out: { texto: 'Se acabó', color: 'red' },
  stock_low: { texto: 'Bajo el mínimo', color: 'orange' },
  not_found: { texto: 'No lo encuentro', color: 'purple' },
};

// El panel lo abre la campana o el «Ver» de la franja: el estado vive en el riel y llega aquí.
export function Campana({ abierto, setAbierto }: { abierto: boolean; setAbierto: (v: boolean) => void }) {
  const { enabled, unread, open } = useAvisos();
  const qc = useQueryClient();
  // El servidor marca de 100 en 100: se manda en tandas.
  const leer = useMutation({
    mutationFn: async (ids: number[]) => {
      for (let i = 0; i < ids.length; i += 100) await notificationsApi.read(ids.slice(i, i + 100));
    },
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['avisos'] }),
  });
  if (!enabled) return null;
  const abrir = () => {
    setAbierto(true);
    medirAccion('avisos', 'abrir');
    const sinLeer = open.filter((n) => !n.read).map((n) => n.id);
    if (sinLeer.length) leer.mutate(sinLeer);
  };
  return (
    <>
      <Button aria-label={`Avisos, ${unread} sin leer`} variant="ghost" color="white" minH="44px" w="100%"
        _hover={{ bg: 'whiteAlpha.200' }} onClick={abrir}>
        <Box position="relative">
          <LuBell size={22} />
          {unread > 0 && (
            <Box position="absolute" top="-6px" right="-10px" minW="16px" h="16px" px="3px" borderRadius="full"
              bg="orange.500" color="white" fontSize="10px" fontWeight="700" lineHeight="16px" textAlign="center">
              {unread > 99 ? '99+' : unread}
            </Box>
          )}
        </Box>
      </Button>
      {abierto && <PanelDeAvisos abiertos={open} onClose={() => setAbierto(false)} />}
    </>
  );
}

export function PanelDeAvisos({ abiertos, onClose }: { abiertos: NotificationView[]; onClose: () => void }) {
  const [tab, setTab] = useState<'abiertas' | 'cerradas'>('abiertas');
  const cerradas = useQuery({ queryKey: ['avisos', 'closed'], queryFn: () => notificationsApi.list('closed'), enabled: tab === 'cerradas' });
  return (
    <DrawerRoot open placement="start" size="md" onOpenChange={(e) => { if (!e.open) onClose(); }}>
      <DrawerBackdrop />
      <DrawerContent ml="76px" display="flex" flexDirection="column" data-testid="panel-de-avisos">
        <DrawerHeader py={2}>
          <Flex w="100%" align="center" justify="space-between">
            <Text fontSize="lg" fontWeight="700">Avisos</Text>
            <Button minH="44px" variant="ghost" onClick={onClose}>Cerrar</Button>
          </Flex>
        </DrawerHeader>
        <DrawerBody flex="1" minH={0} overflowY="auto" pb={4}>
          <Tabs.Root value={tab} onValueChange={(e) => setTab(e.value as 'abiertas' | 'cerradas')}>
            <Tabs.List mb={2}>
              <Tabs.Trigger value="abiertas" minH="44px">Abiertas · {abiertos.length}</Tabs.Trigger>
              <Tabs.Trigger value="cerradas" minH="44px">Cerradas</Tabs.Trigger>
            </Tabs.List>
            <Tabs.Content value="abiertas">
              {abiertos.length === 0 && <Text color="fg.muted" py={4}>No hay avisos abiertos.</Text>}
              <VStack align="stretch" gap={2}>{abiertos.map((n) => <Renglon key={n.id} n={n} onClose={onClose} />)}</VStack>
            </Tabs.Content>
            <Tabs.Content value="cerradas">
              {cerradas.data && (cerradas.data.items ?? []).length === 0 && <Text color="fg.muted" py={4}>Todavía no se ha cerrado ninguno.</Text>}
              <VStack align="stretch" gap={2}>{(cerradas.data?.items ?? []).map((n) => <Renglon key={n.id} n={n} onClose={onClose} />)}</VStack>
            </Tabs.Content>
          </Tabs.Root>
        </DrawerBody>
      </DrawerContent>
    </DrawerRoot>
  );
}

function Renglon({ n, onClose }: { n: NotificationView; onClose: () => void }) {
  const user = useSessionStore((s) => s.user);
  const hora = useHoraDelNegocio();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const [eligiendo, setEligiendo] = useState(false);
  // El insumo elegido espera confirmación: un toque errado en la lista ligaría para siempre un nombre
  // al insumo equivocado.
  const [elegido, setElegido] = useState<{ id: number; name: string } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const insumos = useQuery({ queryKey: ['articles', { q: '' }], queryFn: () => backofficeApi.articles({ q: '' }), enabled: eligiendo });
  const cerrar = useMutation({
    mutationFn: ({ action, ingredientId }: { action: ResolveAction; ingredientId?: number }) => notificationsApi.resolve(n.id, action, ingredientId),
    onSuccess: (_, v) => { medirAccion('avisos', v.action); setEligiendo(false); setElegido(null); void qc.invalidateQueries({ queryKey: ['avisos'] }); },
    // Lo más común es que otra persona ya lo cerró: se vuelve a pedir para que el renglón deje de
    // ofrecer botones que ya no aplican.
    onError: (e) => {
      setError(e instanceof ApiError ? e.message : 'No se pudo; vuelve a intentar.');
      void qc.invalidateQueries({ queryKey: ['avisos'] });
    },
  });
  const abierto = n.status === 'open';
  const configura = can('inventory.configure', user);
  const cantidad = (v: string | null) => (v == null ? '' : formatBaseQty(v, n.unitCode ?? ''));
  const detalle = n.kind === 'stock_low'
    ? `Hay ${cantidad(n.onHandSeen)} · mínimo ${cantidad(n.minSeen)}`
    : n.kind === 'stock_out'
      ? `Lo reportó ${n.openedByName || 'el sistema'}${n.confirmations > 1 ? ` · lo confirmaron ${n.confirmations}` : ''}`
      : `Lo buscó ${n.openedByName || 'alguien'}`;
  return (
    <Box data-row borderWidth="1px" borderRadius="lg" p={3} bg={abierto && !n.read ? 'brand.subtle' : 'bg'}>
      <HStack gap={2} flexWrap="wrap">
        <Badge colorPalette={TIPO[n.kind].color}>{TIPO[n.kind].texto}</Badge>
        {n.needsCount && <Badge colorPalette="gray">Pedir conteo</Badge>}
        <Text fontSize="sm" color="fg.muted">{hora.fechaYHora(n.openedAt)}</Text>
      </HStack>
      <Text fontWeight="700" mt={1}>{n.kind === 'not_found' ? `«${n.queryText}» no está en el sistema` : n.itemName}</Text>
      <Text fontSize="sm" color="fg.muted">{detalle}</Text>
      {!abierto && (
        <Text fontSize="sm" mt={1}>
          {REASON_WORDS[n.resolvedReason ?? ''] ?? 'Cerrado'}{n.resolvedByName ? ` · ${n.resolvedByName}` : ''}
          {n.resolvedAt ? ` · ${hora.fechaYHora(n.resolvedAt)}` : ''}
        </Text>
      )}
      {error && <Text role="alert" color="fg.error" fontSize="sm" mt={1}>{error}</Text>}
      {abierto && (
        <HStack gap={2} mt={2} flexWrap="wrap">
          {n.kind === 'stock_out' && can('inventory.receive', user) && (
            <Button minH="44px" variant="outline" disabled={cerrar.isPending} onClick={() => cerrar.mutate({ action: 'arrived' })}>Ya llegó</Button>
          )}
          {n.kind === 'not_found' && configura && (
            <>
              <Button minH="44px" variant="outline" onClick={() => { onClose(); navigate(`/almacen?tab=insumos&nuevo=${encodeURIComponent(n.queryText ?? '')}&aviso=${n.id}`); }}>Dar de alta</Button>
              {elegido ? null : eligiendo ? (
                <Box minW="220px" flex="1">
                  <Picker value="" title="¿De qué insumo es otro nombre?" placeholder="Elegir insumo"
                    options={(insumos.data ?? []).map((a) => ({ value: String(a.id), label: a.name }))}
                    onChange={(v) => {
                      const a = (insumos.data ?? []).find((x) => String(x.id) === v);
                      if (a) setElegido({ id: a.id, name: a.name });
                    }} />
                </Box>
              ) : (
                <Button minH="44px" variant="outline" onClick={() => setEligiendo(true)}>Es otro nombre de…</Button>
              )}
              {elegido && (
                <Box w="100%" borderWidth="1px" borderRadius="md" p={2}>
                  <Text fontWeight="600" mb={2}>Guardar «{n.queryText}» como otro nombre de «{elegido.name}»</Text>
                  <HStack gap={2} justify="flex-end">
                    <Button minH="44px" variant="ghost" onClick={() => setElegido(null)}>Cancelar</Button>
                    <Button minH="44px" disabled={cerrar.isPending} onClick={() => cerrar.mutate({ action: 'alias', ingredientId: elegido.id })}>Guardar</Button>
                  </HStack>
                </Box>
              )}
              {/* Descartar en su propio renglón, a la derecha: no queda pegado a las acciones frecuentes
                  cuando los botones no caben en una línea del panel. */}
              <Flex w="100%" justify="flex-end">
                <Button minH="44px" variant="ghost" disabled={cerrar.isPending} onClick={() => cerrar.mutate({ action: 'dismiss' })}>Descartar</Button>
              </Flex>
            </>
          )}
          {n.kind === 'stock_low' && can('inventory.view', user) && (
            <Button minH="44px" variant="ghost" onClick={() => { onClose(); navigate('/almacen?tab=compra'); }}>Ver en la lista de compra ›</Button>
          )}
        </HStack>
      )}
    </Box>
  );
}
