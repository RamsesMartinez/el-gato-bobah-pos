import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Box, Button, Heading, HStack, Input, Text } from '@chakra-ui/react';
import { posApi } from '../../api/pos';
import { backofficeApi, type CostingMethod } from '../../api/backoffice';
import { ApiError } from '../../api/client';
import { WasteSettings } from './WasteSettings';
import { CountPlacesSettings } from './CountPlacesSettings';
import { WarehousesSettings } from './WarehousesSettings';
import { can } from '../../app/permissions';
import { useSessionStore } from '../../stores/session';
import { CountLimitsSettings } from './CountLimitsSettings';
import { Switch } from '../../components/ui/switch';

const METHODS: { value: CostingMethod; label: string; hint: string }[] = [
  { value: 'last', label: 'Lo que costó la última compra', hint: 'El costo de cada insumo es el de su compra más reciente.' },
  { value: 'weighted', label: 'Promedio de lo que tienes', hint: 'Cada compra se promedia con lo que ya tenías guardado.' },
];

// InventorySettingsSection es el renglón de almacén de Configuración del negocio (spec 038): cómo
// se costea un insumo al comprarlo y cómo se le dice a lo que se guarda. Va aparte del resto de la
// página porque lo cambia quien configura el almacén, no quien cambia los datos fiscales.
export function InventorySettingsSection() {
  const qc = useQueryClient();
  const user = useSessionStore((s) => s.user);
  const { data } = useQuery({ queryKey: ['business-settings'], queryFn: posApi.businessSettings });
  const [label, setLabel] = useState<string | null>(null);
  const [error, setError] = useState('');
  const save = useMutation({
    mutationFn: (b: { costingMethod: CostingMethod; stockItemLabel: string }) => backofficeApi.updateInventorySettings(b),
    onSuccess: () => {
      setLabel(null);
      setError('');
      void qc.invalidateQueries({ queryKey: ['business-settings'] });
    },
    onError: (e) => setError(e instanceof ApiError && e.status === 400
      ? 'La palabra va de 1 a 20 letras'
      : 'No se pudo guardar'),
  });
  // Spec 096, subida 2: que la merma esperada también descuente del almacén. Su error es suyo: en el
  // de la palabra, un éxito de ella lo borraba y uno del interruptor no.
  const [deductsError, setDeductsError] = useState('');
  const deducts = useMutation({
    mutationFn: (on: boolean) => backofficeApi.setExpectedWasteDeducts(on),
    // Se escribe en la caché lo que se guardó: esperar el refetch dejaba el interruptor regresar un
    // instante (o quedarse) en el valor anterior.
    onSuccess: (_r, on) => {
      setDeductsError('');
      qc.setQueryData(['business-settings'], (o: object | undefined) => o && { ...o, expectedWasteDeducts: on });
    },
    onError: (e) => setDeductsError(e instanceof ApiError && e.status < 500 ? e.message : 'No se pudo guardar'),
  });
  if (!data) return null;
  const method: CostingMethod = data.costingMethod ?? 'last';
  const current = data.stockItemLabel || 'artículo';
  const typed = label ?? current;
  const trimmed = typed.trim();

  return (
    <Box mt={6} borderWidth="1px" borderRadius="lg" p={5}>
      <Heading size="md" mb={3}>Almacén</Heading>
      <Text fontWeight="600" mb={2}>¿Cómo se calcula el costo de un insumo al comprarlo?</Text>
      <HStack gap={2} flexWrap="wrap" mb={1} role="group" aria-label="Método de costeo">
        {METHODS.map((m) => (
          <Button key={m.value} minH="44px" aria-pressed={method === m.value}
            variant={method === m.value ? 'solid' : 'outline'} colorPalette={method === m.value ? 'brand' : 'gray'}
            loading={save.isPending && save.variables?.costingMethod === m.value && method !== m.value}
            onClick={() => method !== m.value && save.mutate({ costingMethod: m.value, stockItemLabel: current })}>
            {m.label}
          </Button>
        ))}
      </HStack>
      <Text fontSize="sm" color="fg.muted" mb={4}>{METHODS.find((m) => m.value === method)!.hint}</Text>

      <Text fontWeight="600" mb={2}>¿Cómo le dices a lo que guardas en el almacén?</Text>
      <HStack gap={2} align="center" flexWrap="wrap">
        <Input aria-label="Palabra del almacén" maxW="260px" minH="44px" value={typed} maxLength={20}
          onChange={(e) => setLabel(e.target.value)} />
        <Button minH="44px" disabled={!trimmed || trimmed === current} loading={save.isPending && label !== null}
          onClick={() => save.mutate({ costingMethod: method, stockItemLabel: trimmed })}>
          Guardar palabra
        </Button>
      </HStack>
      <Text fontSize="sm" color="fg.muted" mt={1}>Se ve en la captura de compras: «Agregar {trimmed || current}».</Text>
      {error && <Text fontSize="sm" color="fg.error" mt={2}>{error}</Text>}
      <HStack justify="space-between" align="start" mt={5}>
        <Box pr={3}>
          <Text fontWeight="600" id="descuenta-merma">La merma esperada también descuenta del almacén</Text>
          <Text fontSize="sm" color="fg.muted">
            Cada venta resta su receta más la merma al preparar. Las ventas de antes no cambian.
          </Text>
        </Box>
        <Switch size="lg" py="10px" aria-labelledby="descuenta-merma" disabled={deducts.isPending}
          checked={deducts.isPending ? deducts.variables : !!data.expectedWasteDeducts}
          onCheckedChange={(e) => deducts.mutate(e.checked)} />
      </HStack>
      {deductsError && <Text fontSize="sm" color="fg.error" mt={1}>{deductsError}</Text>}
      {can('inventory.warehouse_manage', user) && <WarehousesSettings />}
      <CountPlacesSettings />
      <CountLimitsSettings />
      <WasteSettings />
    </Box>
  );
}
