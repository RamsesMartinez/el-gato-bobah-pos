import { useState } from 'react';
import { Box, Heading, Text, Input, Button, VStack, HStack, Center, Spinner, InputGroup } from '@chakra-ui/react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { posApi } from '../../api/pos';
import { backofficeApi } from '../../api/backoffice';
import { toaster } from '../../components/ui/toaster';
import { Switch } from '../../components/ui/switch';
import { Page } from '../../components/Page';
import { InstallAppSection } from '../pwa/InstallAppSection';

// Ajustes de negocio (admin/gerente). Hoy solo el costo de envío por defecto; el backend es
// la autoridad (el PUT exige rol) — esta pantalla es la UX para editarlo.
export function BusinessSettingsPage() {
  const qc = useQueryClient();
  const { data, isLoading } = useQuery({ queryKey: ['business-settings'], queryFn: posApi.businessSettings });
  const { data: methods } = useQuery({ queryKey: ['payment-methods'], queryFn: posApi.paymentMethods });
  // null = sin edición local todavía: refleja el valor cargado. Evita el useEffect+setState
  // (cascading renders) para sincronizar el input con la query.
  const [fee, setFee] = useState<string | null>(null);
  const feeValue = fee ?? data?.deliveryFee ?? '';

  const { data: company } = useQuery({ queryKey: ['company'], queryFn: posApi.company });
  const [coName, setCoName] = useState<string | null>(null);
  const [coSlug, setCoSlug] = useState<string | null>(null);
  const nameValue = coName ?? company?.name ?? '';
  const slugValue = coSlug ?? company?.slug ?? '';

  const saveCompany = useMutation({
    mutationFn: () => posApi.updateCompany(nameValue.trim(), slugValue.trim()),
    onSuccess: (c) => {
      qc.setQueryData(['company'], c);
      toaster.create({ title: 'Empresa actualizada', description: 'El nuevo slug aplica al login (usuario@' + c.slug + ').', type: 'success' });
    },
    onError: (e) => toaster.create({ title: 'No se pudo actualizar', description: String(e), type: 'error' }),
  });

  const toggleAutoDeclare = useMutation({
    mutationFn: (v: { id: number; autoDeclare: boolean }) =>
      backofficeApi.setPaymentMethodAutoDeclare(v.id, v.autoDeclare),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['payment-methods'] }),
    onError: (e) => toaster.create({ title: 'Error', description: String(e), type: 'error' }),
  });

  const save = useMutation({
    mutationFn: () => posApi.updateBusinessSettings(Math.max(0, parseFloat(feeValue) || 0)),
    onSuccess: (bs) => {
      qc.setQueryData(['business-settings'], bs);
      qc.invalidateQueries({ queryKey: ['business-settings'] });
      toaster.create({ title: 'Ajustes guardados', type: 'success' });
    },
    onError: (e) => toaster.create({ title: 'Error', description: String(e), type: 'error' }),
  });

  if (isLoading) return <Center h="60vh"><Spinner size="xl" /></Center>;

  return (
    <Page maxW="560px">
      <Heading size="lg" mb={1}>Negocio</Heading>
      <Text color="fg.muted" mb={6}>Ajustes generales del local.</Text>

      <Box mb={6} borderWidth="1px" borderColor="border" borderRadius="lg" p={5}>
        <Text fontWeight="700" mb={1}>Identidad de la empresa</Text>
        <Text fontSize="sm" color="fg.muted" mb={3}>
          El «slug» es el identificador con el que tu equipo inicia sesión: usuario@<b>{slugValue || 'slug'}</b>.
          Cambiarlo modifica el login de todos los empleados (2–40, minúsculas, dígitos y guiones).
        </Text>
        <VStack align="stretch" gap={3} maxW="360px">
          <Input placeholder="Nombre del negocio" value={nameValue} onChange={(e) => setCoName(e.target.value)} />
          <InputGroup startElement="@">
            <Input placeholder="slug" value={slugValue} autoCapitalize="none"
              onChange={(e) => setCoSlug(e.target.value.toLowerCase())} />
          </InputGroup>
          <Button alignSelf="start" loading={saveCompany.isPending}
            disabled={!nameValue.trim() || slugValue.trim().length < 2}
            onClick={() => saveCompany.mutate()}>Guardar empresa</Button>
        </VStack>
      </Box>

      <Box borderWidth="1px" borderColor="border" borderRadius="lg" p={5}>
        <Text fontWeight="700" mb={1}>Costo de envío</Text>
        <Text fontSize="sm" color="fg.muted" mb={3}>
          Se agrega automáticamente al cobrar un pedido a domicilio (el operador puede ajustarlo o
          ponerlo en 0 en cada pedido).
        </Text>
        <HStack gap={3} align="end">
          <Box flex="1" maxW="220px">
            <InputGroup startElement="$">
              <Input size="lg" type="number" inputMode="decimal" min={0} value={feeValue}
                onChange={(e) => setFee(e.target.value)} />
            </InputGroup>
          </Box>
          <Button size="lg" loading={save.isPending} onClick={() => save.mutate()}>
            Guardar
          </Button>
        </HStack>
      </Box>

      <Box mt={6} borderWidth="1px" borderColor="border" borderRadius="lg" p={5}>
        <Text fontWeight="700" mb={1}>Corte de caja</Text>
        <Text fontSize="sm" color="fg.muted" mb={3}>
          Los métodos marcados como automáticos se declaran solos al cerrar caja (declarado =
          esperado): el cajero no necesita capturarlos a mano. Útil para métodos que no se
          cuentan físicamente, como tarjeta o transferencia.
        </Text>
        <VStack align="stretch" gap={3}>
          {(methods?.items ?? []).map((m) => (
            <HStack key={m.id} justify="space-between">
              <Text>{m.name}</Text>
              <Switch
                checked={m.autoDeclare}
                onCheckedChange={(e) => toggleAutoDeclare.mutate({ id: m.id, autoDeclare: e.checked })}
              />
            </HStack>
          ))}
        </VStack>
      </Box>

      <InstallAppSection />
    </Page>
  );
}
