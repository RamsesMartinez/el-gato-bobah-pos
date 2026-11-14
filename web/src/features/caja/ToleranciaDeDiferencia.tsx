import { useState } from 'react';
import { Box, Button, HStack, Input, Text } from '@chakra-ui/react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { backofficeApi } from '../../api/backoffice';
import { toaster } from '../../components/ui/toaster';
import { mensajeDeError } from '../../api/mensajes';
import { montoTecleado } from '../../domain/numeros';

// ToleranciaDeDiferencia (#290, fase 4; decisión P1): un corte con una diferencia mayor queda «Por
// explicar» en el Histórico. No bloquea el cierre. Un campo vacío no es $0: no se guarda.
export function ToleranciaDeDiferencia() {
  const qc = useQueryClient();
  const { data } = useQuery({ queryKey: ['cash', 'close-settings'], queryFn: backofficeApi.cashCloseSettings });
  const [tecleado, setTecleado] = useState<string | null>(null);
  const valor = tecleado ?? data?.tolerance ?? '';
  const guardar = useMutation({
    mutationFn: (t: string) => backofficeApi.setCashCloseSettings(t),
    onSuccess: () => { setTecleado(null); qc.invalidateQueries({ queryKey: ['cash'] }); toaster.create({ title: 'Tolerancia guardada', type: 'success' }); },
    onError: (e) => toaster.create({ title: 'No se pudo guardar la tolerancia', description: mensajeDeError(e), type: 'error' }),
  });
  const n = montoTecleado(valor);
  const valida = n !== undefined && n >= 0;
  return (
    <Box borderWidth="1px" borderRadius="lg" p={4} bg="bg.panel">
      <Text fontWeight="700" mb={1}>Tolerancia de diferencia</Text>
      <Text fontSize="sm" color="fg.muted" mb={3} maxW="70ch">
        Un corte con una diferencia mayor queda «Por explicar» en el Histórico hasta que alguien la explique.
      </Text>
      <HStack gap={3} flexWrap="wrap">
        <Input aria-label="Tolerancia de diferencia" inputMode="decimal" w="160px" minH="52px" fontSize="lg"
          value={valor} onChange={(e) => setTecleado(e.target.value)} placeholder="0.00" />
        <Button minH="52px" colorPalette="brand" disabled={!valida || tecleado === null || guardar.isPending}
          loading={guardar.isPending} onClick={() => guardar.mutate(String(n))}>
          Guardar
        </Button>
      </HStack>
    </Box>
  );
}
