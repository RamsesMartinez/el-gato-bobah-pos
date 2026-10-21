import { useEffect, useState } from 'react';
import { Box, Button, Flex, HStack, Text, VStack } from '@chakra-ui/react';
import { useQuery } from '@tanstack/react-query';
import { useNavigate } from 'react-router';
import { can } from '../../app/permissions';
import { useSessionStore } from '../../stores/session';
import { DatePickerButton } from '../../components/DatePickerButton';
import { medirAccion } from '../../api/uso';
import { WASTE_STATUS_WORDS as STATUS_WORDS, wasteApi } from '../../api/waste';
import { moneyExact } from '../../utils/format';
import { useHoraDelNegocio } from '../../hooks/useHoraDelNegocio';

// Lo que se echó a perder, por motivo (spec 039, maqueta B): fichas con el total de cada motivo en
// una sola fila que se desplaza a lo ancho, y debajo los registros del periodo. Tocar una ficha
// filtra. La comida del personal sale aparte: no es una pérdida del negocio. Pesos al costo de
// cuando se registró.

type Preset = 'semana' | 'mes' | 'fechas';
const iso = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
const money = (v: string) => moneyExact(v);

function presetRange(p: Preset): { from: string; to: string } {
  const today = new Date();
  if (p === 'mes') return { from: iso(new Date(today.getFullYear(), today.getMonth(), 1)), to: iso(today) };
  const start = new Date(today);
  start.setDate(today.getDate() - 6);
  return { from: iso(start), to: iso(today) };
}

export function WasteTab() {
  const hora = useHoraDelNegocio();
  const navigate = useNavigate();
  const user = useSessionStore((s) => s.user);
  const canRecord = can('inventory.write_off', user);
  const [preset, setPreset] = useState<Preset>('semana');
  const [range, setRange] = useState(() => presetRange('semana'));
  const [reasonId, setReasonId] = useState<number | null>(null);
  useEffect(() => { medirAccion('inventario', 'ver-merma'); }, []);
  const choose = (p: Preset) => { setPreset(p); if (p !== 'fechas') setRange(presetRange(p)); };

  const { data: totals = [] } = useQuery({
    queryKey: ['merma', 'por-motivo', range.from, range.to], queryFn: () => wasteApi.byReason(range.from, range.to),
  });
  const { data: rows = [] } = useQuery({
    queryKey: ['merma', 'registros', range.from, range.to, reasonId], queryFn: () => wasteApi.list(range.from, range.to, reasonId),
  });

  return (
    <VStack align="stretch" gap={2} flex="1" minH={0} py={2}>
      <HStack gap={2} minH="44px" wrap="wrap">
        {(['semana', 'mes', 'fechas'] as const).map((p) => (
          <Button key={p} minH="44px" aria-pressed={preset === p} variant={preset === p ? 'solid' : 'outline'}
            colorPalette={preset === p ? 'brand' : 'gray'} onClick={() => choose(p)}>
            {p === 'semana' ? 'Esta semana' : p === 'mes' ? 'Este mes' : 'Elegir fechas'}
          </Button>
        ))}
        {preset === 'fechas' && (
          <>
            <DatePickerButton label="Desde" value={range.from} max={range.to} onChange={(v) => setRange((r) => ({ ...r, from: v }))} />
            <DatePickerButton label="Hasta" value={range.to} onChange={(v) => setRange((r) => ({ ...r, to: v }))} />
          </>
        )}
        {canRecord && (
          // Se llama igual que la captura, y quien buscaba dónde registrar llegaba aquí.
          <Button minH="44px" ms="auto" colorPalette="brand" onClick={() => navigate('/merma')}>Registrar</Button>
        )}
      </HStack>
      <Flex gap={2} overflowX="auto" flexShrink={0} pb={1}>
        {totals.map((t) => {
          const active = reasonId === t.reasonId;
          return (
            <Button key={`${t.reasonId}-${t.isStaffMeal}`} minH="56px" flexShrink={0} flexDirection="column" alignItems="flex-start"
              aria-pressed={active} variant={active ? 'solid' : 'outline'} onClick={() => setReasonId(active ? null : t.reasonId)}>
              <Text fontSize="xs" fontWeight="500">{t.name}{t.isStaffMeal ? ' · no cuenta como pérdida' : ''}</Text>
              <Text fontVariantNumeric="tabular-nums">{money(t.totalCost)}</Text>
            </Button>
          );
        })}
      </Flex>
      <Box flex="1" minH={0} overflowY="auto">
        {rows.length === 0 && <Text color="fg.muted" py={3}>En este periodo no se registró nada.</Text>}
        {rows.map((r) => (
          <Flex key={r.id} minH="52px" align="center" justify="space-between" gap={3} borderBottomWidth="1px">
            <Box minW={0}>
              <Text fontWeight="600">{r.itemName} · {r.quantity} {r.unitCode}</Text>
              <Text fontSize="sm" color="fg.muted">
                {r.reason} · {r.recordedBy} · {hora.fechaYHora(r.recordedAt)}
                {' · '}{STATUS_WORDS[r.status] ?? r.status}
              </Text>
            </Box>
            <Text fontVariantNumeric="tabular-nums" flexShrink={0}>{r.totalCost ? money(r.totalCost) : 'sin costo'}</Text>
          </Flex>
        ))}
      </Box>
    </VStack>
  );
}
