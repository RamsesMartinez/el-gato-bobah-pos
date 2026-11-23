import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Box, Button, Heading, HStack, Input, Text } from '@chakra-ui/react';
import { posApi } from '../../api/pos';
import { backofficeApi } from '../../api/backoffice';
import { ApiError } from '../../api/client';
import { pctTecleado, pctTexto } from '../../shared/merma/pct';

// CostSettingsSection es Ajustes › Costos (spec 096, maqueta A): la merma general que se suma al costo
// de todo producto con receta que no tenga la suya. La decide quien configura el menú.
export function CostSettingsSection() {
  const qc = useQueryClient();
  const { data } = useQuery({ queryKey: ['business-settings'], queryFn: posApi.businessSettings });
  const [typed, setTyped] = useState<string | null>(null);
  const [error, setError] = useState('');
  const [done, setDone] = useState(false);
  const save = useMutation({
    mutationFn: (pct: number | null) => backofficeApi.setGeneralExpectedWaste(pct),
    onSuccess: (r) => {
      // Lo guardado entra a la caché antes de soltar lo tecleado: sin eso el campo volvía un instante
      // al valor anterior (y se quedaba ahí si el refetch fallaba).
      qc.setQueryData(['business-settings'], (old: object | undefined) => (old ? { ...old, expectedWastePct: r.pct } : old));
      setTyped(null);
      setError('');
      setDone(true);
      void qc.invalidateQueries({ queryKey: ['business-settings'] });
      void qc.invalidateQueries({ queryKey: ['admin'] });
    },
    onError: (e) => setError(e instanceof ApiError && e.status < 500 ? e.message : 'No se pudo guardar'),
  });
  if (!data) return null;
  const saved = pctTexto(data.expectedWastePct);
  const text = typed ?? saved;
  const pct = pctTecleado(text);
  const invalid = pct === 'invalido';

  return (
    <Box borderWidth="1px" borderColor="border" borderRadius="lg" p={5}>
      <Heading size="md" mb={1}>Merma al preparar</Heading>
      <Text fontSize="sm" color="fg.muted" mb={3}>
        Sube el costo de lo que preparas por lo que se pierde al prepararlo. Se aplica a todo producto
        con receta que no tenga la suya. Vacío: no suma nada.
      </Text>
      <HStack gap={2} align="center">
        <Input aria-label="Merma al preparar, en porcentaje" maxW="120px" minH="44px" inputMode="decimal"
          placeholder="Sin merma" value={text} aria-invalid={invalid}
          onChange={(e) => { setTyped(e.target.value); setError(''); setDone(false); }} />
        <Text>%</Text>
        <Button minH="44px" disabled={invalid || text.trim() === saved} loading={save.isPending}
          onClick={() => !invalid && save.mutate(pct)}>
          Guardar
        </Button>
      </HStack>
      {/* Quien lo probó sin capacitación no supo si había quedado: se dice. */}
      {done && <Text fontSize="sm" color="fg.success" mt={2}>Guardado. Los costos ya incluyen la merma.</Text>}
      {invalid && <Text fontSize="sm" color="fg.error" mt={2}>Escribe de 0 a 99.99</Text>}
      {error && <Text fontSize="sm" color="fg.error" mt={2}>{error}</Text>}
    </Box>
  );
}
