import { useState } from 'react';
import { Box, Button, Flex, HStack, Text } from '@chakra-ui/react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useLocation } from 'react-router';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader,
} from '../../components/ui/drawer';
import { can } from '../../app/permissions';
import { useSessionStore } from '../../stores/session';
import { medirAccion } from '../../api/uso';
import { moneyExact } from '../../utils/format';
import { wasteApi } from '../../api/waste';
import { formatBaseQty } from '../../shared/units';

// Mermas por aprobar (spec 039, maqueta B): una franja arriba para quien aprueba, mientras haya
// pendientes. Una pendiente no resta hasta que alguien la aprueba, y olvidada deja el almacén mal.
// No sale en Vender ni en Pedidos: ahí quita una fila de productos a quien cobra, y el aviso es el
// número del riel; tampoco en la captura, que ya lo dice y necesita el alto para «Guardar». Se oculta
// por la sesión hasta que llegue otra pendiente: guarda cuántas había al ocultarla.

const HIDDEN_KEY = 'egb:merma:franja-oculta';
const NOT_HERE = [/^\/pos/, /^\/pedidos/, /^\/merma/];

function hiddenAt(): number {
  try { return Number(sessionStorage.getItem(HIDDEN_KEY) ?? '0') || 0; } catch { return 0; }
}

export function PendingWasteBar() {
  const user = useSessionStore((s) => s.user);
  const { pathname } = useLocation();
  const approver = can('inventory.write_off_approve', user);
  const [hiddenCount, setHiddenCount] = useState(hiddenAt);
  const [open, setOpen] = useState(false);
  const { data: pending = [] } = useQuery({
    queryKey: ['merma', 'por-aprobar'], queryFn: wasteApi.pending, enabled: approver, staleTime: 30_000, refetchInterval: 60_000,
  });
  if (!approver || pending.length === 0 || NOT_HERE.some((r) => r.test(pathname))) return null;
  const hidden = hiddenCount > 0 && pending.length <= hiddenCount;
  const hide = () => {
    setHiddenCount(pending.length);
    try { sessionStorage.setItem(HIDDEN_KEY, String(pending.length)); } catch { /* sin almacenamiento, se oculta solo aquí */ }
  };
  return (
    <>
      {!hidden && (
        <Flex minH="44px" align="center" bg="orange.50" color="orange.800" borderBottomWidth="1px" borderColor="orange.200" px={3} gap={2} flexShrink={0}>
          <Button flex="1" minH="44px" variant="ghost" justifyContent="flex-start" color="inherit" fontWeight="600" onClick={() => setOpen(true)}>
            {pending.length === 1 ? '1 espera tu aprobación' : `${pending.length} esperan tu aprobación`} · Ver
          </Button>
          <Button minH="44px" variant="ghost" color="inherit" onClick={hide}>Ocultar por ahora</Button>
        </Flex>
      )}
      <PendingSheet open={open} onClose={() => setOpen(false)} />
    </>
  );
}

function PendingSheet({ open, onClose }: { open: boolean; onClose: () => void }) {
  const qc = useQueryClient();
  const { data: pending = [] } = useQuery({ queryKey: ['merma', 'por-aprobar'], queryFn: wasteApi.pending });
  const [confirm, setConfirm] = useState<number | null>(null);
  const done = () => { setConfirm(null); void qc.invalidateQueries({ queryKey: ['merma'] }); };
  const approve = useMutation({ mutationFn: (id: number) => wasteApi.approve(id), onSuccess: () => { medirAccion('merma', 'aprobar'); done(); } });
  const reject = useMutation({ mutationFn: (id: number) => wasteApi.reject(id), onSuccess: () => { medirAccion('merma', 'rechazar'); done(); } });
  return (
    <DrawerRoot open={open} onOpenChange={(e) => { if (!e.open) onClose(); }} placement="bottom">
      <DrawerBackdrop />
      <DrawerContent maxH="80dvh">
        <DrawerHeader>
          <Flex justify="space-between" align="center">
            <Text fontWeight="700">Por aprobar</Text>
            <Button minH="44px" variant="ghost" onClick={onClose}>Cerrar</Button>
          </Flex>
        </DrawerHeader>
        <DrawerBody overflowY="auto">
          {pending.length === 0 && <Text color="fg.muted">No hay nada por aprobar.</Text>}
          {pending.map((r) => (
            <Flex key={r.id} data-testid="renglon-por-aprobar" minH="64px" align="center" justify="space-between" gap={3} borderBottomWidth="1px" py={1}>
              <Box minW={0}>
                <Text fontWeight="600">{r.itemName} · {formatBaseQty(r.quantity, r.unitCode)}</Text>
                <Text fontSize="sm" color="fg.muted">{r.reason} · {r.recordedBy}{r.totalCost ? ` · ${moneyExact(r.totalCost)}` : ' · sin costo'}</Text>
              </Box>
              {confirm === r.id ? (
                <HStack gap={3} flexShrink={0}>
                  <Button minH="44px" colorPalette="red" disabled={reject.isPending} onClick={() => reject.mutate(r.id)}>Sí, rechazar</Button>
                  <Button minH="44px" variant="ghost" onClick={() => setConfirm(null)}>No</Button>
                </HStack>
              ) : (
                <HStack gap={4} flexShrink={0}>
                  <Button minH="44px" variant="ghost" colorPalette="red" onClick={() => setConfirm(r.id)}>Rechazar</Button>
                  <Button minH="44px" disabled={approve.isPending} onClick={() => approve.mutate(r.id)}>Aprobar</Button>
                </HStack>
              )}
            </Flex>
          ))}
        </DrawerBody>
      </DrawerContent>
    </DrawerRoot>
  );
}
