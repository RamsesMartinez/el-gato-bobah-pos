import { useMemo, useState } from 'react';
import { Box, Button, HStack, Input, Table, Text, VStack } from '@chakra-ui/react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter } from '../../../components/ui/drawer';
import { ConfirmSheet } from '../../../components/ConfirmSheet';
import { Switch } from '../../../components/ui/switch';
import { ApiError } from '../../../api/client';
import { medirAccion } from '../../../api/uso';
import { lowStockApi, type LowStockRow } from '../../../api/notifications';
import { formatBaseQty } from '../../../shared/units';

// «Alertas por lote» (spec 084, maqueta M8, opción B): una tabla con buscador para encender alertas y
// capturar el mínimo de muchos insumos de una sentada, que es el trabajo real al empezar (casi
// ninguno tiene mínimo). El mínimo va en la unidad base del artículo; vacío = sin mínimo, nunca 0.
// Los eventos y la franja de cada artículo se conservan (se cambian en su ficha). Guardar pide la
// misma confirmación que la ficha, con lo que pasará en cada uno.

type Cambio = { enabled: boolean; min: string };
const clave = (r: LowStockRow) => `${r.itemType}:${r.id}`;
const sinAcentos = (s: string) => s.normalize('NFD').replace(/\p{M}/gu, '').toLowerCase();

