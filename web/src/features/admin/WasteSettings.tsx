import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Box, Button, Flex, HStack, Input, Text, VStack } from '@chakra-ui/react';
import {
  DrawerRoot, DrawerBackdrop, DrawerContent, DrawerBody, DrawerHeader, DrawerFooter,
} from '../../components/ui/drawer';
import { posApi } from '../../api/pos';
import { ApiError } from '../../api/client';
import { wasteApi, type WasteReasonAdmin } from '../../api/waste';

// Lo que se echó a perder en Negocio › Inventario (spec 039, maqueta A): el tope a partir del cual
// una merma espera aprobación y los motivos de la empresa. Editar y juntar van en hojas; juntar es
// irreversible y pide confirmación. Juntar no cambia el motivo de lo ya registrado: el reporte suma
// lo de los dos bajo el que queda.

export function WasteSettings() {
  const qc = useQueryClient();
  const { data: settings } = useQuery({ queryKey: ['business-settings'], queryFn: posApi.businessSettings });
  const { data: reasons = [] } = useQuery({ queryKey: ['merma', 'motivos', 'todos'], queryFn: wasteApi.allReasons });
  const [threshold, setThreshold] = useState<string | null>(null);
  const [error, setError] = useState('');
  const [editing, setEditing] = useState<WasteReasonAdmin | null>(null);
  const [merging, setMerging] = useState<WasteReasonAdmin | null>(null);
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ['merma'] });
    void qc.invalidateQueries({ queryKey: ['business-settings'] });
  };
  const saveThreshold = useMutation({
    mutationFn: (v: string | null) => wasteApi.setThreshold(v),
    onSuccess: () => { setThreshold(null); setError(''); refresh(); },
    onError: (e) => setError(e instanceof ApiError && e.status === 400 ? 'El tope es un monto mayor que 0, o vacío' : 'No se pudo guardar'),
  });
  const current = settings?.wasteApprovalThreshold ?? '';
  const typed = threshold ?? current;
  const active = reasons.filter((r) => !r.archived && r.mergedIntoId === null);

  return (
    <Box mt={6} borderWidth="1px" borderRadius="lg" p={5}>
      <Text fontWeight="700" mb={2}>Lo que se echó a perder</Text>
      <Text fontWeight="600" mb={1}>¿A partir de cuánto espera aprobación?</Text>
      <HStack gap={2} flexWrap="wrap">
        <Input aria-label="Tope de aprobación" maxW="180px" minH="44px" inputMode="decimal" placeholder="Sin tope"
          value={typed} onChange={(e) => setThreshold(e.target.value)} />
        <Button minH="44px" disabled={typed.trim() === current.trim()} loading={saveThreshold.isPending}
          onClick={() => saveThreshold.mutate(typed.trim() === '' ? null : typed.trim())}>
          Guardar tope
        </Button>
      </HStack>
      <Text fontSize="sm" color="fg.muted" mt={1}>Vacío: nada espera aprobación y todo se resta al registrarse.</Text>
      {error && <Text fontSize="sm" color="red.600" mt={1}>{error}</Text>}

      <Text fontWeight="600" mt={4} mb={1}>Motivos</Text>
      {active.map((r) => (
        <Flex key={r.id} data-testid="motivo" minH="56px" align="center" justify="space-between" gap={3} borderBottomWidth="1px">
          <Box minW={0}>
            <Text>{r.name}{r.isStaffMeal ? ' · no cuenta como pérdida del negocio' : ''}</Text>
            <Text fontSize="sm" color="fg.muted">{r.description} · {r.uses} {r.uses === 1 ? 'vez' : 'veces'}</Text>
          </Box>
          <HStack gap={4} flexShrink={0}>
            <Button minH="44px" variant="ghost" colorPalette="red" onClick={() => setMerging(r)}>Juntar</Button>
            <Button minH="44px" variant="outline" onClick={() => setEditing(r)}>Editar</Button>
          </HStack>
        </Flex>
      ))}
      {editing && <EditReason reason={editing} onClose={() => setEditing(null)} onSaved={() => { setEditing(null); refresh(); }} />}
      {merging && (
        <MergeReason reason={merging} options={active.filter((r) => r.id !== merging.id)}
          onClose={() => setMerging(null)} onMerged={() => { setMerging(null); refresh(); }} />
      )}
    </Box>
  );
}

