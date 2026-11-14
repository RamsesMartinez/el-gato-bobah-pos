import { useState } from 'react';
import { Badge, Box, Button, HStack, Input, Table, Text, VStack } from '@chakra-ui/react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Switch } from '../../components/ui/switch';
import { Picker } from '../../components/Picker';
import { ApiError } from '../../api/client';
import { medirAccion } from '../../api/uso';
import { recipientsApi, type RecipientsView } from '../../api/notifications';

// Quién recibe los avisos de inventario por correo (spec 084, maqueta M5, opción A v2): una fila por
// persona o correo extra y una columna por tipo de aviso. Por omisión reciben quienes configuran el
// almacén y tienen correo; aquí se quita a alguien, se agrega un correo que no es de nadie del sistema
// y se elige la hora del resumen de la mañana. Se guarda todo junto.

const HORAS = Array.from({ length: 96 }, (_, i) => {
  const h = String(Math.floor(i / 4)).padStart(2, '0');
  const m = String((i % 4) * 15).padStart(2, '0');
  return { value: `${h}:${m}`, label: `${h}:${m}` };
});

// La zona de toque es el propio interruptor: una caja de 44 px alrededor no recibe el toque.
function Interruptor({ checked, onChange, label, disabled }: { checked: boolean; onChange: (v: boolean) => void; label: string; disabled?: boolean }) {
  return (
    <Switch aria-label={label} minH="44px" minW="44px" px={2} justifyContent="center" checked={checked} disabled={disabled}
      onCheckedChange={(e) => onChange(e.checked)} />
  );
}