export function AlertasPorLote({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient();
  const lista = useQuery({ queryKey: ['alertas', 'lote'], queryFn: lowStockApi.list });
  const [q, setQ] = useState('');
  const [cambios, setCambios] = useState<Record<string, Cambio>>({});
  const [confirmar, setConfirmar] = useState<{ nombre: string; abre: boolean; cierra: boolean; hay: string }[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  const filas = useMemo(() => {
    const k = sinAcentos(q.trim());
    return (lista.data?.items ?? []).filter((r) => !k || sinAcentos(r.name).includes(k));
  }, [lista.data, q]);
  const valor = (r: LowStockRow): Cambio => cambios[clave(r)] ?? { enabled: r.enabled, min: r.minStock == null || Number(r.minStock) === 0 ? '' : String(Number(r.minStock)) };
  const cambia = (r: LowStockRow, c: Partial<Cambio>) => setCambios((cs) => ({ ...cs, [clave(r)]: { ...valor(r), ...c } }));
  const sucios = (lista.data?.items ?? []).filter((r) => {
    const c = cambios[clave(r)];
    if (!c) return false;
    const minAntes = r.minStock == null || Number(r.minStock) === 0 ? '' : String(Number(r.minStock));
    return c.enabled !== r.enabled || c.min.trim() !== minAntes;
  });
  const items = () => sucios.map((r) => {
    const c = cambios[clave(r)];
    return { itemType: r.itemType, id: r.id, enabled: c.enabled, minStock: c.min.trim() === '' ? null : c.min.trim() };
  });

  const fallo = (e: unknown) => setError(e instanceof ApiError ? e.message : 'No se pudo; vuelve a intentar.');
  const ver = useMutation({
    mutationFn: () => lowStockApi.batch(items(), true),
    onSuccess: (res) => {
      setError(null);
      const porClave = new Map((lista.data?.items ?? []).map((r) => [clave(r), r]));
      setConfirmar((res.items ?? []).map((it) => {
        const r = porClave.get(`${it.itemType}:${it.id}`)!;
        return { nombre: r.name, abre: it.effect.opens.length > 0, cierra: it.effect.closes.length > 0, hay: formatBaseQty(it.onHand, r.unitCode) };
      }));
    },
    onError: fallo,
  });
  const guardar = useMutation({
    mutationFn: () => lowStockApi.batch(items(), false),
    onSuccess: () => {
      medirAccion('inventario', 'alertas-lote');
      setConfirmar(null);
      setCambios({});
      void qc.invalidateQueries({ queryKey: ['alertas'] });
      void qc.invalidateQueries({ queryKey: ['avisos'] });
      onClose();
    },
    onError: (e) => { setConfirmar(null); fallo(e); },
  });

  const abren = (confirmar ?? []).filter((c) => c.abre);
  return (
    <>
      <DrawerRoot open placement="bottom" size="full" onOpenChange={(e) => { if (!e.open) onClose(); }}>
        <DrawerBackdrop />
        <DrawerContent display="flex" flexDirection="column">
          <DrawerHeader py={2}>
            <VStack align="stretch" gap={2} w="100%">
              <Text fontSize="lg" fontWeight="700">Alertas por lote</Text>
              <Input aria-label="Buscar insumo" placeholder="Buscar insumo" minH="44px" value={q} onChange={(e) => setQ(e.target.value)} />
            </VStack>
          </DrawerHeader>
          <DrawerBody flex="1" minH={0} overflowY="auto">
            {error && <Text role="alert" color="fg.error" mb={2}>{error}</Text>}
            <Table.Root size="sm" stickyHeader>
              <Table.Header><Table.Row>
                <Table.ColumnHeader>Insumo</Table.ColumnHeader>
                <Table.ColumnHeader textAlign="end">Hay</Table.ColumnHeader>
                <Table.ColumnHeader w="220px">Mínimo</Table.ColumnHeader>
                <Table.ColumnHeader textAlign="center" w="96px">Alerta</Table.ColumnHeader>
              </Table.Row></Table.Header>
              <Table.Body>
                {filas.map((r) => {
                  const v = valor(r);
                  return (
                    <Table.Row key={clave(r)} data-row h="56px">
                      <Table.Cell><Text fontWeight="600">{r.name}</Text></Table.Cell>
                      <Table.Cell textAlign="end" whiteSpace="nowrap">{formatBaseQty(r.onHand, r.unitCode)}</Table.Cell>
                      <Table.Cell>
                        <HStack>
                          <Input aria-label={`Mínimo de ${r.name}`} minH="44px" w="120px" inputMode="decimal" placeholder="Sin mínimo"
                            value={v.min} onChange={(e) => cambia(r, { min: e.target.value.replace(',', '.') })} />
                          <Text color="fg.muted">{r.unitCode === 'pieza' ? 'pzas' : r.unitCode}</Text>
                        </HStack>
                      </Table.Cell>
                      <Table.Cell textAlign="center">
                        <Box display="inline-flex" minH="44px" alignItems="center">
                          <Switch aria-label={`Alerta de ${r.name}`} checked={v.enabled} onCheckedChange={(e) => cambia(r, { enabled: e.checked })} />
                        </Box>
                      </Table.Cell>
                    </Table.Row>
                  );
                })}
              </Table.Body>
            </Table.Root>
            {lista.data && filas.length === 0 && <Text color="fg.muted" py={4}>Nada coincide con la búsqueda.</Text>}
          </DrawerBody>
          <DrawerFooter>
            <Button minH="52px" variant="ghost" onClick={onClose}>Cancelar</Button>
            <Button minH="52px" colorPalette="brand" disabled={sucios.length === 0 || sucios.length > 100} loading={ver.isPending}
              onClick={() => ver.mutate()}>
              {sucios.length > 100 ? 'Hasta 100 cambios a la vez' : `Guardar cambios (${sucios.length})`}
            </Button>
          </DrawerFooter>
        </DrawerContent>
      </DrawerRoot>
      <ConfirmSheet isOpen={confirmar !== null} title="Guardar las alertas" confirmLabel="Guardar" confirmPalette="brand"
        loading={guardar.isPending} onConfirm={() => guardar.mutate()} onCancel={() => setConfirmar(null)}>
        <VStack align="stretch" gap={2}>
          <Text>Se actualizarán las alertas de {confirmar?.length ?? 0} {confirmar?.length === 1 ? 'insumo' : 'insumos'}.</Text>
          {abren.length > 0 && (
            <Text bg="orange.subtle" borderRadius="md" p={3} fontWeight="600">
              {abren.map((c) => `${c.nombre} (quedan ${c.hay})`).join(', ')} {abren.length === 1 ? 'ya está' : 'ya están'} en o bajo su mínimo: se enviará un aviso al guardar.
            </Text>
          )}
          {abren.length === 0 && <Text color="fg.muted">No se envía nada al guardar.</Text>}
        </VStack>
      </ConfirmSheet>
    </>
  );
}
