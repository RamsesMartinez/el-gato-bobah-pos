import { useMemo, useState } from 'react';
import { Button, HStack, Input, Table, Text, VStack } from '@chakra-ui/react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter } from '../../../components/ui/drawer';
import { ConfirmSheet } from '../../../components/ConfirmSheet';
import { Switch } from '../../../components/ui/switch';
import { ApiError } from '../../../api/client';
import { medirAccion } from '../../../api/uso';
import { lowStockApi, type LowStockRow } from '../../../api/notifications';
import { formatBaseQty } from '../../../shared/units';
import { baseQty, bigUnitOf, type Kind } from '../../../shared/articleUnits';

// «Alertas por lote» (spec 084, maqueta M8, opción B): una tabla con buscador para encender alertas y
// capturar el mínimo de muchos insumos de una sentada, que es el trabajo real al empezar (casi
// ninguno tiene mínimo). El mínimo va en la unidad grande (kg, L, m) como en la ficha: con la base, un
// «2» pensado en kilos programaba 2 g. Vacío o 0 = sin mínimo.
// Los eventos y la franja de cada artículo se conservan (se cambian en su ficha). Guardar pide la
// misma confirmación que la ficha, con lo que pasará en cada uno.

// minTocado: el mínimo solo viaja si se editó; si no, el servidor deja el actual (otra persona pudo
// cambiarlo en la ficha después de que se abrió esta tabla).
type Cambio = { enabled: boolean; min: string; minTocado: boolean };
const clave = (r: LowStockRow) => `${r.itemType}:${r.id}`;
const KIND_DE: Record<string, Kind> = { g: 'masa', ml: 'volumen', cm: 'longitud', pieza: 'pieza' };
const kindDe = (r: LowStockRow): Kind | null => KIND_DE[r.unitCode] ?? null;
const grande = (r: LowStockRow) => { const k = kindDe(r); return k ? bigUnitOf(k) : null; };
const unidadEscrita = (r: LowStockRow) => grande(r)?.label ?? (r.unitCode === 'pieza' ? 'pzas' : r.unitCode);
const minDe = (r: LowStockRow) => {
  if (r.minStock == null || Number(r.minStock) === 0) return '';
  const b = grande(r);
  return String(b ? Number(r.minStock) / b.factor : Number(r.minStock));
};
const NUMERO = /^\d+(\.\d+)?$/;
// minimoBase traduce lo escrito a la unidad base: null = sin mínimo, undefined = no es un número.
function minimoBase(r: LowStockRow, texto: string): string | null | undefined {
  const t = texto.trim().replaceAll(',', '.');
  if (t === '') return null;
  if (!NUMERO.test(t)) return undefined;
  if (Number(t) === 0) return null;
  const k = kindDe(r);
  return k ? baseQty(t, k, !!bigUnitOf(k)) ?? undefined : t;
}
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
  const valor = (r: LowStockRow): Cambio => cambios[clave(r)] ?? { enabled: r.enabled, min: minDe(r), minTocado: false };
  const cambia = (r: LowStockRow, c: Partial<Cambio>) => setCambios((cs) => ({ ...cs, [clave(r)]: { ...valor(r), ...c } }));
  const minCambio = (r: LowStockRow, c: Cambio) => c.minTocado && c.min.trim() !== minDe(r);
  const sucios = (lista.data?.items ?? []).filter((r) => {
    const c = cambios[clave(r)];
    return !!c && (c.enabled !== r.enabled || minCambio(r, c));
  });
  const invalidos = sucios.filter((r) => minCambio(r, cambios[clave(r)]) && minimoBase(r, cambios[clave(r)].min) === undefined);
  const items = () => sucios.map((r) => {
    const c = cambios[clave(r)];
    return minCambio(r, c)
      ? { itemType: r.itemType, id: r.id, enabled: c.enabled, minStock: minimoBase(r, c.min) ?? null }
      : { itemType: r.itemType, id: r.id, enabled: c.enabled };
  });
  const [marcar, setMarcar] = useState(false);
  const revisar = () => {
    if (invalidos.length) {
      setMarcar(true);
      setError(`Revisa el mínimo de ${invalidos.map((r) => r.name).join(' y ')}: se escribe con números, sin signo.`);
      return;
    }
    ver.mutate();
  };

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
                  const malo = marcar && invalidos.includes(r);
                  return (
                    <Table.Row key={clave(r)} data-row h="56px">
                      <Table.Cell><Text fontWeight="600">{r.name}</Text></Table.Cell>
                      <Table.Cell textAlign="end" whiteSpace="nowrap">{formatBaseQty(r.onHand, r.unitCode)}</Table.Cell>
                      <Table.Cell>
                        <HStack>
                          <Input aria-label={`Mínimo de ${r.name}`} minH="44px" w="120px" inputMode="decimal" placeholder="Sin mínimo"
                            aria-invalid={malo || undefined} value={v.min} onChange={(e) => cambia(r, { min: e.target.value, minTocado: true })} />
                          <Text color="fg.muted">{unidadEscrita(r)}</Text>
                        </HStack>
                      </Table.Cell>
                      <Table.Cell textAlign="center">
                        {/* La zona de toque es el propio interruptor: una caja de 44 px alrededor no recibe el toque. */}
                        <Switch aria-label={`Alerta de ${r.name}`} minH="44px" minW="44px" px={2} justifyContent="center"
                          checked={v.enabled} onCheckedChange={(e) => cambia(r, { enabled: e.checked })} />
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
              onClick={revisar}>
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
