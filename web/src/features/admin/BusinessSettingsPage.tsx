import { useState } from 'react';
import { Box, Heading, Text, Input, Button, VStack, HStack, Center, Spinner, InputGroup } from '@chakra-ui/react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { posApi } from '../../api/pos';
import { backofficeApi } from '../../api/backoffice';
import { toaster } from '../../components/ui/toaster';
import { Picker } from '../../components/Picker';
import { Switch } from '../../components/ui/switch';
import { Page } from '../../components/Page';
import { InstallAppSection } from '../../shared/pwa/InstallAppSection';
import { ZONAS_MEXICO } from './zonas';
import { montoTecleado } from '../../domain/numeros';

// Los tres momentos en los que se puede limpiar la lista de entregados. El texto dice QUÉ pasa, no
// por qué: el porqué de cada uno es una decisión del dueño, no algo que el operador tenga que leer
// cada vez que abre los ajustes.
const CORTES = [
  { value: 'medianoche', label: 'A medianoche' },
  { value: 'turno', label: 'Al abrir el siguiente turno' },
  { value: 'cierre_de_caja', label: 'Al cerrar la caja' },
];

// Con qué se nombra cada pedido. Las etiquetas dicen cuántos son porque es lo que decide: con una
// lista más corta, el mismo nombre vuelve antes.
const ESQUEMAS = [
  { value: 'razas', label: 'Razas de gato (88 nombres)' },
  { value: 'animales', label: 'Animales (100 nombres)' },
];

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
  const [tz, setTz] = useState<string | null>(null);
  const tzValue = tz ?? data?.timezone ?? 'America/Mexico_City';
  const [corte, setCorte] = useState<string | null>(null);
  const [esquema, setEsquema] = useState<string | null>(null);

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

  const saveTz = useMutation({
    mutationFn: () => posApi.updateTimezone(tzValue),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['business-settings'] });
      toaster.create({ title: 'Zona horaria guardada', type: 'success' });
    },
    onError: (e: unknown) => toaster.create({
      title: 'No se pudo guardar la zona',
      description: e instanceof Error ? e.message : String(e),
      type: 'error',
    }),
  });

  const saveCorte = useMutation({
    mutationFn: () => posApi.updateCorteDeVista(corte ?? data?.corteDeVista ?? 'medianoche'),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['business-settings'] });
      // Y la lista de entregados, que es lo que el ajuste acaba de cambiar: sin esto el operador
      // guarda y no ve nada distinto hasta el siguiente refresco.
      qc.invalidateQueries({ queryKey: ['orders', 'delivered'] });
      toaster.create({ title: 'Guardado', type: 'success' });
    },
    onError: (e: unknown) => toaster.create({
      title: 'No se pudo guardar',
      description: e instanceof Error ? e.message : String(e),
      type: 'error',
    }),
  });

  const saveEsquema = useMutation({
    mutationFn: () => posApi.updateFolioScheme(esquema ?? data?.folioScheme ?? 'razas'),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['business-settings'] });
      // Y la lista de nombres que el punto de venta propone: sin esto las cuentas nuevas seguirían
      // bautizándose con la lista anterior hasta recargar la aplicación.
      qc.invalidateQueries({ queryKey: ['pos', 'folio-names'] });
      toaster.create({ title: 'Guardado', type: 'success' });
    },
    onError: (e: unknown) => toaster.create({
      title: 'No se pudo guardar',
      description: e instanceof Error ? e.message : String(e),
      type: 'error',
    }),
  });

  const save = useMutation({
    mutationFn: () => posApi.updateBusinessSettings(Math.max(0, montoTecleado(feeValue) ?? 0)),
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
        <Text fontWeight="700" mb={1}>Zona horaria</Text>
        <Text fontSize="sm" color="fg.muted" mb={3}>
          Define a qué día pertenece cada venta, corte y gasto. Cámbiala solo si el negocio opera
          en otro huso.
        </Text>
        <HStack gap={2} align="end" flexWrap="wrap">
          <Box flex="1" minW="260px">
            {/* Picker táctil, no <select> nativo: en una tablet de 7" el desplegable del
                sistema tapa la pantalla con renglones de 20px. Ver la constitución. */}
            <Picker
              size="lg"
              value={tzValue}
              onChange={setTz}
              options={ZONAS_MEXICO.map((z) => ({ value: z.value, label: z.label }))}
              title="Zona horaria del negocio"
            />
          </Box>
          <Button size="lg" loading={saveTz.isPending} onClick={() => saveTz.mutate()}>
            Guardar zona
          </Button>
        </HStack>
        {/* El aviso solo aparece cuando de verdad se está cambiando. Todas las horas de todas las
            pantallas se mueven de golpe, y sin decirlo antes se lee como que los datos se
            corrompieron. Las dos frases importan: la primera explica lo que se va a ver, la segunda
            desactiva el miedo de que se haya movido dinero. */}
        {tz && tz !== data?.timezone && (
          <Text mt={3} fontSize="sm" color="orange.600">
            Al guardar, las horas que muestran las pantallas y los tickets cambian a la nueva zona.
            Las ventas ya registradas no cambian de día ni de corte.
          </Text>
        )}
      </Box>

      <Box mt={6} borderWidth="1px" borderColor="border" borderRadius="lg" p={5}>
        <Text fontWeight="700" mb={1}>Pedidos entregados en pantalla</Text>
        <Text fontSize="sm" color="fg.muted" mb={3}>
          Hasta cuándo se siguen viendo los pedidos que ya entregaste. No cambia de qué día es una
          venta.
        </Text>
        <HStack gap={2} align="end" flexWrap="wrap">
          <Box flex="1" minW="260px">
            {/* size="lg": el default del componente es `md`, que en Chakra mide 40px — por debajo
                del piso de 44 con el que un dedo acierta a la primera. Y el botón de guardar que va
                a su lado ya mide 44, así que la fila se veía disparejа. */}
            <Picker
              size="lg"
              value={corte ?? data?.corteDeVista ?? 'medianoche'}
              onChange={setCorte}
              options={CORTES}
              title="¿Cuándo se limpian los entregados?"
            />
          </Box>
          <Button size="lg" loading={saveCorte.isPending} onClick={() => saveCorte.mutate()}>
            Guardar
          </Button>
        </HStack>
      </Box>

      <Box mt={6} borderWidth="1px" borderColor="border" borderRadius="lg" p={5}>
        <Text fontWeight="700" mb={1}>Nombres de los pedidos</Text>
        <Text fontSize="sm" color="fg.muted" mb={3}>
          Con qué se nombra cada pedido para cantarlo en cocina. Se usan todos los nombres de la
          lista antes de repetir alguno. Los pedidos ya vendidos conservan el nombre con el que
          salieron.
        </Text>
        <HStack gap={2} align="end" flexWrap="wrap">
          <Box flex="1" minW="260px">
            <Picker
              size="lg"
              value={esquema ?? data?.folioScheme ?? 'razas'}
              onChange={setEsquema}
              options={ESQUEMAS}
              title="¿Con qué se nombran los pedidos?"
            />
          </Box>
          <Button size="lg" loading={saveEsquema.isPending} onClick={() => saveEsquema.mutate()}>
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