export function Destinatarios() {
  const qc = useQueryClient();
  const datos = useQuery({ queryKey: ['destinatarios'], queryFn: recipientsApi.get });
  const [v, setVista] = useState<RecipientsView | null>(null);
  const [nuevo, setNuevo] = useState('');
  const [aviso, setAviso] = useState<{ texto: string; error: boolean } | null>(null);
  if (datos.data && v === null) setVista(datos.data);
  // Editar quita el «Guardado.»: ya no es cierto.
  const setV = (x: RecipientsView) => { setVista(x); setAviso(null); };
  const guardar = useMutation({
    mutationFn: (x: RecipientsView) => recipientsApi.save(x),
    // Se toma lo que el servidor dejó (normaliza los correos): comparar contra lo escrito dejaba
    // «Guardar» encendido para siempre.
    onSuccess: async () => {
      medirAccion('inventario', 'destinatarios');
      const fresco = await qc.fetchQuery({ queryKey: ['destinatarios'], queryFn: recipientsApi.get, staleTime: 0 });
      setVista(fresco);
      setAviso({ texto: 'Guardado.', error: false });
    },
    onError: (e) => setAviso({ texto: e instanceof ApiError ? e.message : 'No se pudo guardar; vuelve a intentar.', error: true }),
  });
  if (!v) return null;
  const cambiado = JSON.stringify(v) !== JSON.stringify(datos.data);
  const agregar = () => {
    const e = nuevo.trim();
    if (!e) return;
    setV({ ...v, extras: [...v.extras, { email: e, stock: true, notFound: true }] });
    setNuevo('');
  };
  return (
    <VStack align="stretch" gap={3} flex="1" minH={0}>
      {/* Lo que crece se desplaza; «Guardar» se queda abajo, siempre a la vista. */}
      <VStack data-testid="destinatarios-lista" align="stretch" gap={3} flex="1" minH={0} overflowY="auto">
      <Box>
        <Text fontSize="lg" fontWeight="700">Avisos de inventario</Text>
        <Text color="fg.muted">Quién recibe el correo de lo que se acabó y de lo que alguien no encontró. Si no eliges, reciben quienes configuran el almacén.</Text>
      </Box>
      <HStack gap={3} borderWidth="1px" borderRadius="lg" p={3} flexWrap="wrap" bg="bg.panel">
        <Box flex="1" minW="240px">
          <Text fontWeight="700">Resumen de la mañana</Text>
          <Text fontSize="sm" color="fg.muted">Lo pendiente y la lista de compra, una vez al día</Text>
        </Box>
        <Box w="160px"><Picker value={v.morningAt} options={HORAS} title="Hora del resumen" onChange={(morningAt) => setV({ ...v, morningAt })} /></Box>
      </HStack>
      <Box borderWidth="1px" borderRadius="lg" overflowX="auto" bg="bg.panel">
        <Table.Root size="md">
          <Table.Header><Table.Row>
            <Table.ColumnHeader>Quién</Table.ColumnHeader>
            <Table.ColumnHeader textAlign="center">Se acabó y bajo el mínimo</Table.ColumnHeader>
            <Table.ColumnHeader textAlign="center">No lo encuentro</Table.ColumnHeader>
          </Table.Row></Table.Header>
          <Table.Body>
            {v.people.map((p, i) => (
              <Table.Row key={`p${p.userId}`} data-row>
                <Table.Cell>
                  <Text fontWeight="600">{p.name} <Badge ml={1}>{p.role}</Badge></Text>
                  <Text fontSize="sm" color={p.hasEmail ? 'fg.muted' : 'fg.error'}>{p.hasEmail ? p.email : 'Sin correo: captúralo en Empleados para que reciba'}</Text>
                </Table.Cell>
                <Table.Cell textAlign="center">
                  <Interruptor label={`${p.name} recibe «se acabó»`} checked={p.stock && p.hasEmail} disabled={!p.hasEmail}
                    onChange={(stock) => setV({ ...v, people: v.people.map((x, j) => (j === i ? { ...x, stock } : x)) })} />
                </Table.Cell>
                <Table.Cell textAlign="center">
                  <Interruptor label={`${p.name} recibe «no lo encuentro»`} checked={p.notFound && p.hasEmail} disabled={!p.hasEmail}
                    onChange={(notFound) => setV({ ...v, people: v.people.map((x, j) => (j === i ? { ...x, notFound } : x)) })} />
                </Table.Cell>
              </Table.Row>
            ))}
            {v.extras.map((e, i) => (
              <Table.Row key={`e${i}`} data-row>
                <Table.Cell>
                  <Text fontWeight="600">{e.email}</Text>
                  <HStack gap={2}>
                    <Text fontSize="sm" color="fg.muted">Correo extra</Text>
                    <Button minH="44px" variant="ghost" colorPalette="red" onClick={() => setV({ ...v, extras: v.extras.filter((_, j) => j !== i) })}>Quitar</Button>
                  </HStack>
                </Table.Cell>
                <Table.Cell textAlign="center">
                  <Interruptor label={`${e.email} recibe «se acabó»`} checked={e.stock}
                    onChange={(stock) => setV({ ...v, extras: v.extras.map((x, j) => (j === i ? { ...x, stock } : x)) })} />
                </Table.Cell>
                <Table.Cell textAlign="center">
                  <Interruptor label={`${e.email} recibe «no lo encuentro»`} checked={e.notFound}
                    onChange={(notFound) => setV({ ...v, extras: v.extras.map((x, j) => (j === i ? { ...x, notFound } : x)) })} />
                </Table.Cell>
              </Table.Row>
            ))}
          </Table.Body>
        </Table.Root>
      </Box>
      <HStack gap={2} flexWrap="wrap">
        <Input aria-label="Correo extra" placeholder="Agregar un correo, por ejemplo compras@tunegocio.mx" minH="48px" maxW="420px" type="email"
          value={nuevo} onChange={(e) => setNuevo(e.target.value)} onKeyDown={(e) => { if (e.key === 'Enter') agregar(); }} />
        <Button minH="48px" variant="outline" disabled={!nuevo.trim()} onClick={agregar}>Agregar</Button>
      </HStack>
      </VStack>
      <HStack gap={3} flexShrink={0} borderTopWidth="1px" pt={3}>
        {aviso && <Text flex="1" role={aviso.error ? 'alert' : 'status'} color={aviso.error ? 'fg.error' : 'fg.success'}>{aviso.texto}</Text>}
        <Box flex={aviso ? undefined : '1'} />
        <Button minH="48px" colorPalette="brand" disabled={!cambiado} loading={guardar.isPending} onClick={() => { setAviso(null); guardar.mutate(v); }}>Guardar</Button>
      </HStack>
    </VStack>
  );
}
