import { TOQUE_44 } from './piezasDeCaja';
import { useState } from 'react';
import { ConfirmSheet } from '../../components/ConfirmSheet';
import { Box, Text, Button, VStack, HStack, Table, Input, Center, Spinner } from '@chakra-ui/react';
import { LuPlus } from 'react-icons/lu';
import { ApiError } from '../../api/client';
import { toaster } from '../../components/ui/toaster';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { backofficeApi, type CashRegisterConfig } from '../../api/backoffice';
import { Switch } from '../../components/ui/switch';
import { DialogRoot, DialogBackdrop, DialogContent, DialogBody, DialogHeader, DialogTitle, DialogCloseTrigger } from '../../components/ui/dialog';

// ---- Tab: administrar cajas (admin/gerente) ----
const COUNTS_CASH_ERRORS: Record<string, string> = {
  REGISTER_HAS_OPEN_SHIFT: 'Cierra el turno de esta caja primero.',
  PRIMARY_REGISTER_COUNTS: 'La caja principal siempre lleva arqueo.',
};

export function ManageRegistersTab() {
  const qc = useQueryClient();
  const { data, isLoading } = useQuery({ queryKey: ['cash', 'registers', 'all'], queryFn: backofficeApi.allCashRegisters });
  const [name, setName] = useState('');
  const [edit, setEdit] = useState<CashRegisterConfig | null>(null);

  const invalidate = () => qc.invalidateQueries({ queryKey: ['cash'] });
  const create = useMutation({
    mutationFn: () => backofficeApi.createCashRegister(name.trim()),
    onSuccess: () => { invalidate(); setName(''); },
    onError: (e) => toaster.create({ title: 'Error', description: String(e), type: 'error' }),
  });
  const update = useMutation({
    mutationFn: (r: CashRegisterConfig) => backofficeApi.updateCashRegister(r.id, { name: r.name.trim(), isActive: r.isActive }),
    onSuccess: () => { invalidate(); setEdit(null); },
    onError: (e) => toaster.create({ title: 'Error', description: String(e), type: 'error' }),
  });
  // Nombre y «activa» viajan tal cual: el servidor los exige, y uno vacío desactivaría la caja.
  const setCounts = useMutation({
    mutationFn: (v: { id: number; name: string; isActive: boolean; countsCash: boolean }) =>
      backofficeApi.updateCashRegister(v.id, { name: v.name, isActive: v.isActive, countsCash: v.countsCash }),
    onSuccess: invalidate,
    onError: (e) => {
      const known = e instanceof ApiError ? COUNTS_CASH_ERRORS[e.code] : undefined;
      toaster.create(known ? { title: known, type: 'error' } : { title: 'No se pudo cambiar', description: String(e), type: 'error' });
    },
  });

  const [toSell, setToSell] = useState<CashRegisterConfig | null>(null);
  const sell = useMutation({
    mutationFn: (id: number) => backofficeApi.makeRegisterSell(id),
    onSuccess: invalidate,
    onError: (e) => {
      if (e instanceof ApiError && e.code === 'REGISTERS_MUST_BE_CLOSED') {
        toaster.create({ title: 'Cierra los turnos de las dos cajas primero.', type: 'error' });
        return;
      }
      toaster.create({ title: 'No se pudo cambiar', description: e instanceof ApiError ? e.message : String(e), type: 'error' });
    },
  });

  if (isLoading) return <Center h="40vh"><Spinner size="xl" /></Center>;

  return (
    <VStack align="stretch" gap={4}>
      <Box bg="bg.panel" p={4} borderRadius="lg" borderWidth="1px">
        <Text fontWeight="700" mb={3}>Nueva caja</Text>
        <HStack flexWrap="wrap" gap={3}>
          <Input placeholder="Nombre (ej. Caja fuerte)" value={name} onChange={(e) => setName(e.target.value)} flex="1" minW="180px" />
          <Button disabled={!name.trim()} loading={create.isPending} onClick={() => create.mutate()}><LuPlus /> Agregar</Button>
        </HStack>
      </Box>
      <Box bg="bg.panel" borderRadius="lg" borderWidth="1px" overflowX="auto">
        <Table.Root size="sm">
          <Table.Header><Table.Row>
            <Table.ColumnHeader>Nombre</Table.ColumnHeader>
            <Table.ColumnHeader>Vende</Table.ColumnHeader>
            <Table.ColumnHeader>Activa</Table.ColumnHeader>
            <Table.ColumnHeader>Lleva arqueo</Table.ColumnHeader>
            <Table.ColumnHeader></Table.ColumnHeader>
          </Table.Row></Table.Header>
          <Table.Body>
            {(data?.items ?? []).map((r) => (
              <Table.Row key={r.id}>
                <Table.Cell>{r.name}</Table.Cell>
                {/* Cada interruptor ocupa una zona de 44 px que también recibe el toque (spec 050). */}
                <Table.Cell>
                  <Switch {...TOQUE_44} checked={r.isPrimary} disabled={r.isPrimary || !r.isActive}
                    title={r.isPrimary ? 'Para cambiarla, enciende otra' : undefined}
                    inputProps={{ 'aria-label': `Vende: ${r.name}` }}
                    onCheckedChange={(e) => { if (e.checked) setToSell(r); }} />
                </Table.Cell>
                <Table.Cell>
                  {/* La caja principal no se puede desactivar (el POS necesita dónde cuadrar). */}
                  <Switch {...TOQUE_44} checked={r.isActive} disabled={r.isPrimary}
                    inputProps={{ 'aria-label': `Activa: ${r.name}` }}
                    onCheckedChange={(e) => update.mutate({ ...r, isActive: e.checked })} />
                </Table.Cell>
                <Table.Cell>
                  <Switch {...TOQUE_44} checked={r.countsCash !== false} disabled={r.isPrimary}
                    inputProps={{ 'aria-label': `Lleva arqueo: ${r.name}` }}
                    onCheckedChange={(e) => setCounts.mutate({ id: r.id, name: r.name, isActive: r.isActive, countsCash: e.checked })} />
                </Table.Cell>
                <Table.Cell textAlign="end"><Button size="sm" minH="44px" variant="outline" onClick={() => setEdit(r)}>Editar</Button></Table.Cell>
              </Table.Row>
            ))}
          </Table.Body>
        </Table.Root>
      </Box>

      <ConfirmSheet isOpen={toSell !== null} title={`¿Pasar las ventas a ${toSell?.name ?? ''}?`}
        cancelLabel="Volver" confirmLabel={`Pasar las ventas a ${toSell?.name ?? ''}`} confirmPalette="brand"
        onCancel={() => setToSell(null)}
        onConfirm={() => { const r = toSell; setToSell(null); if (r) sell.mutate(r.id); }} />

      <DialogRoot open={edit !== null} onOpenChange={(e) => { if (!e.open) setEdit(null); }} placement="center" size="sm">
        <DialogBackdrop />
        <DialogContent>
          <DialogHeader><DialogTitle>Editar caja</DialogTitle></DialogHeader>
          <DialogCloseTrigger />
          <DialogBody pb={6}>
            {edit && (
              <VStack align="stretch" gap={3}>
                <Input placeholder="Nombre" value={edit.name} onChange={(e) => setEdit({ ...edit, name: e.target.value })} />
                {!edit.isPrimary && (
                  <Switch checked={edit.isActive} onCheckedChange={(e) => setEdit({ ...edit, isActive: e.checked })}>Activa</Switch>
                )}
                <Button disabled={!edit.name.trim()} loading={update.isPending} onClick={() => update.mutate(edit)}>Guardar</Button>
              </VStack>
            )}
          </DialogBody>
        </DialogContent>
      </DialogRoot>
    </VStack>
  );
}