function EditReason({ reason, onClose, onSaved }: { reason: WasteReasonAdmin; onClose: () => void; onSaved: () => void }) {
  const [name, setName] = useState(reason.name);
  const [description, setDescription] = useState(reason.description);
  const [staff, setStaff] = useState(reason.isStaffMeal);
  const [error, setError] = useState('');
  const save = useMutation({
    mutationFn: (archived: boolean) => wasteApi.updateReason(reason.id, { name, description, isStaffMeal: staff, archived }),
    onSuccess: onSaved,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'No se pudo guardar'),
  });
  return (
    <DrawerRoot open onOpenChange={(e) => { if (!e.open) onClose(); }} placement="bottom">
      <DrawerBackdrop />
      <DrawerContent>
        <DrawerHeader><Text fontWeight="700">Editar motivo</Text></DrawerHeader>
        <DrawerBody>
          <VStack align="stretch" gap={3}>
            <Input aria-label="Nombre del motivo" minH="44px" maxLength={40} value={name} onChange={(e) => setName(e.target.value)} />
            <Input aria-label="Descripción" minH="44px" maxLength={200} value={description} onChange={(e) => setDescription(e.target.value)} />
            <Button alignSelf="start" minH="44px" aria-pressed={staff} variant={staff ? 'solid' : 'outline'} onClick={() => setStaff(!staff)}>
              No cuenta como pérdida del negocio
            </Button>
            {error && <Text color="red.600">{error}</Text>}
          </VStack>
        </DrawerBody>
        <DrawerFooter>
          <Button minH="44px" variant="ghost" colorPalette="red" mr="auto" onClick={() => save.mutate(true)}>Archivar</Button>
          <Button minH="44px" variant="ghost" onClick={onClose}>Cancelar</Button>
          <Button minH="44px" disabled={!name.trim() || !description.trim()} loading={save.isPending} onClick={() => save.mutate(false)}>
            Guardar motivo
          </Button>
        </DrawerFooter>
      </DrawerContent>
    </DrawerRoot>
  );
}

function MergeReason({ reason, options, onClose, onMerged }: {
  reason: WasteReasonAdmin; options: WasteReasonAdmin[]; onClose: () => void; onMerged: () => void;
}) {
  const [into, setInto] = useState<WasteReasonAdmin | null>(null);
  const merge = useMutation({ mutationFn: () => wasteApi.mergeReason(reason.id, into!.id), onSuccess: onMerged });
  return (
    <DrawerRoot open onOpenChange={(e) => { if (!e.open) onClose(); }} placement="bottom">
      <DrawerBackdrop />
      <DrawerContent maxH="80dvh">
        <DrawerHeader><Text fontWeight="700">Juntar «{reason.name}» con…</Text></DrawerHeader>
        <DrawerBody overflowY="auto">
          {into ? (
            <VStack align="stretch" gap={3}>
              <Text>«{reason.name}» deja de ofrecerse y lo registrado con él se suma a «{into.name}». No se puede deshacer.</Text>
              <HStack gap={3}>
                <Button minH="44px" variant="ghost" onClick={() => setInto(null)}>No</Button>
                <Button minH="44px" colorPalette="red" loading={merge.isPending} onClick={() => merge.mutate()}>Sí, juntar</Button>
              </HStack>
            </VStack>
          ) : options.map((o) => (
            <Button key={o.id} w="100%" minH="52px" justifyContent="space-between" variant="ghost" onClick={() => setInto(o)}>
              {o.name}<Text as="span" color="fg.muted" fontSize="sm">{o.uses} {o.uses === 1 ? 'vez' : 'veces'}</Text>
            </Button>
          ))}
        </DrawerBody>
      </DrawerContent>
    </DrawerRoot>
  );
}
